"""LLM 多轮编排核心。

run_agent() 的循环：

    [discover_tools] ── MCP 启动子进程，列出 15+ 个诊断工具
            ↓
    [build_prompt]   ── 拼出系统 prompt：诊断目标、技能列表、输出格式
            ↓
    ┌─ for turn in 0..MAX_TURNS:
    │     ┌─ Claude 流式 API 推理
    │     │     ├─ 收到 text 块 → 扫 JSON finding，加入 findings[]
    │     │     ├─ 收到 tool_use 块 → 转 call_mcp_tool() 拿结果
    │     │     └─ 把工具结果回喂给 LLM 作为 user 消息
    │     └─ stop_reason == "end_turn" → break
    └────────────────────────────────────────────
            ↓
    return findings  ── reporter 逐条 POST 给 controller

特殊处理：
    - 用 _stream_message() 直接走 SSE，绕开 Anthropic SDK 在某些代理上的累加 bug
    - thinking 块（extended thinking）丢弃不送给下一轮（防 token 浪费）
    - OUTPUT_LANGUAGE=zh 时 LLM 用中文写 title/desc/suggestion，
      枚举字段（dimension/severity）保持英文
"""
import json
import os
from typing import List

from . import logger
from . import tracer as _tracer_mod
from .mcp_client import discover_tools, call_mcp_tool
from .model_chain import ModelChain
from .skill_loader import Skill


TARGET_NAMESPACES = os.environ.get("TARGET_NAMESPACES", "default")

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


def build_prompt(skills: List[Skill]) -> str:
    skill_list = "\n".join(
        f"- **{s.name}** ({s.dimension}): {s.prompt[:200]}..."
        for s in skills
    )
    output_lang = os.environ.get("OUTPUT_LANGUAGE", "en")
    if output_lang == "zh":
        lang_instruction = (
            "Output the `title`, `description`, and `suggestion` fields in Simplified Chinese "
            "(简体中文). Keep enum fields (dimension, severity, resource_kind, resource_namespace, "
            "resource_name) as English values."
        )
    else:
        lang_instruction = (
            "Output the `title`, `description`, and `suggestion` fields in English. "
            "Keep enum fields (dimension, severity, resource_kind, resource_namespace, "
            "resource_name) as English values."
        )
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


def run_agent(skills: List[Skill], tracer=None) -> List[dict]:
    """Run the agentic loop using streaming API and return a list of findings."""
    if tracer is None:
        tracer = _tracer_mod._NoOp()

    chain = ModelChain.from_env()
    model = chain.endpoints[0].model  # Used as Langfuse generation label

    tools = discover_tools()
    logger.info("discovered MCP tools", count=len(tools))
    if tools:
        logger.info("tools", names=[t['name'] for t in tools])

    prompt = build_prompt(skills)
    messages = [{"role": "user", "content": prompt}]

    findings = []
    max_turns = int(os.environ.get("MAX_TURNS", "10"))
    max_tokens_continue_limit = int(os.environ.get("MAX_TOKENS_CONTINUE_LIMIT", "3"))
    max_tokens_behavior = os.environ.get("MAX_TOKENS_BEHAVIOR", "continue")
    if max_tokens_behavior not in ("continue", "fail"):
        logger.warn(
            "unknown MAX_TOKENS_BEHAVIOR value, defaulting to continue",
            value=max_tokens_behavior,
        )
        max_tokens_behavior = "continue"
    consecutive_max_tokens = 0

    for turn in range(max_turns):
        logger.info("turn", turn=turn + 1, max_turns=max_turns)

        # ModelChain handles retry + fallback decisions internally;
        # raises ModelChainExhausted if every endpoint fails.
        response = chain.invoke(tools, messages, tracer)
        logger.info("response", stop_reason=response['stop_reason'], blocks=len(response['content']))

        # Record LLM turn as Langfuse generation.
        # sanitize_messages strips raw tool_result payloads before sending to
        # an external service to prevent sensitive cluster data leakage.
        text_output = next((b["text"] for b in response["content"] if b["type"] == "text"), "")
        tracer.generation(
            name=f"turn-{turn + 1}",
            model=model,
            input=_tracer_mod.sanitize_messages(messages),
            output=text_output,
            usage={"input": response.get("input_tokens", 0), "output": response.get("output_tokens", 0)},
            metadata={"stop_reason": response["stop_reason"], "turn": turn + 1},
        )

        # Build assistant message content for conversation history
        assistant_content = []
        for block in response["content"]:
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
                assistant_content.append(block)
                logger.debug("tool_use", tool=block['name'], input=block.get('input', {}))

        if not assistant_content:
            logger.warn("empty response from model, stopping")
            break

        messages.append({"role": "assistant", "content": assistant_content})

        if response["stop_reason"] == "end_turn":
            break

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
                    turn=turn + 1,
                    consecutive=consecutive_max_tokens,
                    limit=max_tokens_continue_limit,
                )
                break
            # behavior == "continue": loop iterates again with the now-extended
            # messages history; assistant_content has already been appended above.
        else:
            logger.warn("unexpected stop_reason, stopping", stop_reason=response['stop_reason'])
            break

    return findings

