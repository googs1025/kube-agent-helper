"""Tests for runtime.mcp_client."""
import json
from unittest.mock import patch, MagicMock
from runtime.mcp_client import (
    _mcp_cmd,
    _mcp_to_anthropic_tool,
    discover_tools,
    call_mcp_tool,
)


class TestMcpCmd:
    def test_without_prometheus(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.MCP_SERVER_PATH", "/usr/bin/mcp")
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        assert _mcp_cmd() == ["/usr/bin/mcp", "--in-cluster"]

    def test_with_prometheus(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.MCP_SERVER_PATH", "/usr/bin/mcp")
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "http://prom:9090")
        assert _mcp_cmd() == ["/usr/bin/mcp", "--in-cluster", "--prometheus-url", "http://prom:9090"]


class TestMcpToAnthropicTool:
    def test_converts_format(self):
        mcp_tool = {
            "name": "kubectl_get",
            "description": "Get resources",
            "inputSchema": {"type": "object", "properties": {"kind": {"type": "string"}}},
        }
        result = _mcp_to_anthropic_tool(mcp_tool)
        assert result["name"] == "kubectl_get"
        assert result["description"] == "Get resources"
        assert "kind" in result["input_schema"]["properties"]

    def test_missing_fields_have_defaults(self):
        result = _mcp_to_anthropic_tool({"name": "test"})
        assert result["description"] == ""
        assert result["input_schema"] == {"type": "object", "properties": {}}


class TestDiscoverTools:
    def test_parses_tools_list(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.MCP_SERVER_PATH", "/bin/echo")
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")

        init_resp = json.dumps({"jsonrpc": "2.0", "id": 1, "result": {"protocolVersion": "2024-11-05"}})
        tools_resp = json.dumps({
            "jsonrpc": "2.0", "id": 2,
            "result": {"tools": [
                {"name": "kubectl_get", "description": "get", "inputSchema": {"type": "object", "properties": {}}},
                {"name": "kubectl_logs", "description": "logs", "inputSchema": {"type": "object", "properties": {}}},
            ]},
        })

        mock_proc = MagicMock()
        mock_proc.stdout = f"{init_resp}\n{tools_resp}\n"

        with patch("runtime.mcp_client.subprocess.run", return_value=mock_proc):
            tools = discover_tools()

        assert len(tools) == 2
        assert tools[0]["name"] == "kubectl_get"
        assert tools[1]["name"] == "kubectl_logs"

    def test_returns_empty_on_failure(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.MCP_SERVER_PATH", "/nonexistent")
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        # subprocess.run will raise FileNotFoundError
        tools = discover_tools()
        assert tools == []

    def test_returns_empty_on_no_tools_response(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        mock_proc = MagicMock()
        mock_proc.stdout = '{"jsonrpc":"2.0","id":1,"result":{}}\n'

        with patch("runtime.mcp_client.subprocess.run", return_value=mock_proc):
            tools = discover_tools()
        assert tools == []


class TestCallMcpTool:
    def test_returns_text_content(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        resp = json.dumps({
            "jsonrpc": "2.0", "id": 1,
            "result": {"content": [{"type": "text", "text": '{"items":[]}'}]},
        })
        mock_proc = MagicMock()
        mock_proc.stdout = f'{{"jsonrpc":"2.0","id":0,"result":{{}}}}\n{resp}\n'

        with patch("runtime.mcp_client.subprocess.run", return_value=mock_proc):
            result = call_mcp_tool("kubectl_get", {"kind": "Pod"})
        assert result == '{"items":[]}'

    def test_returns_error_string_on_exception(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        with patch("runtime.mcp_client.subprocess.run", side_effect=Exception("boom")):
            result = call_mcp_tool("kubectl_get", {})
        assert "tool error" in result

    def test_returns_empty_on_no_content(self, monkeypatch):
        monkeypatch.setattr("runtime.mcp_client.PROMETHEUS_URL", "")
        mock_proc = MagicMock()
        mock_proc.stdout = '{"jsonrpc":"2.0","id":0,"result":{}}\n'

        with patch("runtime.mcp_client.subprocess.run", return_value=mock_proc):
            result = call_mcp_tool("test", {})
        assert result == ""
