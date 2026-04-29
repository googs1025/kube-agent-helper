"""Tests for runtime.reporter."""
import json
from unittest.mock import patch, MagicMock
from runtime.reporter import post_findings


class TestPostFindings:
    def test_posts_each_finding(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        findings = [
            {"dimension": "health", "title": "f1"},
            {"dimension": "security", "title": "f2"},
        ]
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()

        with patch("runtime.reporter.requests.post", return_value=mock_resp) as mock_post:
            post_findings("run-123", findings)

        assert mock_post.call_count == 2
        mock_post.assert_any_call(
            "http://ctrl:8080/internal/runs/run-123/findings",
            json={"dimension": "health", "title": "f1"},
            timeout=10,
        )

    def test_retries_on_failure(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        monkeypatch.setattr("runtime.reporter.BACKOFF_BASE", 0.01)  # fast retry
        findings = [{"title": "f1"}, {"title": "f2"}]

        call_count = {"n": 0}
        def failing_post(*args, **kwargs):
            call_count["n"] += 1
            if call_count["n"] == 2:
                # First attempt for f2 fails
                raise ConnectionError("network error")
            resp = MagicMock()
            resp.raise_for_status = MagicMock()
            return resp

        with patch("runtime.reporter.requests.post", side_effect=failing_post):
            post_findings("run-1", findings)

        # f1: 1 call, f2: 1 fail + 1 retry = 3 total
        assert call_count["n"] == 3

    def test_gives_up_after_max_retries(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        monkeypatch.setattr("runtime.reporter.BACKOFF_BASE", 0.01)
        monkeypatch.setattr("runtime.reporter.MAX_RETRIES", 1)

        with patch("runtime.reporter.requests.post", side_effect=ConnectionError("down")):
            post_findings("run-1", [{"title": "f1"}])
            # Should not raise — just logs error

    def test_empty_findings(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        with patch("runtime.reporter.requests.post") as mock_post:
            post_findings("run-1", [])
        mock_post.assert_not_called()


from runtime import reporter as _reporter


class TestRecordLLMEvent:
    def test_appends_to_buffer(self):
        _reporter._LLM_BUFFER.clear()
        _reporter.record_llm_event("retry", {"model": "m"})
        _reporter.record_llm_event("fallback", {"from_model": "m", "to_model": "m2"})
        assert len(_reporter._LLM_BUFFER) == 2
        assert _reporter._LLM_BUFFER[0] == {"type": "retry", "labels": {"model": "m"}}


class TestFlushLLMMetrics:
    def test_posts_buffer_and_clears(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        _reporter._LLM_BUFFER.clear()
        _reporter.record_llm_event("retry", {"model": "m"})

        with patch("runtime.reporter.requests.post") as mock_post:
            mock_post.return_value = MagicMock(status_code=204)
            _reporter.flush_llm_metrics()

        mock_post.assert_called_once()
        args, kwargs = mock_post.call_args
        assert args[0] == "http://ctrl:8080/internal/llm-metrics"
        assert kwargs["json"] == {"events": [{"type": "retry", "labels": {"model": "m"}}]}
        assert _reporter._LLM_BUFFER == []

    def test_empty_buffer_no_op(self):
        _reporter._LLM_BUFFER.clear()
        with patch("runtime.reporter.requests.post") as mock_post:
            _reporter.flush_llm_metrics()
        mock_post.assert_not_called()

    def test_swallows_post_error_and_clears(self):
        _reporter._LLM_BUFFER.clear()
        _reporter.record_llm_event("retry", {"model": "m"})

        with patch("runtime.reporter.requests.post", side_effect=RuntimeError("net down")):
            _reporter.flush_llm_metrics()  # MUST NOT raise

        assert _reporter._LLM_BUFFER == []
