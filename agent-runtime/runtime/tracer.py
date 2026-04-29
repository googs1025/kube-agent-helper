"""Langfuse 可观测性集成 — 未配置时自动降级为 no-op。

环境变量（全部可选）：
    LANGFUSE_PUBLIC_KEY  — Langfuse 项目公钥
    LANGFUSE_SECRET_KEY  — Langfuse 项目密钥
    LANGFUSE_HOST        — Langfuse 服务地址（默认 https://cloud.langfuse.com）

设计原则：
    - Langfuse 故障不得影响主工作流，所有 SDK 调用均捕获异常并降级为 no-op
    - 发送给 Langfuse 的 payload 仅包含元数据，不上传原始 tool_result 内容
"""
import os
from typing import Any, List


def init(run_id: str, skill_names: List[str]) -> "_Tracer | _NoOp":
    """初始化 Langfuse tracer；未配置时返回 no-op。"""
    if not (os.environ.get("LANGFUSE_PUBLIC_KEY") and os.environ.get("LANGFUSE_SECRET_KEY")):
        return _NoOp()
    try:
        from langfuse import Langfuse  # type: ignore
        lf = Langfuse()
        trace = lf.trace(
            id=run_id,
            name="diagnostic-run",
            metadata={"skills": skill_names},
            tags=["kube-agent-helper"],
        )
        return _Tracer(lf, trace)
    except Exception:
        return _NoOp()


def init_fix(finding_id: str, run_id: str) -> "_Tracer | _NoOp":
    """为修复生成任务初始化 tracer。"""
    if not (os.environ.get("LANGFUSE_PUBLIC_KEY") and os.environ.get("LANGFUSE_SECRET_KEY")):
        return _NoOp()
    try:
        from langfuse import Langfuse  # type: ignore
        lf = Langfuse()
        trace = lf.trace(
            name="fix-generation",
            metadata={"finding_id": finding_id, "run_id": run_id},
            tags=["kube-agent-helper", "fix"],
        )
        return _Tracer(lf, trace)
    except Exception:
        return _NoOp()


def sanitize_messages(messages: List[dict], max_content_chars: int = 300) -> List[dict]:
    """裁剪 messages 用于上传 Langfuse，防止原始 tool_result 泄漏到外部服务。

    - tool_result 内容截断到 max_content_chars 字符
    - text 内容截断到 max_content_chars 字符
    - 保留消息结构和角色信息，便于调试
    """
    result = []
    for msg in messages:
        role = msg.get("role", "")
        content = msg.get("content", "")
        if isinstance(content, str):
            result.append({"role": role, "content": content[:max_content_chars]})
        elif isinstance(content, list):
            sanitized_blocks = []
            for block in content:
                btype = block.get("type", "")
                if btype == "tool_result":
                    raw = block.get("content", "")
                    preview = raw[:max_content_chars] if isinstance(raw, str) else str(raw)[:max_content_chars]
                    sanitized_blocks.append({
                        "type": "tool_result",
                        "tool_use_id": block.get("tool_use_id", ""),
                        "content": preview,
                    })
                elif btype == "text":
                    sanitized_blocks.append({
                        "type": "text",
                        "text": block.get("text", "")[:max_content_chars],
                    })
                elif btype == "tool_use":
                    sanitized_blocks.append({
                        "type": "tool_use",
                        "name": block.get("name", ""),
                        "input": block.get("input", {}),
                    })
                else:
                    sanitized_blocks.append({"type": btype})
            result.append({"role": role, "content": sanitized_blocks})
        else:
            result.append({"role": role})
    return result


class _NoOp:
    """Langfuse 未配置时的占位实现，所有方法均为空操作。"""
    def generation(self, **kw) -> "_NoOp":
        return self

    def span(self, **kw) -> "_NoOp":
        return self

    def event(self, **kw) -> None:
        pass

    def end(self, **kw) -> None:
        pass

    def flush(self) -> None:
        pass


class _Tracer:
    def __init__(self, lf: Any, trace: Any) -> None:
        self._lf = lf
        self._trace = trace
        self._degraded = False

    def generation(self, **kw) -> Any:
        if self._degraded:
            return _NoOp()
        try:
            return self._trace.generation(**kw)
        except Exception as exc:
            import sys
            print(f"[warn] langfuse generation() failed, degrading to no-op: {exc}", file=sys.stderr)
            self._degraded = True
            return _NoOp()

    def span(self, **kw) -> Any:
        if self._degraded:
            return _NoOp()
        try:
            return self._trace.span(**kw)
        except Exception as exc:
            import sys
            print(f"[warn] langfuse span() failed, degrading to no-op: {exc}", file=sys.stderr)
            self._degraded = True
            return _NoOp()

    def event(self, *, name: str, level: str = "DEFAULT", metadata: dict | None = None) -> None:
        """Record a discrete trace event (model_retry / model_fallback / etc).

        Failures degrade silently — observability never blocks the main flow.
        """
        if self._degraded:
            return
        try:
            self._trace.event(name=name, level=level, metadata=metadata or {})
        except Exception as exc:
            import sys
            print(f"[warn] langfuse event() failed, degrading to no-op: {exc}", file=sys.stderr)
            self._degraded = True

    def flush(self) -> None:
        try:
            self._lf.flush()
        except Exception:
            pass
