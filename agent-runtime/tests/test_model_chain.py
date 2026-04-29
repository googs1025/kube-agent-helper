"""Tests for runtime.model_chain."""
from __future__ import annotations

import pytest

from runtime.model_chain import ModelChain, ModelEndpoint


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