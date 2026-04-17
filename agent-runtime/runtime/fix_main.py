"""Fix generator entry point — single LLM call to propose a patch for one finding.

Called as: python -m runtime.fix_main
Reads env var FIX_INPUT_JSON (finding + target), fetches current target YAML
via MCP, asks the LLM for a patch JSON, POSTs the result to the controller.
"""
import base64
import json
import os
import sys

import httpx

from .mcp_client import call_mcp_tool


CONTROLLER_URL = os.environ["CONTROLLER_URL"]
OUTPUT_LANG = os.environ.get("OUTPUT_LANGUAGE", "en")
MODEL = os.environ.get("MODEL", "claude-sonnet-4-6")


def main() -> int:
    finding = json.loads(os.environ["FIX_INPUT_JSON"])

    target = finding["target"]
    print(f"[info] generating fix for finding {finding['findingID']} on "
          f"{target['kind']}/{target['namespace']}/{target['name']}")

    # 1. Fetch current target resource via MCP
    raw = call_mcp_tool("kubectl_get", {
        "kind": target["kind"],
        "namespace": target["namespace"],
        "name": target["name"],
        "apiVersion": _api_version_for_kind(target["kind"]),
    })
    if not raw or raw.startswith("tool error") or raw.startswith("{\"error\""):
        print(f"[error] failed to fetch target: {raw[:200]}", file=sys.stderr)
        return 1
    # raw is usually a JSON document (single object) from the MCP server.
    # Pretty-print it for display + as the "before" snapshot.
    try:
        obj = json.loads(raw)
        current_yaml = json.dumps(obj, indent=2)
    except json.JSONDecodeError:
        current_yaml = raw

    # 2. Single LLM call to produce patch.
    # Use httpx streaming (not the SDK's non-streaming call) to work around
    # proxies that omit the initial text field in content_block_start events.
    prompt = build_prompt(finding, current_yaml, OUTPUT_LANG)
    try:
        text = _stream_llm_call(prompt)
    except Exception as e:
        print(f"[error] LLM call failed: {e}", file=sys.stderr)
        return 2
    try:
        parsed = parse_patch_json(text)
    except (json.JSONDecodeError, ValueError) as e:
        print(f"[error] invalid patch JSON from LLM: {e}\nraw:\n{text}", file=sys.stderr)
        return 2

    # 3. POST callback
    payload = {
        "findingID": finding["findingID"],
        "diagnosticRunRef": finding["runID"],
        "findingTitle": finding.get("title", ""),
        "target": target,
        "patch": {
            "type": parsed.get("type", "strategic-merge"),
            "content": parsed.get("content", ""),
        },
        "beforeSnapshot": base64.b64encode(current_yaml.encode("utf-8")).decode(),
        "explanation": parsed.get("explanation", ""),
    }
    r = httpx.post(f"{CONTROLLER_URL}/internal/fixes", json=payload, timeout=30)
    r.raise_for_status()
    print(f"[info] fix created: {r.json().get('metadata', {}).get('name', '')}")
    return 0


def build_prompt(finding: dict, current_yaml: str, lang: str) -> str:
    lang_clause = (
        "Write the `explanation` field in Simplified Chinese (简体中文)."
        if lang == "zh"
        else "Write the `explanation` field in English."
    )
    return f"""You are a Kubernetes fix suggestion generator.

## Finding
Title: {finding.get('title', '')}
Description: {finding.get('description', '')}
Suggestion: {finding.get('suggestion', '')}

## Current target resource (JSON)
```
{current_yaml}
```

## Instructions
Output a single JSON object with this exact schema:
{{"type": "strategic-merge" | "json-patch", "content": "<patch body as a JSON string>", "explanation": "<1-3 sentences>"}}

- Prefer strategic-merge for typical Deployment/StatefulSet/Service changes.
- Use json-patch only when the edit cannot be expressed as strategic-merge.
- The `content` field must itself be a valid JSON string (you are allowed to double-encode).
- {lang_clause}
- Output ONLY the JSON object. No prose, no code fences.
"""


def parse_patch_json(raw: str) -> dict:
    """Tolerate code fences and stray whitespace around the JSON body."""
    s = raw.strip()
    if s.startswith("```"):
        # strip leading fence (optionally with language tag) and trailing fence
        s = s.lstrip("`")
        if s.startswith("json"):
            s = s[4:]
        # drop trailing fence if present
        if s.endswith("```"):
            s = s[:-3]
    s = s.strip()
    result = json.loads(s)
    if not isinstance(result, dict):
        raise ValueError("expected JSON object")
    if "content" not in result:
        raise ValueError("missing 'content' field")
    return result


def _api_version_for_kind(kind: str) -> str:
    """Return the standard apiVersion for a DiagnosticFix target kind.

    The CRD restricts target.kind to the enum below. Empty for anything else
    makes MCP infer, which typically fails for non-core kinds.
    """
    return {
        "Deployment":  "apps/v1",
        "StatefulSet": "apps/v1",
        "DaemonSet":   "apps/v1",
        "Service":     "v1",
        "ConfigMap":   "v1",
    }.get(kind, "")


def _stream_llm_call(prompt: str) -> str:
    """Call Anthropic via raw SSE streaming, accumulate text, return it.

    Uses httpx directly instead of the Anthropic SDK's non-streaming call to
    match the behavior of proxies like myphxtwo.reborn.tk that only respond
    correctly on the streaming endpoint.
    """
    api_key = os.environ["ANTHROPIC_API_KEY"]
    base_url = os.environ.get("ANTHROPIC_BASE_URL", "https://api.anthropic.com").rstrip("/")
    if base_url.endswith("/v1/messages"):
        url = base_url
    else:
        url = base_url + "/v1/messages"

    headers = {
        "x-api-key": api_key,
        "anthropic-version": "2023-06-01",
        "content-type": "application/json",
        "accept": "text/event-stream",
    }
    payload = {
        "model": MODEL,
        "max_tokens": 2048,
        "messages": [{"role": "user", "content": prompt}],
        "stream": True,
    }

    text_buf = ""
    with httpx.stream("POST", url, headers=headers, json=payload, timeout=60) as resp:
        resp.raise_for_status()
        for raw_line in resp.iter_lines():
            raw_line = raw_line.strip()
            if not raw_line or not raw_line.startswith("data:"):
                continue
            data_str = raw_line[len("data:"):].strip()
            if data_str == "[DONE]":
                break
            try:
                event = json.loads(data_str)
            except json.JSONDecodeError:
                continue
            if event.get("type") == "content_block_delta":
                delta = event.get("delta", {})
                if delta.get("type") == "text_delta":
                    text_buf += delta.get("text", "")
    return text_buf


if __name__ == "__main__":
    sys.exit(main())
