"""Builds the orchestrator prompt and runs the agentic loop."""
import json
import os
from typing import List

import anthropic

from .mcp_client import discover_tools, call_mcp_tool
from .skill_loader import Skill


TARGET_NAMESPACES = os.environ.get("TARGET_NAMESPACES", "default")


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


def run_agent(skills: List[Skill]) -> List[dict]:
    """Run the agentic loop using streaming API and return a list of findings."""
    client = anthropic.Anthropic()

    tools = discover_tools()
    print(f"[info] discovered {len(tools)} MCP tools")
    if tools:
        print(f"[info] tools: {[t['name'] for t in tools]}")

    prompt = build_prompt(skills)
    messages = [{"role": "user", "content": prompt}]

    findings = []
    max_turns = int(os.environ.get("MAX_TURNS", "10"))

    for turn in range(max_turns):
        print(f"[info] turn {turn+1}/{max_turns}")

        # Use streaming to work with proxies that only support stream mode
        response = _stream_message(client, tools, messages)
        print(f"[info] stop_reason={response['stop_reason']}, blocks={len(response['content'])}")

        # Build assistant message content for conversation history
        assistant_content = []
        for block in response["content"]:
            if block["type"] == "text" and block.get("text"):
                assistant_content.append({"type": "text", "text": block["text"]})
                print(f"[debug] text ({len(block['text'])} chars): {block['text'][:200]}")
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
                print(f"[debug] tool_use: {block['name']}({json.dumps(block.get('input', {}))})")

        if not assistant_content:
            print("[warn] empty response from model, stopping")
            break

        messages.append({"role": "assistant", "content": assistant_content})

        if response["stop_reason"] == "end_turn":
            break

        if response["stop_reason"] == "tool_use":
            tool_results = []
            for block in response["content"]:
                if block["type"] == "tool_use":
                    result = call_mcp_tool(block["name"], block["input"])
                    print(f"[debug] tool result for {block['name']}: {result[:200]}")
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block["id"],
                        "content": result,
                    })
            messages.append({"role": "user", "content": tool_results})
        else:
            print(f"[warn] unexpected stop_reason: {response['stop_reason']}, stopping")
            break

    return findings


def _stream_message(client, tools, messages) -> dict:
    """Stream a message via raw SSE and reconstruct the full response.

    Uses httpx directly to avoid the Anthropic SDK accumulator bug where
    proxies that omit the initial text field in content_block_start cause
    NoneType += str errors.
    """
    import httpx

    headers = {
        "x-api-key": client.api_key,
        "anthropic-version": "2023-06-01",
        "content-type": "application/json",
        "accept": "text/event-stream",
    }

    base_url = os.environ.get("ANTHROPIC_BASE_URL") or "https://api.anthropic.com"
    base_url = base_url.rstrip("/")
    # If base_url already ends with /v1/messages (some proxies), use as-is
    if base_url.endswith("/v1/messages"):
        url = base_url
    else:
        url = base_url + "/v1/messages"

    payload = {
        "model": os.environ.get("MODEL", "claude-sonnet-4-6"),
        "max_tokens": 4096,
        "messages": messages,
        "stream": True,
    }
    if tools:
        payload["tools"] = tools

    content_blocks = {}  # index -> block dict
    stop_reason = "end_turn"

    with httpx.stream("POST", url, headers=headers, json=payload, timeout=120) as resp:
        resp.raise_for_status()
        for raw_line in resp.iter_lines():
            raw_line = raw_line.strip()
            if not raw_line or not raw_line.startswith("data:"):
                continue
            data_str = raw_line[len("data:"):].strip()
            if data_str == "[DONE]":
                break
            try:
                event = json.loads(data_str)
            except json.JSONDecodeError:
                continue

            etype = event.get("type", "")

            if etype == "content_block_start":
                idx = event.get("index", 0)
                block = event.get("content_block", {})
                btype = block.get("type", "")
                if btype == "text":
                    content_blocks[idx] = {"type": "text", "text": ""}
                elif btype == "tool_use":
                    content_blocks[idx] = {
                        "type": "tool_use",
                        "id": block.get("id", ""),
                        "name": block.get("name", ""),
                        "input": "",
                    }
                elif btype == "thinking":
                    content_blocks[idx] = {"type": "thinking", "text": ""}

            elif etype == "content_block_delta":
                idx = event.get("index", 0)
                delta = event.get("delta", {})
                dtype = delta.get("type", "")
                if idx in content_blocks:
                    if dtype == "text_delta":
                        content_blocks[idx]["text"] += delta.get("text", "")
                    elif dtype == "input_json_delta":
                        content_blocks[idx]["input"] += delta.get("partial_json", "")
                    elif dtype == "thinking_delta":
                        content_blocks[idx]["text"] += delta.get("thinking", "")

            elif etype == "message_delta":
                delta = event.get("delta", {})
                if delta.get("stop_reason"):
                    stop_reason = delta["stop_reason"]

    # Post-process: parse tool_use input JSON, drop thinking blocks
    result_blocks = []
    for idx in sorted(content_blocks.keys()):
        block = content_blocks[idx]
        if block["type"] == "thinking":
            continue
        if block["type"] == "tool_use" and isinstance(block["input"], str):
            try:
                block["input"] = json.loads(block["input"]) if block["input"] else {}
            except json.JSONDecodeError:
                block["input"] = {}
        result_blocks.append(block)

    return {"content": result_blocks, "stop_reason": stop_reason}
