"""Tests for runtime.tracer."""
from unittest.mock import MagicMock

from runtime.tracer import _NoOp, _Tracer


class TestNoOpEvent:
    def test_does_not_raise(self):
        t = _NoOp()
        # Should not raise no matter what kwargs are given
        t.event(name="x", level="WARNING", metadata={"k": "v"})
        t.event()


class TestTracerEvent:
    def test_calls_underlying_trace(self):
        fake_lf = MagicMock()
        fake_trace = MagicMock()
        t = _Tracer(fake_lf, fake_trace)

        t.event(name="model_retry", level="WARNING", metadata={"attempt": 2})

        fake_trace.event.assert_called_once_with(
            name="model_retry",
            level="WARNING",
            metadata={"attempt": 2},
        )

    def test_default_metadata_is_empty_dict(self):
        fake_lf = MagicMock()
        fake_trace = MagicMock()
        t = _Tracer(fake_lf, fake_trace)

        t.event(name="x")

        _, kwargs = fake_trace.event.call_args
        assert kwargs["metadata"] == {}
        assert kwargs["level"] == "DEFAULT"

    def test_failure_degrades_silently(self):
        fake_lf = MagicMock()
        fake_trace = MagicMock()
        fake_trace.event.side_effect = RuntimeError("network down")

        t = _Tracer(fake_lf, fake_trace)
        # Should NOT raise — observability must not break the main flow
        t.event(name="x")
        assert t._degraded is True

        # Subsequent calls should be skipped (no extra SDK call)
        fake_trace.event.reset_mock()
        t.event(name="y")
        fake_trace.event.assert_not_called()
