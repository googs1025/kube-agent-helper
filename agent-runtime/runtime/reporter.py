"""POSTs findings back to the Controller with exponential backoff retry."""
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
