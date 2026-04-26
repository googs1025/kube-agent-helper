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
