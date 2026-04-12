"""POSTs findings back to the Controller."""
import json
import os

import requests

CONTROLLER_URL = os.environ.get("CONTROLLER_URL", "http://controller.kube-agent-helper.svc:8080")


def post_findings(run_id: str, findings: list) -> None:
    url = f"{CONTROLLER_URL}/internal/runs/{run_id}/findings"
    for f in findings:
        try:
            resp = requests.post(url, json=f, timeout=10)
            resp.raise_for_status()
        except Exception as e:
            print(f"[warn] failed to post finding: {e}")
    print(f"[info] posted {len(findings)} findings for run {run_id}")
