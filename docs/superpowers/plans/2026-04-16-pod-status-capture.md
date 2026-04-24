# Pod Status Capture & Dashboard Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the reconciler detect Pod-level errors (ImagePullBackOff, CrashLoopBackOff, etc.) during Running phase, write them into `status.message`, and optionally time out stalled Runs. Display the message in the dashboard. Add a Fixes dashboard page.

**Architecture:** The reconciler's Running-phase check currently only reads Job counters. We add a Pod-status inspection step that lists Pods owned by the Job, reads container `waiting.reason`, and writes any error into `status.message` (without transitioning phase — the Job controller still owns terminal transitions). An optional `spec.timeoutSeconds` field lets users set a deadline; if omitted, no timeout applies. The dashboard Runs list and detail pages surface the `Message` field.

**Tech Stack:** Go (controller-runtime), Kubernetes fake client (tests), Next.js/React (dashboard)

---

### Task 1: Reconciler — detect Pod waiting errors and update status.message

**Files:**
- Modify: `internal/controller/reconciler/run_reconciler.go:88-117`
- Test: `internal/controller/reconciler/run_reconciler_test.go`

- [ ] **Step 1: Write the failing test — Pod in ImagePullBackOff updates message**

Add to `run_reconciler_test.go`:

```go
func TestRunReconciler_RunningPodImagePullBackOff(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	now := metav1.Now()
	run.Status.StartedAt = &now

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run-abc", Namespace: "default",
			Labels: map[string]string{"job-name": "agent-test-run"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "agent",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ImagePullBackOff",
						Message: "Back-off pulling image \"ghcr.io/kube-agent-helper/agent-runtime:latest\"",
					},
				},
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job, pod).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should still requeue — not terminal yet")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase, "phase stays Running")
	assert.Contains(t, updated.Status.Message, "ImagePullBackOff")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/reconciler/... -run TestRunReconciler_RunningPodImagePullBackOff -v`
Expected: FAIL — Message is empty because the reconciler doesn't inspect pods yet.

- [ ] **Step 3: Write the failing test — Pod in CrashLoopBackOff updates message**

Add to `run_reconciler_test.go`:

```go
func TestRunReconciler_RunningPodCrashLoopBackOff(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	now := metav1.Now()
	run.Status.StartedAt = &now

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run-xyz", Namespace: "default",
			Labels: map[string]string{"job-name": "agent-test-run"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "agent",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: "back-off 5m0s restarting failed container",
					},
				},
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job, pod).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter)

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "CrashLoopBackOff")
}
```

- [ ] **Step 4: Implement Pod status inspection in the reconciler**

In `run_reconciler.go`, replace the "Still running — requeue" block (lines 115-116) with Pod inspection logic. Add the import `corev1 "k8s.io/api/core/v1"` and add a helper method:

```go
// Add import:
// corev1 "k8s.io/api/core/v1"

// Replace lines 114-116 (after the job.Status.Failed block, before the closing brace):

		// Still running — check Pod health for early error signals
		msg := r.podWaitingReason(ctx, job.Name, run.Namespace)
		if msg != "" && msg != run.Status.Message {
			run.Status.Message = msg
			if err := r.Status().Update(ctx, &run); err != nil {
				logger.Error(err, "failed to update run message")
			}
			_ = r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseRunning, msg)
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
```

Add the helper method after `failRun`:

```go
// podWaitingReason lists pods for the given job and returns a human-readable
// message if any container is in a waiting state (e.g. ImagePullBackOff).
func (r *DiagnosticRunReconciler) podWaitingReason(ctx context.Context, jobName, namespace string) string {
	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"job-name": jobName},
	); err != nil {
		return ""
	}
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				reason := cs.State.Waiting.Reason
				detail := cs.State.Waiting.Message
				if detail != "" {
					return fmt.Sprintf("pod %s: %s — %s", pod.Name, reason, detail)
				}
				return fmt.Sprintf("pod %s: %s", pod.Name, reason)
			}
		}
		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				reason := cs.State.Waiting.Reason
				detail := cs.State.Waiting.Message
				if detail != "" {
					return fmt.Sprintf("pod %s (init): %s — %s", pod.Name, reason, detail)
				}
				return fmt.Sprintf("pod %s (init): %s", pod.Name, reason)
			}
		}
	}
	return ""
}
```

- [ ] **Step 5: Run all reconciler tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/reconciler/... -v`
Expected: All tests pass, including the two new Pod-status tests and the 5 existing tests.

- [ ] **Step 6: Commit**

```bash
git add internal/controller/reconciler/run_reconciler.go internal/controller/reconciler/run_reconciler_test.go
git commit -m "feat(reconciler): detect pod waiting errors and surface in status.message"
```

---

### Task 2: Optional timeout — spec.timeoutSeconds field

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go:52-56`
- Modify: `deploy/helm/templates/crds/k8sai.io_diagnosticruns.yaml`
- Modify: `internal/controller/reconciler/run_reconciler.go`
- Test: `internal/controller/reconciler/run_reconciler_test.go`

- [ ] **Step 1: Add timeoutSeconds to DiagnosticRunSpec**

In `internal/controller/api/v1alpha1/types.go`, add to `DiagnosticRunSpec`:

```go
type DiagnosticRunSpec struct {
	Target         TargetSpec `json:"target"`
	Skills         []string   `json:"skills,omitempty"`
	ModelConfigRef string     `json:"modelConfigRef"`
	// +optional
	TimeoutSeconds *int32     `json:"timeoutSeconds,omitempty"`
}
```

- [ ] **Step 2: Add timeoutSeconds to CRD YAML schema**

In `deploy/helm/templates/crds/k8sai.io_diagnosticruns.yaml`, add after the `target` block inside `spec.properties` (around line 67):

```yaml
              timeoutSeconds:
                type: integer
                minimum: 0
```

- [ ] **Step 3: Write the failing test — timeout triggers failure**

Add to `run_reconciler_test.go`:

```go
func TestRunReconciler_RunningTimeout(t *testing.T) {
	timeout := int32(60) // 60 seconds
	run := testRun()
	run.Spec.TimeoutSeconds = &timeout
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	// StartedAt was 2 minutes ago — past the 60s timeout
	past := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.Zero(t, result.RequeueAfter, "terminal — should not requeue")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Failed", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "timed out")
}
```

- [ ] **Step 4: Write the passing test — no timeout when field is nil**

Add to `run_reconciler_test.go`:

```go
func TestRunReconciler_RunningNoTimeoutWhenNil(t *testing.T) {
	run := testRun()
	// No TimeoutSeconds set — run.Spec.TimeoutSeconds is nil
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should keep polling — no timeout configured")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
}
```

- [ ] **Step 5: Run tests to verify the new ones fail**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/reconciler/... -run "TestRunReconciler_Running(Timeout|NoTimeout)" -v`
Expected: `TestRunReconciler_RunningTimeout` FAILS (no timeout logic yet), `TestRunReconciler_RunningNoTimeoutWhenNil` PASSES (existing requeue behavior).

- [ ] **Step 6: Implement timeout check in reconciler**

In `run_reconciler.go`, add timeout check at the start of the Running phase block (after fetching the Job, before checking `job.Status.Succeeded`). Insert between lines 99-101:

```go
		// Optional timeout
		if run.Spec.TimeoutSeconds != nil && run.Status.StartedAt != nil {
			deadline := run.Status.StartedAt.Time.Add(time.Duration(*run.Spec.TimeoutSeconds) * time.Second)
			if time.Now().After(deadline) {
				return r.failRun(ctx, &run, fmt.Sprintf("run timed out after %ds", *run.Spec.TimeoutSeconds))
			}
		}
```

- [ ] **Step 7: Run all reconciler tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/reconciler/... -v`
Expected: All tests pass (7 existing + 4 new = 11 total).

- [ ] **Step 8: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go deploy/helm/templates/crds/k8sai.io_diagnosticruns.yaml internal/controller/reconciler/run_reconciler.go internal/controller/reconciler/run_reconciler_test.go
git commit -m "feat(reconciler): add optional spec.timeoutSeconds for run deadline"
```

---

### Task 3: Dashboard — show Message in Runs list and detail page

**Files:**
- Modify: `dashboard/src/app/page.tsx`
- Modify: `dashboard/src/app/runs/[id]/page.tsx`

- [ ] **Step 1: Add Message column to Runs list**

In `dashboard/src/app/page.tsx`, add a "Message" column to the table. After the `<TableHead>Target</TableHead>` line, add:

```tsx
<TableHead>Message</TableHead>
```

After the `<TableCell>` for `target`, add:

```tsx
<TableCell className="max-w-xs truncate text-sm text-gray-600" title={run.Message || ""}>
  {run.Message || "-"}
</TableCell>
```

- [ ] **Step 2: Style the Message in run detail page**

In `dashboard/src/app/runs/[id]/page.tsx`, replace the existing Message display (line 55):

```tsx
{run.Message && <p className="mt-2 text-sm text-gray-700">{run.Message}</p>}
```

With a more visible styled block:

```tsx
{run.Message && (
  <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
    run.Status === "Failed"
      ? "border-red-200 bg-red-50 text-red-700"
      : run.Status === "Running"
        ? "border-yellow-200 bg-yellow-50 text-yellow-800"
        : "border-gray-200 bg-gray-50 text-gray-700"
  }`}>
    {run.Message}
  </div>
)}
```

- [ ] **Step 3: Build dashboard to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/page.tsx dashboard/src/app/runs/[id]/page.tsx
git commit -m "feat(dashboard): display run status message in list and detail views"
```

---

### Task 4: Add timeoutSeconds to create-run dialog (frontend)

**Files:**
- Modify: `dashboard/src/components/create-run-dialog.tsx`
- Modify: `dashboard/src/lib/types.ts`

- [ ] **Step 1: Add timeoutSeconds to CreateRunRequest type**

In `dashboard/src/lib/types.ts`, add to `CreateRunRequest`:

```typescript
export interface CreateRunRequest {
  name?: string;
  namespace: string;
  target: {
    scope: "namespace" | "cluster";
    namespaces?: string[];
    labelSelector?: Record<string, string>;
  };
  skills?: string[];
  modelConfigRef: string;
  timeoutSeconds?: number;
}
```

- [ ] **Step 2: Add timeout input to CreateRunDialog**

In `dashboard/src/components/create-run-dialog.tsx`, add state:

```tsx
const [timeoutSeconds, setTimeoutSeconds] = useState<string>("");
```

Add to the `body` construction in `handleSubmit` (before `setLoading(true)`):

```typescript
const body: CreateRunRequest = {
  name: name || undefined,
  namespace,
  target: {
    scope,
    namespaces: scope === "namespace" && namespaces.length > 0 ? namespaces : undefined,
    labelSelector: labelSelector.length > 0 ? parseLabelSelector(labelSelector) : undefined,
  },
  skills: skills.length > 0 ? skills : undefined,
  modelConfigRef,
  timeoutSeconds: timeoutSeconds ? Number(timeoutSeconds) : undefined,
};
```

Add the input field after the ModelConfigRef section (before the submit buttons div):

```tsx
<div className="space-y-1.5">
  <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
    Timeout <span className="font-normal normal-case text-gray-400">（秒，留空 = 不超时）</span>
  </label>
  <input type="number" min={0} value={timeoutSeconds} onChange={(e) => setTimeoutSeconds(e.target.value)}
    placeholder="600"
    className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
</div>
```

Add `setTimeoutSeconds("")` to the form reset in the success handler.

- [ ] **Step 3: Build dashboard to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/lib/types.ts dashboard/src/components/create-run-dialog.tsx
git commit -m "feat(dashboard): add optional timeout field to create-run dialog"
```

---

### Task 5: Backend — accept timeoutSeconds in POST /api/runs

**Files:**
- Modify: `internal/controller/httpserver/server.go:298-348`
- Test: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `server_test.go`:

```go
func TestPostRunWithTimeout(t *testing.T) {
	fs := fakeStore()
	fc := newFakeK8sClient()
	srv := httpserver.New(fs, fc)

	body := `{"namespace":"default","target":{"scope":"namespace"},"modelConfigRef":"creds","timeoutSeconds":300}`
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"timeoutSeconds":300`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -run TestPostRunWithTimeout -v`
Expected: FAIL — timeoutSeconds not in the request struct or CR creation.

- [ ] **Step 3: Implement timeoutSeconds in handleAPIRunsPost**

In `server.go`, add `TimeoutSeconds *int32` to the request struct in `handleAPIRunsPost`:

```go
func (s *Server) handleAPIRunsPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Target    struct {
			Scope         string            `json:"scope"`
			Namespaces    []string          `json:"namespaces"`
			LabelSelector map[string]string `json:"labelSelector"`
		} `json:"target"`
		Skills         []string `json:"skills"`
		ModelConfigRef string   `json:"modelConfigRef"`
		TimeoutSeconds *int32   `json:"timeoutSeconds"`
	}
	// ... (rest stays the same until CR creation)

	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: v1alpha1.DiagnosticRunSpec{
			Target: v1alpha1.TargetSpec{
				Scope:         req.Target.Scope,
				Namespaces:    req.Target.Namespaces,
				LabelSelector: req.Target.LabelSelector,
			},
			Skills:         req.Skills,
			ModelConfigRef: req.ModelConfigRef,
			TimeoutSeconds: req.TimeoutSeconds,
		},
	}
	// ... (rest stays the same)
}
```

- [ ] **Step 4: Run all httpserver tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/server.go internal/controller/httpserver/server_test.go
git commit -m "feat(httpserver): pass timeoutSeconds through to DiagnosticRun CR"
```

---

### Task 6: Rebuild and deploy

- [ ] **Step 1: Build controller image**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
eval $(minikube docker-env)
docker build -t kube-agent-helper/controller:dev -f Dockerfile .
```

- [ ] **Step 2: Apply updated CRD**

```bash
kubectl apply -f deploy/helm/templates/crds/k8sai.io_diagnosticruns.yaml
```

- [ ] **Step 3: Restart controller**

```bash
kubectl rollout restart deploy/kah-controller -n kube-agent-helper
kubectl rollout status deploy/kah-controller -n kube-agent-helper --timeout=60s
```

- [ ] **Step 4: Re-establish port-forward**

```bash
kill $(pgrep -f 'port-forward svc/kah') 2>/dev/null
kubectl port-forward svc/kah -n kube-agent-helper 8080:8080 &
```

- [ ] **Step 5: Verify POST /api/runs works**

```bash
curl -s -X POST http://localhost:8080/api/runs \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"kube-agent-helper","target":{"scope":"namespace"},"modelConfigRef":"anthropic-credentials","timeoutSeconds":300}' \
  -w "\nHTTP %{http_code}"
```

Expected: HTTP 201 with CR JSON including `timeoutSeconds`.

- [ ] **Step 6: Commit any remaining changes**

```bash
git add -A
git commit -m "chore: rebuild and deploy with pod status capture"
```

---

### Task 7: Dashboard — Fixes page with list, stats, and detail

**Files:**
- Create: `dashboard/src/app/fixes/page.tsx`
- Create: `dashboard/src/app/fixes/[id]/page.tsx`
- Modify: `dashboard/src/lib/types.ts` (add Fix type)
- Modify: `dashboard/src/lib/api.ts` (add useFixes, useFix hooks)
- Modify: `dashboard/src/app/layout.tsx` (add Fixes nav link)

- [ ] **Step 1: Add Fix type to types.ts**

Append to `dashboard/src/lib/types.ts`:

```typescript
export interface Fix {
  ID: string;
  RunID: string;
  FindingTitle: string;
  TargetKind: string;
  TargetNamespace: string;
  TargetName: string;
  Strategy: string;
  ApprovalRequired: boolean;
  PatchType: string;
  PatchContent: string;
  Phase: "PendingApproval" | "Approved" | "Applying" | "Succeeded" | "Failed" | "RolledBack" | "DryRunComplete";
  ApprovedBy: string;
  RollbackSnapshot: string;
  Message: string;
  CreatedAt: string;
  UpdatedAt: string;
}
```

- [ ] **Step 2: Add SWR hooks for Fixes to api.ts**

Append to `dashboard/src/lib/api.ts`:

```typescript
export function useFixes() {
  return useSWR<Fix[]>("/api/fixes", fetcher, { refreshInterval: 5000 });
}

export function useFix(id: string) {
  return useSWR<Fix>(`/api/fixes/${id}`, fetcher, { refreshInterval: 5000 });
}

export async function approveFix(id: string, approvedBy: string): Promise<void> {
  const res = await fetch(`/api/fixes/${id}/approve`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approvedBy }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}

export async function rejectFix(id: string): Promise<void> {
  const res = await fetch(`/api/fixes/${id}/reject`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}
```

Update the import line at the top of `api.ts`:

```typescript
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest, Fix } from "./types";
```

- [ ] **Step 3: Add Fixes nav link to layout.tsx**

In `dashboard/src/app/layout.tsx`, add after the Skills link:

```tsx
<Link href="/fixes" className="text-gray-600 hover:text-gray-900">Fixes</Link>
```

- [ ] **Step 4: Create Fixes list page**

Create `dashboard/src/app/fixes/page.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useFixes } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800",
  Approved: "bg-blue-100 text-blue-800",
  Applying: "bg-blue-100 text-blue-800",
  Succeeded: "bg-green-100 text-green-800",
  Failed: "bg-red-100 text-red-800",
  RolledBack: "bg-orange-100 text-orange-800",
  DryRunComplete: "bg-purple-100 text-purple-800",
};

export default function FixesPage() {
  const { data: fixes, error, isLoading } = useFixes();
  if (isLoading) return <p className="text-gray-500">Loading fixes...</p>;
  if (error) return <p className="text-red-600">Failed to load fixes.</p>;

  const total = fixes?.length ?? 0;
  const pending = fixes?.filter((f) => f.Phase === "PendingApproval").length ?? 0;
  const succeeded = fixes?.filter((f) => f.Phase === "Succeeded").length ?? 0;
  const failed = fixes?.filter((f) => ["Failed", "RolledBack"].includes(f.Phase)).length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Fixes</h1>
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: "Total", value: total, color: "text-gray-900" },
          { label: "Pending Approval", value: pending, color: "text-yellow-600" },
          { label: "Succeeded", value: succeeded, color: "text-green-600" },
          { label: "Failed / Rolled Back", value: failed, color: "text-red-600" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {fixes && fixes.length === 0 ? (
        <p className="text-gray-500">No fixes yet.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Phase</TableHead>
                <TableHead>Finding</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Strategy</TableHead>
                <TableHead>Message</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {fixes?.map((fix) => (
                <TableRow key={fix.ID}>
                  <TableCell>
                    <Link href={`/fixes/${fix.ID}`} className="font-mono text-sm text-blue-600 hover:underline">
                      {fix.ID.slice(0, 8)}...
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge className={phaseColors[fix.Phase] || ""}>{fix.Phase}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate text-sm">{fix.FindingTitle}</TableCell>
                  <TableCell className="text-sm text-gray-600">
                    {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}
                  </TableCell>
                  <TableCell><Badge variant="outline">{fix.Strategy}</Badge></TableCell>
                  <TableCell className="max-w-xs truncate text-sm text-gray-600" title={fix.Message || ""}>
                    {fix.Message || "-"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Create Fix detail page with approve/reject**

Create `dashboard/src/app/fixes/[id]/page.tsx`:

```tsx
"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useFix, approveFix, rejectFix } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800",
  Approved: "bg-blue-100 text-blue-800",
  Applying: "bg-blue-100 text-blue-800",
  Succeeded: "bg-green-100 text-green-800",
  Failed: "bg-red-100 text-red-800",
  RolledBack: "bg-orange-100 text-orange-800",
  DryRunComplete: "bg-purple-100 text-purple-800",
};

export default function FixDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: fix, error, isLoading, mutate } = useFix(id);
  const [acting, setActing] = useState(false);

  if (isLoading) return <p className="text-gray-500">Loading fix...</p>;
  if (error) return <p className="text-red-600">Failed to load fix.</p>;
  if (!fix) return <p className="text-gray-500">Fix not found.</p>;

  async function handleApprove() {
    setActing(true);
    try {
      await approveFix(id, "dashboard-user");
      mutate();
    } catch { /* ignore */ } finally { setActing(false); }
  }

  async function handleReject() {
    setActing(true);
    try {
      await rejectFix(id);
      mutate();
    } catch { /* ignore */ } finally { setActing(false); }
  }

  return (
    <div>
      <Link href="/fixes" className="text-sm text-blue-600 hover:underline">&larr; Back to Fixes</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{fix.ID.slice(0, 8)}</h1>
          <Badge className={phaseColors[fix.Phase] || ""}>{fix.Phase}</Badge>
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4">
          <div><span className="font-medium">Target:</span> {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}</div>
          <div><span className="font-medium">Strategy:</span> {fix.Strategy}</div>
          <div><span className="font-medium">Approval:</span> {fix.ApprovalRequired ? "Required" : "Auto"}</div>
          <div><span className="font-medium">Run:</span>
            <Link href={`/runs/${fix.RunID}`} className="ml-1 text-blue-600 hover:underline">{fix.RunID.slice(0, 8)}</Link>
          </div>
        </div>
        {fix.Message && (
          <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
            fix.Phase === "Failed" || fix.Phase === "RolledBack"
              ? "border-red-200 bg-red-50 text-red-700"
              : fix.Phase === "PendingApproval"
                ? "border-yellow-200 bg-yellow-50 text-yellow-800"
                : "border-gray-200 bg-gray-50 text-gray-700"
          }`}>
            {fix.Message}
          </div>
        )}
      </div>

      {fix.Phase === "PendingApproval" && (
        <div className="mb-6 flex gap-3">
          <Button onClick={handleApprove} disabled={acting}>
            {acting ? "Processing..." : "Approve"}
          </Button>
          <Button variant="outline" onClick={handleReject} disabled={acting}>
            Reject
          </Button>
        </div>
      )}

      <Separator className="mb-6" />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Patch Content</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="outline">{fix.PatchType}</Badge>
            <span className="text-xs text-gray-500">Finding: {fix.FindingTitle}</span>
          </div>
          <pre className="overflow-x-auto rounded-lg bg-gray-900 p-4 text-sm text-gray-100">
            {fix.PatchContent}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 6: Build dashboard to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 7: Commit**

```bash
git add dashboard/src/lib/types.ts dashboard/src/lib/api.ts dashboard/src/app/layout.tsx dashboard/src/app/fixes/
git commit -m "feat(dashboard): add Fixes page with list, detail, approve/reject"
```