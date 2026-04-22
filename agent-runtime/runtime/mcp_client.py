"""MCP stdio client for the in-cluster k8s-mcp-server.

Extracted from orchestrator.py so fix_main.py can reuse the same helpers
without circular imports.
"""
import json
import os
import subprocess

MCP_SERVER_PATH = os.environ.get("MCP_SERVER_PATH", "/usr/local/bin/k8s-mcp-server")
PROMETHEUS_URL = os.environ.get("PROMETHEUS_URL", "")

def _mcp_cmd() -> list:
    """Build the base MCP server command, optionally with --prometheus-url."""
    cmd = [MCP_SERVER_PATH, "--in-cluster"]
    if PROMETHEUS_URL:
        cmd += ["--prometheus-url", PROMETHEUS_URL]
    return cmd


def discover_tools() -> list:
    """Query k8s-mcp-server for available tools via MCP initialize."""
    try:
        proc = subprocess.run(
            _mcp_cmd(),
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
        from . import logger
        logger.warn("tool discovery failed", error=str(e))
    return []


def _mcp_to_anthropic_tool(t: dict) -> dict:
    return {
        "name": t["name"],
        "description": t.get("description", ""),
        "input_schema": t.get("inputSchema", {"type": "object", "properties": {}}),
    }


def call_mcp_tool(name: str, args: dict) -> str:
    """Call a tool on k8s-mcp-server via MCP stdio protocol.

    Returns the raw text result string, or an error-prefixed string.
    """
    request = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "tools/call",
        "params": {"name": name, "arguments": args},
    })
    try:
        proc = subprocess.run(
            _mcp_cmd(),
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
