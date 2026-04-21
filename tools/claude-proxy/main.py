#!/usr/bin/env python3
"""
Anthropic API proxy for local testing.

Turn 1 (no tool_results in messages):
  - Returns hardcoded tool_use blocks to trigger real MCP tools
    (prometheus_query + prometheus_alerts + kubectl_get)

Turn 2+ (tool_results present):
  - Builds a rich prompt with all context + tool results
  - Calls `claude -p` for actual analysis
  - Returns SSE end_turn response with findings

Listens on :18080 by default.
"""

import json
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 18080

# Hardcoded tool calls for Turn 1
TOOL_USE_BLOCKS = [
    {
        "type": "tool_use",
        "id": "tu_prom_query",
        "name": "prometheus_query",
        "input": {"query": "up", "mode": "instant"},
    },
    {
        "type": "tool_use",
        "id": "tu_prom_alerts",
        "name": "prometheus_alerts",
        "input": {"state": "firing"},
    },
    {
        "type": "tool_use",
        "id": "tu_kubectl_get",
        "name": "kubectl_get",
        "input": {"kind": "Pod"},
    },
]


def _sse(event: dict) -> bytes:
    return ("data: " + json.dumps(event) + "\n\n").encode()


def _has_tool_results(messages: list) -> bool:
    """Check if any message contains tool_result blocks."""
    for msg in messages:
        content = msg.get("content", "")
        if isinstance(content, list):
            for block in content:
                if isinstance(block, dict) and block.get("type") == "tool_result":
                    return True
    return False


def _build_prompt(body: dict) -> str:
    """Build a flat text prompt from the full conversation for claude -p."""
    lines = []

    # System prompt
    system = body.get("system", "")
    if isinstance(system, list):
        system = " ".join(b.get("text", "") for b in system if isinstance(b, dict))
    if system:
        lines.append(f"[System]: {system}\n")

    # Messages
    for msg in body.get("messages", []):
        role = msg.get("role", "")
        content = msg.get("content", "")

        if isinstance(content, str):
            lines.append(f"[{role}]: {content}")
        elif isinstance(content, list):
            parts = []
            for block in content:
                if not isinstance(block, dict):
                    continue
                btype = block.get("type", "")
                if btype == "text":
                    parts.append(block.get("text", ""))
                elif btype == "tool_use":
                    parts.append(
                        f"[called tool '{block.get('name')}' with args: {json.dumps(block.get('input', {}))}]"
                    )
                elif btype == "tool_result":
                    result_text = ""
                    rc = block.get("content", "")
                    if isinstance(rc, str):
                        result_text = rc
                    elif isinstance(rc, list):
                        result_text = "\n".join(
                            b.get("text", "") for b in rc if isinstance(b, dict)
                        )
                    parts.append(f"[tool result]: {result_text}")
            lines.append(f"[{role}]: " + "\n".join(parts))

    lines.append(
        "\nBased on the tool results above, provide a concise diagnostic analysis "
        "in the same language as requested. List any findings with severity (critical/warning/info), "
        "affected resource, and suggestion."
    )
    return "\n".join(lines)


class ProxyHandler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print(f"[proxy] {fmt % args}", file=sys.stderr)

    def do_POST(self):
        if not self.path.rstrip("/").endswith("/v1/messages"):
            self.send_error(404, "Not Found")
            return

        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        messages = body.get("messages", [])

        if not _has_tool_results(messages):
            # Turn 1: return hardcoded tool_use blocks
            self._send_tool_use_sse(body)
        else:
            # Turn 2+: call claude -p with full context, return findings
            self._send_claude_sse(body)

    def _send_tool_use_sse(self, body: dict):
        """Return SSE stream with tool_use blocks."""
        print("[proxy] turn 1 → returning tool_use blocks", file=sys.stderr)
        self._start_sse()

        self._write_sse({"type": "message_start", "message": {
            "id": "msg_proxy_t1", "type": "message", "role": "assistant",
            "model": body.get("model", "claude-proxy"),
            "content": [], "stop_reason": None,
            "usage": {"input_tokens": 0, "output_tokens": 0},
        }})

        for i, block in enumerate(TOOL_USE_BLOCKS):
            self._write_sse({
                "type": "content_block_start", "index": i,
                "content_block": {"type": "tool_use", "id": block["id"],
                                  "name": block["name"], "input": {}},
            })
            self._write_sse({
                "type": "content_block_delta", "index": i,
                "delta": {"type": "input_json_delta",
                          "partial_json": json.dumps(block["input"])},
            })
            self._write_sse({"type": "content_block_stop", "index": i})

        self._write_sse({
            "type": "message_delta",
            "delta": {"stop_reason": "tool_use", "stop_sequence": None},
            "usage": {"output_tokens": 0},
        })
        self._write_sse({"type": "message_stop"})

    def _send_claude_sse(self, body: dict):
        """Call claude -p with full context and stream the response."""
        prompt = _build_prompt(body)
        print(f"[proxy] turn 2 → calling claude -p (prompt length={len(prompt)})", file=sys.stderr)

        try:
            result = subprocess.run(
                ["claude", "-p", prompt],
                capture_output=True, text=True, timeout=120,
            )
            text = result.stdout.strip()
            if result.returncode != 0 or not text:
                text = result.stderr.strip() or "claude CLI returned no output"
        except subprocess.TimeoutExpired:
            text = "Analysis timed out."
        except FileNotFoundError:
            text = "claude CLI not found."

        print(f"[proxy] claude response length={len(text)}", file=sys.stderr)
        self._start_sse()

        self._write_sse({"type": "message_start", "message": {
            "id": "msg_proxy_t2", "type": "message", "role": "assistant",
            "model": body.get("model", "claude-proxy"),
            "content": [], "stop_reason": None,
            "usage": {"input_tokens": 0, "output_tokens": 0},
        }})
        self._write_sse({
            "type": "content_block_start", "index": 0,
            "content_block": {"type": "text", "text": ""},
        })
        self._write_sse({
            "type": "content_block_delta", "index": 0,
            "delta": {"type": "text_delta", "text": text},
        })
        self._write_sse({"type": "content_block_stop", "index": 0})
        self._write_sse({
            "type": "message_delta",
            "delta": {"stop_reason": "end_turn", "stop_sequence": None},
            "usage": {"output_tokens": 0},
        })
        self._write_sse({"type": "message_stop"})

    def _start_sse(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()

    def _write_sse(self, event: dict):
        try:
            self.wfile.write(_sse(event))
            self.wfile.flush()
        except BrokenPipeError:
            pass

    def _json_error(self, code, msg):
        body = json.dumps({"error": {"type": "proxy_error", "message": msg}}).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else PORT
    print(f"[proxy] listening on :{port}", file=sys.stderr)
    HTTPServer(("", port), ProxyHandler).serve_forever()
