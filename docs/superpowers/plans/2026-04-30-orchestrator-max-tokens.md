# Orchestrator `max_tokens` ENV + Continue 续写护栏 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修 #42 — agent-runtime 的 `max_tokens` 默认从 4096 提到 8192；orchestrator 在 `stop_reason == "max_tokens"` 命中时默认 continue 续写（带死循环护栏），可通过 ENV `MAX_TOKENS_BEHAVIOR=fail` 切换为 fail；Translator 把 `MAX_TOKENS` 注入到 Agent Pod env，单测覆盖。

**Architecture:** 三层修改：
1. **`agent-runtime/runtime/model_chain.py:89`** — ENV 已经在读，只把默认值从 `"4096"` 改成 `"8192"`。
2. **`agent-runtime/runtime/orchestrator.py:140-157`** — 在 `tool_use` 与 `else` 之间插入 `elif stop_reason == "max_tokens":` 分支：默认 `continue`（让 LLM 在已存的 messages 历史上续写下一 turn），通过同一 turn 链上的连续 `max_tokens` 计数实现死循环护栏（`MAX_TOKENS_CONTINUE_LIMIT`，默认 3）；ENV `MAX_TOKENS_BEHAVIOR=fail` 时直接 break。
3. **`internal/controller/translator/translator.go:185`** — `baseEnv` 加入 `MAX_TOKENS`，值取自新加的 `Config.MaxTokens`（默认 8192）。

**Tech Stack:** Python 3.11+ (`pytest`、`unittest.mock`)、Go (controller-runtime、testify)、httpx 直连 SSE。

**Spec:** GitHub issue #42

---

## File Structure

**Modify:**
- `agent-runtime/runtime/model_chain.py:89` — bump default
- `agent-runtime/runtime/orchestrator.py` — `run_agent` 内加 max_tokens 分支与死循环护栏
- `agent-runtime/tests/test_model_chain.py` — 加 default 8192 验证 test
- `agent-runtime/tests/test_orchestrator.py` — 加 continue / death-loop / fail 三个 test
- `internal/controller/translator/translator.go` — `Config.MaxTokens` 字段 + baseEnv 注入
- `internal/controller/translator/translator_test.go` — 验证 MAX_TOKENS env 注入

**Do not modify:**
- ModelConfig CRD（per-run override 是更大的需求，超出 #42 范围）
- `_stream_one` 函数体本身（除 default 外）

---

## Task 1: `_stream_one` `max_tokens` default 4096 → 8192

**Files:**
- Modify: `agent-runtime/runtime/model_chain.py:89`
- Test: `agent-runtime/tests/test_model_chain.py`

- [ ] **Step 1.1: Write failing test asserting default max_tokens is 8192**

在 `agent-runtime/tests/test_model_chain.py` 的 `class TestStreamOne` 末尾追加（紧跟 `test_thinking_blocks_dropped` 后）：

```python
    def test_default_max_tokens_is_8192(self, monkeypatch):
        """When MAX_TOKENS env is unset, _stream_one must request 8192."""
        monkeypatch.delenv("MAX_TOKENS", raising=False)
        events = ['{"type":"message_stop"}']
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch("runtime.model_chain.httpx.stream") as mock_stream:
            mock_stream.return_value = _fake_stream(_make_sse_lines(events))
            _stream_one(ep, tools=[], messages=[])

        _, kwargs = mock_stream.call_args
        assert kwargs["json"]["max_tokens"] == 8192

    def test_max_tokens_env_override(self, monkeypatch):
        """MAX_TOKENS env overrides the default."""
        monkeypatch.setenv("MAX_TOKENS", "16384")
        events = ['{"type":"message_stop"}']
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch("runtime.model_chain.httpx.stream") as mock_stream:
            mock_stream.return_value = _fake_stream(_make_sse_lines(events))
            _stream_one(ep, tools=[], messages=[])

        _, kwargs = mock_stream.call_args
        assert kwargs["json"]["max_tokens"] == 16384
```

- [ ] **Step 1.2: Run the failing test**

Run: `cd agent-runtime && python -m pytest tests/test_model_chain.py::TestStreamOne::test_default_max_tokens_is_8192 -v`
Expected: FAIL — `assert 4096 == 8192`

- [ ] **Step 1.3: Bump default**

替换 `agent-runtime/runtime/model_chain.py:89`：

```python
        "max_tokens": int(os.environ.get("MAX_TOKENS", "4096")),
```

为：

```python
        "max_tokens": int(os.environ.get("MAX_TOKENS", "8192")),
```

- [ ] **Step 1.4: Run the tests, expect pass**

Run: `cd agent-runtime && python -m pytest tests/test_model_chain.py::TestStreamOne -v`
Expected: 全部 PASS（含 6 个原有 + 2 个新增 = 8 个）

- [ ] **Step 1.5: Run full model_chain test file to confirm no regression**

Run: `cd agent-runtime && python -m pytest tests/test_model_chain.py -v`
Expected: 全部 PASS

- [ ] **Step 1.6: Commit**

```bash
git add agent-runtime/runtime/model_chain.py agent-runtime/tests/test_model_chain.py
git commit -m "fix(agent-runtime): bump default MAX_TOKENS from 4096 to 8192

Claude 4.x models support far higher single-turn output limits than 4096.
At 4096, large-cluster diagnostics routinely hit max_tokens during
tool_use input_json_delta streaming, corrupting tool calls. Default to
8192 (still a safe lower bound; ENV override unchanged).

Refs #42"
```

---

## Task 2: orchestrator `stop_reason == "max_tokens"` continue + 护栏

**Files:**
- Modify: `agent-runtime/runtime/orchestrator.py`
- Test: `agent-runtime/tests/test_orchestrator.py`

- [ ] **Step 2.1: Write failing tests for continue + death-loop + fail**

在 `agent-runtime/tests/test_orchestrator.py` 的 `class TestRunAgent` 末尾追加：

```python
    def test_max_tokens_continues_by_default(self, monkeypatch):
        """stop_reason=max_tokens with default behavior should continue to next turn."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "5")
        monkeypatch.delenv("MAX_TOKENS_BEHAVIOR", raising=False)
        monkeypatch.delenv("MAX_TOKENS_CONTINUE_LIMIT", raising=False)

        finding_json = json.dumps({
            "dimension": "health", "severity": "critical",
            "title": "Found", "resource_kind": "Pod",
            "resource_namespace": "default", "resource_name": "p",
        })
        truncated = {
            "content": [{"type": "text", "text": "partial analysis..."}],
            "stop_reason": "max_tokens",
            "input_tokens": 0, "output_tokens": 0,
        }
        completion = {
            "content": [{"type": "text", "text": f"{finding_json}\nFINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock([truncated, completion])

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert chain.invoke.call_count == 2, "should continue past max_tokens"
        assert len(findings) == 1
        assert findings[0]["title"] == "Found"

    def test_max_tokens_death_loop_breaks_at_limit(self, monkeypatch):
        """Consecutive max_tokens beyond the configured limit must stop the loop."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "10")
        monkeypatch.setenv("MAX_TOKENS_CONTINUE_LIMIT", "3")
        monkeypatch.delenv("MAX_TOKENS_BEHAVIOR", raising=False)

        truncated = {
            "content": [{"type": "text", "text": "..."}],
            "stop_reason": "max_tokens",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock(truncated)  # always returns max_tokens

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        # Limit is 3 → expect at most 3 invokes before the death-loop guard fires.
        assert chain.invoke.call_count == 3, f"expected 3 invokes, got {chain.invoke.call_count}"
        assert findings == []

    def test_max_tokens_behavior_fail_stops_immediately(self, monkeypatch):
        """MAX_TOKENS_BEHAVIOR=fail must break the loop on the first max_tokens hit."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "10")
        monkeypatch.setenv("MAX_TOKENS_BEHAVIOR", "fail")

        truncated = {
            "content": [{"type": "text", "text": "..."}],
            "stop_reason": "max_tokens",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock(truncated)

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert chain.invoke.call_count == 1, "fail behavior must stop after first hit"
        assert findings == []

    def test_max_tokens_counter_resets_on_non_max_tokens(self, monkeypatch):
        """A non-max_tokens turn must reset the consecutive counter."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "10")
        # limit=3: with `>=` semantics, allows 2 consecutive max_tokens before
        # break; the test sequence has 2-then-2 around a tool_use reset, so
        # with limit=3 neither pair triggers the guard.
        monkeypatch.setenv("MAX_TOKENS_CONTINUE_LIMIT", "3")
        monkeypatch.delenv("MAX_TOKENS_BEHAVIOR", raising=False)

        max_t = {
            "content": [{"type": "text", "text": "..."}],
            "stop_reason": "max_tokens",
            "input_tokens": 0, "output_tokens": 0,
        }
        tool_call = {
            "content": [{"type": "tool_use", "id": "tu", "name": "kubectl_get", "input": {}}],
            "stop_reason": "tool_use",
            "input_tokens": 0, "output_tokens": 0,
        }
        end = {
            "content": [{"type": "text", "text": "FINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
            "input_tokens": 0, "output_tokens": 0,
        }
        # Sequence: max, max, tool_use (resets counter), max, max, end_turn → 6 invokes total
        chain = _make_chain_mock([max_t, max_t, tool_call, max_t, max_t, end])

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                with patch("runtime.orchestrator.call_mcp_tool", return_value="{}"):
                    run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert chain.invoke.call_count == 6, f"counter should reset after tool_use; got {chain.invoke.call_count}"
```

- [ ] **Step 2.2: Run the failing tests**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py::TestRunAgent -v -k max_tokens`
Expected: 4 个新测试全 FAIL（第一个会因为 max_tokens 进入 else 分支并 break，invoke 仅 1 次而非 2 次；第二个会因为没有 limit 跑满 MAX_TURNS=10）

- [ ] **Step 2.3: Implement the `max_tokens` branch in `run_agent`**

修改 `agent-runtime/runtime/orchestrator.py` —— 在 `run_agent` 函数体内：

**(a)** 在 `findings = []` 之后（约 line 91）追加初始化：

```python
    findings = []
    max_turns = int(os.environ.get("MAX_TURNS", "10"))
    max_tokens_continue_limit = int(os.environ.get("MAX_TOKENS_CONTINUE_LIMIT", "3"))
    max_tokens_behavior = os.environ.get("MAX_TOKENS_BEHAVIOR", "continue")
    consecutive_max_tokens = 0
```

**(b)** 替换原有的 `tool_use` / `else` 分支（约 line 143-157）：

旧代码：

```python
        if response["stop_reason"] == "tool_use":
            tool_results = []
            for block in response["content"]:
                if block["type"] == "tool_use":
                    result = call_mcp_tool(block["name"], block["input"])
                    logger.debug("tool result", tool=block['name'], preview=result[:200])
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block["id"],
                        "content": result,
                    })
            messages.append({"role": "user", "content": tool_results})
        else:
            logger.warn("unexpected stop_reason, stopping", stop_reason=response['stop_reason'])
            break
```

替换为：

```python
        if response["stop_reason"] == "tool_use":
            consecutive_max_tokens = 0
            tool_results = []
            for block in response["content"]:
                if block["type"] == "tool_use":
                    result = call_mcp_tool(block["name"], block["input"])
                    logger.debug("tool result", tool=block['name'], preview=result[:200])
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block["id"],
                        "content": result,
                    })
            messages.append({"role": "user", "content": tool_results})
        elif response["stop_reason"] == "max_tokens":
            logger.warn(
                "hit max_tokens",
                turn=turn + 1,
                behavior=max_tokens_behavior,
                consecutive=consecutive_max_tokens + 1,
                limit=max_tokens_continue_limit,
            )
            tracer.event(
                name="max_tokens_hit",
                level="WARNING",
                metadata={
                    "turn": turn + 1,
                    "behavior": max_tokens_behavior,
                    "consecutive": consecutive_max_tokens + 1,
                },
            )
            if max_tokens_behavior == "fail":
                break
            consecutive_max_tokens += 1
            if consecutive_max_tokens >= max_tokens_continue_limit:
                logger.warn(
                    "max_tokens continue limit reached, stopping",
                    limit=max_tokens_continue_limit,
                )
                break
            # behavior == "continue": loop iterates again with the now-extended
            # messages history; assistant_content has already been appended above.
        else:
            logger.warn("unexpected stop_reason, stopping", stop_reason=response['stop_reason'])
            break
```

注意三处细节：
1. `tool_use` 分支首行加 `consecutive_max_tokens = 0`（成功的 tool_use 也算 reset 的依据）。
2. `end_turn` 直接 break，无需 reset（出 loop 后变量销毁）。
3. `max_tokens` 分支先 increment 后判断（保证第 N 次连续命中是真的第 N 次而不是 N+1）。修正：plan 中 `consecutive_max_tokens + 1` 用在 logger 的展示，递增放在 if 之外，比较时是 `>=`。看实现，逻辑等价于：每次进入 max_tokens 分支就 +1，达到 limit 即 break。

- [ ] **Step 2.4: Run the failing tests, expect pass**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py::TestRunAgent -v -k max_tokens`
Expected: 4 个 test 全部 PASS

- [ ] **Step 2.5: Run full orchestrator test file to confirm no regression**

Run: `cd agent-runtime && python -m pytest tests/test_orchestrator.py -v`
Expected: 全部 PASS（4 个 + 4 个原 = 8 个 in TestRunAgent；TestBuildPrompt 4 个不动）

- [ ] **Step 2.6: Commit**

```bash
git add agent-runtime/runtime/orchestrator.py agent-runtime/tests/test_orchestrator.py
git commit -m "feat(orchestrator): handle stop_reason=max_tokens with continue + death-loop guard

Default behavior is continue: keep iterating run_agent's loop with the
existing messages history so the LLM can pick up where it left off. A
consecutive-hit counter (MAX_TOKENS_CONTINUE_LIMIT, default 3) prevents
runaway loops when the model can't fit a single response. ENV
MAX_TOKENS_BEHAVIOR=fail switches to immediate break + Langfuse trace.

Refs #42"
```

---

## Task 3: Translator 注入 `MAX_TOKENS` 到 Agent Pod env

**Files:**
- Modify: `internal/controller/translator/translator.go`
- Test: `internal/controller/translator/translator_test.go`

- [ ] **Step 3.1: Write failing Go test**

在 `internal/controller/translator/translator_test.go` 末尾追加（确认 `envByName` 或 `envMap` helper 已存在 — 看现有测试用什么模式）：

先 grep 现有 envMap 模式：

Run: `grep -n "envMap\|TARGET_NAMESPACES" internal/controller/translator/translator_test.go | head -10`

预期看到 `envMap[\"TARGET_NAMESPACES\"]` 类似用法。沿用同模式追加：

```go
func TestTranslator_InjectsMaxTokens(t *testing.T) {
	tr := translator.New(translator.Config{
		AgentImage:    "agent:test",
		ControllerURL: "http://ctrl:8080",
		MaxTokens:     8192,
	}, &mockSkillProvider{skills: []*store.Skill{
		{Name: "pod-health", Dimension: "health", Prompt: "p", ToolsJSON: "[]", Enabled: true},
	}})

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "uid"},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health"},
			ModelConfigRef: "claude-default",
		},
	}

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	// Find the Job and inspect env
	var job *batchv1.Job
	for _, obj := range objects {
		if j, ok := obj.(*batchv1.Job); ok {
			job = j
			break
		}
	}
	require.NotNil(t, job, "Compile must produce a Job")

	envMap := make(map[string]string)
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "8192", envMap["MAX_TOKENS"])
}

func TestTranslator_DefaultMaxTokensWhenZero(t *testing.T) {
	// When Config.MaxTokens is 0 (unset), Translator must inject 8192 default.
	tr := translator.New(translator.Config{
		AgentImage:    "agent:test",
		ControllerURL: "http://ctrl:8080",
		// MaxTokens: 0 (default zero-value)
	}, &mockSkillProvider{skills: []*store.Skill{
		{Name: "pod-health", Dimension: "health", Prompt: "p", ToolsJSON: "[]", Enabled: true},
	}})

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "uid"},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health"},
			ModelConfigRef: "claude-default",
		},
	}

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)
	var job *batchv1.Job
	for _, obj := range objects {
		if j, ok := obj.(*batchv1.Job); ok {
			job = j
			break
		}
	}
	require.NotNil(t, job)
	envMap := make(map[string]string)
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "8192", envMap["MAX_TOKENS"], "zero MaxTokens must fall back to 8192 default")
}
```

注意：测试假设 `mockSkillProvider` 已经定义在 `translator_test.go`（`internal/controller/translator/translator_test.go`）—— 如果不在，看现有测试用什么 provider 套路，沿用。

注：如果 `Compile` 需要 `client.Client` 等额外依赖（看现有的 `TestTranslator_*` 测试怎么构造），按现有模式搭。如果现有测试用的是 `tr := translator.New(translator.Config{...}, provider)` 这种最简形式，直接抄。

- [ ] **Step 3.2: Run the failing tests**

Run: `go test ./internal/controller/translator/ -run TestTranslator_InjectsMaxTokens -v`
Expected: FAIL — `Config has no field MaxTokens` (compile error)

- [ ] **Step 3.3: Add `Config.MaxTokens` field + baseEnv injection**

修改 `internal/controller/translator/translator.go`：

(a) 在 `Config` struct (line 39) 加字段：

旧：

```go
type Config struct {
	AgentImage          string
	ControllerURL       string
	AnthropicBaseURL    string
	Model               string
	PrometheusURL       string
	LangfuseSecretName  string // optional; if set, injects LANGFUSE_* env vars
}
```

新：

```go
type Config struct {
	AgentImage          string
	ControllerURL       string
	AnthropicBaseURL    string
	Model               string
	PrometheusURL       string
	LangfuseSecretName  string // optional; if set, injects LANGFUSE_* env vars
	MaxTokens           int    // optional; 0 = use defaultMaxTokens (8192)
}
```

(b) 在文件顶部（imports 之后，types 之前）加常量：

```go
// defaultMaxTokens is the per-request output cap injected into the agent pod
// env when Config.MaxTokens is unset. Matches agent-runtime's own default so
// behavior is identical whether the env is injected or not.
const defaultMaxTokens = 8192
```

(c) 在 `baseEnv` 列表 (line 185-198) 内追加 MAX_TOKENS:

旧：

```go
	baseEnv := []corev1.EnvVar{
		{Name: "RUN_ID", Value: runID},
		{Name: "TARGET_NAMESPACES", Value: strings.Join(run.Spec.Target.Namespaces, ",")},
		{Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
		{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
		{Name: "PROMETHEUS_URL", Value: t.cfg.PrometheusURL},
		{Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
		{Name: "OUTPUT_LANGUAGE", Value: func() string {
			if run.Spec.OutputLanguage != "" {
				return run.Spec.OutputLanguage
			}
			return "en"
		}()},
	}
```

新（在 OUTPUT_LANGUAGE 之后追加 MAX_TOKENS）：

```go
	maxTokens := t.cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}
	baseEnv := []corev1.EnvVar{
		{Name: "RUN_ID", Value: runID},
		{Name: "TARGET_NAMESPACES", Value: strings.Join(run.Spec.Target.Namespaces, ",")},
		{Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
		{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
		{Name: "PROMETHEUS_URL", Value: t.cfg.PrometheusURL},
		{Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
		{Name: "OUTPUT_LANGUAGE", Value: func() string {
			if run.Spec.OutputLanguage != "" {
				return run.Spec.OutputLanguage
			}
			return "en"
		}()},
		{Name: "MAX_TOKENS", Value: strconv.Itoa(maxTokens)},
	}
```

并 import `strconv`（如未导入）。

- [ ] **Step 3.4: Run the failing tests, expect pass**

Run: `go test ./internal/controller/translator/ -run TestTranslator_InjectsMaxTokens -v && go test ./internal/controller/translator/ -run TestTranslator_DefaultMaxTokensWhenZero -v`
Expected: 两个 test 全部 PASS

- [ ] **Step 3.5: Run full translator suite + build to confirm no regression**

Run: `go test ./internal/controller/translator/... && go build ./...`
Expected: 全部 PASS，零 build error

- [ ] **Step 3.6: Commit**

```bash
git add internal/controller/translator/translator.go internal/controller/translator/translator_test.go
git commit -m "feat(translator): inject MAX_TOKENS env var into agent pod

Adds Config.MaxTokens (default 8192 via defaultMaxTokens const) so the
controller can set the per-request output cap consumed by agent-runtime.
Without this injection, the pod fell back to its own runtime default,
making MAX_TOKENS ungovernable from the controller side.

Refs #42"
```

---

## Task 4: 调用方 Translator 配置位置 — 是否需要 wire-up

调用 `translator.New(translator.Config{...})` 的地方需要确认是否要传 `MaxTokens`。在不破坏现有调用的前提下保持零改动（zero-value 0 → falls back to 8192 by Task 3 的 default 逻辑）。

- [ ] **Step 4.1: Identify all `translator.New` call sites**

Run: `grep -rn "translator.New" --include="*.go"`
Expected output 列出所有调用方（一般在 `cmd/controller/main.go` 或 `internal/controller/manager.go` 之类）。

- [ ] **Step 4.2: Verify zero-value compatibility**

读每个调用方的 Config 字面量。如果都没设 `MaxTokens`，Task 3 的零值兜底会自动用 8192，无需改动。

如果 main 已经从 ENV 读 controller-level 配置（如 `os.Getenv("MAX_TOKENS")`），可以选择性追加：

```go
maxTokens, _ := strconv.Atoi(os.Getenv("CONTROLLER_AGENT_MAX_TOKENS"))
translator.New(translator.Config{
    ...,
    MaxTokens: maxTokens,  // 0 → falls back to 8192 default in translator
}, provider)
```

但**这一步不强制做**——保留为可选增强，不在 #42 PR 范围内（除非调用方现有就有读 env 的 idiom）。

- [ ] **Step 4.3: Do nothing or commit a tiny call-site change**

如果做了修改，commit；否则跳过。

---

## Task 5: 验收前最终检查

- [ ] **Step 5.1: Lint + format**

Run: `gofmt -l internal/controller/translator/`
Expected: 无输出（如有 pre-existing 违规，按 #43 PR 的处理风格在 PR body 中说明）

Run: `go vet ./...`
Expected: 无输出

Run: `cd agent-runtime && python -m pyflakes runtime/`
Expected: 无输出

- [ ] **Step 5.2: Run full Go test suite**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 5.3: Run full Python test suite**

Run: `cd agent-runtime && python -m pytest tests/ -v`
Expected: 全部 PASS

- [ ] **Step 5.4: 留待 user push + open PR**

不在本会话内 push。Commits 留在本地 `fix/orchestrator-max-tokens` 分支由用户自行 push。

---

## 验收对照表（issue #42）

| issue 验收项 | 对应 task |
|------|---------|
| `MAX_TOKENS` ENV 生效，默认 8192 | Task 1 |
| Translator 注入到 Agent Pod | Task 3 |
| `stop_reason == "max_tokens"` 有显式日志/trace | Task 2（logger.warn + tracer.event） |
| 单测覆盖 | Task 1（default 8192 + ENV override）+ Task 2（continue / death-loop / fail / counter reset）+ Task 3（env injected + zero-value default） |
