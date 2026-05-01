"""Extended tests for runtime.tracer to cover init/, sanitize_messages,
_NoOp helpers, _Tracer.generation/span/flush, and degradation paths.
"""
import sys
from unittest.mock import MagicMock, patch

import pytest

from runtime import tracer
from runtime.tracer import _NoOp, _Tracer


# ── init / init_fix ─────────────────────────────────────────────────────────


class TestInit:
    def test_no_keys_returns_noop(self, monkeypatch):
        monkeypatch.delenv("LANGFUSE_PUBLIC_KEY", raising=False)
        monkeypatch.delenv("LANGFUSE_SECRET_KEY", raising=False)
        t = tracer.init("run-1", ["skill-a"])
        assert isinstance(t, _NoOp)

    def test_only_public_key_returns_noop(self, monkeypatch):
        monkeypatch.setenv("LANGFUSE_PUBLIC_KEY", "pk")
        monkeypatch.delenv("LANGFUSE_SECRET_KEY", raising=False)
        t = tracer.init("run-1", [])
        assert isinstance(t, _NoOp)

    def test_with_keys_constructs_tracer(self, monkeypatch):
        monkeypatch.setenv("LANGFUSE_PUBLIC_KEY", "pk")
        monkeypatch.setenv("LANGFUSE_SECRET_KEY", "sk")

        fake_lf = MagicMock()
        fake_trace = MagicMock()
        fake_lf.trace.return_value = fake_trace

        with patch.dict(sys.modules, {"langfuse": MagicMock(Langfuse=lambda: fake_lf)}):
            t = tracer.init("run-1", ["s1", "s2"])

        assert isinstance(t, _Tracer)
        fake_lf.trace.assert_called_once()
        kwargs = fake_lf.trace.call_args.kwargs
        assert kwargs["id"] == "run-1"
        assert kwargs["name"] == "diagnostic-run"
        assert kwargs["metadata"] == {"skills": ["s1", "s2"]}

    def test_sdk_failure_degrades_to_noop(self, monkeypatch):
        monkeypatch.setenv("LANGFUSE_PUBLIC_KEY", "pk")
        monkeypatch.setenv("LANGFUSE_SECRET_KEY", "sk")

        broken = MagicMock()
        broken.Langfuse = MagicMock(side_effect=RuntimeError("sdk boom"))
        with patch.dict(sys.modules, {"langfuse": broken}):
            t = tracer.init("run-1", [])
        assert isinstance(t, _NoOp), "init must return _NoOp when SDK explodes"


class TestInitFix:
    def test_no_keys_returns_noop(self, monkeypatch):
        monkeypatch.delenv("LANGFUSE_PUBLIC_KEY", raising=False)
        t = tracer.init_fix("finding-1", "run-1")
        assert isinstance(t, _NoOp)

    def test_with_keys_constructs_tracer(self, monkeypatch):
        monkeypatch.setenv("LANGFUSE_PUBLIC_KEY", "pk")
        monkeypatch.setenv("LANGFUSE_SECRET_KEY", "sk")

        fake_lf = MagicMock()
        fake_trace = MagicMock()
        fake_lf.trace.return_value = fake_trace

        with patch.dict(sys.modules, {"langfuse": MagicMock(Langfuse=lambda: fake_lf)}):
            t = tracer.init_fix("finding-9", "run-9")

        assert isinstance(t, _Tracer)
        kwargs = fake_lf.trace.call_args.kwargs
        assert kwargs["name"] == "fix-generation"
        assert kwargs["metadata"] == {"finding_id": "finding-9", "run_id": "run-9"}
        assert "fix" in kwargs["tags"]

    def test_sdk_failure_degrades_to_noop(self, monkeypatch):
        monkeypatch.setenv("LANGFUSE_PUBLIC_KEY", "pk")
        monkeypatch.setenv("LANGFUSE_SECRET_KEY", "sk")
        broken = MagicMock()
        broken.Langfuse = MagicMock(side_effect=ValueError("nope"))
        with patch.dict(sys.modules, {"langfuse": broken}):
            t = tracer.init_fix("f", "r")
        assert isinstance(t, _NoOp)


# ── sanitize_messages ───────────────────────────────────────────────────────


class TestSanitizeMessages:
    def test_string_content_truncated(self):
        out = tracer.sanitize_messages([{"role": "user", "content": "x" * 1000}], max_content_chars=10)
        assert out == [{"role": "user", "content": "x" * 10}]

    def test_tool_result_truncated(self):
        msg = {
            "role": "user",
            "content": [
                {"type": "tool_result", "tool_use_id": "t1", "content": "y" * 800},
            ],
        }
        out = tracer.sanitize_messages([msg], max_content_chars=20)
        block = out[0]["content"][0]
        assert block["type"] == "tool_result"
        assert block["tool_use_id"] == "t1"
        assert block["content"] == "y" * 20

    def test_tool_result_non_string_content_is_stringified(self):
        msg = {
            "role": "user",
            "content": [
                {"type": "tool_result", "tool_use_id": "t1", "content": {"k": "v" * 200}},
            ],
        }
        out = tracer.sanitize_messages([msg], max_content_chars=15)
        # Non-str content gets str()'d then truncated
        assert isinstance(out[0]["content"][0]["content"], str)
        assert len(out[0]["content"][0]["content"]) <= 15

    def test_text_block_truncated(self):
        msg = {
            "role": "assistant",
            "content": [{"type": "text", "text": "z" * 500}],
        }
        out = tracer.sanitize_messages([msg], max_content_chars=8)
        assert out[0]["content"][0] == {"type": "text", "text": "z" * 8}

    def test_tool_use_preserves_name_and_input(self):
        msg = {
            "role": "assistant",
            "content": [
                {"type": "tool_use", "name": "kubectl_get", "input": {"resource": "pod"}, "id": "tu1"},
            ],
        }
        out = tracer.sanitize_messages([msg])
        assert out[0]["content"][0] == {
            "type": "tool_use",
            "name": "kubectl_get",
            "input": {"resource": "pod"},
        }

    def test_unknown_block_type_keeps_only_type(self):
        msg = {
            "role": "user",
            "content": [{"type": "unknown_block", "secret": "hush"}],
        }
        out = tracer.sanitize_messages([msg])
        assert out[0]["content"][0] == {"type": "unknown_block"}

    def test_non_str_non_list_content_keeps_role_only(self):
        msg = {"role": "system", "content": 42}
        out = tracer.sanitize_messages([msg])
        assert out == [{"role": "system"}]

    def test_default_max_content_chars(self):
        long = "x" * 1000
        out = tracer.sanitize_messages([{"role": "user", "content": long}])
        # default is 300
        assert len(out[0]["content"]) == 300


# ── _NoOp ───────────────────────────────────────────────────────────────────


class TestNoOpAllMethods:
    def test_generation_returns_noop_chain(self):
        n = _NoOp()
        nested = n.generation(name="x")
        assert isinstance(nested, _NoOp)
        # Method chaining keeps returning _NoOp
        assert isinstance(nested.span(name="y"), _NoOp)

    def test_event_end_flush_no_raise(self):
        n = _NoOp()
        assert n.end(meta="x") is None
        assert n.flush() is None


# ── _Tracer.generation / span / flush ──────────────────────────────────────


class TestTracerGenerationSpan:
    def test_generation_delegates(self):
        fake_lf = MagicMock()
        fake_trace = MagicMock()
        fake_trace.generation.return_value = "gen-handle"
        t = _Tracer(fake_lf, fake_trace)
        assert t.generation(name="x") == "gen-handle"
        fake_trace.generation.assert_called_once_with(name="x")

    def test_span_delegates(self):
        fake_lf = MagicMock()
        fake_trace = MagicMock()
        fake_trace.span.return_value = "span-handle"
        t = _Tracer(fake_lf, fake_trace)
        assert t.span(name="x") == "span-handle"

    def test_generation_failure_degrades(self):
        fake_trace = MagicMock()
        fake_trace.generation.side_effect = RuntimeError("boom")
        t = _Tracer(MagicMock(), fake_trace)
        result = t.generation(name="x")
        assert isinstance(result, _NoOp)
        assert t._degraded is True

        # Subsequent call short-circuits to _NoOp without invoking SDK.
        fake_trace.generation.reset_mock()
        result2 = t.generation(name="y")
        assert isinstance(result2, _NoOp)
        fake_trace.generation.assert_not_called()

    def test_span_failure_degrades(self):
        fake_trace = MagicMock()
        fake_trace.span.side_effect = RuntimeError("boom")
        t = _Tracer(MagicMock(), fake_trace)
        result = t.span(name="x")
        assert isinstance(result, _NoOp)
        assert t._degraded is True

        # Once degraded, span returns _NoOp without calling underlying SDK.
        fake_trace.span.reset_mock()
        assert isinstance(t.span(name="y"), _NoOp)
        fake_trace.span.assert_not_called()


class TestTracerFlush:
    def test_flush_calls_lf(self):
        fake_lf = MagicMock()
        t = _Tracer(fake_lf, MagicMock())
        t.flush()
        fake_lf.flush.assert_called_once()

    def test_flush_swallows_errors(self):
        fake_lf = MagicMock()
        fake_lf.flush.side_effect = RuntimeError("flush boom")
        t = _Tracer(fake_lf, MagicMock())
        t.flush()  # must not raise


# ── Integration: a degraded tracer keeps event() silent ─────────────────────


def test_event_after_degradation_is_silent():
    fake_trace = MagicMock()
    fake_trace.span.side_effect = RuntimeError("trip")

    t = _Tracer(MagicMock(), fake_trace)
    t.span(name="x")  # trips degraded flag
    fake_trace.event.reset_mock()

    t.event(name="ev1")
    fake_trace.event.assert_not_called()
