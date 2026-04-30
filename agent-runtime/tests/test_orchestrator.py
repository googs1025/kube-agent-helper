"""Tests for runtime.orchestrator.

Note: SSE parsing tests previously here have moved to test_model_chain.py
(TestStreamOne) where the lifted _stream_one function now lives. Tests in
this file focus on prompt construction and the agentic loop wiring.
"""
import json
from unittest.mock import patch, MagicMock

import pytest

from runtime.orchestrator import build_prompt, run_agent
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
        assert "..." in prompt


def _make_chain_mock(invoke_responses):
    """Build a MagicMock that ModelChain.from_env() returns.

    invoke_responses is either a single dict (returned every call) or a
    list passed as side_effect.
    """
    chain = MagicMock()
    if isinstance(invoke_responses, list):
        chain.invoke.side_effect = invoke_responses
    else:
        chain.invoke.return_value = invoke_responses
    chain.endpoints = [MagicMock(model="claude-sonnet-4-6")]
    return chain


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
            "input_tokens": 0,
            "output_tokens": 0,
        }
        chain = _make_chain_mock(response)

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert len(findings) == 1
        assert findings[0]["title"] == "CrashLoopBackOff"

    def test_handles_tool_use_turn(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "3")

        turn1 = {
            "content": [{"type": "tool_use", "id": "tu1", "name": "kubectl_get", "input": {"kind": "Pod"}}],
            "stop_reason": "tool_use",
            "input_tokens": 0, "output_tokens": 0,
        }
        turn2 = {
            "content": [{"type": "text", "text": "No issues found.\nFINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock([turn1, turn2])

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                with patch("runtime.orchestrator.call_mcp_tool", return_value='{"items":[]}'):
                    findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert findings == []
        assert chain.invoke.call_count == 2

    def test_stops_on_empty_response(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "5")

        response = {
            "content": [], "stop_reason": "end_turn",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock(response)

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                findings = run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        assert findings == []

    def test_propagates_chain_exhausted(self, monkeypatch):
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "1")

        from runtime.model_chain import ModelChainExhausted

        chain = MagicMock()
        chain.invoke.side_effect = ModelChainExhausted("all endpoints failed")
        chain.endpoints = [MagicMock(model="m")]

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                with pytest.raises(ModelChainExhausted):
                    run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

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
        monkeypatch.setenv("MAX_TOKENS_CONTINUE_LIMIT", "3")  # limit=3: allows 2 consecutive, breaks on 3rd
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

    def test_unknown_max_tokens_behavior_defaults_to_continue(self, monkeypatch):
        """An unrecognized MAX_TOKENS_BEHAVIOR value must fall back to continue, not fail silently."""
        monkeypatch.setenv("OUTPUT_LANGUAGE", "en")
        monkeypatch.setenv("MAX_TURNS", "5")
        monkeypatch.setenv("MAX_TOKENS_BEHAVIOR", "FAIL")  # uppercase typo — not "fail"
        monkeypatch.delenv("MAX_TOKENS_CONTINUE_LIMIT", raising=False)

        truncated = {
            "content": [{"type": "text", "text": "..."}],
            "stop_reason": "max_tokens",
            "input_tokens": 0, "output_tokens": 0,
        }
        end = {
            "content": [{"type": "text", "text": "done\nFINDINGS_COMPLETE"}],
            "stop_reason": "end_turn",
            "input_tokens": 0, "output_tokens": 0,
        }
        chain = _make_chain_mock([truncated, end])

        with patch("runtime.orchestrator.discover_tools", return_value=[]):
            with patch("runtime.orchestrator.ModelChain.from_env", return_value=chain):
                run_agent([Skill(name="t", dimension="h", tools=[], prompt="p")])

        # If "FAIL" had been silently treated as fail, invoke count would be 1.
        # With validation, it falls back to continue → 2 invokes.
        assert chain.invoke.call_count == 2, (
            "unknown behavior must fall back to continue (not silently fail)"
        )
