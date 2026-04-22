"""Tests for runtime.orchestrator."""
import json
import os
from unittest.mock import patch, MagicMock
from runtime.orchestrator import build_prompt, run_agent, _stream_message
from runtime.skill_loader import Skill


class TestBuildPrompt:
    def test_contains_skill_info(self, monkeypatch):
        monkeypatch.setenv("TARGET_NAMESPACES", "default")
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        skills = [
            Skill(name="pod-health", dimension="health", tools=["kubectl_get"], prompt="Check pods"),
            Skill(name="sec-check", dimension="security", tools=["kubectl_get"], prompt="Check security"),
        ]
        prompt = build_prompt(skills)
        assert "pod-health" in prompt
        assert "sec-check" in prompt
        assert "default" in prompt
        assert "FINDINGS_COMPLETE" in prompt

    def test_chinese_language(self, monkeypatch):
        monkeypatch.setenv("TARGET_NAMESPACES", "kube-system")
        monkeypatch.setenv("OUTPUT_LANGUAGE", "zh")
        prompt = build_prompt([Skill(name="t", dimension="h", tools=[], prompt="p")])
        assert "简体中文" in prompt

    def test_english_language(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        prompt = build_prompt([Skill(name="t", dimension="h", tools=[], prompt="p")])
        assert "English" in prompt

    def test_truncates_long_prompts(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        long_prompt = "x" * 500
        prompt = build_prompt([Skill(name="t", dimension="h", tools=[], prompt=long_prompt)])
        # The skill line should show truncated prompt with ...
        assert "..." in prompt


class TestStreamMessage:
    """Tests for SSE parsing in _stream_message."""

    def _make_sse(self, events: list) -> str:
        lines = []
        for ev in events:
            lines.append(f"data: {json.dumps(ev)}\n")
        return "\n".join(lines)

    def test_parses_text_block(self):
        events = [
            {"type": "message_start", "message": {"id": "m1", "content": []}},
            {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello "}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "world"}},
            {"type": "content_block_stop", "index": 0},
            {"type": "message_delta", "delta": {"stop_reason": "end_turn"}},
        ]
        sse_text = self._make_sse(events)

        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.iter_lines.return_value = iter(sse_text.strip().split("\n"))
        mock_resp.__enter__ = MagicMock(return_value=mock_resp)
        mock_resp.__exit__ = MagicMock(return_value=False)

        mock_client = MagicMock()
        mock_client.api_key = "test-key"

        with patch("httpx.stream", return_value=mock_resp):
            result = _stream_message(mock_client, [], [{"role": "user", "content": "hi"}])

        assert len(result["content"]) == 1
        assert result["content"][0]["type"] == "text"
        assert result["content"][0]["text"] == "Hello world"
        assert result["stop_reason"] == "end_turn"

    def test_parses_tool_use_block(self):
        events = [
            {"type": "message_start", "message": {"id": "m1"}},
            {"type": "content_block_start", "index": 0,
             "content_block": {"type": "tool_use", "id": "tu1", "name": "kubectl_get", "input": {}}},
            {"type": "content_block_delta", "index": 0,
             "delta": {"type": "input_json_delta", "partial_json": '{"kind":'}},
            {"type": "content_block_delta", "index": 0,
             "delta": {"type": "input_json_delta", "partial_json": '"Pod"}'}},
            {"type": "content_block_stop", "index": 0},
            {"type": "message_delta", "delta": {"stop_reason": "tool_use"}},
        ]
        sse_text = self._make_sse(events)

        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.iter_lines.return_value = iter(sse_text.strip().split("\n"))
        mock_resp.__enter__ = MagicMock(return_value=mock_resp)
        mock_resp.__exit__ = MagicMock(return_value=False)

        mock_client = MagicMock()
        mock_client.api_key = "test-key"

        with patch("httpx.stream", return_value=mock_resp):
            result = _stream_message(mock_client, [], [])

        assert result["stop_reason"] == "tool_use"
        assert len(result["content"]) == 1
        block = result["content"][0]
        assert block["type"] == "tool_use"
        assert block["name"] == "kubectl_get"
        assert block["input"] == {"kind": "Pod"}

    def test_drops_thinking_blocks(self):
        events = [
            {"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "text": ""}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "hmm"}},
            {"type": "content_block_start", "index": 1, "content_block": {"type": "text", "text": ""}},
            {"type": "content_block_delta", "index": 1, "delta": {"type": "text_delta", "text": "answer"}},
            {"type": "message_delta", "delta": {"stop_reason": "end_turn"}},
        ]
        sse_text = self._make_sse(events)

        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.iter_lines.return_value = iter(sse_text.strip().split("\n"))
        mock_resp.__enter__ = MagicMock(return_value=mock_resp)
        mock_resp.__exit__ = MagicMock(return_value=False)

        mock_client = MagicMock()
        mock_client.api_key = "test-key"

        with patch("httpx.stream", return_value=mock_resp):
            result = _stream_message(mock_client, [], [])

        # Thinking block should be dropped
        assert len(result["content"]) == 1
        assert result["content"][0]["text"] == "answer"

    def test_invalid_tool_input_json(self):
        events = [
            {"type": "content_block_start", "index": 0,
             "content_block": {"type": "tool_use", "id": "tu1", "name": "test"}},
            {"type": "content_block_delta", "index": 0,
             "delta": {"type": "input_json_delta", "partial_json": "not valid json"}},
            {"type": "message_delta", "delta": {"stop_reason": "tool_use"}},
        ]
        sse_text = self._make_sse(events)

        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.iter_lines.return_value = iter(sse_text.strip().split("\n"))
        mock_resp.__enter__ = MagicMock(return_value=mock_resp)
        mock_resp.__exit__ = MagicMock(return_value=False)

        mock_client = MagicMock()
        mock_client.api_key = "test-key"

        with patch("httpx.stream", return_value=mock_resp):
            result = _stream_message(mock_client, [], [])

        # Invalid JSON should fallback to empty dict
        assert result["content"][0]["input"] == {}


class TestRunAgent:
    def test_extracts_findings_from_text(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "1")

        finding_json = json.dumps({
            "dimension": "health", "severity": "critical",
            "title": "CrashLoopBackOff", "resource_kind": "Pod",
            "resource_namespace": "default", "resource_name": "nginx",
        })
        response = {
            "content": [{"type": "text", "text": f"Here is a finding:\n{finding_json}\nFINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
        }

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator._stream_message", return_value=response):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert len(findings) == 1
        assert findings[0]["title"] == "CrashLoopBackOff"

    def test_handles_tool_use_turn(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "3")

        turn1 = {
            "content": [{"type": "tool_use", "id": "tu1", "name": "kubectl_get", "input": {"kind": "Pod"}}],
            "stop_reason": "tool_use",
        }
        turn2 = {
            "content": [{"type": "text", "text": "No issues found.\nFINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
        }

        call_count = {"n": 0}
        def mock_stream(*a, **kw):
            call_count["n"] += 1
            return turn1 if call_count["n"] == 1 else turn2

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator._stream_message", side_effect=mock_stream):
                with patch("runtime.orchestrator.call_mcp_tool", return_value='{"items":[]}'):
                    findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert findings == []  # No findings in this case
        assert call_count["n"] == 2  # Two turns

    def test_stops_on_empty_response(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "5")

        response = {"content": [], "stop_reason": "end_turn"}

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator._stream_message", return_value=response):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert findings == []
