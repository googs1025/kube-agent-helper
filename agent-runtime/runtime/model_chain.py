"""ModelChain: 单 turn 内的重试 + fallback 决策。

读 Translator 注入的 MODEL_<i>_* env 构建一个端点链，按主→fallback 顺序
尝试调用 LLM。每个端点内根据 retries 字段做指数退避重试；4xx 永不重试，
SSE 中段直接切下一端点（避免 token 重复消费）。

完整设计见 docs/superpowers/specs/2026-04-29-modelconfig-fallback-and-retry-design.md。
"""
from __future__ import annotations

import os
from dataclasses import dataclass


_BACKOFF_SCHEDULE = [1, 2, 4]  # 秒，指数；超过封顶 4s


def _backoff_for(attempt: int) -> float:
    """attempt 是从 1 起的重试序号（attempt=1 表示第一次重试）。"""
    idx = attempt - 1
    if 0 <= idx < len(_BACKOFF_SCHEDULE):
        return float(_BACKOFF_SCHEDULE[idx])
    return float(_BACKOFF_SCHEDULE[-1])


@dataclass(frozen=True)
class ModelEndpoint:
    """单个 LLM 端点的不可变配置。base_url 为空时走 SDK 默认。"""

    base_url: str
    model: str
    api_key: str
    retries: int  # 0 = 不重试


class ModelChainExhausted(Exception):
    """所有 endpoint + 重试用尽后抛出。"""


class ModelChain:
    """主+fallback 端点链。invoke() 在 turn 内做完所有重试 + fallback 决策。"""

    def __init__(self, endpoints: list[ModelEndpoint]):
        if not endpoints:
            raise ValueError("ModelChain requires at least one endpoint")
        self.endpoints = endpoints

    @classmethod
    def from_env(cls) -> "ModelChain":
        """从 MODEL_COUNT / MODEL_<i>_* env 构建。

        MODEL_COUNT 缺失时降级读 ANTHROPIC_BASE_URL/MODEL/ANTHROPIC_API_KEY
        构造单端点链（旧版兼容）。两者都缺则 ValueError。
        """
        count_str = os.environ.get("MODEL_COUNT", "").strip()
        if count_str:
            n = int(count_str)
            endpoints = [
                ModelEndpoint(
                    base_url=os.environ.get(f"MODEL_{i}_BASE_URL", ""),
                    model=os.environ.get(f"MODEL_{i}_MODEL", ""),
                    api_key=os.environ.get(f"MODEL_{i}_API_KEY", ""),
                    retries=int(os.environ.get(f"MODEL_{i}_RETRIES", "0") or "0"),
                )
                for i in range(n)
            ]
            return cls(endpoints)

        api_key = os.environ.get("ANTHROPIC_API_KEY", "")
        if not api_key:
            raise ValueError(
                "ModelChain requires at least one endpoint "
                "(set MODEL_COUNT or ANTHROPIC_API_KEY)"
            )
        return cls(
            [
                ModelEndpoint(
                    base_url=os.environ.get("ANTHROPIC_BASE_URL", ""),
                    model=os.environ.get("MODEL", "claude-sonnet-4-6"),
                    api_key=api_key,
                    retries=0,
                )
            ]
        )
