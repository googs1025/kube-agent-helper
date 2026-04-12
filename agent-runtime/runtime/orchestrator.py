"""Builds the orchestrator prompt and runs the agentic loop."""
import json
import os
import subprocess
from typing import List

import anthropic

from .skill_loader import Skill


MCP_SERVER_PATH = os.environ.get("MCP_SERVER_PATH", "/usr/local/bin/k8s-mcp-server")
TARGET_NAMESPACES = os.environ.get("TARGET_NAMESPACES", "default")


def build_prompt(skills: List[Skill]) -> str:
    skill_list = "\n".join(
        f"- **{s.name}** ({s.dimension}): {s.prompt[:200]}..."
        for s in skills
    )
    return f"""You are a Kubernetes diagnostic orchestrator.

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
    """Run the agentic loop and return a list of findings."""
    client = anthropic.Anthropic()

    # Build MCP tool definitions by querying k8s-mcp-server
    tools = _discover_tools()

    prompt = build_prompt(skills)
    messages = [{"role": "user", "content": prompt}]

    findings = []
    max_turns = int(os.environ.get("MAX_TURNS", "20"))

    for _ in range(max_turns):
        response = client.messages.create(
            model=os.environ.get("MODEL", "claude-sonnet-4-6"),
            max_tokens=4096,
            tools=tools,
            messages=messages,
        )

        messages.append({"role": "assistant", "content": response.content})

        # Extract text blocks for finding detection
        for block in response.content:
            if hasattr(block, "text"):
                for line in block.text.split("\n"):
                    line = line.strip()
                    if line.startswith("{") and "dimension" in line:
                        try:
                            f = json.loads(line)
                            findings.append(f)
                        except json.JSONDecodeError:
                            pass

        if response.stop_reason == "end_turn":
            break

        if response.stop_reason == "tool_use":
            tool_results = []
            for block in response.content:
                if block.type == "tool_use":
                    result = _call_mcp_tool(block.name, block.input)
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block.id,
                        "content": result,
                    })
            messages.append({"role": "user", "content": tool_results})
        elif response.stop_reason not in ("end_turn", "tool_use"):
            print(f"[warn] unexpected stop_reason: {response.stop_reason}, aborting loop")
            break

    return findings


def _discover_tools() -> list:
    """Query k8s-mcp-server for available tools via MCP initialize."""
    try:
        proc = subprocess.run(
            [MCP_SERVER_PATH, "--in-cluster"],
            input=json.dumps({"jsonrpc":"2.0","id":1,"method":"initialize",
                              "params":{"protocolVersion":"2024-11-05",
                                        "clientInfo":{"name":"agent","version":"0.1"},
                                        "capabilities":{}}}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","method":"notifications/initialized"}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","id":2,"method":"tools/list"}) + "\n",
            capture_output=True, text=True, timeout=10,
        )
        lines = [l for l in proc.stdout.split("\n") if l.strip()]
        for line in lines:
            parsed = json.loads(line)
            if parsed.get("id") == 2 and "result" in parsed:
                mcp_tools = parsed["result"].get("tools", [])
                return [_mcp_to_anthropic_tool(t) for t in mcp_tools]
    except Exception as e:
        print(f"[warn] tool discovery failed: {e}")
    return []


def _mcp_to_anthropic_tool(t: dict) -> dict:
    return {
        "name": t["name"],
        "description": t.get("description", ""),
        "input_schema": t.get("inputSchema", {"type": "object", "properties": {}}),
    }


def _call_mcp_tool(name: str, args: dict) -> str:
    """Call a tool on k8s-mcp-server via MCP stdio protocol."""
    request = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "tools/call",
        "params": {"name": name, "arguments": args},
    })
    try:
        proc = subprocess.run(
            [MCP_SERVER_PATH, "--in-cluster"],
            input=json.dumps({"jsonrpc":"2.0","id":0,"method":"initialize",
                              "params":{"protocolVersion":"2024-11-05",
                                        "clientInfo":{"name":"agent","version":"0.1"},
                                        "capabilities":{}}}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","method":"notifications/initialized"}) + "\n" +
                  request + "\n",
            capture_output=True, text=True, timeout=30,
        )
        for line in proc.stdout.split("\n"):
            if not line.strip():
                continue
            parsed = json.loads(line)
            if parsed.get("id") == 1 and "result" in parsed:
                content = parsed["result"].get("content", [])
                if content:
                    return content[0].get("text", "")
    except Exception as e:
        return f"tool error: {e}"
    return ""
