# Critical-Path Test Coverage — Design

**Date:** 2026-05-01
**Author:** Claude (with @googs1025)
**Status:** Approved, pending implementation plan

## Goal

Raise unit-test coverage on the **8 highest-risk packages** of `kube-agent-helper`
to **≥80%** each, prioritising business-critical and bug-prone code over
"easy" boilerplate. CI thresholds are not modified by this work — they may be
revisited after merge based on the new baseline.

## Non-Goals

- Touching CLI entrypoints (`cmd/...`, `cmd/kah/cmd/*`) — they are 0% but low
  business risk and high test cost.
- Touching CRD generated code (`internal/controller/api/v1alpha1`) — generated.
- Refactoring production code beyond the **3 explicitly-listed minor changes**
  required to make tests possible (see Risks).
- Raising coverage of already-strong packages
  (`audit` 97.5%, `registry` 95%, `sanitize` 93%, `trimmer` 86%, `translator` 85%).

## Baseline (snapshot, 2026-05-01)

| # | Package | Before | Target | Lang |
|---|---|---|---|---|
| 1 | `internal/agent` | 0.0% | ≥80% | Go |
| 2 | `internal/k8sclient` | 34.3% | ≥80% | Go |
| 3 | `internal/store/sqlite` | 44.0% | ≥80% | Go |
| 4 | `internal/collector` | 19.8% | ≥80% | Go |
| 5 | `internal/controller/httpserver` | 58.9% | ≥80% | Go |
| 6 | `internal/controller/reconciler` | 49.1% | ≥80% | Go |
| 7 | `agent-runtime/runtime/tracer.py` | 53% | ≥80% | Python |
| 8 | `agent-runtime/runtime/fix_main.py` | 0% | ≥80% | Python |

## Workflow

- New branch off `main`: `test/critical-path-coverage`.
- 8 **test commits** on that branch, one per target package, in the order above
  (bottom-up: dependencies first, then their consumers). Up to 2 small
  `refactor(...)` commits may be inserted before specific test commits to add
  test seams (see Risks). Total commit count: 8–10.
- Single PR into `main`.
- Existing test style is preserved:
  - Go: `fake.NewClientBuilder()` (controller-runtime), `fake.NewSimpleClientset()`
    (client-go), real SQLite tmpfile, `httptest`, table-driven tests.
  - Python: `pytest` + `unittest.mock` + `monkeypatch`. No new test deps.

## Per-Commit Strategy

### [1] `test(agent): cover EmitEvent log emission`

Only `EmitEvent` is uncovered (writes structured JSON to stdout).

- Pipe `os.Stdout`, call `EmitEvent`, decode back into `LogEntry`, assert fields.
- Cases: happy path with non-nil `Data`, `nil` data omits the field, RFC3339Nano
  timestamp format, each `LogType*` constant value used.
- ~3-4 cases.

### [2] `test(k8sclient): cover Build, mapper.{NewMapper,ResolveGVR,Discovery}`

- `Build`: error path with malformed kubeconfig; success path with a fake
  `*rest.Config`. Skip the in-cluster branch.
- `mapper.ResolveGVR`: table-driven over GVK→GVR resolution using
  `fake.NewSimpleClientset()`'s discovery; cover `unknown kind` error.
- ~5-6 cases.

### [3] `test(sqlite): cover paginated/notification/log/batch ops`

- `setupTestStore(t)` (existing helper) + tmpfile.
- `AppendRunLog`/`ListRunLogs`: ordering, tail limit.
- `*Paginated` (`ListRunsPaginated`, `ListFixesPaginated`, `ListEventsPaginated`):
  multi-page, sort-column whitelist via `sanitizeSortOrder`, page out-of-range.
- `sanitizeSortOrder`: table-driven, including injection attempts and
  ASC/DESC normalisation.
- `NotificationConfig` 5 CRUDs: create→get→list→update→delete lifecycle plus
  Get/Update/Delete-on-missing.
- `DeleteRuns` (cascade), `BatchUpdateFixPhase` (multi-id update).
- `ListSkills` (currently 0%).
- ~15-20 cases.

### [4] `test(collector): cover watch/scrape/purge loops`

- `DefaultConfig`, `NeedLeaderElection`: single-line asserts.
- `watchEvents`: `fake.NewSimpleClientset()` event watcher; inject an Event,
  assert sanitised version reaches the store; `ctx.Cancel` exits cleanly.
- `scrapeAll`: fake `metrics.k8s.io` clientset; inject `PodMetrics`, assert
  `InsertMetricSnapshot` called with correct fields.
- `runPurge`: stub `Store` counting `PurgeOldEvents`/`PurgeOldMetrics`;
  `ctx.Cancel` exits.
- `Start`: only that startup→cancel→exit does not panic; inner loop body is
  exercised via the helper-level tests above.
- ~10-12 cases.

### [5] `test(httpserver): cover 0% handlers and weak branches`

- 0% handlers to cover end-to-end: `handleAPIModelConfigs`,
  `handleAPINotificationConfigs`, `handleAPINotificationConfigDetail`,
  `reloadNotificationChannels`, `runFromK8s`, `WithNotifier`/
  `WithNotificationManager`.
- Weak handlers — add error branches (parse failure, k8s `NotFound`, missing
  namespace, partial-failure batches): `handleAPIK8sResources` (53%),
  `handleK8sResource` (61%), `handleAPIRunCRD` (45%), `handleAPIFindingAction`
  (55%), `handleAPIFixesBatchReject` (47%), `handleAPIFixDetail` (57%),
  `handleInternal` (55%), `findFixCRByStoreID` (42%), `streamLogsFromPod` (36%).
- `parsePodLogLine` (53%): table-driven across all formats.
- Use `httptest.NewRecorder()` + fake k8s client + tmpfile sqlite.
- A small `newTestServer(t, opts...)` factory may be added in a `_test.go`
  file if the existing helper proves insufficient.
- Skip `Start` (real listener).
- ~25-30 cases.

### [6] `test(reconciler): cover fix patch/rollback, scheduled_run, run.collectPodLogs`

- `fix_reconciler.applyPatch`/`rollback`: build a Fix CR and target Deployment
  with `fake.NewClientBuilder()`; assert post-patch fields and rollback restore.
- `fix_reconciler.kindToGVK`: table-driven.
- `scheduled_run_reconciler.Reconcile`: requires a clock injection seam — see
  Risks. Cases: trigger child Run on schedule, `enforceHistoryLimit` deletes
  old runs, `appendUnique`/`removeFromSlice` direct unit tests.
- `run_reconciler.collectPodLogs` (11%): fake clientset + fake corev1 pod logs
  stream (existing `ParsePodLogStream` is already 88% covered, reuse it).
- `run_reconciler.completeRun` (56%) and `podWaitingReason` (53%): cover each
  pod phase / waiting reason branch.
- Skip the 6 `SetupWithManager` calls.
- ~25-30 cases.

### [7] `test(tracer.py): cover error paths and disabled state`

- Inspect `coverage report -m` for `tracer.py` missing line numbers.
- Add cases for: exception inside traced block, context-manager exit on error,
  no-op when tracing disabled, long-trace truncation.
- ~5-8 cases.

### [8] `test(fix_main.py): cover apply/rollback/CLI`

- New `tests/test_fix_main.py`. Mock `kubernetes.client` and the LLM client.
- Cases: success path (Job→patch→apply), apply failure → rollback,
  reporter callback invoked, `argparse` errors on bad CLI input,
  `KUBECONFIG` env var resolution.
- ~8-10 cases.

## Exemptions (Out of Scope of the 80%)

The following kinds of code are explicitly **not required to be tested**.
Note that `go test -coverprofile` reports coverage over **all** statements,
so heavily-exempted packages may not be able to reach exactly 80% even when
all reasonable code is covered. Operating rule: **aim for 80% reported
coverage; if exempted code blocks make 80% unreachable, the package may land
at up to 5 points below 80%, provided every non-exempt function is covered.**
Exempted categories:

- `SetupWithManager(mgr ctrl.Manager)` — needs a real manager; low value.
- `Start(ctx)` long-running goroutines — only that startup/exit doesn't panic
  is tested; inner loop body is split into helpers tested separately.
- Generated code (`MarshalJSON` boilerplate, `zz_generated_*.go`).
- `main()` entrypoints (none in scope here anyway).

## Verification (per commit)

```bash
# Go
go test -race -count=1 -coverprofile=/tmp/before.out ./<pkg>
# ... write tests ...
go test -race -count=1 -coverprofile=/tmp/after.out ./<pkg>
go tool cover -func=/tmp/after.out | tail -1   # confirm ≥ 80%
go vet ./<pkg>
golangci-lint run ./<pkg>
go test -race -count=1 ./...                   # full regression

# Python
pytest agent-runtime/tests/ --cov=agent-runtime/runtime/<x> \
    --cov-report=term-missing
pytest agent-runtime/tests/                    # full regression
```

Each commit must:
- Reach ≥80% on the target package.
- Pass full `go test ./...` and full `pytest`.
- Add no new `go vet` / `golangci-lint` warnings.
- Modify **no business code** (see Risks for the 3 exceptions).

## Commit Message Format

Existing convention is preserved:

```
test(<pkg>): cover <area>

- <case 1>
- <case 2>
- ...

Coverage: X.X% → Y.Y%
```

If a test exposes a real bug, it goes in a separate `fix(<pkg>): ...` commit
**inserted before** the test commit. Test commits never bundle production fixes.

## Risks & Required Production-Code Changes

3 minor production changes are accepted as separate refactor commits inserted
before the relevant test commit. They are pure seam additions (no behaviour
change):

1. **Clock injection in `scheduled_run_reconciler`** —
   `refactor(reconciler): inject clock seam for scheduled_run`. Adds a `now
   func() time.Time` field, default `time.Now`. Inserted before commit [6].
2. **Optional `metrics.k8s.io` lister extraction in `collector`** —
   only if the `metrics/pkg/client/clientset/versioned/fake` fake proves
   insufficient. Inserted before commit [4]. Wraps the lister so tests can
   inject `PodMetrics`.
3. **Test helper `newTestServer` in `httpserver` `_test.go`** — *not* production
   code, just a test file. Listed here for visibility because it expands the
   testing surface. Inserted as part of commit [5].

If any of these turn out to need more invasive surgery than expected, the
relevant commit is reduced in scope (e.g. `httpserver` lands at 75% instead
of 80%) rather than extending the production-code change.

## Deliverables

- This design (`docs/superpowers/specs/2026-05-01-critical-path-test-coverage-design.md`).
- Implementation plan (`docs/superpowers/plans/2026-05-01-critical-path-test-coverage.md`)
  produced by the writing-plans skill after this design is approved.
- Branch `test/critical-path-coverage` off `main`, 8 commits, single PR.
- Net result: each of the 8 target packages ≥80%, total ~100 new test cases,
  zero new dependencies.

## Open Questions

None. Design approved 2026-05-01.