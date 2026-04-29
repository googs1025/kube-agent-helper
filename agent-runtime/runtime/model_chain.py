"""ModelChain: 单 turn 内的重试 + fallback 决策。

读 Translator 注入的 MODEL_<i>_* env 构建一个端点链，按主→fallback 顺序
尝试调用 LLM。每个端点内根据 retries 字段做指数退避重试；4xx 永不重试，
SSE 中段直接切下一端点（避免 token 重复消费）。

完整设计见 docs/superpowers/specs/2026-04-29-modelconfig-fallback-and-retry-design.md。
"""
from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any

import httpx


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


class _SSEStreamBroken(Exception):
    """SSE 流提前 EOF 且未收到 [DONE] / message_stop。私有：仅在 invoke
    内部捕获，转化为 fallback 决策（不重试同模型，避免 token 重复消费）。"""


def _stream_one(endpoint: ModelEndpoint, tools, messages) -> dict:
    """对单个 endpoint 发一次 Anthropic SSE 请求并重组完整响应。

    返回字典与 orchestrator 既有 _stream_message 完全一致：
        {"content": [...], "stop_reason": str,
         "input_tokens": int, "output_tokens": int}

    抛出：
      _SSEStreamBroken — 流提前 EOF（无 [DONE]/message_stop）
      httpx.HTTPStatusError — 4xx/5xx；ModelChain.invoke 决定重试还是 raise
      httpx.TimeoutException / ConnectError / RemoteProtocolError — 网络层
    """
    headers = {
        "x-api-key": endpoint.api_key,
        "anthropic-version": "2023-06-01",
        "content-type": "application/json",
        "accept": "text/event-stream",
    }
    base_url = (endpoint.base_url or "https://api.anthropic.com").rstrip("/")
    url = base_url if base_url.endswith("/v1/messages") else base_url + "/v1/messages"

    payload: dict[str, Any] = {
        "model": endpoint.model,
        "max_tokens": int(os.environ.get("MAX_TOKENS", "4096")),
        "messages": messages,
        "stream": True,
    }
    if tools:
        payload["tools"] = tools

    content_blocks: dict[int, dict] = {}
    stop_reason = "end_turn"
    input_tokens = 0
    output_tokens = 0
    stream_complete = False

    with httpx.stream("POST", url, headers=headers, json=payload, timeout=120) as resp:
        resp.raise_for_status()
        for raw_line in resp.iter_lines():
            raw_line = raw_line.strip()
            if not raw_line or not raw_line.startswith("data:"):
                continue
            data_str = raw_line[len("data:"):].strip()
            if data_str == "[DONE]":
                stream_complete = True
                break
            try:
                event = json.loads(data_str)
            except json.JSONDecodeError:
                continue

            etype = event.get("type", "")
            if etype == "message_start":
                usage = event.get("message", {}).get("usage", {})
                input_tokens = usage.get("input_tokens", 0)
            elif etype == "content_block_start":
                idx = event.get("index", 0)
                block = event.get("content_block", {})
                btype = block.get("type", "")
                if btype == "text":
                    content_blocks[idx] = {"type": "text", "text": ""}
                elif btype == "tool_use":
                    content_blocks[idx] = {
                        "type": "tool_use",
                        "id": block.get("id", ""),
                        "name": block.get("name", ""),
                        "input": "",
                    }
                elif btype == "thinking":
                    content_blocks[idx] = {"type": "thinking", "text": ""}
            elif etype == "content_block_delta":
                idx = event.get("index", 0)
                delta = event.get("delta", {})
                dtype = delta.get("type", "")
                if idx in content_blocks:
                    if dtype == "text_delta":
                        content_blocks[idx]["text"] += delta.get("text", "")
                    elif dtype == "input_json_delta":
                        content_blocks[idx]["input"] += delta.get("partial_json", "")
                    elif dtype == "thinking_delta":
                        content_blocks[idx]["text"] += delta.get("thinking", "")
            elif etype == "message_delta":
                delta = event.get("delta", {})
                if delta.get("stop_reason"):
                    stop_reason = delta["stop_reason"]
                usage = event.get("usage", {})
                output_tokens = usage.get("output_tokens", output_tokens)
                if input_tokens == 0:
                    input_tokens = usage.get("input_tokens", 0)
            elif etype == "message_stop":
                stream_complete = True

    if not stream_complete:
        raise _SSEStreamBroken("stream ended without [DONE] or message_stop")

    result_blocks = []
    for idx in sorted(content_blocks.keys()):
        block = content_blocks[idx]
        if block["type"] == "thinking":
            continue
        if block["type"] == "tool_use" and isinstance(block["input"], str):
            try:
                block["input"] = json.loads(block["input"]) if block["input"] else {}
            except json.JSONDecodeError:
                block["input"] = {}
        result_blocks.append(block)

    return {
        "content": result_blocks,
        "stop_reason": stop_reason,
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
    }


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
