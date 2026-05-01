"""Tests for runtime.fix_main entrypoint and helpers.

The module reads CONTROLLER_URL at import time, so we set it on os.environ
before importing. Helpers like build_prompt, parse_patch_json, and
_api_version_for_kind are pure and can be exercised directly. main() is
exercised end-to-end with mocked MCP, LLM, tracer, and httpx.
"""
import base64
import importlib
import json
import os
import sys
from unittest.mock import MagicMock, patch

import pytest

# Ensure module-level env vars exist before fix_main is first imported anywhere
# in this test process. fix_main reads CONTROLLER_URL at import time.
os.environ.setdefault("CONTROLLER_URL", "http://controller.test:8080")

from runtime import fix_main  # noqa: E402


# ── Pure helpers ────────────────────────────────────────────────────────────


class TestApiVersionForKind:
    def test_known_kinds(self):
        cases = {
            "Deployment": "apps/v1",
            "StatefulSet": "apps/v1",
            "DaemonSet": "apps/v1",
            "Pod": "v1",
            "Service": "v1",
            "ConfigMap": "v1",
            "Secret": "v1",
            "ServiceAccount": "v1",
            "Namespace": "v1",
            "PersistentVolumeClaim": "v1",
            "ResourceQuota": "v1",
            "LimitRange": "v1",
            "Job": "batch/v1",
            "CronJob": "batch/v1",
            "Ingress": "networking.k8s.io/v1",
            "NetworkPolicy": "networking.k8s.io/v1",
            "PodDisruptionBudget": "policy/v1",
            "HorizontalPodAutoscaler": "autoscaling/v2",
        }
        for kind, expected in cases.items():
            assert fix_main._api_version_for_kind(kind) == expected, kind

    def test_unknown_kind_returns_empty(self):
        assert fix_main._api_version_for_kind("WidgetCRD") == ""


class TestBuildPrompt:
    def test_english_prompt(self):
        finding = {"title": "T", "description": "D", "suggestion": "S"}
        out = fix_main.build_prompt(finding, "current: yaml", "en")
        assert "Title: T" in out
        assert "Description: D" in out
        assert "current: yaml" in out
        assert "English" in out

    def test_chinese_prompt(self):
        finding = {"title": "T"}
        out = fix_main.build_prompt(finding, "y", "zh")
        assert "简体中文" in out


class TestBuildCreatePrompt:
    def test_english_prompt(self):
        finding = {
            "title": "T",
            "description": "D",
            "suggestion": "S",
            "target": {"namespace": "ns1"},
        }
        out = fix_main.build_create_prompt(finding, "en")
        assert "Title: T" in out
        assert "Target namespace: ns1" in out
        assert "does not exist yet" in out
        assert "English" in out

    def test_chinese_prompt(self):
        out = fix_main.build_create_prompt({"target": {"namespace": "ns"}}, "zh")
        assert "简体中文" in out

    def test_default_namespace_used_when_missing(self):
        out = fix_main.build_create_prompt({"target": {}}, "en")
        assert "Target namespace: default" in out


class TestParsePatchJson:
    def test_plain_json(self):
        s = '{"type":"strategic-merge","content":"{}","explanation":"x"}'
        out = fix_main.parse_patch_json(s)
        assert out["type"] == "strategic-merge"

    def test_with_code_fence(self):
        s = '```json\n{"content":"x"}\n```'
        out = fix_main.parse_patch_json(s)
        assert out["content"] == "x"

    def test_with_bare_fence(self):
        s = '```\n{"content":"x"}\n```'
        out = fix_main.parse_patch_json(s)
        assert out["content"] == "x"

    def test_with_surrounding_whitespace(self):
        s = '   \n  {"content":"x"} \n  '
        assert fix_main.parse_patch_json(s)["content"] == "x"

    def test_non_object_raises(self):
        with pytest.raises(ValueError, match="expected JSON object"):
            fix_main.parse_patch_json("[1,2,3]")

    def test_missing_content_raises(self):
        with pytest.raises(ValueError, match="missing 'content'"):
            fix_main.parse_patch_json('{"type":"strategic-merge"}')

    def test_invalid_json_raises(self):
        with pytest.raises(json.JSONDecodeError):
            fix_main.parse_patch_json("not json at all")


# ── _stream_llm_call ────────────────────────────────────────────────────────


class TestStreamLLMCall:
    def _stub_resp(self, lines):
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.raise_for_status = MagicMock()
        ctx.iter_lines = MagicMock(return_value=iter(lines))
        return ctx

    def test_parses_text_deltas_and_token_counts(self, monkeypatch):
        monkeypatch.setenv("ANTHROPIC_API_KEY", "key")
        events = [
            f"data: {json.dumps({'type':'message_start','message':{'usage':{'input_tokens':123}}})}",
            f"data: {json.dumps({'type':'content_block_delta','delta':{'type':'text_delta','text':'hello '}})}",
            f"data: {json.dumps({'type':'content_block_delta','delta':{'type':'text_delta','text':'world'}})}",
            f"data: {json.dumps({'type':'message_delta','usage':{'output_tokens':45}})}",
            "data: [DONE]",
        ]
        with patch.object(fix_main.httpx, "stream", return_value=self._stub_resp(events)):
            text, in_tok, out_tok = fix_main._stream_llm_call("prompt")
        assert text == "hello world"
        assert in_tok == 123
        assert out_tok == 45

    def test_skips_empty_lines_and_non_data(self, monkeypatch):
        monkeypatch.setenv("ANTHROPIC_API_KEY", "key")
        events = [
            "",
            "event: ping",
            "data:",
            "data: not json either",
            f"data: {json.dumps({'type':'content_block_delta','delta':{'type':'text_delta','text':'ok'}})}",
            "data: [DONE]",
        ]
        with patch.object(fix_main.httpx, "stream", return_value=self._stub_resp(events)):
            text, _, _ = fix_main._stream_llm_call("p")
        assert text == "ok"

    def test_uses_explicit_messages_endpoint_when_provided(self, monkeypatch):
        monkeypatch.setenv("ANTHROPIC_API_KEY", "key")
        monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://proxy.example.com/v1/messages")

        captured = {}

        def fake_stream(method, url, headers, json, timeout):
            captured["url"] = url
            return self._stub_resp([])

        with patch.object(fix_main.httpx, "stream", side_effect=fake_stream):
            fix_main._stream_llm_call("p")
        assert captured["url"] == "https://proxy.example.com/v1/messages"

    def test_appends_messages_path_otherwise(self, monkeypatch):
        monkeypatch.setenv("ANTHROPIC_API_KEY", "key")
        monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com/")
        captured = {}

        def fake_stream(method, url, headers, json, timeout):
            captured["url"] = url
            return self._stub_resp([])

        with patch.object(fix_main.httpx, "stream", side_effect=fake_stream):
            fix_main._stream_llm_call("p")
        assert captured["url"] == "https://api.anthropic.com/v1/messages"


# ── main() end-to-end ───────────────────────────────────────────────────────


def _finding_env(monkeypatch, **overrides):
    finding = {
        "findingID": "f1",
        "runID": "r1",
        "title": "Pod missing limits",
        "description": "no resource limits set",
        "suggestion": "add limits",
        "target": {"kind": "Deployment", "namespace": "default", "name": "nginx"},
    }
    finding.update(overrides)
    monkeypatch.setenv("FIX_INPUT_JSON", json.dumps(finding))
    return finding


class TestMain:
    def test_patch_existing_resource_happy_path(self, monkeypatch):
        _finding_env(monkeypatch)

        # MCP returns a JSON-serialised Deployment.
        mcp = MagicMock(return_value=json.dumps({
            "apiVersion": "apps/v1", "kind": "Deployment",
            "metadata": {"name": "nginx", "namespace": "default"},
            "spec": {"replicas": 1},
        }))
        # LLM returns a wrapped patch.
        llm = MagicMock(return_value=(
            json.dumps({"type": "strategic-merge", "content": "{\"spec\":{\"replicas\":3}}", "explanation": "scale up"}),
            200, 50,
        ))
        post = MagicMock()
        post.return_value.json = MagicMock(return_value={"metadata": {"name": "fix-1"}})
        post.return_value.raise_for_status = MagicMock()
        # tracer.init_fix returns a stub.
        tr = MagicMock()
        tr_init = MagicMock(return_value=tr)

        with patch.object(fix_main, "call_mcp_tool", mcp), \
             patch.object(fix_main, "_stream_llm_call", llm), \
             patch.object(fix_main, "_tracer", MagicMock(init_fix=tr_init)), \
             patch.object(fix_main.httpx, "post", post):
            rc = fix_main.main()

        assert rc == 0
        # The POST body should encode strategy=dry-run when resource exists.
        body = post.call_args.kwargs["json"]
        assert body["strategy"] == "dry-run"
        assert body["target"]["kind"] == "Deployment"
        assert body["explanation"] == "scale up"
        # beforeSnapshot should be base64 of the current YAML.
        assert isinstance(body["beforeSnapshot"], str) and body["beforeSnapshot"]
        decoded = base64.b64decode(body["beforeSnapshot"]).decode("utf-8")
        assert "nginx" in decoded
        # tracer.generation + flush were both called.
        tr.generation.assert_called_once()
        tr.flush.assert_called()

    def test_create_when_target_missing(self, monkeypatch):
        _finding_env(monkeypatch)

        # MCP returns "not found" → code path swaps to create strategy.
        mcp = MagicMock(return_value="tool error: not found")
        llm = MagicMock(return_value=(
            json.dumps({"type": "create", "content": "{\"apiVersion\":\"v1\"}"}),
            10, 5,
        ))
        post = MagicMock()
        post.return_value.json = MagicMock(return_value={"metadata": {"name": "fix-x"}})
        post.return_value.raise_for_status = MagicMock()

        with patch.object(fix_main, "call_mcp_tool", mcp), \
             patch.object(fix_main, "_stream_llm_call", llm), \
             patch.object(fix_main, "_tracer", MagicMock(init_fix=MagicMock(return_value=MagicMock()))), \
             patch.object(fix_main.httpx, "post", post):
            rc = fix_main.main()

        assert rc == 0
        body = post.call_args.kwargs["json"]
        assert body["strategy"] == "create"
        # No snapshot when resource didn't exist.
        assert body["beforeSnapshot"] == ""

    def test_llm_failure_returns_2(self, monkeypatch):
        _finding_env(monkeypatch)

        mcp = MagicMock(return_value=json.dumps({"kind": "Deployment"}))
        llm = MagicMock(side_effect=RuntimeError("api down"))
        tr = MagicMock()

        with patch.object(fix_main, "call_mcp_tool", mcp), \
             patch.object(fix_main, "_stream_llm_call", llm), \
             patch.object(fix_main, "_tracer", MagicMock(init_fix=MagicMock(return_value=tr))):
            rc = fix_main.main()

        assert rc == 2
        tr.flush.assert_called()  # tracer must be flushed even on failure

    def test_invalid_patch_json_returns_2(self, monkeypatch):
        _finding_env(monkeypatch)

        mcp = MagicMock(return_value=json.dumps({"kind": "Deployment"}))
        llm = MagicMock(return_value=("{not really json}", 1, 1))

        with patch.object(fix_main, "call_mcp_tool", mcp), \
             patch.object(fix_main, "_stream_llm_call", llm), \
             patch.object(fix_main, "_tracer", MagicMock(init_fix=MagicMock(return_value=MagicMock()))):
            rc = fix_main.main()
        assert rc == 2

    def test_mcp_returns_invalid_json_falls_back_to_raw_text(self, monkeypatch):
        _finding_env(monkeypatch)

        # Non-JSON response that doesn't match the "not found" / "tool error"
        # markers — the code falls through to current_yaml = raw.
        mcp = MagicMock(return_value="some plaintext describing the resource")
        llm = MagicMock(return_value=(
            json.dumps({"type": "strategic-merge", "content": "{}"}),
            1, 1,
        ))
        post = MagicMock()
        post.return_value.json = MagicMock(return_value={"metadata": {"name": "fix"}})
        post.return_value.raise_for_status = MagicMock()

        with patch.object(fix_main, "call_mcp_tool", mcp), \
             patch.object(fix_main, "_stream_llm_call", llm), \
             patch.object(fix_main, "_tracer", MagicMock(init_fix=MagicMock(return_value=MagicMock()))), \
             patch.object(fix_main.httpx, "post", post):
            rc = fix_main.main()
        assert rc == 0
        body = post.call_args.kwargs["json"]
        assert body["strategy"] == "dry-run", "raw text counts as 'resource exists'"
