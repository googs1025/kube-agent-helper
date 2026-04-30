"""Tests for runtime.model_chain."""
from __future__ import annotations

from contextlib import contextmanager
from unittest.mock import patch

import pytest

from runtime.model_chain import (
    ModelChain,
    ModelChainExhausted,
    ModelEndpoint,
    _SSEStreamBroken,
    _stream_one,
)


class _NoopTracer:
    """Stand-in tracer used in invoke tests; just absorbs calls."""

    def event(self, **kwargs):
        pass

    def generation(self, **kwargs):
        pass


def _http_status_error(status: int, headers: dict | None = None):
    import httpx

    req = httpx.Request("POST", "https://x")
    return httpx.HTTPStatusError(
        f"{status}",
        request=req,
        response=httpx.Response(status, headers=headers or {}, request=req),
    )


def _make_sse_lines(events: list[str]) -> list[str]:
    """events 中每条已是 SSE data 行内容（JSON 串或 [DONE]）。
    httpx.Response.iter_lines() 返回 str（UTF-8 decoded），所以这里给 str。"""
    return [f"data: {e}" for e in events]


@contextmanager
def _fake_stream(lines: list[str], status: int = 200):
    """伪 httpx.stream 上下文管理器，yield 一个支持 iter_lines/raise_for_status 的对象。"""

    class _Resp:
        def raise_for_status(self):
            if status >= 400:
                import httpx

                req = httpx.Request("POST", "https://x")
                raise httpx.HTTPStatusError(
                    f"{status}",
                    request=req,
                    response=httpx.Response(status, request=req),
                )

        def iter_lines(self):
            for ln in lines:
                yield ln

    yield _Resp()


def _clear_chain_env(monkeypatch):
    """删 MODEL_*  与兼容性 ANTHROPIC_* env 让测试从干净状态构建。"""
    monkeypatch.delenv("MODEL_COUNT", raising=False)
    for i in range(10):
        for suffix in ("BASE_URL", "MODEL", "API_KEY", "RETRIES"):
            monkeypatch.delenv(f"MODEL_{i}_{suffix}", raising=False)
    monkeypatch.delenv("ANTHROPIC_BASE_URL", raising=False)
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("MODEL", raising=False)


class TestFromEnv:
    def test_multi_endpoint(self, monkeypatch):
        _clear_chain_env(monkeypatch)
        monkeypatch.setenv("MODEL_COUNT", "2")
        monkeypatch.setenv("MODEL_0_BASE_URL", "https://p.example.com")
        monkeypatch.setenv("MODEL_0_MODEL", "sonnet")
        monkeypatch.setenv("MODEL_0_RETRIES", "3")
        monkeypatch.setenv("MODEL_0_API_KEY", "key-0")
        monkeypatch.setenv("MODEL_1_BASE_URL", "")
        monkeypatch.setenv("MODEL_1_MODEL", "haiku")
        monkeypatch.setenv("MODEL_1_RETRIES", "0")
        monkeypatch.setenv("MODEL_1_API_KEY", "key-1")

        chain = ModelChain.from_env()
        assert len(chain.endpoints) == 2
        assert chain.endpoints[0] == ModelEndpoint(
            base_url="https://p.example.com",
            model="sonnet",
            api_key="key-0",
            retries=3,
        )
        assert chain.endpoints[1] == ModelEndpoint(
            base_url="",
            model="haiku",
            api_key="key-1",
            retries=0,
        )

    def test_single_endpoint_legacy(self, monkeypatch):
        _clear_chain_env(monkeypatch)
        monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://legacy.example.com")
        monkeypatch.setenv("MODEL", "sonnet")
        monkeypatch.setenv("ANTHROPIC_API_KEY", "legacy-key")

        chain = ModelChain.from_env()
        assert len(chain.endpoints) == 1
        assert chain.endpoints[0].base_url == "https://legacy.example.com"
        assert chain.endpoints[0].model == "sonnet"
        assert chain.endpoints[0].api_key == "legacy-key"
        assert chain.endpoints[0].retries == 0

    def test_no_endpoints_raises(self, monkeypatch):
        _clear_chain_env(monkeypatch)
        with pytest.raises(ValueError, match="at least one"):
            ModelChain.from_env()

    def test_empty_retries_defaults_to_zero(self, monkeypatch):
        _clear_chain_env(monkeypatch)
        monkeypatch.setenv("MODEL_COUNT", "1")
        monkeypatch.setenv("MODEL_0_BASE_URL", "")
        monkeypatch.setenv("MODEL_0_MODEL", "sonnet")
        monkeypatch.setenv("MODEL_0_API_KEY", "k")
        # MODEL_0_RETRIES intentionally unset → should default to 0
        chain = ModelChain.from_env()
        assert chain.endpoints[0].retries == 0


class TestModelChainConstruction:
    def test_empty_endpoints_raises(self):
        with pytest.raises(ValueError, match="at least one"):
            ModelChain([])


class TestBackoffFor:
    def test_schedule(self):
        from runtime.model_chain import _backoff_for

        assert _backoff_for(1) == 1
        assert _backoff_for(2) == 2
        assert _backoff_for(3) == 4
        assert _backoff_for(4) == 4   # 封顶
        assert _backoff_for(99) == 4


class TestStreamOne:
    def test_happy_path(self):
        events = [
            '{"type":"message_start","message":{"usage":{"input_tokens":5}}}',
            '{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}',
            '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}',
            "[DONE]",
        ]
        ep = ModelEndpoint(base_url="https://api.example.com", model="m", api_key="k", retries=0)

        with patch(
            "runtime.model_chain.httpx.stream",
            return_value=_fake_stream(_make_sse_lines(events)),
        ):
            result = _stream_one(ep, tools=[], messages=[{"role": "user", "content": "x"}])

        assert result["stop_reason"] == "end_turn"
        assert result["content"][0]["type"] == "text"
        assert result["content"][0]["text"] == "hi"
        assert result["input_tokens"] == 5
        assert result["output_tokens"] == 2

    def test_sse_stream_broken(self):
        # No [DONE] / message_stop terminator
        events = [
            '{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}',
        ]
        ep = ModelEndpoint(base_url="https://api.example.com", model="m", api_key="k", retries=0)

        with patch(
            "runtime.model_chain.httpx.stream",
            return_value=_fake_stream(_make_sse_lines(events)),
        ):
            with pytest.raises(_SSEStreamBroken):
                _stream_one(ep, tools=[], messages=[])

    def test_message_stop_terminates_stream(self):
        events = [
            '{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}',
            '{"type":"message_delta","delta":{"stop_reason":"end_turn"}}',
            '{"type":"message_stop"}',
        ]
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch(
            "runtime.model_chain.httpx.stream",
            return_value=_fake_stream(_make_sse_lines(events)),
        ):
            result = _stream_one(ep, tools=[], messages=[])
        assert result["content"][0]["text"] == "ok"

    def test_tool_use_partial_json_assembled(self):
        events = [
            '{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"kubectl_get","input":{}}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\\"namespace\\":"}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\\"prod\\"}"}}',
            '{"type":"message_stop"}',
        ]
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch(
            "runtime.model_chain.httpx.stream",
            return_value=_fake_stream(_make_sse_lines(events)),
        ):
            result = _stream_one(ep, tools=[{"name": "kubectl_get"}], messages=[])
        assert result["content"][0]["type"] == "tool_use"
        assert result["content"][0]["input"] == {"namespace": "prod"}

    def test_url_with_v1_messages_suffix_used_as_is(self):
        """Some proxies require /v1/messages already in baseURL."""
        events = ['{"type":"message_stop"}']
        ep = ModelEndpoint(
            base_url="https://proxy.example.com/v1/messages",
            model="m",
            api_key="k",
            retries=0,
        )

        with patch("runtime.model_chain.httpx.stream") as mock_stream:
            mock_stream.return_value = _fake_stream(_make_sse_lines(events))
            _stream_one(ep, tools=[], messages=[])

        args, kwargs = mock_stream.call_args
        # Second positional arg is URL; verify no double /v1/messages append
        assert args[1] == "https://proxy.example.com/v1/messages"

    def test_thinking_blocks_dropped(self):
        events = [
            '{"type":"content_block_start","index":0,"content_block":{"type":"thinking","text":""}}',
            '{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning..."}}',
            '{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}',
            '{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}',
            '{"type":"message_stop"}',
        ]
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch(
            "runtime.model_chain.httpx.stream",
            return_value=_fake_stream(_make_sse_lines(events)),
        ):
            result = _stream_one(ep, tools=[], messages=[])
        assert len(result["content"]) == 1
        assert result["content"][0]["type"] == "text"
        assert result["content"][0]["text"] == "answer"

    def test_default_max_tokens_is_8192(self, monkeypatch):
        """When MAX_TOKENS env is unset, _stream_one must request 8192."""
        monkeypatch.delenv("MAX_TOKENS", raising=False)
        events = ['{"type":"message_stop"}']
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch("runtime.model_chain.httpx.stream") as mock_stream:
            mock_stream.return_value = _fake_stream(_make_sse_lines(events))
            _stream_one(ep, tools=[], messages=[])

        _, kwargs = mock_stream.call_args
        assert kwargs["json"]["max_tokens"] == 8192

    def test_max_tokens_env_override(self, monkeypatch):
        """MAX_TOKENS env overrides the default."""
        monkeypatch.setenv("MAX_TOKENS", "16384")
        events = ['{"type":"message_stop"}']
        ep = ModelEndpoint(base_url="", model="m", api_key="k", retries=0)

        with patch("runtime.model_chain.httpx.stream") as mock_stream:
            mock_stream.return_value = _fake_stream(_make_sse_lines(events))
            _stream_one(ep, tools=[], messages=[])

        _, kwargs = mock_stream.call_args
        assert kwargs["json"]["max_tokens"] == 16384


class TestInvoke:
    def test_succeeds_first_try(self):
        chain = ModelChain([ModelEndpoint(base_url="", model="m", api_key="k", retries=0)])
        with patch("runtime.model_chain._stream_one") as mock_stream:
            mock_stream.return_value = {
                "content": [{"type": "text", "text": "ok"}],
                "stop_reason": "end_turn",
                "input_tokens": 1,
                "output_tokens": 1,
            }
            result = chain.invoke(tools=[], messages=[], tracer=_NoopTracer())
        assert result["stop_reason"] == "end_turn"
        assert mock_stream.call_count == 1

    def test_retries_on_5xx_and_succeeds(self):
        chain = ModelChain([ModelEndpoint(base_url="", model="m", api_key="k", retries=2)])
        calls = {"n": 0}

        def fake(ep, tools, messages):
            calls["n"] += 1
            if calls["n"] <= 2:
                raise _http_status_error(503)
            return {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0}

        with patch("runtime.model_chain._stream_one", side_effect=fake), \
             patch("runtime.model_chain.time.sleep"):
            result = chain.invoke([], [], _NoopTracer())
        assert result["stop_reason"] == "end_turn"
        assert calls["n"] == 3

    def test_fallbacks_after_retries_exhausted(self):
        eps = [
            ModelEndpoint(base_url="", model="primary", api_key="k0", retries=1),
            ModelEndpoint(base_url="", model="backup", api_key="k1", retries=0),
        ]
        chain = ModelChain(eps)
        side = [
            _http_status_error(503),
            _http_status_error(503),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0},
        ]
        with patch("runtime.model_chain._stream_one", side_effect=side), \
             patch("runtime.model_chain.time.sleep"):
            result = chain.invoke([], [], _NoopTracer())
        assert result["stop_reason"] == "end_turn"

    def test_4xx_no_retry_no_fallback(self):
        eps = [
            ModelEndpoint("", "m", "k", retries=3),
            ModelEndpoint("", "m2", "k2", retries=3),
        ]
        chain = ModelChain(eps)
        import httpx

        with patch("runtime.model_chain._stream_one", side_effect=_http_status_error(403)), \
             patch("runtime.model_chain.time.sleep"):
            with pytest.raises(httpx.HTTPStatusError):
                chain.invoke([], [], _NoopTracer())

    def test_sse_broken_skips_to_fallback_no_retry(self):
        eps = [
            ModelEndpoint("", "m", "k", retries=3),  # retries=3 should NOT be used
            ModelEndpoint("", "m2", "k2", retries=0),
        ]
        chain = ModelChain(eps)
        # _stream_one called: once on primary (broken), once on fallback (success)
        side = [
            _SSEStreamBroken("eof"),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0},
        ]
        with patch("runtime.model_chain._stream_one", side_effect=side) as mock_stream, \
             patch("runtime.model_chain.time.sleep"):
            result = chain.invoke([], [], _NoopTracer())
        assert result["stop_reason"] == "end_turn"
        assert mock_stream.call_count == 2  # NOT 4 (would be 4 if retried on primary)

    def test_429_uses_retry_after_header(self):
        chain = ModelChain([ModelEndpoint("", "m", "k", retries=1)])
        side = [
            _http_status_error(429, headers={"Retry-After": "10"}),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0},
        ]
        sleeps: list[float] = []

        with patch("runtime.model_chain._stream_one", side_effect=side), \
             patch("runtime.model_chain.time.sleep", side_effect=lambda s: sleeps.append(s)):
            chain.invoke([], [], _NoopTracer())
        assert sleeps == [10.0]

    def test_chain_exhausted_raises(self):
        eps = [
            ModelEndpoint("", "m", "k", retries=0),
            ModelEndpoint("", "m2", "k2", retries=0),
        ]
        chain = ModelChain(eps)
        with patch("runtime.model_chain._stream_one", side_effect=_http_status_error(503)), \
             patch("runtime.model_chain.time.sleep"):
            with pytest.raises(ModelChainExhausted):
                chain.invoke([], [], _NoopTracer())

    def test_429_retry_after_caps_at_60(self):
        chain = ModelChain([ModelEndpoint("", "m", "k", retries=1)])
        side = [
            _http_status_error(429, headers={"Retry-After": "9999"}),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0},
        ]
        sleeps: list[float] = []

        with patch("runtime.model_chain._stream_one", side_effect=side), \
             patch("runtime.model_chain.time.sleep", side_effect=lambda s: sleeps.append(s)):
            chain.invoke([], [], _NoopTracer())
        assert sleeps == [60.0]
