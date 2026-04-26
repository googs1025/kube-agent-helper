# Agent Log Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #31 - Stream Agent Pod logs to Dashboard

## Goal

Enable real-time streaming of agent pod logs to the dashboard UI. Agent pods emit structured JSON logs which are persisted to the database and streamed to the frontend via Server-Sent Events (SSE). Users can follow live logs during diagnostic runs and review historical logs after completion.

## Architecture

```
Agent Pod (stdout JSON) --> Controller (k8s log stream) --> SQLite (run_logs table)
                                                        --> SSE endpoint --> Dashboard LogViewer
```

The controller tails agent pod logs via the Kubernetes API, persists them to a `run_logs` table, and exposes an SSE endpoint. The dashboard connects via EventSource and renders logs in a scrollable, color-coded viewer.

## Tech Stack

- Kubernetes `corev1.PodLogOptions{Follow: true}`
- Server-Sent Events (SSE) via `text/event-stream`
- Next.js `EventSource` API
- SQLite migration 006

## File Map

| File | Status |
|------|--------|
| `internal/agent/logging.go` | New |
| `internal/store/migrations/006_run_logs.sql` | New |
| `internal/store/store.go` | Modified |
| `internal/store/sqlite/sqlite.go` | Modified |
| `internal/controller/httpserver/logs_handler.go` | New |
| `internal/controller/httpserver/server.go` | Modified |
| `internal/controller/reconciler/diagnosticrun_reconciler.go` | Modified |
| `dashboard/src/app/api/runs/[id]/logs/route.ts` | New |
| `dashboard/src/components/LogViewer.tsx` | New |
| `dashboard/src/app/runs/[id]/page.tsx` | Modified |
| `cmd/controller/main.go` | Modified |
| `internal/controller/httpserver/logs_handler_test.go` | New |

## Tasks

### Task 1: Structured JSON log output in agent-runtime

- [ ] Create `internal/agent/logging.go` with `EmitEvent()` function
- [ ] Define log entry struct: `{timestamp, run_id, type, message, data}`
- [ ] Types: `step`, `finding`, `fix`, `error`, `info`

**Files:** `internal/agent/logging.go`

**Steps:**

```go
package agent

type LogEntry struct {
    Timestamp string      `json:"timestamp"`
    RunID     string      `json:"run_id"`
    Type      string      `json:"type"`
    Message   string      `json:"message"`
    Data      interface{} `json:"data,omitempty"`
}

func EmitEvent(runID, logType, message string, data interface{}) {
    entry := LogEntry{
        Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        RunID:     runID,
        Type:      logType,
        Message:   message,
        Data:      data,
    }
    json.NewEncoder(os.Stdout).Encode(entry)
}
```

**Test:** `go test ./internal/agent/ -run TestEmitEvent`

**Commit:** `feat(agent): add structured JSON log output`

### Task 2: Database migration 006 - run_logs table

- [ ] Create migration `006_run_logs.sql`
- [ ] Add `AppendRunLog(ctx, RunLog)` and `ListRunLogs(ctx, runID)` to store interface
- [ ] Implement in SQLite store

**Files:** `internal/store/migrations/006_run_logs.sql`, `internal/store/store.go`, `internal/store/sqlite/sqlite.go`

**Steps:**

```sql
-- 006_run_logs.sql
CREATE TABLE IF NOT EXISTS run_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    data TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (run_id) REFERENCES diagnostic_runs(id)
);
CREATE INDEX idx_run_logs_run_id ON run_logs(run_id);
CREATE INDEX idx_run_logs_timestamp ON run_logs(timestamp);
```

Store interface additions:
```go
type RunLog struct {
    ID        int64  `json:"id"`
    RunID     string `json:"run_id"`
    Timestamp string `json:"timestamp"`
    Type      string `json:"type"`
    Message   string `json:"message"`
    Data      string `json:"data,omitempty"`
}

AppendRunLog(ctx context.Context, log RunLog) error
ListRunLogs(ctx context.Context, runID string, afterID int64) ([]RunLog, error)
```

**Test:** `go test ./internal/store/sqlite/ -run TestRunLogs`

**Commit:** `feat(store): add run_logs table migration and store methods`

### Task 3: Backend SSE streaming endpoint

- [ ] Create `GET /api/runs/{id}/logs` handler
- [ ] Support `?follow=true` for SSE streaming
- [ ] Without follow, return JSON array of historical logs
- [ ] With follow, poll database every 500ms for new logs

**Files:** `internal/controller/httpserver/logs_handler.go`, `internal/controller/httpserver/server.go`

**Steps:**

```go
func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
    runID := chi.URLParam(r, "id")
    follow := r.URL.Query().Get("follow") == "true"

    if !follow {
        logs, _ := s.store.ListRunLogs(r.Context(), runID, 0)
        json.NewEncoder(w).Encode(logs)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    flusher := w.(http.Flusher)

    var lastID int64
    for {
        select {
        case <-r.Context().Done():
            return
        case <-time.After(500 * time.Millisecond):
            logs, _ := s.store.ListRunLogs(r.Context(), runID, lastID)
            for _, log := range logs {
                data, _ := json.Marshal(log)
                fmt.Fprintf(w, "data: %s\n\n", data)
                lastID = log.ID
            }
            flusher.Flush()
        }
    }
}
```

Register route: `mux.Get("/api/runs/{id}/logs", s.handleRunLogs)`

**Test:** `curl -N "localhost:8080/api/runs/test-run/logs?follow=true"`

**Commit:** `feat(server): add SSE log streaming endpoint`

### Task 4: Reconciler persists pod logs on run completion

- [ ] In DiagnosticRun reconciler, tail pod logs via Kubernetes clientset
- [ ] Parse JSON lines and call `store.AppendRunLog()` for each
- [ ] Start log collection when run enters Running phase
- [ ] Stop when run reaches terminal phase

**Files:** `internal/controller/reconciler/diagnosticrun_reconciler.go`

**Steps:**

- Use `clientset.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{Follow: true})`
- Launch goroutine to stream and persist logs
- Cancel via context when run completes

**Test:** `go test ./internal/controller/reconciler/ -run TestLogCollection`

**Commit:** `feat(reconciler): collect and persist agent pod logs`

### Task 5: Dashboard API proxy SSE pass-through

- [ ] Create Next.js API route `/api/runs/[id]/logs/route.ts`
- [ ] Proxy to backend, pass through SSE headers
- [ ] Support both JSON (no follow) and SSE (follow) modes

**Files:** `dashboard/src/app/api/runs/[id]/logs/route.ts`

**Steps:**

```typescript
export async function GET(req: Request, { params }: { params: { id: string } }) {
  const { searchParams } = new URL(req.url);
  const follow = searchParams.get('follow');
  const backendUrl = `${process.env.BACKEND_URL}/api/runs/${params.id}/logs?follow=${follow}`;

  if (follow === 'true') {
    const response = await fetch(backendUrl);
    return new Response(response.body, {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
      },
    });
  }

  const response = await fetch(backendUrl);
  const data = await response.json();
  return Response.json(data);
}
```

**Test:** Access `http://localhost:3000/api/runs/test/logs?follow=true` in browser.

**Commit:** `feat(dashboard): add SSE proxy route for log streaming`

### Task 6: Frontend LogViewer component

- [ ] Create `LogViewer.tsx` with auto-scroll, type-based styling
- [ ] Use `EventSource` for live following
- [ ] Color code by type: step=blue, finding=yellow, fix=green, error=red, info=gray
- [ ] Add toggle for auto-scroll and follow mode
- [ ] Integrate into run detail page

**Files:** `dashboard/src/components/LogViewer.tsx`, `dashboard/src/app/runs/[id]/page.tsx`

**Steps:**

```tsx
'use client';
import { useEffect, useRef, useState } from 'react';

interface LogEntry {
  id: number; run_id: string; timestamp: string;
  type: string; message: string; data?: string;
}

const typeColors: Record<string, string> = {
  step: 'text-blue-400', finding: 'text-yellow-400',
  fix: 'text-green-400', error: 'text-red-400', info: 'text-gray-400',
};

export function LogViewer({ runId, follow }: { runId: string; follow: boolean }) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!follow) {
      fetch(`/api/runs/${runId}/logs`).then(r => r.json()).then(setLogs);
      return;
    }
    const es = new EventSource(`/api/runs/${runId}/logs?follow=true`);
    es.onmessage = (e) => {
      const entry = JSON.parse(e.data);
      setLogs(prev => [...prev, entry]);
    };
    return () => es.close();
  }, [runId, follow]);

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs, autoScroll]);

  return (
    <div className="bg-gray-900 rounded-lg p-4 font-mono text-sm max-h-96 overflow-y-auto">
      {logs.map(log => (
        <div key={log.id} className={typeColors[log.type] || 'text-gray-300'}>
          <span className="text-gray-500">{log.timestamp}</span>{' '}
          <span className="font-bold">[{log.type}]</span>{' '}
          {log.message}
        </div>
      ))}
      <div ref={bottomRef} />
    </div>
  );
}
```

**Test:** `cd dashboard && npm test -- --grep LogViewer`

**Commit:** `feat(dashboard): add LogViewer component with SSE streaming`

### Task 7: Wire kubernetes.Clientset into controller main

- [ ] Create Kubernetes clientset in `main.go`
- [ ] Pass to reconciler for pod log access
- [ ] Handle in-cluster and kubeconfig modes

**Files:** `cmd/controller/main.go`

**Steps:**

```go
import "k8s.io/client-go/kubernetes"

config, err := rest.InClusterConfig()
if err != nil {
    config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
}
clientset, err := kubernetes.NewForConfig(config)
```

Pass `clientset` to `NewDiagnosticRunReconciler(...)`.

**Test:** `go build ./cmd/controller/`

**Commit:** `feat(main): wire kubernetes clientset for pod log access`

### Task 8: Integration testing

- [ ] Test SSE endpoint with mock store
- [ ] Test log persistence round-trip
- [ ] Test EventSource connection lifecycle
- [ ] Test LogViewer rendering with sample data

**Files:** `internal/controller/httpserver/logs_handler_test.go`

**Steps:**

- Create httptest server with logs handler
- POST sample logs, then GET with follow=false, assert returned
- Test SSE: connect with follow=true, insert logs, verify events received
- Test context cancellation terminates SSE stream

**Test:** `go test ./internal/controller/httpserver/ -run TestLogs -v`

**Commit:** `test(logs): add integration tests for log streaming`
