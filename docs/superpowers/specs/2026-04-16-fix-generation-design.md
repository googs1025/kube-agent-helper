# Fix Generation Design

**Status:** Approved
**Date:** 2026-04-16
**Goal:** Let users trigger automatic Fix generation for any finding in a completed Run, producing a DiagnosticFix CR with an LLM-generated patch and a Before/After diff view.

---

## 1. Overview

The existing `DiagnosticFix` CRD and its reconciler apply patches and optionally roll them back, but nothing in the codebase creates Fix CRs today. This design fills that gap: the user clicks "Generate Fix" on a finding in the Run detail page, a short-lived Pod reuses the agent-runtime image with a fix-generator entry point, the Pod reads the target resource via MCP, asks the LLM for a patch, and calls back to the controller to create the Fix CR. The dashboard then shows a Before/After diff of the resource.

## 2. User Flow

```
[Run 详情页: 展开某个 finding]
    ↓
[点击 "生成修复建议" 按钮]
    ↓
POST /api/findings/{findingID}/generate-fix
    ↓
Controller creates a "Fix generator" Job (reuses agent-runtime image, different entrypoint)
    ↓
Job runs:
  1. Reads target resource's current YAML via MCP (used as "Before" snapshot)
  2. Single-turn LLM call: generates patch + natural-language explanation
  3. POST /internal/fixes callback with { patch, beforeSnapshot, explanation }
    ↓
Controller creates DiagnosticFix CR (Phase: PendingApproval)
    ↓
Dashboard polling picks up the new fixID on the finding → UI changes to "View Fix →"
    ↓
User clicks → Fix detail page shows Before/After diff + Approve/Reject buttons
```

**Idempotency:** If a Fix already exists for the given `findingID`, the POST returns the existing Fix immediately without spawning a new Job.

**Timeout:** Fix generator Job has a 120-second deadline. Longer than that → Fix is marked Failed with message "fix generator timed out".

## 3. CRD Changes

### 3.1 DiagnosticFix spec — add `findingID`

```go
type DiagnosticFixSpec struct {
    DiagnosticRunRef string    `json:"diagnosticRunRef"`
    FindingTitle     string    `json:"findingTitle"`
    Target           FixTarget `json:"target"`
    Strategy         string    `json:"strategy"`
    ApprovalRequired bool      `json:"approvalRequired"`
    Patch            FixPatch  `json:"patch"`
    Rollback         RollbackConfig `json:"rollback,omitempty"`
    // +optional
    FindingID string `json:"findingID,omitempty"`
}
```

### 3.2 `beforeSnapshot` — store-only, not in CRD

`BeforeSnapshot` is stored in the `store.Fix` struct (SQLite), not in the CRD. Rationale: the Kubernetes reconciler doesn't need it (it's purely a dashboard diff-viewer artifact), and embedding a full resource YAML in every CR bloats etcd and API responses. The dashboard reads it via `GET /api/fixes/{id}` which returns the store record.

Tradeoff: Fix CRs created by `kubectl apply` directly (e.g. CI smoke tests) won't have a `beforeSnapshot` and thus won't render a diff. This is acceptable — manual applies already bypass the store layer today, and the diff is a convenience feature.

### 3.3 Store layer — add `FindingID` and `BeforeSnapshot`

```go
type Fix struct {
    // existing fields...
    FindingID       string
    BeforeSnapshot  string
}
```

SQLite migration adds two columns to the `fixes` table (both TEXT, nullable).

### 3.4 CRD YAML schema

Add `findingID` under `spec.properties` and `beforeSnapshot` under `status.properties` in
`deploy/helm/templates/crds/k8sai.io_diagnosticfixes.yaml`.

## 4. Finding → Fix Linkage

Findings don't have a stable ID across restarts today (`store.Finding.ID` exists as SQLite PK but is an opaque UUID). Two implementation choices:

1. Expose the existing `store.Finding.ID` as `findingID` in the HTTP API — already the case because `store.Finding` is JSON-marshalled directly.
2. Synthesize a deterministic hash from `runID + title + resourceName` — more stable but risks collisions.

**Decision:** Use the existing `store.Finding.ID` (option 1). No new field needed in the store layer; the HTTP response already includes `ID`.

**GET /api/runs/{runID}/findings response** gets a new `fixID` field per finding (computed by the httpserver via `store.ListFixesByRun(runID)` + matching on `Fix.FindingID`):

```json
{
  "ID": "finding-uuid",
  "Title": "...",
  ...
  "FixID": "fix-uuid-or-empty"
}
```

## 5. Controller Components

### 5.1 New translator: `FixGeneratorTranslator`

A dedicated translator that compiles a "generate fix" request into a single Kubernetes Job. Much simpler than the Run translator — no ConfigMap, no skill list, no RBAC binding beyond the existing reader SA. The Job spec:

- Image: `kube-agent-helper/agent-runtime:dev` (same as diagnostic agent)
- Command: `["python", "-m", "runtime.fix_main"]` (new entrypoint)
- ActiveDeadlineSeconds: 120
- BackoffLimit: 0
- ServiceAccount: reuse the run's existing per-run ServiceAccount (`run-<runName>`) created by the Run translator. That SA already has read-only cluster access via the `view` ClusterRoleBinding. The Fix generator only needs read access (no write — applying the patch is the reconciler's job via the controller SA).
- Env vars:
  - `FIX_INPUT_JSON` — full finding + target info
  - `CONTROLLER_URL`, `OUTPUT_LANGUAGE`, `MODEL`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY` — same as diagnostic agent
  - `MCP_SERVER_PATH`
- OwnerReference: points to nothing initially (the Fix CR doesn't exist yet). The Job sets `ttlSecondsAfterFinished: 600` for cleanup.

### 5.2 New HTTP endpoints

| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/findings/{findingID}/generate-fix` | `handleAPIGenerateFix` |
| POST | `/internal/fixes` | `handleInternalFixCallback` |

#### `handleAPIGenerateFix(findingID)`
1. Load finding from store → 404 if not found
2. Load run by `finding.RunID` to resolve target info → 500 if run missing
3. Check for existing Fix with `FindingID = findingID` → if found, return 200 with `{fixID}` immediately
4. Call `FixGeneratorTranslator.Compile(finding, run)` → Job object
5. Create Job via k8sClient
6. Return 202 Accepted `{status: "generating"}`

The UI polls the finding's `fixID` until non-empty.

#### `handleInternalFixCallback(body)`
Body shape:
```json
{
  "findingID": "...",
  "diagnosticRunRef": "...",
  "findingTitle": "...",
  "target": { "kind": "...", "namespace": "...", "name": "..." },
  "patch": { "type": "strategic-merge", "content": "..." },
  "beforeSnapshot": "<base64 YAML>",
  "explanation": "..."
}
```
Handler:
1. Validate required fields
2. Create DiagnosticFix CR via k8sClient with:
   - `spec.findingID = body.findingID`
   - `spec.diagnosticRunRef = body.diagnosticRunRef`
   - `spec.findingTitle = body.findingTitle`
   - `spec.target = body.target`
   - `spec.strategy = "dry-run"` (default; user can later change)
   - `spec.approvalRequired = true`
   - `spec.patch = body.patch`
   - `spec.rollback.enabled = true`, `snapshotBefore = true`
3. Call `store.CreateFix(...)` with `BeforeSnapshot = body.beforeSnapshot`, `Message = body.explanation`, `FindingID = body.findingID`, `ID = string(cr.UID)` (so the store record shares the CR's UID — simplifies GET /api/fixes/{id} lookups)
4. Return 201 with CR JSON

**Authentication:** the `/internal/*` endpoints are already unauthenticated in this codebase (they're meant for in-cluster agent callbacks). No change.

### 5.3 Existing FixReconciler — unchanged

The reconciler already handles `PendingApproval → Approved → Applying → Succeeded/Failed/RolledBack`. No logic changes needed. `BeforeSnapshot` is a status field that's only read by the dashboard, not by the reconciler (the reconciler's own `RollbackSnapshot` is separate — captured at `Applying` time for rollback, whereas `BeforeSnapshot` is captured at generation time for the diff view).

## 6. Agent Runtime — `runtime/fix_main.py`

```python
import base64
import json
import os
import sys
import httpx

from runtime.mcp_client import call_mcp_tool  # extracted from orchestrator.py if reusable
from runtime.skill_loader import Skill  # not used here, but imports module fine
import anthropic


CONTROLLER_URL = os.environ["CONTROLLER_URL"]
OUTPUT_LANG = os.environ.get("OUTPUT_LANGUAGE", "en")
MODEL = os.environ.get("MODEL", "claude-sonnet-4-6")


def main() -> int:
    finding = json.loads(os.environ["FIX_INPUT_JSON"])

    target = finding["target"]
    print(f"[info] generating fix for finding {finding['findingID']} on {target['kind']}/{target['namespace']}/{target['name']}")

    # 1. Fetch current target YAML via MCP (single tool call, no loop)
    current = call_mcp_tool("kubectl_get", {
        "kind": target["kind"],
        "namespace": target["namespace"],
        "name": target["name"],
        "apiVersion": "",
    })
    if not current or "error" in current:
        print(f"[error] failed to fetch target: {current}", file=sys.stderr)
        return 1

    current_yaml = json.dumps(current, indent=2)  # served as "Before"

    # 2. Single LLM call
    client = anthropic.Anthropic()
    response = client.messages.create(
        model=MODEL,
        max_tokens=2048,
        messages=[{
            "role": "user",
            "content": build_prompt(finding, current_yaml, OUTPUT_LANG),
        }],
    )
    raw = response.content[0].text
    parsed = parse_patch_json(raw)  # see below

    # 3. POST back to controller
    payload = {
        "findingID": finding["findingID"],
        "diagnosticRunRef": finding["runID"],
        "findingTitle": finding["title"],
        "target": target,
        "patch": {"type": parsed["type"], "content": parsed["content"]},
        "beforeSnapshot": base64.b64encode(current_yaml.encode()).decode(),
        "explanation": parsed.get("explanation", ""),
    }
    r = httpx.post(f"{CONTROLLER_URL}/internal/fixes", json=payload, timeout=30)
    r.raise_for_status()
    print(f"[info] fix created: {r.json().get('metadata', {}).get('name', '')}")
    return 0


def build_prompt(finding: dict, current_yaml: str, lang: str) -> str:
    lang_clause = "Write the `explanation` field in Simplified Chinese." if lang == "zh" else "Write the `explanation` field in English."
    return f"""You are a Kubernetes fix suggestion generator.

## Finding
Title: {finding["title"]}
Description: {finding["description"]}
Suggestion: {finding["suggestion"]}

## Current target resource YAML
```yaml
{current_yaml}
```

## Instructions
Output a single JSON object with this schema:
{{"type": "strategic-merge" | "json-patch", "content": "<patch body as a JSON string>", "explanation": "<1-3 sentences explaining the fix>"}}

- Prefer strategic-merge for typical Deployment/StatefulSet/Service changes.
- Use json-patch only when the edit cannot be expressed as strategic-merge (e.g. deleting an array element by index).
- The content must be a valid JSON string (strategic-merge patches are also written as JSON).
- {lang_clause}
- Output ONLY the JSON object. No prose, no code fences.
"""


def parse_patch_json(raw: str) -> dict:
    """Tolerate code fences and stray whitespace around the JSON body."""
    s = raw.strip()
    if s.startswith("```"):
        s = s.strip("`")
        # drop optional language tag
        if s.startswith("json"):
            s = s[4:]
    s = s.strip()
    return json.loads(s)


if __name__ == "__main__":
    sys.exit(main())
```

**`runtime/mcp_client.py`** (extract from orchestrator.py if not already a module): a thin helper that runs the MCP server in-cluster for one tool call. The existing orchestrator has `_call_mcp_tool` — that helper should either be extracted into a shared module or duplicated here. For this design: extract into a new `runtime/mcp_client.py` module, import from both places.

## 7. Dashboard Changes

### 7.1 Finding card — "Generate Fix" button

In `dashboard/src/app/runs/[id]/page.tsx`, each finding card's footer gets a new action row:

```tsx
<div className="mt-3 flex justify-end">
  {f.FixID
    ? <Link href={`/fixes/${f.FixID}`} className="text-sm text-blue-600 hover:underline dark:text-blue-400">{t("runs.findings.viewFix")} →</Link>
    : <Button size="sm" variant="outline" onClick={() => handleGenerate(f.ID)} disabled={generating[f.ID]}>
        {generating[f.ID] ? t("runs.findings.generating") : t("runs.findings.generateFix")}
      </Button>
  }
</div>
```

`generating` state is a `Record<string, boolean>` kept in the page component. `handleGenerate` POSTs to `/api/findings/{findingID}/generate-fix`, sets `generating[id] = true`, then relies on SWR polling `useFindings()` to pick up the new `FixID`.

### 7.2 Fix detail page — Before/After diff block

New component: `dashboard/src/components/resource-diff.tsx`

```tsx
"use client";

import ReactDiffViewer, { DiffMethod } from "react-diff-viewer-continued";
import { useTheme } from "@/theme/context";

interface Props {
  before: string;  // raw YAML
  after: string;   // raw YAML
}

export function ResourceDiff({ before, after }: Props) {
  const { theme } = useTheme();
  return (
    <ReactDiffViewer
      oldValue={before}
      newValue={after}
      splitView
      useDarkTheme={theme === "dark"}
      compareMethod={DiffMethod.LINES}
    />
  );
}
```

In `dashboard/src/app/fixes/[id]/page.tsx`, above the existing Patch Content card:

```tsx
{fix.BeforeSnapshot && (
  <Card className="mb-4">
    <CardHeader>
      <CardTitle className="text-base">{t("fixes.detail.diffTitle")}</CardTitle>
    </CardHeader>
    <CardContent>
      <ResourceDiff before={decodeBefore(fix.BeforeSnapshot)} after={applyPatchForPreview(fix)} />
    </CardContent>
  </Card>
)}
```

**`applyPatchForPreview(fix)`** is a client-side helper:
- For `strategic-merge`: parse `BeforeSnapshot` YAML to object, deep-merge `patch.content` (parsed as JSON), stringify back to YAML
- For `json-patch`: apply the RFC 6902 patch using the `fast-json-patch` npm package

Both libraries (`js-yaml`, `fast-json-patch`) are npm-add-able and lightweight (<20KB combined gzipped).

For v1: only implement strategic-merge. If the patch type is `json-patch`, show "Preview unavailable for json-patch" and hide the diff.

### 7.3 i18n keys

Add to `dashboard/src/i18n/zh.json` and `en.json`:

```json
{
  "runs": {
    "findings": {
      "generateFix": "生成修复建议" / "Generate Fix",
      "generating": "生成中..." / "Generating...",
      "viewFix": "查看修复建议" / "View Fix"
    }
  },
  "fixes": {
    "detail": {
      "diffTitle": "资源变更预览" / "Resource Change Preview",
      "diffUnavailable": "json-patch 类型的预览暂不支持" / "Preview unavailable for json-patch"
    }
  }
}
```

## 8. Data Flow Summary

```
Dashboard Run detail
  ↓ [user clicks Generate Fix]
POST /api/findings/{id}/generate-fix
  ↓
httpserver.handleAPIGenerateFix
  ├─ finding = store.ListFindings(...) → filter by id
  ├─ existing = store.ListFixesByRun(finding.RunID) → match FindingID
  ├─ if existing: return 200 { fixID: existing.ID }
  └─ FixGeneratorTranslator.Compile(finding, run) → Job object
     ↓
k8sClient.Create(Job)
  ↓
Agent pod starts → runtime/fix_main.py
  ├─ MCP kubectl_get → current YAML
  ├─ Anthropic API → patch + explanation
  └─ POST /internal/fixes { findingID, patch, beforeSnapshot, explanation, ... }
       ↓
       httpserver.handleInternalFixCallback
         ├─ k8sClient.Create(DiagnosticFix CR)
         └─ store.CreateFix(...)
              ↓
Dashboard polling on /api/runs/{runID}/findings
  ↓ picks up fixID on the finding
UI changes: "Generate Fix" → "View Fix →"
  ↓ [user clicks]
Fix detail page renders
  ├─ Before (decode status.beforeSnapshot)
  ├─ After (client-side apply of spec.patch to Before)
  └─ Approve/Reject buttons
```

## 9. Error Handling

| Failure | Handling |
|---------|----------|
| Fix generator Job times out (120s) | Job controller marks Failed. Dashboard finding button stays on "Generate Fix" (user can retry). No Fix CR is created. |
| MCP kubectl_get fails (target deleted, RBAC denied) | `fix_main.py` exits non-zero. Same as above — no Fix CR. User retries. |
| LLM returns invalid JSON | `parse_patch_json` raises `json.JSONDecodeError` → non-zero exit. Same as above. |
| `/internal/fixes` callback fails (controller down) | Agent prints error and exits non-zero. Retry via button. |
| Two concurrent generate-fix requests for the same finding | The 2nd request's "existing Fix" check prevents a duplicate Job. Race window is small because Job creation is fast. |
| User clicks "Generate Fix" before the 1st Job has finished | UI sets `generating[id]=true` optimistically, so button is disabled. Backend still checks existing-fix to prevent duplicate Jobs if the UI state is stale. |

## 10. Non-Goals

- Editing the patch content in the UI before approval (show-and-approve only in v1)
- Multi-turn LLM refinement of the fix
- Preview of json-patch type (strategic-merge only in v1)
- Batch generation (generate fixes for all findings at once)
- Retry UI for failed fix generation (user just clicks the button again)
- Per-user auditing of who triggered generation (logging goes to stdout only)

## 11. File Structure After Change

```
internal/controller/
├── api/v1alpha1/types.go             (modify — add FindingID, BeforeSnapshot)
├── api/v1alpha1/zz_generated.deepcopy.go  (regen)
├── translator/
│   ├── translator.go                 (unchanged — Run translator)
│   └── fix_generator.go              (new — FixGeneratorTranslator)
│   └── fix_generator_test.go         (new — TDD)
├── httpserver/server.go              (modify — new handlers)
├── httpserver/server_test.go         (modify — TDD)

internal/store/
├── store.go                          (modify — Fix struct adds FindingID, BeforeSnapshot)
└── sqlite/sqlite.go                  (modify — migration + SQL updates)

deploy/helm/templates/crds/
└── k8sai.io_diagnosticfixes.yaml     (modify — add new fields to schema)

agent-runtime/runtime/
├── mcp_client.py                     (new — extracted from orchestrator.py)
├── orchestrator.py                   (modify — import mcp_client instead of private helpers)
└── fix_main.py                       (new — fix generator entrypoint)

dashboard/src/
├── i18n/zh.json                      (modify — add 5 keys)
├── i18n/en.json                      (modify — add 5 keys)
├── lib/types.ts                      (modify — Fix.FindingID, Fix.BeforeSnapshot, Finding.FixID)
├── lib/api.ts                        (modify — add generateFix())
├── components/resource-diff.tsx      (new)
├── app/runs/[id]/page.tsx            (modify — Generate/View button per finding)
└── app/fixes/[id]/page.tsx           (modify — diff block)
```

## 12. Testing Strategy

### Unit tests
- `translator/fix_generator_test.go` — CR → Job shape: correct image, command, env vars, deadline, labels
- `httpserver/server_test.go`:
  - `TestPostGenerateFix_FindingNotFound` → 404
  - `TestPostGenerateFix_CreatesJob` → 202 + Job in fake client
  - `TestPostGenerateFix_IdempotentOnExistingFix` → 200 with existing fixID, no new Job
  - `TestPostInternalFixes_CreatesFixCR` → 201 + CR in fake client + store entry

### Manual QA
- Complete a Run that produces at least 1 finding
- Click "Generate Fix" on one finding → wait ~30s
- Confirm the Fix CR appears, dashboard finding button switches to "View Fix"
- Open Fix detail → verify Before/After diff shows realistic changes
- Click Approve → Fix reconciles to Succeeded (or Failed if strategic-merge is rejected by API server)

## 13. Migration Notes

- Existing Fix CRs in the cluster without `spec.findingID` or `status.beforeSnapshot` are backward-compatible: both fields are optional. Dashboard simply doesn't show the diff for those.
- Store `fixes` table gets two new columns (both TEXT NULL). SQLite `ALTER TABLE ADD COLUMN` is safe and non-blocking.
