"""把 findings 回报给 controller，带指数退避重试。

调用关系：
    Agent Pod ──POST── /internal/runs/{run_id}/findings ──▶ controller HTTP server
                                                              └─▶ Store.CreateFinding

为什么逐条 POST 而不是批量：
    - 一旦其中一条失败也只丢这一条，其它仍可写入
    - controller 端 finding 写入是幂等的（按 RunID+Title 去重）

CONTROLLER_URL 默认指 cluster-internal Service：
    http://controller.kube-agent-helper.svc:8080
跨集群场景下，远端 Pod 通过 NodePort/Ingress/双向 VPC 回到 controller。
"""
import os
import time

import requests

from . import logger

CONTROLLER_URL = os.environ.get("CONTROLLER_URL", "http://controller.kube-agent-helper.svc:8080")

MAX_RETRIES = 3
BACKOFF_BASE = 2  # seconds

# Buffer for LLM observability events (retry / fallback / exhausted) emitted
# from ModelChain. Flushed once at end of run via flush_llm_metrics() — fire
# and forget; controller side increments Prometheus counters.
_LLM_BUFFER: list[dict] = []


def record_llm_event(event_type: str, labels: dict) -> None:
    """Buffer an LLM event for later batch upload to /internal/llm-metrics.

    event_type: any short string identifying the event class. Current
        producers: "retry", "fallback", "exhausted" (model_chain.py),
        "finding_parse_error" (orchestrator.py).
    labels: free-form k/v sent as Prometheus labels server-side.
    """
    _LLM_BUFFER.append({"type": event_type, "labels": labels})


def flush_llm_metrics() -> None:
    """POST the buffered events to controller, then clear the buffer.

    Failures are logged but never raised — observability must not block
    the agent shutdown path.
    """
    if not _LLM_BUFFER:
        return
    url = f"{CONTROLLER_URL}/internal/llm-metrics"
    try:
        requests.post(url, json={"events": list(_LLM_BUFFER)}, timeout=10)
    except Exception as e:
        logger.warn("flush_llm_metrics failed", error=str(e), buffered=len(_LLM_BUFFER))
    finally:
        _LLM_BUFFER.clear()


def post_findings(run_id: str, findings: list) -> None:
    url = f"{CONTROLLER_URL}/internal/runs/{run_id}/findings"
    posted = 0
    for f in findings:
        if _post_with_retry(url, f):
            posted += 1
    logger.info("findings posted", run_id=run_id, total=len(findings), posted=posted)


def _post_with_retry(url: str, payload: dict) -> bool:
    for attempt in range(MAX_RETRIES + 1):
        try:
            resp = requests.post(url, json=payload, timeout=10)
            resp.raise_for_status()
            return True
        except Exception as e:
            if attempt < MAX_RETRIES:
                delay = BACKOFF_BASE ** attempt
                logger.warn(
                    "finding post failed, retrying",
                    attempt=attempt + 1,
                    max_retries=MAX_RETRIES,
                    delay=delay,
                    error=str(e),
                )
                time.sleep(delay)
            else:
                logger.error("finding post failed after retries", error=str(e))
                return False
    return False
