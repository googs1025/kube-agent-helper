# Dashboard Interaction Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #32 - Dashboard interaction improvements

## Goal

Add pagination, filtering, sorting, and batch operations to all dashboard list views (Runs, Fixes, Events). Provide a consistent paginated API envelope, reusable UI components, and batch approve/reject for fixes.

## Architecture

```
Frontend (useTableState) --> API Routes --> Backend HTTP Handlers --> Store (LIMIT/OFFSET + COUNT)
                                                                         |
FilterBar / Pagination / BatchToolbar <--- PaginatedResult <--- ListRunsOpts/ListFixesOpts
```

The store layer accepts option structs with pagination, filtering, and sorting parameters. The HTTP layer wraps results in a `{items, total, page, pageSize}` envelope. The frontend uses a `useTableState` hook to manage state across all list pages.

## Tech Stack

- Go SQL: `COUNT(*) OVER()`, `LIMIT/OFFSET`
- React hooks for table state management
- Tailwind CSS for UI components
- i18n via existing translation framework

## File Map

| File | Status |
|------|--------|
| `internal/store/store.go` | Modified |
| `internal/store/sqlite/sqlite.go` | Modified |
| `internal/store/sqlite/sqlite_test.go` | Modified |
| `internal/store/fakestore/fakestore.go` | Modified |
| `internal/controller/httpserver/server.go` | Modified |
| `internal/controller/httpserver/server_test.go` | Modified |
| `dashboard/src/types/api.ts` | Modified |
| `dashboard/src/hooks/useTableState.ts` | New |
| `dashboard/src/hooks/useApi.ts` | Modified |
| `dashboard/src/components/Pagination.tsx` | New |
| `dashboard/src/components/FilterBar.tsx` | New |
| `dashboard/src/components/BatchToolbar.tsx` | New |
| `dashboard/src/app/runs/page.tsx` | Modified |
| `dashboard/src/app/fixes/page.tsx` | Modified |
| `dashboard/src/app/events/page.tsx` | Modified |
| `dashboard/src/i18n/en.json` | Modified |
| `dashboard/src/i18n/zh.json` | Modified |

## Tasks

### Task 1: Extend store interface with pagination

- [ ] Define `ListOpts` struct with Page, PageSize, SortBy, SortOrder, Filters
- [ ] Define `PaginatedResult[T]` generic struct
- [ ] Update `ListRuns`, `ListFixes`, `ListEvents` signatures

**Files:** `internal/store/store.go`

**Steps:**

```go
type ListOpts struct {
    Page      int               `json:"page"`
    PageSize  int               `json:"pageSize"`
    SortBy    string            `json:"sortBy"`
    SortOrder string            `json:"sortOrder"` // "asc" or "desc"
    Filters   map[string]string `json:"filters"`
}

type PaginatedResult[T any] struct {
    Items    []T `json:"items"`
    Total    int `json:"total"`
    Page     int `json:"page"`
    PageSize int `json:"pageSize"`
}

func DefaultListOpts() ListOpts {
    return ListOpts{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}
}
```

Update interface:
```go
ListRuns(ctx context.Context, opts ListOpts) (PaginatedResult[DiagnosticRun], error)
ListFixes(ctx context.Context, opts ListOpts) (PaginatedResult[DiagnosticFix], error)
ListEvents(ctx context.Context, opts ListOpts) (PaginatedResult[Event], error)
```

**Test:** `go build ./internal/store/...`

**Commit:** `feat(store): add paginated list options and result types`

### Task 2: SQLite implementation

- [ ] Implement COUNT + LIMIT/OFFSET queries for all list methods
- [ ] Support filter keys: `namespace`, `cluster`, `phase`, `severity`, `status`
- [ ] Support sort by: `created_at`, `updated_at`, `name`, `severity`
- [ ] Sanitize sort column names against allowlist

**Files:** `internal/store/sqlite/sqlite.go`, `internal/store/sqlite/sqlite_test.go`

**Steps:**

```go
func (s *SQLiteStore) ListRuns(ctx context.Context, opts store.ListOpts) (store.PaginatedResult[store.DiagnosticRun], error) {
    allowedSort := map[string]bool{"created_at": true, "updated_at": true, "name": true}
    if !allowedSort[opts.SortBy] {
        opts.SortBy = "created_at"
    }

    where, args := buildWhereClause(opts.Filters)
    countQuery := "SELECT COUNT(*) FROM diagnostic_runs" + where
    var total int
    s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

    query := fmt.Sprintf("SELECT * FROM diagnostic_runs %s ORDER BY %s %s LIMIT ? OFFSET ?",
        where, opts.SortBy, opts.SortOrder)
    args = append(args, opts.PageSize, (opts.Page-1)*opts.PageSize)
    // execute and scan rows...

    return store.PaginatedResult[store.DiagnosticRun]{
        Items: runs, Total: total, Page: opts.Page, PageSize: opts.PageSize,
    }, nil
}
```

**Test:** `go test ./internal/store/sqlite/ -run TestPagination -v`

**Commit:** `feat(sqlite): implement paginated queries with filtering and sorting`

### Task 3: Update HTTP handlers for paginated envelope

- [ ] Parse query params: `page`, `pageSize`, `sortBy`, `sortOrder`, filter params
- [ ] Return `{items, total, page, pageSize}` JSON envelope
- [ ] Default page=1, pageSize=20
- [ ] Cap pageSize at 100

**Files:** `internal/controller/httpserver/server.go`

**Steps:**

```go
func parseListOpts(r *http.Request) store.ListOpts {
    opts := store.DefaultListOpts()
    if p := r.URL.Query().Get("page"); p != "" {
        opts.Page, _ = strconv.Atoi(p)
    }
    if ps := r.URL.Query().Get("pageSize"); ps != "" {
        opts.PageSize, _ = strconv.Atoi(ps)
        if opts.PageSize > 100 { opts.PageSize = 100 }
    }
    if sb := r.URL.Query().Get("sortBy"); sb != "" { opts.SortBy = sb }
    if so := r.URL.Query().Get("sortOrder"); so != "" { opts.SortOrder = so }
    opts.Filters = map[string]string{}
    for _, key := range []string{"namespace", "cluster", "phase", "severity", "status"} {
        if v := r.URL.Query().Get(key); v != "" { opts.Filters[key] = v }
    }
    return opts
}
```

**Test:** `go test ./internal/controller/httpserver/ -run TestPaginatedHandlers`

**Commit:** `feat(server): return paginated envelope from list endpoints`

### Task 4: Update fakeStore

- [ ] Implement pagination logic in fakeStore for all list methods
- [ ] Support basic filtering and sorting for test scenarios

**Files:** `internal/store/fakestore/fakestore.go`

**Steps:**

- Apply filters by iterating and matching
- Sort using `sort.Slice`
- Compute offset/limit slice
- Return `PaginatedResult` with correct total

**Test:** `go test ./internal/store/fakestore/...`

**Commit:** `feat(fakestore): implement paginated list methods`

### Task 5: Frontend types and API hooks

- [ ] Define `PaginatedResult<T>` TypeScript type
- [ ] Update API hooks to accept pagination/filter params
- [ ] Return total count alongside items

**Files:** `dashboard/src/types/api.ts`, `dashboard/src/hooks/useApi.ts`

**Steps:**

```typescript
// types/api.ts
export interface PaginatedResult<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface ListParams {
  page?: number;
  pageSize?: number;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  [key: string]: string | number | undefined;
}
```

```typescript
// hooks/useApi.ts
export function useListRuns(params: ListParams) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => { if (v !== undefined) query.set(k, String(v)); });
  return useSWR<PaginatedResult<Run>>(`/api/runs?${query.toString()}`);
}
```

**Test:** `cd dashboard && npm run type-check`

**Commit:** `feat(dashboard): add paginated API types and hooks`

### Task 6: Create reusable UI components

- [ ] `Pagination.tsx` - page navigation with page numbers, prev/next
- [ ] `FilterBar.tsx` - filter inputs for namespace, cluster, status/phase/severity
- [ ] `BatchToolbar.tsx` - selected count, batch action buttons
- [ ] `useTableState.ts` - hook managing page, filters, sort, selection

**Files:** `dashboard/src/components/Pagination.tsx`, `dashboard/src/components/FilterBar.tsx`, `dashboard/src/components/BatchToolbar.tsx`, `dashboard/src/hooks/useTableState.ts`

**Steps:**

```typescript
// hooks/useTableState.ts
export function useTableState(defaults?: Partial<ListParams>) {
  const [page, setPage] = useState(defaults?.page ?? 1);
  const [pageSize, setPageSize] = useState(defaults?.pageSize ?? 20);
  const [sortBy, setSortBy] = useState(defaults?.sortBy ?? 'created_at');
  const [sortOrder, setSortOrder] = useState<'asc'|'desc'>(defaults?.sortOrder ?? 'desc');
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const params: ListParams = { page, pageSize, sortBy, sortOrder, ...filters };
  const toggleSelect = (id: string) => { ... };
  const selectAll = (ids: string[]) => { ... };
  const clearSelection = () => setSelected(new Set());

  return { page, setPage, pageSize, setPageSize, sortBy, setSortBy,
           sortOrder, setSortOrder, filters, setFilters, selected,
           toggleSelect, selectAll, clearSelection, params };
}
```

```tsx
// Pagination.tsx
export function Pagination({ page, pageSize, total, onPageChange }: Props) {
  const totalPages = Math.ceil(total / pageSize);
  return (
    <nav className="flex items-center gap-2">
      <button disabled={page <= 1} onClick={() => onPageChange(page - 1)}>Previous</button>
      <span>{page} / {totalPages}</span>
      <button disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>Next</button>
    </nav>
  );
}
```

**Test:** `cd dashboard && npm test -- --grep "Pagination|FilterBar|BatchToolbar"`

**Commit:** `feat(dashboard): add reusable Pagination, FilterBar, BatchToolbar components`

### Task 7: Integrate into Runs page

- [ ] Use `useTableState` hook
- [ ] Add FilterBar with namespace, cluster, phase filters
- [ ] Add Pagination component
- [ ] Add sortable column headers

**Files:** `dashboard/src/app/runs/page.tsx`

**Steps:**

- Replace direct API call with `useListRuns(tableState.params)`
- Render FilterBar above table
- Render Pagination below table
- Add click handlers on column headers to toggle sort

**Test:** Manual testing + `cd dashboard && npm test -- --grep RunsPage`

**Commit:** `feat(dashboard): integrate pagination and filters into runs page`

### Task 8: Integrate into Fixes page with batch approve/reject

- [ ] Use `useTableState` hook with selection support
- [ ] Add BatchToolbar with Approve/Reject buttons
- [ ] Implement batch API calls for selected fixes
- [ ] Add FilterBar with namespace, cluster, status filters

**Files:** `dashboard/src/app/fixes/page.tsx`

**Steps:**

- Add checkbox column to table rows
- Show BatchToolbar when `selected.size > 0`
- On batch approve: `POST /api/fixes/batch` with `{ids: [...], action: "approve"}`
- Refresh data after batch action

**Test:** `cd dashboard && npm test -- --grep FixesPage`

**Commit:** `feat(dashboard): integrate pagination, filters, batch actions into fixes page`

### Task 9: Integrate into Events page

- [ ] Use `useTableState` hook
- [ ] Add FilterBar with namespace, cluster, reason filters
- [ ] Add Pagination component

**Files:** `dashboard/src/app/events/page.tsx`

**Steps:**

- Same pattern as Runs page
- Filter options: namespace, cluster, reason

**Test:** `cd dashboard && npm test -- --grep EventsPage`

**Commit:** `feat(dashboard): integrate pagination and filters into events page`

### Task 10: i18n keys

- [ ] Add translation keys for all new UI elements
- [ ] Cover: pagination labels, filter labels, batch action labels, empty states

**Files:** `dashboard/src/i18n/en.json`, `dashboard/src/i18n/zh.json`

**Steps:**

English keys:
```json
{
  "pagination.previous": "Previous",
  "pagination.next": "Next",
  "pagination.showing": "Showing {start}-{end} of {total}",
  "filter.namespace": "Namespace",
  "filter.cluster": "Cluster",
  "filter.phase": "Phase",
  "filter.status": "Status",
  "filter.severity": "Severity",
  "filter.clear": "Clear Filters",
  "batch.selected": "{count} selected",
  "batch.approve": "Approve Selected",
  "batch.reject": "Reject Selected",
  "batch.confirm": "Are you sure?",
  "table.empty": "No results found",
  "table.sortAsc": "Sort ascending",
  "table.sortDesc": "Sort descending"
}
```

Add corresponding Chinese translations to `zh.json`.

**Test:** Verify all keys are used in components (no orphan keys).

**Commit:** `feat(i18n): add pagination, filter, and batch action translations`

### Task 11: E2E integration test

- [ ] Test paginated API response structure
- [ ] Test filter parameters produce correct results
- [ ] Test sort ordering
- [ ] Test batch approve/reject endpoint

**Files:** `internal/controller/httpserver/server_test.go`

**Steps:**

- Seed store with 50 runs across 3 namespaces
- GET `/api/runs?page=2&pageSize=10` - assert 10 items, total=50, page=2
- GET `/api/runs?namespace=default` - assert filtered count
- GET `/api/runs?sortBy=name&sortOrder=asc` - assert ordering
- POST `/api/fixes/batch` with approve action - assert all updated

**Test:** `go test ./internal/controller/httpserver/ -run TestPaginatedE2E -v`

**Commit:** `test(e2e): add pagination, filtering, and batch operation tests`
