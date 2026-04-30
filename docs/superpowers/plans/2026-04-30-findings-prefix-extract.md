# Findings 改用 `FINDING_JSON:` 前缀解析 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修 #44 — Findings 提取从行级 `startswith("{") + "dimension" in line` 替换为 `FINDING_JSON: <single-line json>` 前缀方案。新解析强 schema（必填字段 + dimension/severity 枚举）；失败有 `logger.warn` + `tracer.event` + `reporter.record_llm_event`（不引入 Prometheus 依赖 —— 控制器侧已经统计 findings 数量）；prompt 模板更新；旧路径完全删除。

**Architecture:**
- Prompt 模板里把 "output a finding JSON object on its own line" 改成 "Each finding MUST appear on its own line as `FINDING_JSON: {...}` (literal prefix, single-line JSON)"。
- 抽出 `extract_findings(text: str) -> tuple[list[dict], list[str]]` 纯函数：返回 `(valid_findings, parse_errors)`，把抓取与校验完全独立于 `run_agent` 的循环，便于单测。
- `run_agent` 在每个 text 块里调一次 `extract_findings`，对 `parse_errors` 触发 `logger.warn` / `tracer.event` / `reporter.record_llm_event`。
- 不引入 `pydantic` —— 校验用一个 `_FINDING_REQUIRED_FIELDS` set + `_DIMENSION_ENUM` / `_SEVERITY_ENUM` set 手工校验（最小依赖原则）。

**Tech Stack:** Python 3.11+ (`pytest`、`unittest.mock`)、`json`、`logger` (structlog 风格)、`tracer.event` (Langfuse)、`reporter.record_llm_event`（既有 controller-bound 自定义事件通道）。

**Spec:** GitHub issue #44 — 选 "方案 B"（FINDING_JSON 前缀 + 兜底解析）。Pydantic 跳过；Prometheus 跳过（用 reporter 既有事件通道替代）。

---

## File Structure

**Modify:**
- `agent-runtime/runtime/orchestrator.py` —
  - `build_prompt` 更新 instructions 块为 FINDING_JSON 前缀格式
  - `run_agent` 删除 inline 旧 startswith 路径，改调 `extract_findings(text)`
  - 新增模块级 `extract_findings(text) -> tuple[list[dict], list[str]]` 纯函数 + 校验常量
- `agent-runtime/tests/test_orchestrator.py` —
  - 新增 `class TestExtractFindings`（5+ 个 case）
  - 修改 `TestRunAgent.test_extracts_findings_from_text` 用新格式（旧 case 已不适用）

---

## Task 1: 抽出纯函数 `extract_findings` + 校验

**Files:**
- Modify: `agent-runtime/runtime/orchestrator.py`
- Test: `agent-runtime/tests/test_orchestrator.py`

新建独立纯函数，便于 TDD。新函数 / 常量先于 prompt + run_agent 的调整落地，避免一次改太大。

- [ ] **Step 1.1: Append failing tests for `extract_findings`**

在 `agent-runtime/tests/test_orchestrator.py` 顶部 imports 后、第一个 class 之前，新增独立测试类（不在 TestRunAgent 内）：

```python
class TestExtractFindings:
    """Tests for the FINDING_JSON: prefix extractor."""

    def test_single_valid_finding(self):
        text = (
            "Some preamble.\n"
            'FINDING_JSON: {"dimension":"health","severity":"critical","title":"Pod CrashLoopBackOff","description":"d","resource_kind":"Pod","resource_namespace":"default","resource_name":"nginx","suggestion":"Restart"}\n'
            "FINDINGS_COMPLETE"
        )
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert errors == []
        assert len(findings) == 1
        assert findings[0]["title"] == "Pod CrashLoopBackOff"

    def test_multiple_findings_one_invalid_enum(self):
        text = (
            'FINDING_JSON: {"dimension":"health","severity":"critical","title":"A","description":"d","resource_kind":"Pod","resource_namespace":"ns","resource_name":"a","suggestion":"s"}\n'
            'FINDING_JSON: {"dimension":"BOGUS","severity":"critical","title":"B","description":"d","resource_kind":"Pod","resource_namespace":"ns","resource_name":"b","suggestion":"s"}\n'
            'FINDING_JSON: {"dimension":"security","severity":"medium","title":"C","description":"d","resource_kind":"Pod","resource_namespace":"ns","resource_name":"c","suggestion":"s"}\n'
        )
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert len(findings) == 2  # A and C accepted, B rejected
        assert {f["title"] for f in findings} == {"A", "C"}
        assert len(errors) == 1
        assert "dimension" in errors[0]

    def test_missing_required_field_rejected(self):
        # No `suggestion` key
        text = 'FINDING_JSON: {"dimension":"health","severity":"low","title":"X","description":"d","resource_kind":"Pod","resource_namespace":"n","resource_name":"x"}\n'
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert findings == []
        assert len(errors) == 1
        assert "suggestion" in errors[0]

    def test_invalid_severity_enum_rejected(self):
        text = 'FINDING_JSON: {"dimension":"health","severity":"PANIC","title":"X","description":"d","resource_kind":"Pod","resource_namespace":"n","resource_name":"x","suggestion":"s"}\n'
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert findings == []
        assert len(errors) == 1
        assert "severity" in errors[0]

    def test_markdown_code_fence_json_not_extracted(self):
        """Old startswith heuristic falsely captured JSON inside ``` blocks. New parser must not."""
        text = (
            "Here is an example finding format:\n"
            "```json\n"
            '{"dimension":"health","severity":"critical","title":"EXAMPLE","description":"d","resource_kind":"Pod","resource_namespace":"n","resource_name":"e","suggestion":"s"}\n'
            "```\n"
            "Now the real one:\n"
            'FINDING_JSON: {"dimension":"health","severity":"low","title":"REAL","description":"d","resource_kind":"Pod","resource_namespace":"n","resource_name":"r","suggestion":"s"}\n'
        )
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert len(findings) == 1
        assert findings[0]["title"] == "REAL"
        assert errors == []

    def test_pretty_printed_multiline_json_not_extracted(self):
        """Multi-line JSON without prefix on first line is rejected — keeps the contract single-line."""
        text = (
            "FINDING_JSON: {\n"
            '  "dimension": "health",\n'
            '  "severity": "critical"\n'
            "}\n"
        )
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        # Either parsed nothing (only the prefix line is invalid JSON), or
        # captured an error — both are acceptable, but findings must be empty.
        assert findings == []

    def test_garbage_after_prefix_logged(self):
        text = "FINDING_JSON: not-json-at-all\n"
        from runtime.orchestrator import extract_findings

        findings, errors = extract_findings(text)
        assert findings == []
        assert len(errors) == 1
```

- [ ] **Step 1.2: Run failing tests**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py::TestExtractFindings -v`
Expected: FAIL — `cannot import name 'extract_findings' from 'runtime.orchestrator'`

- [ ] **Step 1.3: Implement `extract_findings` + constants**

在 `agent-runtime/runtime/orchestrator.py` 顶部 `TARGET_NAMESPACES = ...` 之后追加：

```python
# Findings extraction (issue #44): the LLM emits one finding per line as
# "FINDING_JSON: <single-line json>". Validation runs in extract_findings;
# malformed entries are returned as parse errors so callers can log + emit
# observability events.
_FINDING_PREFIX = "FINDING_JSON: "
_FINDING_REQUIRED_FIELDS = frozenset({
    "dimension",
    "severity",
    "title",
    "description",
    "resource_kind",
    "resource_namespace",
    "resource_name",
    "suggestion",
})
_DIMENSION_ENUM = frozenset({"health", "security", "cost", "reliability"})
_SEVERITY_ENUM = frozenset({"critical", "high", "medium", "low", "info"})


def extract_findings(text: str) -> tuple[list[dict], list[str]]:
    """Parse FINDING_JSON: prefixed lines from LLM text output.

    Returns (valid_findings, parse_errors). Each parse_error is a short
    human-readable description suitable for logging or trace events.
    The caller is responsible for emitting logs / metrics / events.
    """
    findings: list[dict] = []
    errors: list[str] = []
    for raw in text.split("\n"):
        line = raw.strip()
        if not line.startswith(_FINDING_PREFIX):
            continue
        payload = line[len(_FINDING_PREFIX):].strip()
        try:
            obj = json.loads(payload)
        except json.JSONDecodeError as exc:
            errors.append(f"json parse failed: {exc.msg}")
            continue
        if not isinstance(obj, dict):
            errors.append("finding payload is not a JSON object")
            continue
        missing = _FINDING_REQUIRED_FIELDS - obj.keys()
        if missing:
            errors.append(f"missing required field(s): {sorted(missing)}")
            continue
        if obj["dimension"] not in _DIMENSION_ENUM:
            errors.append(f"invalid dimension: {obj['dimension']!r}")
            continue
        if obj["severity"] not in _SEVERITY_ENUM:
            errors.append(f"invalid severity: {obj['severity']!r}")
            continue
        findings.append(obj)
    return findings, errors
```

- [ ] **Step 1.4: Run failing tests, expect pass**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py::TestExtractFindings -v`
Expected: 7 个 test 全部 PASS

- [ ] **Step 1.5: Run full orchestrator test file**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py -v`
Expected: 既有的 TestRunAgent::test_extracts_findings_from_text 仍然 PASS（旧路径还没拆，后续 Task 2 会覆盖）。

- [ ] **Step 1.6: Commit**

```bash
git add agent-runtime/runtime/orchestrator.py agent-runtime/tests/test_orchestrator.py
git commit -m "feat(orchestrator): add extract_findings with FINDING_JSON: prefix + schema check

Pure function that parses LLM text for FINDING_JSON: prefixed lines and
validates against required-field set + dimension/severity enums. Returns
(valid_findings, parse_errors) so callers can decide how to emit logs,
metrics, or trace events. Used by run_agent in the next commit.

Refs #44"
```

---

## Task 2: `run_agent` 改用 `extract_findings` + 失败上报；prompt 更新

**Files:**
- Modify: `agent-runtime/runtime/orchestrator.py` (run_agent + build_prompt)
- Test: `agent-runtime/tests/test_orchestrator.py`

- [ ] **Step 2.1: Update existing test for the new format**

旧 test `TestRunAgent.test_extracts_findings_from_text`（line ~64）的 finding 文本字段不全（缺 description / suggestion 等），且没有 FINDING_JSON: 前缀。需要让它走新格式：

替换 `agent-runtime/tests/test_orchestrator.py` 该 test 函数（行号大概 64-86）：

```python
    def test_extracts_findings_from_text(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "1")

        finding_obj = {
            "dimension": "health", "severity": "critical",
            "title": "CrashLoopBackOff",
            "description": "Pod restarting repeatedly",
            "resource_kind": "Pod",
            "resource_namespace": "default", "resource_name": "nginx",
            "suggestion": "Inspect container logs",
        }
        text = (
            "Here is a finding:\n"
            f"FINDING_JSON: {json.dumps(finding_obj)}\n"
            "FINDINGS_COMPLETE"
        )
        response = {
            "content": [{"type": "text", "text": text}],
            "stop_reason": "end_turn",
            "input_tokens": 0,
            "output_tokens": 0,
        }
        chain = _make_chain_mock(response)

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert len(findings) == 1
        assert findings[0]["title"] == "CrashLoopBackOff"
```

并新增一个 test 验证 parse 错误会触发 reporter 上报：

在 TestRunAgent 末尾追加：

```python
    def test_parse_errors_surfaced_via_reporter(self, monkeypatch):
        """A malformed FINDING_JSON line must be reported via reporter.record_llm_event."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "1")

        text = (
            'FINDING_JSON: {"dimension":"BOGUS","severity":"critical","title":"X","description":"d","resource_kind":"Pod","resource_namespace":"n","resource_name":"x","suggestion":"s"}\n'
            "FINDINGS_COMPLETE"
        )
        response = {
            "content": [{"type": "text", "text": text}],
            "stop_reason": "end_turn",
            "input_tokens": 0,
            "output_tokens": 0,
        }
        chain = _make_chain_mock(response)

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                with patch("runtime.orchestrator.reporter.record_llm_event") as mock_event:
                    findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert findings == []  # rejected by enum check
        mock_event.assert_called()
        args, _ = mock_event.call_args
        assert args[0] == "finding_parse_error"
        assert "dimension" in args[1].get("error", "")
```

- [ ] **Step 2.2: Run failing tests**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py::TestRunAgent::test_extracts_findings_from_text tests/test_orchestrator.py::TestRunAgent::test_parse_errors_surfaced_via_reporter -v`
Expected: FAIL — first test fails because old startswith path captures the new FINDING_JSON line as a half-formed JSON (the prefix `FINDING_JSON: {...}` does NOT start with `{`); both tests fail because no reporter call happens for invalid enum (old path doesn't validate enums).

- [ ] **Step 2.3: Update `run_agent` to use `extract_findings` + emit on errors**

修改 `agent-runtime/runtime/orchestrator.py` 的 `run_agent`：

(a) 顶部 import 之后追加 `from . import reporter`（如未导入）。检查现有 imports，model_chain 内已经 `from . import reporter`，orchestrator 大概率没有，需要加。

(b) 替换 `run_agent` 内的 inline finding extract（current line 121-129）：

旧：

```python
            if block["type"] == "text" and block.get("text"):
                assistant_content.append({"type": "text", "text": block["text"]})
                logger.debug("text block", chars=len(block['text']), preview=block['text'][:200])
                # Extract findings from text
                for line in block["text"].split("\n"):
                    line = line.strip()
                    if line.startswith("{") and "dimension" in line:
                        try:
                            f = json.loads(line)
                            findings.append(f)
                        except json.JSONDecodeError:
                            pass
            elif block["type"] == "tool_use":
```

新：

```python
            if block["type"] == "text" and block.get("text"):
                assistant_content.append({"type": "text", "text": block["text"]})
                logger.debug("text block", chars=len(block['text']), preview=block['text'][:200])
                block_findings, parse_errors = extract_findings(block["text"])
                findings.extend(block_findings)
                for err in parse_errors:
                    logger.warn("finding parse failed", error=err, turn=turn + 1)
                    tracer.event(
                        name="finding_parse_error",
                        level="WARNING",
                        metadata={"turn": turn + 1, "error": err},
                    )
                    reporter.record_llm_event(
                        "finding_parse_error",
                        {"turn": str(turn + 1), "error": err},
                    )
            elif block["type"] == "tool_use":
```

(c) 更新 prompt 模板。修改 `build_prompt` (line 57-72)：

旧：

```python
    return f"""You are a Kubernetes diagnostic orchestrator.

{lang_instruction}

Target namespaces: {TARGET_NAMESPACES}

Available diagnostic skills:
{skill_list}

Instructions:
1. For each skill, analyze the cluster in the target namespaces.
2. Use the available MCP tools to gather data.
3. For each issue found, output a finding JSON object on its own line:
   {{"dimension":"<dim>","severity":"<critical|high|medium|low|info>","title":"<title>","description":"<desc>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<suggestion>"}}
4. After all skills complete, output: FINDINGS_COMPLETE
"""
```

新：

```python
    return f"""You are a Kubernetes diagnostic orchestrator.

{lang_instruction}

Target namespaces: {TARGET_NAMESPACES}

Available diagnostic skills:
{skill_list}

Instructions:
1. For each skill, analyze the cluster in the target namespaces.
2. Use the available MCP tools to gather data.
3. For each issue found, emit ONE line in this exact format (literal `FINDING_JSON: ` prefix, single-line JSON, no markdown fence, no trailing commentary on the same line):
   FINDING_JSON: {{"dimension":"<health|security|cost|reliability>","severity":"<critical|high|medium|low|info>","title":"<title>","description":"<desc>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<suggestion>"}}
   All eight fields are REQUIRED. `dimension` and `severity` MUST be one of the listed enum values.
4. After all skills complete, output: FINDINGS_COMPLETE
"""
```

注：原来的 `test_contains_skill_info` 检查 `"FINDINGS_COMPLETE" in prompt`，新模板仍然包含此字符串，无须改 test。

- [ ] **Step 2.4: Run the failing + previously failing tests, expect pass**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py -v`
Expected: 全部 PASS（4 + 5 = 9 in TestRunAgent；TestBuildPrompt 4 个；TestExtractFindings 7 个）

- [ ] **Step 2.5: Verify build_prompt content sanity**

Run: `cd agent-runtime && python -c "from runtime.orchestrator import build_prompt; from runtime.skill_loader import Skill; print(build_prompt([Skill(name='t', dimension='h', tools=[], prompt='p')]))"`
Expected: 输出包含 `FINDING_JSON: ` 字符串和 `FINDINGS_COMPLETE`。

- [ ] **Step 2.6: Commit**

```bash
git add agent-runtime/runtime/orchestrator.py agent-runtime/tests/test_orchestrator.py
git commit -m "refactor(orchestrator): switch findings extraction to FINDING_JSON: prefix

Replaces the legacy startswith('{') + 'dimension' in line heuristic with
extract_findings (issue #44 schema-checked parser). Parse errors flow
through logger.warn + tracer.event + reporter.record_llm_event for
ops visibility. Prompt template updated to instruct the LLM to use the
explicit prefix; markdown code fences and pretty-printed JSON no longer
contaminate the findings table.

Refs #44"
```

---

## Task 3: 验收前最终检查

- [ ] **Step 3.1: Lint Python**

Run: `cd agent-runtime && python -m pyflakes runtime/`
Expected: 无新增 warning。

- [ ] **Step 3.2: Run full Python test suite**

Run: `cd agent-runtime && python -m pytest tests/ -v`
Expected: 全部 PASS。

- [ ] **Step 3.3: Run full Go test suite (sanity)**

Run: `go test ./...`
Expected: 全部 PASS（本 PR 不触 Go，纯 sanity check）.

- [ ] **Step 3.4: Confirm no `startswith("{") and "dimension"` heuristic remains**

Run: `grep -rn 'startswith("{")' agent-runtime/runtime/`
Expected: 无输出（旧路径已删）。

- [ ] **Step 3.5: 留待 user push + open PR**

不在本会话内 push。

---

## 验收对照表（issue #44）

| issue 验收项 | 对应 task |
|------|---------|
| `FINDING_JSON:` 前缀生效，旧 startswith 路径删除 | Task 1（添加） + Task 2（替换调用方 + 删除旧路径） |
| 解析失败有 metric/log | Task 2（logger.warn + tracer.event + reporter.record_llm_event）— Prometheus 跳过（agent-runtime 未引入 prometheus_client；reporter 事件通道作为替代，控制器侧统一聚合） |
| markdown code fence 内的示例 JSON 不再被误抓（回归测试） | Task 1 (`test_markdown_code_fence_json_not_extracted`) |
| 缺字段 / 错误 enum 不进 findings 表 | Task 1 (`test_missing_required_field_rejected`, `test_invalid_severity_enum_rejected`, `test_multiple_findings_one_invalid_enum`) |

## 已知未实现 / 推迟

- **Pydantic schema 校验** — 用 frozenset + 手工校验代替（避免引入新依赖）。如果未来引入 pydantic，可平移此处的校验逻辑。
- **Prometheus `findings_parse_errors_total` / `findings_submitted_total` counter** — 推迟。reporter.record_llm_event 已经把 finding_parse_error 上报到控制器；控制器侧的 Prometheus 由 `metrics` 包统一处理。如果以后需要单独的 agent-pod 内 Prometheus 端点，再加依赖与采集 endpoint。
