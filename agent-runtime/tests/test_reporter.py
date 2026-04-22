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

    def test_continues_on_failure(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        findings = [{"title": "f1"}, {"title": "f2"}, {"title": "f3"}]

        call_count = {"n": 0}
        def failing_post(*args, **kwargs):
            call_count["n"] += 1
            if call_count["n"] == 2:
                raise ConnectionError("network error")
            resp = MagicMock()
            resp.raise_for_status = MagicMock()
            return resp

        with patch("runtime.reporter.requests.post", side_effect=failing_post):
            # Should not raise even though second call fails
            post_findings("run-1", findings)

        assert call_count["n"] == 3  # all 3 attempted

    def test_empty_findings(self, monkeypatch):
        monkeypatch.setattr("runtime.reporter.CONTROLLER_URL", "http://ctrl:8080")
        with patch("runtime.reporter.requests.post") as mock_post:
            post_findings("run-1", [])
        mock_post.assert_not_called()
