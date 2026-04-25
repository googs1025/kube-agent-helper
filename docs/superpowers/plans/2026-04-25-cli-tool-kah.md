# CLI Tool `kah` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #33 - CLI tool `kah`

## Goal

Build a CLI tool `kah` that interacts with the kube-agent-helper HTTP API. Support CRUD operations for diagnostic runs and fixes, plus utility commands for skills, clusters, and status. Output in table, JSON, or YAML format.

## Architecture

```
cmd/kah/main.go --> root command (cobra)
  |-- cmd/kah/client/       HTTP client wrapper
  |-- cmd/kah/formatter/    Output formatting (table/json/yaml)
  |-- run (create|list|get|delete)
  |-- fix (list|get|approve|reject)
  |-- skill list
  |-- cluster list
  |-- status
```

The CLI connects to the controller API via a configurable base URL (`--server` flag or `KAH_SERVER` env var). All commands share a common HTTP client and output formatter.

## Tech Stack

- `github.com/spf13/cobra` for CLI framework
- `github.com/olekukonez/tablewriter` for table output
- `encoding/json` and `sigs.k8s.io/yaml` for structured output
- Standard `net/http` client

## File Map

| File | Status |
|------|--------|
| `cmd/kah/main.go` | New |
| `cmd/kah/root.go` | New |
| `cmd/kah/client/client.go` | New |
| `cmd/kah/formatter/formatter.go` | New |
| `cmd/kah/cmd_run.go` | New |
| `cmd/kah/cmd_fix.go` | New |
| `cmd/kah/cmd_skill.go` | New |
| `cmd/kah/cmd_cluster.go` | New |
| `cmd/kah/cmd_status.go` | New |
| `internal/controller/httpserver/server.go` | Modified |
| `Makefile` | Modified |
| `cmd/kah/cmd_run_test.go` | New |

## Tasks

### Task 1: Add cobra dependency, scaffold main.go + root command

- [ ] `go get github.com/spf13/cobra`
- [ ] Create `cmd/kah/main.go` with `main()` calling `Execute()`
- [ ] Create `cmd/kah/root.go` with root command, persistent flags

**Files:** `cmd/kah/main.go`, `cmd/kah/root.go`

**Steps:**

```go
// cmd/kah/main.go
package main

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

```go
// cmd/kah/root.go
package main

var (
    serverURL string
    outputFmt string
)

var rootCmd = &cobra.Command{
    Use:   "kah",
    Short: "Kube Agent Helper CLI",
    Long:  "CLI tool for interacting with the kube-agent-helper controller API",
}

func init() {
    rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", "", "Controller API URL (env: KAH_SERVER)")
    rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table|json|yaml")

    if serverURL == "" {
        serverURL = os.Getenv("KAH_SERVER")
    }
    if serverURL == "" {
        serverURL = "http://localhost:8080"
    }
}
```

**Test:** `go build ./cmd/kah/ && ./kah --help`

**Commit:** `feat(cli): scaffold kah CLI with cobra root command`

### Task 2: HTTP client wrapper

- [ ] Create `cmd/kah/client/client.go`
- [ ] Methods: `Get`, `Post`, `Delete` with JSON handling
- [ ] Configurable base URL and timeout
- [ ] Error response handling with status codes

**Files:** `cmd/kah/client/client.go`

**Steps:**

```go
package client

type Client struct {
    BaseURL    string
    HTTPClient *http.Client
}

func New(baseURL string) *Client {
    return &Client{
        BaseURL:    strings.TrimRight(baseURL, "/"),
        HTTPClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
    resp, err := c.HTTPClient.Get(c.BaseURL + path)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
    }
    return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) Post(ctx context.Context, path string, body, result interface{}) error { ... }
func (c *Client) Delete(ctx context.Context, path string) error { ... }
```

**Test:** `go test ./cmd/kah/client/ -v`

**Commit:** `feat(cli): add HTTP client wrapper`

### Task 3: Output formatter

- [ ] Create `cmd/kah/formatter/formatter.go`
- [ ] Support table, JSON, and YAML output
- [ ] Table format uses `tablewriter` for aligned columns
- [ ] Generic `Print(format string, headers []string, rows [][]string, data interface{})` function

**Files:** `cmd/kah/formatter/formatter.go`

**Steps:**

```go
package formatter

func Print(format string, headers []string, rows [][]string, data interface{}) error {
    switch format {
    case "json":
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        return enc.Encode(data)
    case "yaml":
        b, err := yaml.Marshal(data)
        if err != nil { return err }
        fmt.Print(string(b))
        return nil
    default: // table
        table := tablewriter.NewWriter(os.Stdout)
        table.SetHeader(headers)
        table.SetBorder(false)
        table.SetAutoWrapText(false)
        table.AppendBulk(rows)
        table.Render()
        return nil
    }
}
```

**Test:** `go test ./cmd/kah/formatter/ -v`

**Commit:** `feat(cli): add table/json/yaml output formatter`

### Task 4: `run` subcommands

- [ ] `kah run list [--namespace NS] [--cluster CL] [--phase PHASE]`
- [ ] `kah run get <run-id>`
- [ ] `kah run create --namespace NS --target-resource RES [--skills s1,s2]`
- [ ] `kah run delete <run-id>`

**Files:** `cmd/kah/cmd_run.go`

**Steps:**

```go
var runCmd = &cobra.Command{Use: "run", Short: "Manage diagnostic runs"}
var runListCmd = &cobra.Command{
    Use: "list", Short: "List diagnostic runs",
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        params := url.Values{}
        if ns, _ := cmd.Flags().GetString("namespace"); ns != "" { params.Set("namespace", ns) }
        if cl, _ := cmd.Flags().GetString("cluster"); cl != "" { params.Set("cluster", cl) }
        if ph, _ := cmd.Flags().GetString("phase"); ph != "" { params.Set("phase", ph) }

        var result store.PaginatedResult[store.DiagnosticRun]
        if err := c.Get(cmd.Context(), "/api/runs?"+params.Encode(), &result); err != nil { return err }

        headers := []string{"ID", "NAMESPACE", "PHASE", "CLUSTER", "CREATED"}
        rows := make([][]string, len(result.Items))
        for i, r := range result.Items {
            rows[i] = []string{r.ID, r.Namespace, r.Phase, r.Cluster, r.CreatedAt}
        }
        return formatter.Print(outputFmt, headers, rows, result)
    },
}

var runGetCmd = &cobra.Command{
    Use: "get <id>", Short: "Get diagnostic run details", Args: cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        var run store.DiagnosticRun
        if err := c.Get(cmd.Context(), "/api/runs/"+args[0], &run); err != nil { return err }
        return formatter.Print(outputFmt, nil, nil, run)
    },
}
```

Register subcommands in `init()`.

**Test:** `go build ./cmd/kah/ && ./kah run list --help`

**Commit:** `feat(cli): add run subcommands (list, get, create, delete)`

### Task 5: `fix` subcommands

- [ ] `kah fix list [--namespace NS] [--status STATUS]`
- [ ] `kah fix get <fix-id>`
- [ ] `kah fix approve <fix-id>`
- [ ] `kah fix reject <fix-id>`

**Files:** `cmd/kah/cmd_fix.go`

**Steps:**

```go
var fixApproveCmd = &cobra.Command{
    Use: "approve <id>", Short: "Approve a diagnostic fix", Args: cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        if err := c.Post(cmd.Context(), "/api/fixes/"+args[0]+"/approve", nil, nil); err != nil {
            return err
        }
        fmt.Printf("Fix %s approved\n", args[0])
        return nil
    },
}

var fixRejectCmd = &cobra.Command{
    Use: "reject <id>", Short: "Reject a diagnostic fix", Args: cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        if err := c.Post(cmd.Context(), "/api/fixes/"+args[0]+"/reject", nil, nil); err != nil {
            return err
        }
        fmt.Printf("Fix %s rejected\n", args[0])
        return nil
    },
}
```

**Test:** `./kah fix list --help && ./kah fix approve --help`

**Commit:** `feat(cli): add fix subcommands (list, get, approve, reject)`

### Task 6: `skill list`, `cluster list`, `status` subcommands

- [ ] `kah skill list` - list registered skills
- [ ] `kah cluster list` - list configured clusters
- [ ] `kah status` - show controller health and summary stats

**Files:** `cmd/kah/cmd_skill.go`, `cmd/kah/cmd_cluster.go`, `cmd/kah/cmd_status.go`

**Steps:**

```go
// cmd_status.go
var statusCmd = &cobra.Command{
    Use: "status", Short: "Show controller status",
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        var status struct {
            Healthy    bool   `json:"healthy"`
            Version    string `json:"version"`
            ActiveRuns int    `json:"active_runs"`
            TotalRuns  int    `json:"total_runs"`
            TotalFixes int    `json:"total_fixes"`
        }
        if err := c.Get(cmd.Context(), "/api/status", &status); err != nil { return err }
        return formatter.Print(outputFmt, nil, nil, status)
    },
}
```

```go
// cmd_skill.go
var skillListCmd = &cobra.Command{
    Use: "list", Short: "List registered skills",
    RunE: func(cmd *cobra.Command, args []string) error {
        c := client.New(serverURL)
        var skills []map[string]interface{}
        if err := c.Get(cmd.Context(), "/api/skills", &skills); err != nil { return err }
        headers := []string{"NAME", "DESCRIPTION", "ENABLED"}
        rows := make([][]string, len(skills))
        for i, s := range skills {
            rows[i] = []string{fmt.Sprint(s["name"]), fmt.Sprint(s["description"]), fmt.Sprint(s["enabled"])}
        }
        return formatter.Print(outputFmt, headers, rows, skills)
    },
}
```

**Test:** `./kah skill list --help && ./kah cluster list --help && ./kah status --help`

**Commit:** `feat(cli): add skill, cluster, and status subcommands`

### Task 7: Add DELETE endpoint to server for `run delete`

- [ ] Add `DELETE /api/runs/{id}` endpoint
- [ ] Add `DeleteRun(ctx, id)` to store interface and implementations
- [ ] Return 204 on success, 404 if not found

**Files:** `internal/controller/httpserver/server.go`, `internal/store/store.go`, `internal/store/sqlite/sqlite.go`

**Steps:**

```go
func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    err := s.store.DeleteRun(r.Context(), id)
    if err != nil {
        if errors.Is(err, store.ErrNotFound) {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

Register: `mux.Delete("/api/runs/{id}", s.handleDeleteRun)`

**Test:** `go test ./internal/controller/httpserver/ -run TestDeleteRun`

**Commit:** `feat(server): add DELETE /api/runs/{id} endpoint`

### Task 8: Makefile `build-kah` target

- [ ] Add `build-kah` target to Makefile
- [ ] Cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- [ ] Include version and commit info via ldflags

**Files:** `Makefile`

**Steps:**

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  ?= $(shell git rev-parse --short HEAD)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build-kah
build-kah:
	go build -ldflags "$(LDFLAGS)" -o bin/kah ./cmd/kah/

.PHONY: build-kah-all
build-kah-all:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kah-linux-amd64 ./cmd/kah/
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kah-linux-arm64 ./cmd/kah/
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kah-darwin-amd64 ./cmd/kah/
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kah-darwin-arm64 ./cmd/kah/
```

**Test:** `make build-kah && ./bin/kah version`

**Commit:** `build: add Makefile targets for kah CLI`

### Task 9: Integration smoke test

- [ ] Start test HTTP server with fakeStore
- [ ] Run CLI commands against it
- [ ] Verify output format and exit codes

**Files:** `cmd/kah/cmd_run_test.go`

**Steps:**

```go
func TestCLISmoke(t *testing.T) {
    // Start test server
    srv := httptest.NewServer(setupTestRouter())
    defer srv.Close()

    tests := []struct {
        args     []string
        wantCode int
        wantOut  string
    }{
        {[]string{"run", "list", "-s", srv.URL}, 0, "ID"},
        {[]string{"status", "-s", srv.URL}, 0, "healthy"},
        {[]string{"skill", "list", "-s", srv.URL}, 0, "NAME"},
        {[]string{"run", "list", "-s", srv.URL, "-o", "json"}, 0, "items"},
    }

    for _, tt := range tests {
        rootCmd.SetArgs(tt.args)
        // capture output and verify
    }
}
```

**Test:** `go test ./cmd/kah/ -run TestCLISmoke -v`

**Commit:** `test(cli): add integration smoke tests`
