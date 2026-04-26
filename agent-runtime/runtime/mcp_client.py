"""MCP stdio 客户端，封装与同 Pod 内 k8s-mcp-server 二进制的 JSON-RPC 通信。

为什么用 subprocess+stdio 而不是 HTTP/socket：
    - MCP 协议官方推荐 stdio 传输（mcp-go 的 server.NewStdioServer）
    - Pod 内进程间通信最简单，无端口、无证书、无超时配置
    - 每次调用都是"启动 → 三次 JSON-RPC（initialize / initialized / call）→ 退出"
      短生命周期 + 隔离，进程崩溃不影响其它工具

两个核心函数：
    - discover_tools()：tools/list，用于 LLM 工具清单（Anthropic schema 格式）
    - call_mcp_tool(name, args)：tools/call，返回工具的 text 输出

被 orchestrator.py 和 fix_main.py 共用，所以单独抽出来。
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
