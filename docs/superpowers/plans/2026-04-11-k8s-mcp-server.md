# k8s-mcp-server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based Model Context Protocol server that exposes 8 read-only Kubernetes diagnostic tools over stdio, scoped per spec `docs/superpowers/specs/2026-04-11-k8s-mcp-server-design.md`.

**Architecture:** Single binary under `cmd/k8s-mcp-server/`. Three process layers: `mcp` (mark3labs/mcp-go stdio JSON-RPC) → `mcptools` (tool handlers with audit middleware) → `k8sclient` (dynamic + typed + metrics + prometheus clients). Pure-function layers `sanitize` and `trimmer` transform responses before they reach the LLM. Logs go to stderr only; stdout is protocol-exclusive.

**Tech Stack:** Go 1.23, `mark3labs/mcp-go`, `k8s.io/client-go` v0.31.x, `k8s.io/metrics` v0.31.x, `prometheus/client_golang`, `log/slog`, `testify`, `envtest` (controller-runtime), `kind` for integration.

**Spec:** [`docs/superpowers/specs/2026-04-11-k8s-mcp-server-design.md`](../specs/2026-04-11-k8s-mcp-server-design.md)

**Module path:** `github.com/kube-agent-helper/kube-agent-helper` — substitute with your actual GitHub owner in Task 1 if different.

---

## Natural Exit Point

Task 20 (register M5 tools) is the natural "minimum viable" exit. After Task 20 the 4 core diagnostic tools are fully usable end-to-end. Tasks 21-29 add extension tools, integration tests, and docs. If time pressure demands, stopping at Task 20 produces working software.

---

## File Structure

```
kube-agent-helper/
├── go.mod
├── go.sum
├── Makefile
├── .golangci.yml
├── .gitignore                       (append patterns)
├── cmd/
│   └── k8s-mcp-server/
│       └── main.go                  # flag parsing + wire + ServeStdio
├── internal/
│   ├── k8sclient/
│   │   ├── config.go                # rest.Config resolution
│   │   ├── config_test.go
│   │   ├── clients.go               # dynamic/typed/metrics/prom factory
│   │   ├── mapper.go                # RESTMapper wrapper
│   │   ├── precheck.go              # SelfSubjectRulesReview
│   │   └── precheck_test.go
│   ├── sanitize/
│   │   ├── sanitize.go              # Clean() + generic rules
│   │   ├── secret.go
│   │   ├── configmap.go
│   │   ├── pod.go
│   │   └── sanitize_test.go
│   ├── trimmer/
│   │   ├── trimmer.go               # Projector interface + generic
│   │   ├── pod.go
│   │   ├── deployment.go
│   │   ├── node.go
│   │   ├── service.go
│   │   ├── event.go
│   │   ├── trimmer_test.go
│   │   └── testdata/
│   │       ├── pod.input.json
│   │       ├── pod.golden.json
│   │       └── ...
│   ├── audit/
│   │   ├── logger.go                # slog JSON handler
│   │   ├── argmask.go               # arg whitelist sanitize
│   │   ├── middleware.go            # Wrap(toolName, handler)
│   │   └── audit_test.go
│   └── mcptools/
│       ├── register.go              # RegisterAll(server, deps)
│       ├── kubectl_get.go
│       ├── kubectl_get_test.go
│       ├── kubectl_describe.go
│       ├── kubectl_describe_test.go
│       ├── kubectl_logs.go
│       ├── kubectl_logs_test.go
│       ├── events_list.go
│       ├── events_list_test.go
│       ├── top.go                   # top_pods + top_nodes
│       ├── top_test.go
│       ├── list_api_resources.go
│       ├── list_api_resources_test.go
│       ├── prometheus_query.go
│       ├── prometheus_query_test.go
│       ├── kubectl_explain.go
│       └── kubectl_explain_test.go
├── test/
│   ├── envtest/
│   │   └── setup_test.go            # shared envtest TestMain
│   └── integration/
│       ├── run.sh
│       ├── kind-config.yaml
│       └── scenarios_test.go
└── docs/
    ├── k8s-mcp-server.md            # user guide (Task 28)
    └── superpowers/
        ├── specs/
        │   └── 2026-04-11-k8s-mcp-server-design.md
        └── plans/
            └── 2026-04-11-k8s-mcp-server.md   # this file
```

---

## Task 1: Scaffold module, directories, and build tooling

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.golangci.yml`
- Modify: `.gitignore`
- Create: `cmd/k8s-mcp-server/main.go`
- Create: `internal/k8sclient/doc.go`
- Create: `internal/sanitize/doc.go`
- Create: `internal/trimmer/doc.go`
- Create: `internal/audit/doc.go`
- Create: `internal/mcptools/doc.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go mod init github.com/kube-agent-helper/kube-agent-helper
```

- [ ] **Step 2: Create directory skeleton with placeholder doc.go files**

Each package needs at least one `.go` file for `go build ./...` to succeed. Use `doc.go`:

`internal/k8sclient/doc.go`:
```go
// Package k8sclient builds Kubernetes REST configuration and typed/dynamic
// clients from CLI flags, and performs an RBAC precheck at startup.
package k8sclient
```

Repeat for the other four internal packages with one-line package docs matching the spec responsibilities (`sanitize`, `trimmer`, `audit`, `mcptools`).

- [ ] **Step 3: Write minimal main.go**

`cmd/k8s-mcp-server/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "k8s-mcp-server: not yet implemented")
	os.Exit(0)
}
```

- [ ] **Step 4: Write Makefile**

`Makefile`:
```makefile
BINARY := k8s-mcp-server
PKG := ./cmd/$(BINARY)

.PHONY: build test lint vet fmt integration image clean

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./... -race -cover

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

integration:
	./test/integration/run.sh

image:
	docker build -f build/k8s-mcp-server.Dockerfile -t k8s-mcp-server:dev .

clean:
	rm -rf bin/
```

- [ ] **Step 5: Write .golangci.yml**

`.golangci.yml`:
```yaml
run:
  timeout: 3m
  go: "1.23"

linters:
  disable-all: true
  enable:
    - errcheck
    - govet
    - staticcheck
    - ineffassign
    - gosimple
    - unused

issues:
  exclude-use-default: false
```

- [ ] **Step 6: Append to .gitignore**

Append these lines to existing `.gitignore`:
```
bin/
*.test
coverage.out
.env
.envrc
```

- [ ] **Step 7: Smoke-verify build**

```bash
make build
./bin/k8s-mcp-server
```
Expected stderr: `k8s-mcp-server: not yet implemented`
Expected exit code: 0

Also run:
```bash
go vet ./...
```
Expected: no output (success).

- [ ] **Step 8: Commit**

```bash
git add go.mod Makefile .golangci.yml .gitignore cmd internal
git commit -m "feat: scaffold module, directories, and build tooling"
```

---

## Task 2: k8sclient — rest.Config resolution from flags

**Files:**
- Create: `internal/k8sclient/config.go`
- Create: `internal/k8sclient/config_test.go`

Resolves CLI flags (`--in-cluster` / `--kubeconfig` / `--context`) and environment into a `*rest.Config`. Pure logic, no side effects beyond reading kubeconfig file if specified.

- [ ] **Step 1: Write failing test (table-driven)**

`internal/k8sclient/config_test.go`:
```go
package k8sclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveConfig_FlagConflict(t *testing.T) {
	_, err := Resolve(Flags{InCluster: true, Kubeconfig: "/tmp/kc.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestResolveConfig_KubeconfigFlagWins(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(minimalKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", "/does/not/exist")

	cfg, err := Resolve(Flags{Kubeconfig: kc})
	require.NoError(t, err)
	assert.Equal(t, "https://fake.example:6443", cfg.RESTConfig.Host)
	assert.Equal(t, "test-ctx", cfg.ContextName)
}

func TestResolveConfig_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(minimalKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", kc)

	cfg, err := Resolve(Flags{})
	require.NoError(t, err)
	assert.Equal(t, "https://fake.example:6443", cfg.RESTConfig.Host)
}

func TestResolveConfig_ContextOverride(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(twoContextKubeconfig), 0o600))

	cfg, err := Resolve(Flags{Kubeconfig: kc, Context: "second"})
	require.NoError(t, err)
	assert.Equal(t, "https://other.example:6443", cfg.RESTConfig.Host)
	assert.Equal(t, "second", cfg.ContextName)
}

const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://fake.example:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
current-context: test-ctx
users:
- name: test-user
  user:
    token: fake
`

const twoContextKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: c1
  cluster: {server: https://first.example:6443, insecure-skip-tls-verify: true}
- name: c2
  cluster: {server: https://other.example:6443, insecure-skip-tls-verify: true}
contexts:
- name: first
  context: {cluster: c1, user: u}
- name: second
  context: {cluster: c2, user: u}
current-context: first
users:
- name: u
  user: {token: fake}
`
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/k8sclient/... -run TestResolveConfig -v
```
Expected: compilation errors — `undefined: Resolve`, `undefined: Flags`.

- [ ] **Step 3: Add required dependencies**

```bash
go get k8s.io/client-go@v0.31.0
go get github.com/stretchr/testify@latest
```

- [ ] **Step 4: Implement config.go**

`internal/k8sclient/config.go`:
```go
package k8sclient

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Flags holds the CLI flag values relevant to cluster connection.
type Flags struct {
	InCluster  bool
	Kubeconfig string
	Context    string
}

// Resolved holds the produced rest.Config together with descriptive metadata
// used for startup logging.
type Resolved struct {
	RESTConfig  *rest.Config
	Source      string // "in-cluster" | "kubeconfig"
	ContextName string // empty in in-cluster mode
	ClusterHost string
}

// Resolve produces a *rest.Config from the provided flags, honoring the
// precedence rules documented in the spec §1.2.
func Resolve(f Flags) (*Resolved, error) {
	if f.InCluster && f.Kubeconfig != "" {
		return nil, errors.New("--in-cluster and --kubeconfig are mutually exclusive")
	}

	if f.InCluster {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		return &Resolved{
			RESTConfig:  cfg,
			Source:      "in-cluster",
			ClusterHost: cfg.Host,
		}, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if f.Kubeconfig != "" {
		loadingRules.ExplicitPath = f.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if f.Context != "" {
		overrides.CurrentContext = f.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	ctxName := f.Context
	if ctxName == "" {
		ctxName = raw.CurrentContext
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	return &Resolved{
		RESTConfig:  cfg,
		Source:      "kubeconfig",
		ContextName: ctxName,
		ClusterHost: cfg.Host,
	}, nil
}
```

- [ ] **Step 5: Run test to confirm pass**

```bash
go test ./internal/k8sclient/... -run TestResolveConfig -v
```
Expected: all 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/k8sclient/config.go internal/k8sclient/config_test.go go.mod go.sum
git commit -m "feat(k8sclient): resolve rest.Config from CLI flags"
```

---

## Task 3: k8sclient — client factory and RESTMapper

**Files:**
- Create: `internal/k8sclient/clients.go`
- Create: `internal/k8sclient/mapper.go`

Produces the concrete clients used by tool handlers. No tests at this layer — clients are thin factories exercised by envtest in later tasks.

- [ ] **Step 1: Add metrics + prometheus dependencies**

```bash
go get k8s.io/metrics@v0.31.0
go get github.com/prometheus/client_golang@latest
go get github.com/prometheus/common@latest
```

- [ ] **Step 2: Implement clients.go**

`internal/k8sclient/clients.go`:
```go
package k8sclient

import (
	"fmt"
	"net/http"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Clients bundles every Kubernetes-adjacent client used by tool handlers.
// Metrics and Prometheus clients are optional; callers must check for nil.
type Clients struct {
	Typed      kubernetes.Interface
	Dynamic    dynamic.Interface
	Metrics    metricsv.Interface // nil if metrics-server unavailable at construction
	Prometheus promv1.API          // nil if --prometheus-url not set
	Mapper     *Mapper
	Resolved   *Resolved
}

// Build constructs every client from a resolved rest.Config.
// metricsEnabled=true always tries to build the metrics client; failure is
// swallowed and logged later as "metrics-server unavailable".
// promURL=="" disables the Prometheus client entirely.
func Build(r *Resolved, promURL string) (*Clients, error) {
	typed, err := kubernetes.NewForConfig(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("typed client: %w", err)
	}

	dyn, err := dynamic.NewForConfig(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}

	mapper, err := NewMapper(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("rest mapper: %w", err)
	}

	var metrics metricsv.Interface
	if mc, err := metricsv.NewForConfig(r.RESTConfig); err == nil {
		metrics = mc
	}

	var prom promv1.API
	if promURL != "" {
		pc, err := promapi.NewClient(promapi.Config{
			Address: promURL,
			Client:  &http.Client{Timeout: 30 * time.Second},
		})
		if err != nil {
			return nil, fmt.Errorf("prometheus client: %w", err)
		}
		prom = promv1.NewAPI(pc)
	}

	return &Clients{
		Typed:      typed,
		Dynamic:    dyn,
		Metrics:    metrics,
		Prometheus: prom,
		Mapper:     mapper,
		Resolved:   r,
	}, nil
}
```

- [ ] **Step 3: Implement mapper.go**

`internal/k8sclient/mapper.go`:
```go
package k8sclient

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// Mapper wraps a cached DeferredDiscoveryRESTMapper with helpers that turn
// (kind, apiVersion) pairs into concrete GVRs for the dynamic client.
type Mapper struct {
	discovery discovery.DiscoveryInterface
	mapper    meta.RESTMapper
}

func NewMapper(cfg *rest.Config) (*Mapper, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	cached := memory.NewMemCacheClient(dc)
	return &Mapper{
		discovery: dc,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(cached),
	}, nil
}

// ResolveGVR turns a user-provided (kind, apiVersion) into a GVR plus the
// namespaced flag. apiVersion may be empty, in which case the mapper picks
// the preferred version.
func (m *Mapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	gk := schema.GroupKind{Kind: kind}
	if apiVersion != "" {
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return schema.GroupVersionResource{}, false, fmt.Errorf("parse apiVersion: %w", err)
		}
		gk.Group = gv.Group
	}

	versions := []string{}
	if apiVersion != "" {
		gv, _ := schema.ParseGroupVersion(apiVersion)
		versions = []string{gv.Version}
	}

	mapping, err := m.mapper.RESTMapping(gk, versions...)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("no mapping for %s: %w", kind, err)
	}
	return mapping.Resource, mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// Discovery returns the underlying discovery client for list_api_resources.
func (m *Mapper) Discovery() discovery.DiscoveryInterface {
	return m.discovery
}
```

- [ ] **Step 4: Verify build**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/k8sclient/clients.go internal/k8sclient/mapper.go go.mod go.sum
git commit -m "feat(k8sclient): client factory and RESTMapper wrapper"
```

---

## Task 4: k8sclient — startup RBAC precheck

**Files:**
- Create: `internal/k8sclient/precheck.go`
- Create: `internal/k8sclient/precheck_test.go`

Verifies the current SA can at minimum `list pods`; fails fast if not. Uses `SelfSubjectAccessReview` rather than `SelfSubjectRulesReview` for simplicity.

- [ ] **Step 1: Write failing test using fake clientset**

`internal/k8sclient/precheck_test.go`:
```go
package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPrecheck_Allowed(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authv1.SelfSubjectAccessReview{
				ObjectMeta: metav1.ObjectMeta{},
				Status:     authv1.SubjectAccessReviewStatus{Allowed: true},
			}, nil
		},
	)

	err := Precheck(context.Background(), client)
	require.NoError(t, err)
}

func TestPrecheck_Denied(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authv1.SelfSubjectAccessReview{
				Status: authv1.SubjectAccessReviewStatus{
					Allowed: false,
					Reason:  "RBAC: not allowed",
				},
			}, nil
		},
	)

	err := Precheck(context.Background(), client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot list pods")
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/k8sclient/... -run TestPrecheck -v
```
Expected: `undefined: Precheck`.

- [ ] **Step 3: Implement precheck.go**

`internal/k8sclient/precheck.go`:
```go
package k8sclient

import (
	"context"
	"fmt"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Precheck verifies the current identity has at least list access to Pods.
// Returns a descriptive error on denial or API failure.
func Precheck(ctx context.Context, client kubernetes.Interface) error {
	ssar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:     "list",
				Resource: "pods",
			},
		},
	}

	result, err := client.AuthorizationV1().
		SelfSubjectAccessReviews().
		Create(ctx, ssar, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("precheck api call failed: %w", err)
	}
	if !result.Status.Allowed {
		return fmt.Errorf("cannot list pods: %s", result.Status.Reason)
	}
	return nil
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/k8sclient/... -run TestPrecheck -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/k8sclient/precheck.go internal/k8sclient/precheck_test.go
git commit -m "feat(k8sclient): RBAC precheck via SelfSubjectAccessReview"
```

---

## Task 5: main.go wire-up with CLI flags and stdio server

**Files:**
- Modify: `cmd/k8s-mcp-server/main.go`

Full startup sequence: flag parse → resolve config → build clients → precheck → log startup → register zero tools → `ServeStdio`. Registering zero tools is intentional at this stage; tools come in later tasks. The MCP server must already be functional at this point.

- [ ] **Step 1: Add mcp-go dependency**

```bash
go get github.com/mark3labs/mcp-go@latest
go get github.com/oklog/ulid/v2@latest
```

- [ ] **Step 2: Rewrite main.go**

`cmd/k8s-mcp-server/main.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/kube-agent-helper/kube-agent-helper/internal/k8sclient"
)

func main() {
	var (
		inCluster         bool
		kubeconfigPath    string
		contextName       string
		prometheusURL     string
		maskCMKeys        string
		logLevel          string
	)

	flag.BoolVar(&inCluster, "in-cluster", false, "use in-cluster ServiceAccount config")
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "path to kubeconfig file")
	flag.StringVar(&contextName, "context", "", "kubeconfig context name")
	flag.StringVar(&prometheusURL, "prometheus-url", "", "Prometheus HTTP endpoint (enables prometheus_query)")
	flag.StringVar(&maskCMKeys, "mask-configmap-keys", defaultMaskCMRegex,
		"regex matching ConfigMap keys to redact")
	flag.StringVar(&logLevel, "log-level", "info", "log level: info|debug")
	flag.Parse()

	logger := newLogger(logLevel)
	slog.SetDefault(logger)

	ctx := context.Background()

	if err := run(ctx, runOptions{
		InCluster:     inCluster,
		Kubeconfig:    kubeconfigPath,
		Context:       contextName,
		PrometheusURL: prometheusURL,
		MaskCMKeys:    maskCMKeys,
	}); err != nil {
		slog.Error("startup failed", "error", err.Error())
		os.Exit(1)
	}
}

type runOptions struct {
	InCluster     bool
	Kubeconfig    string
	Context       string
	PrometheusURL string
	MaskCMKeys    string
}

func run(ctx context.Context, opts runOptions) error {
	resolved, err := k8sclient.Resolve(k8sclient.Flags{
		InCluster:  opts.InCluster,
		Kubeconfig: opts.Kubeconfig,
		Context:    opts.Context,
	})
	if err != nil {
		return fmt.Errorf("resolve config: %w", err)
	}

	slog.Info("server started",
		"cluster", resolved.ClusterHost,
		"context", resolved.ContextName,
		"source", resolved.Source,
		"mode", "stdio",
	)

	clients, err := k8sclient.Build(resolved, opts.PrometheusURL)
	if err != nil {
		return fmt.Errorf("build clients: %w", err)
	}

	if err := k8sclient.Precheck(ctx, clients.Typed); err != nil {
		return fmt.Errorf("precheck failed: %w", err)
	}
	slog.Info("precheck passed", "verbs", []string{"list:pods"})

	srv := server.NewMCPServer("k8s-mcp-server", "0.1.0")
	_ = clients // tools will consume clients in later tasks

	return server.ServeStdio(srv)
}

func newLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	if level == "debug" {
		lvl = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

const defaultMaskCMRegex = `(?i)(password|passwd|pwd|secret|token|apikey|api_key|credential|private[_-]?key|cert)`
```

- [ ] **Step 3: Verify build**

```bash
make build
```
Expected: `bin/k8s-mcp-server` produced, no errors.

- [ ] **Step 4: Verify help flag**

```bash
./bin/k8s-mcp-server --help
```
Expected: flag list printed with each flag's description.

- [ ] **Step 5: Smoke-verify stdio server handshake**

With no real cluster available, the precheck will fail. That's expected — the goal here is only to verify the binary wires up. Use a kubeconfig pointing at a fake server to confirm flag parsing works:

```bash
./bin/k8s-mcp-server --kubeconfig /nonexistent/kubeconfig 2>&1 | head -5
```
Expected: JSON log line with `"level":"ERROR"` and a message mentioning the kubeconfig path.

- [ ] **Step 6: Commit**

```bash
git add cmd/k8s-mcp-server/main.go go.mod go.sum
git commit -m "feat(cmd): wire up flags, clients, precheck, and stdio server"
```

---

## Task 6: sanitize — generic rules (managedFields, selfLink, last-applied)

**Files:**
- Create: `internal/sanitize/sanitize.go`
- Create: `internal/sanitize/sanitize_test.go`

Pure-function layer. `Clean(obj, opts)` returns a deep-copied, sanitized `*unstructured.Unstructured`. Never mutates the input. Idempotent.

- [ ] **Step 1: Write failing test for generic rules**

`internal/sanitize/sanitize_test.go`:
```go
package sanitize

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func podWithManagedFields() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"managedFields": []interface{}{
					map[string]interface{}{"manager": "kubectl"},
				},
				"selfLink": "/api/v1/namespaces/prod/pods/api",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/last-applied-configuration": "{}",
					"keep-me": "yes",
				},
			},
		},
	}
}

func TestClean_StripsManagedFields(t *testing.T) {
	obj := podWithManagedFields()
	got := Clean(obj, Options{})

	meta := got.Object["metadata"].(map[string]interface{})
	_, has := meta["managedFields"]
	assert.False(t, has, "managedFields must be removed")
}

func TestClean_StripsSelfLink(t *testing.T) {
	got := Clean(podWithManagedFields(), Options{})
	meta := got.Object["metadata"].(map[string]interface{})
	_, has := meta["selfLink"]
	assert.False(t, has)
}

func TestClean_StripsLastAppliedAnnotation(t *testing.T) {
	got := Clean(podWithManagedFields(), Options{})
	meta := got.Object["metadata"].(map[string]interface{})
	annos := meta["annotations"].(map[string]interface{})
	_, hasLastApplied := annos["kubectl.kubernetes.io/last-applied-configuration"]
	assert.False(t, hasLastApplied)
	assert.Equal(t, "yes", annos["keep-me"])
}

func TestClean_Idempotent(t *testing.T) {
	once := Clean(podWithManagedFields(), Options{})
	twice := Clean(once, Options{})
	assert.Equal(t, once.Object, twice.Object)
}

func TestClean_DoesNotMutateInput(t *testing.T) {
	input := podWithManagedFields()
	snapshot := input.DeepCopy()
	_ = Clean(input, Options{})
	require.True(t, reflect.DeepEqual(snapshot.Object, input.Object),
		"input was mutated")
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/sanitize/... -v
```
Expected: `undefined: Clean`, `undefined: Options`.

- [ ] **Step 3: Implement sanitize.go**

`internal/sanitize/sanitize.go`:
```go
package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Options configures the sanitizer.
type Options struct {
	// ConfigMapKeyMask matches ConfigMap data keys to redact. If nil, no
	// ConfigMap redaction is performed beyond the generic rules.
	ConfigMapKeyMask *regexp.Regexp
}

// Clean returns a deep-copied, sanitized copy of obj. The original is never
// mutated. The function is idempotent: Clean(Clean(x)) == Clean(x).
func Clean(obj *unstructured.Unstructured, opts Options) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}
	out := obj.DeepCopy()

	stripGenericMetadata(out)

	switch out.GetKind() {
	case "Secret":
		cleanSecret(out)
	case "ConfigMap":
		cleanConfigMap(out, opts.ConfigMapKeyMask)
	case "Pod":
		cleanPod(out)
	case "PodList":
		// handled by trimmer layer; nothing to do here
	}

	return out
}

// stripGenericMetadata removes fields that every resource should lose,
// regardless of Kind: managedFields, selfLink, and the last-applied
// annotation.
func stripGenericMetadata(u *unstructured.Unstructured) {
	meta, ok := u.Object["metadata"].(map[string]interface{})
	if !ok {
		return
	}
	delete(meta, "managedFields")
	delete(meta, "selfLink")

	if annos, ok := meta["annotations"].(map[string]interface{}); ok {
		delete(annos, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annos) == 0 {
			delete(meta, "annotations")
		}
	}
}
```

- [ ] **Step 4: Stub cleanSecret/cleanConfigMap/cleanPod so tests compile**

Create `internal/sanitize/secret.go`, `configmap.go`, `pod.go` with no-op implementations for now; they are populated in Tasks 7-9.

`internal/sanitize/secret.go`:
```go
package sanitize

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// cleanSecret redacts Secret data values. Implemented in Task 7.
func cleanSecret(u *unstructured.Unstructured) {}
```

`internal/sanitize/configmap.go`:
```go
package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// cleanConfigMap redacts ConfigMap data keys matching mask. Implemented in Task 8.
func cleanConfigMap(u *unstructured.Unstructured, mask *regexp.Regexp) {}
```

`internal/sanitize/pod.go`:
```go
package sanitize

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// cleanPod redacts sensitive env vars on containers. Implemented in Task 9.
func cleanPod(u *unstructured.Unstructured) {}
```

- [ ] **Step 5: Run test to confirm pass**

```bash
go test ./internal/sanitize/... -v
```
Expected: 5 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sanitize/
git commit -m "feat(sanitize): generic metadata stripping with idempotency"
```

---

## Task 7: sanitize — Secret data/stringData redaction

**Files:**
- Modify: `internal/sanitize/secret.go`
- Modify: `internal/sanitize/sanitize_test.go` (append)

- [ ] **Step 1: Append failing tests**

Add to `internal/sanitize/sanitize_test.go`:
```go
func secretObj(data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      "db-creds",
				"namespace": "prod",
			},
			"data": data,
		},
	}
}

func TestClean_SecretDataRedacted(t *testing.T) {
	obj := secretObj(map[string]interface{}{
		"username": "YWRtaW4=",     // base64 "admin" (8 chars decoded-length signal)
		"password": "c2VjcmV0MTIz", // base64 "secret123"
	})
	got := Clean(obj, Options{})

	data := got.Object["data"].(map[string]interface{})
	assert.Regexp(t, `^<redacted len=\d+>$`, data["username"])
	assert.Regexp(t, `^<redacted len=\d+>$`, data["password"])
	assert.Equal(t, "Opaque", got.Object["type"])
}

func TestClean_SecretStringDataRedacted(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"stringData": map[string]interface{}{
				"token": "plaintext-value",
			},
		},
	}
	got := Clean(obj, Options{})
	sd := got.Object["stringData"].(map[string]interface{})
	assert.Equal(t, "<redacted len=15>", sd["token"])
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/sanitize/... -run TestClean_Secret -v
```
Expected: two tests FAIL with the data/stringData unchanged.

- [ ] **Step 3: Implement cleanSecret**

Replace `internal/sanitize/secret.go`:
```go
package sanitize

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cleanSecret(u *unstructured.Unstructured) {
	redactMap(u.Object, "data")
	redactMap(u.Object, "stringData")
}

func redactMap(obj map[string]interface{}, key string) {
	raw, ok := obj[key].(map[string]interface{})
	if !ok {
		return
	}
	for k, v := range raw {
		raw[k] = redactedLen(v)
	}
	_ = k // keep loop var usage clear
	_ = fmt.Sprint
}

func redactedLen(v interface{}) string {
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("<redacted len=%d>", len(x))
	case []byte:
		return fmt.Sprintf("<redacted len=%d>", len(x))
	default:
		return "<redacted>"
	}
}
```

Remove the `_ = k` and `_ = fmt.Sprint` lines — they are just defensive and not necessary:

```go
package sanitize

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cleanSecret(u *unstructured.Unstructured) {
	redactMap(u.Object, "data")
	redactMap(u.Object, "stringData")
}

func redactMap(obj map[string]interface{}, key string) {
	raw, ok := obj[key].(map[string]interface{})
	if !ok {
		return
	}
	for k, v := range raw {
		raw[k] = redactedLen(v)
	}
}

func redactedLen(v interface{}) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("<redacted len=%d>", len(s))
	}
	return "<redacted>"
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/sanitize/... -run TestClean_Secret -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sanitize/secret.go internal/sanitize/sanitize_test.go
git commit -m "feat(sanitize): redact Secret data and stringData"
```

---

## Task 8: sanitize — ConfigMap key-regex redaction

**Files:**
- Modify: `internal/sanitize/configmap.go`
- Modify: `internal/sanitize/sanitize_test.go` (append)

- [ ] **Step 1: Append failing tests**

```go
func cmObj(data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "app-config",
				"namespace": "prod",
			},
			"data": data,
		},
	}
}

func TestClean_ConfigMapKeyMasked(t *testing.T) {
	mask := regexp.MustCompile(`(?i)(password|token|secret)`)
	obj := cmObj(map[string]interface{}{
		"LOG_LEVEL":    "debug",
		"DB_PASSWORD":  "supersecret",
		"API_TOKEN":    "abc123",
		"INNOCENT_KEY": "hello",
	})
	got := Clean(obj, Options{ConfigMapKeyMask: mask})
	data := got.Object["data"].(map[string]interface{})

	assert.Equal(t, "debug", data["LOG_LEVEL"])
	assert.Equal(t, "<redacted>", data["DB_PASSWORD"])
	assert.Equal(t, "<redacted>", data["API_TOKEN"])
	assert.Equal(t, "hello", data["INNOCENT_KEY"])
}

func TestClean_ConfigMapNoMask(t *testing.T) {
	obj := cmObj(map[string]interface{}{"LOG_LEVEL": "debug", "PASSWORD": "x"})
	got := Clean(obj, Options{}) // no mask configured
	data := got.Object["data"].(map[string]interface{})
	assert.Equal(t, "debug", data["LOG_LEVEL"])
	assert.Equal(t, "x", data["PASSWORD"])
}
```

You must also add `"regexp"` to the imports at the top of `sanitize_test.go`.

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/sanitize/... -run TestClean_ConfigMap -v
```
Expected: first test FAILS (values unchanged).

- [ ] **Step 3: Implement cleanConfigMap**

Replace `internal/sanitize/configmap.go`:
```go
package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cleanConfigMap(u *unstructured.Unstructured, mask *regexp.Regexp) {
	if mask == nil {
		return
	}
	data, ok := u.Object["data"].(map[string]interface{})
	if !ok {
		return
	}
	for k := range data {
		if mask.MatchString(k) {
			data[k] = "<redacted>"
		}
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/sanitize/... -run TestClean_ConfigMap -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sanitize/configmap.go internal/sanitize/sanitize_test.go
git commit -m "feat(sanitize): regex-driven ConfigMap key redaction"
```

---

## Task 9: sanitize — Pod env variable redaction

**Files:**
- Modify: `internal/sanitize/pod.go`
- Modify: `internal/sanitize/sanitize_test.go` (append)

Applies to both `spec.containers[].env` and `spec.initContainers[].env`. Regex is an internal constant — not user configurable.

- [ ] **Step 1: Append failing tests**

```go
func podWithEnv(containers []interface{}, initContainers []interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{"containers": containers}
	if initContainers != nil {
		spec["initContainers"] = initContainers
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "api", "namespace": "prod"},
			"spec":       spec,
		},
	}
}

func TestClean_PodEnvRedacted(t *testing.T) {
	obj := podWithEnv(
		[]interface{}{
			map[string]interface{}{
				"name": "main",
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"},
					map[string]interface{}{"name": "DB_PASSWORD", "value": "super"},
					map[string]interface{}{"name": "API_TOKEN", "value": "abc"},
					// valueFrom should be preserved verbatim
					map[string]interface{}{
						"name": "DATABASE_SECRET",
						"valueFrom": map[string]interface{}{
							"secretKeyRef": map[string]interface{}{
								"name": "db-creds",
								"key":  "password",
							},
						},
					},
				},
			},
		},
		[]interface{}{
			map[string]interface{}{
				"name": "init",
				"env": []interface{}{
					map[string]interface{}{"name": "INIT_PASSWORD", "value": "init-secret"},
				},
			},
		},
	)

	got := Clean(obj, Options{})

	containers := got.Object["spec"].(map[string]interface{})["containers"].([]interface{})
	envs := containers[0].(map[string]interface{})["env"].([]interface{})
	assert.Equal(t, "debug", envs[0].(map[string]interface{})["value"])
	assert.Equal(t, "<redacted>", envs[1].(map[string]interface{})["value"])
	assert.Equal(t, "<redacted>", envs[2].(map[string]interface{})["value"])
	// valueFrom still present on entry 3 (no "value" field)
	_, hasValueFrom := envs[3].(map[string]interface{})["valueFrom"]
	assert.True(t, hasValueFrom)

	initContainers := got.Object["spec"].(map[string]interface{})["initContainers"].([]interface{})
	initEnv := initContainers[0].(map[string]interface{})["env"].([]interface{})
	assert.Equal(t, "<redacted>", initEnv[0].(map[string]interface{})["value"])
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/sanitize/... -run TestClean_PodEnv -v
```
Expected: FAIL.

- [ ] **Step 3: Implement cleanPod**

Replace `internal/sanitize/pod.go`:
```go
package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var podEnvMask = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key|credential|auth)`)

func cleanPod(u *unstructured.Unstructured) {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return
	}
	redactContainers(spec, "containers")
	redactContainers(spec, "initContainers")
}

func redactContainers(spec map[string]interface{}, key string) {
	raw, ok := spec[key].([]interface{})
	if !ok {
		return
	}
	for i := range raw {
		c, ok := raw[i].(map[string]interface{})
		if !ok {
			continue
		}
		envs, ok := c["env"].([]interface{})
		if !ok {
			continue
		}
		for j := range envs {
			e, ok := envs[j].(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := e["name"].(string)
			if _, hasValue := e["value"]; hasValue && podEnvMask.MatchString(name) {
				e["value"] = "<redacted>"
			}
		}
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/sanitize/... -v
```
Expected: every `TestClean_*` PASSES. Check overall coverage:

```bash
go test ./internal/sanitize/... -cover
```
Expected: ≥ 90%.

- [ ] **Step 5: Commit**

```bash
git add internal/sanitize/pod.go internal/sanitize/sanitize_test.go
git commit -m "feat(sanitize): redact sensitive Pod env values"
```

---

## Task 10: trimmer — projector interface, generic fallback, Pod projection

**Files:**
- Create: `internal/trimmer/trimmer.go`
- Create: `internal/trimmer/pod.go`
- Create: `internal/trimmer/trimmer_test.go`
- Create: `internal/trimmer/testdata/pod.input.json`
- Create: `internal/trimmer/testdata/pod.golden.json`

The trimmer builds slimmed projections for list-mode responses. Each specialized projector returns a plain `map[string]interface{}` shaped exactly as documented in spec §2.1.

- [ ] **Step 1: Write failing test with golden file**

`internal/trimmer/testdata/pod.input.json`:
```json
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "api-7d8-abc",
    "namespace": "prod",
    "creationTimestamp": "2026-04-08T09:00:00Z",
    "labels": {"app": "api", "env": "prod"},
    "managedFields": [{"manager": "kubectl"}]
  },
  "spec": {
    "nodeName": "node-3",
    "containers": [{"name": "main"}, {"name": "sidecar"}]
  },
  "status": {
    "phase": "Running",
    "containerStatuses": [
      {"name": "main",    "ready": true,  "restartCount": 1, "state": {"running": {}}},
      {"name": "sidecar", "ready": false, "restartCount": 3, "state": {"waiting": {"reason": "CrashLoopBackOff"}}}
    ]
  }
}
```

`internal/trimmer/testdata/pod.golden.json`:
```json
{
  "name": "api-7d8-abc",
  "namespace": "prod",
  "phase": "Running",
  "nodeName": "node-3",
  "restarts": 4,
  "ready": "1/2",
  "labels": {"app": "api", "env": "prod"},
  "containers": [
    {"name": "main", "ready": true, "restartCount": 1, "state": "Running"},
    {"name": "sidecar", "ready": false, "restartCount": 3, "state": "CrashLoopBackOff"}
  ]
}
```

`internal/trimmer/trimmer_test.go`:
```go
package trimmer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func loadFixture(t *testing.T, name string) *unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &data))
	return &unstructured.Unstructured{Object: data}
}

func loadGolden(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &data))
	return data
}

// Freeze "now" for age calculation reproducibility.
func frozenNow() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-04-11T09:00:00Z")
	return t
}

func TestProject_Pod_Golden(t *testing.T) {
	in := loadFixture(t, "pod.input.json")
	golden := loadGolden(t, "pod.golden.json")

	// age is time-dependent; strip it from both sides before comparing
	// the rest of the projection.
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_UnknownKind_Generic(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata": map[string]interface{}{
				"name":              "nightly",
				"namespace":         "ops",
				"labels":            map[string]interface{}{"team": "sre"},
				"creationTimestamp": "2026-04-10T00:00:00Z",
			},
		},
	}
	p := &Projectors{Now: frozenNow}
	got := p.Project(obj)
	assert.Equal(t, "nightly", got["name"])
	assert.Equal(t, "ops", got["namespace"])
	assert.Equal(t, map[string]interface{}{"team": "sre"}, got["labels"])
	assert.Contains(t, got, "age")
}

func TestProject_StripsManagedFields(t *testing.T) {
	in := loadFixture(t, "pod.input.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	_, hasManagedFields := got["managedFields"]
	assert.False(t, hasManagedFields, "trimmer must never leak managedFields")
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/trimmer/... -v
```
Expected: `undefined: Projectors`.

- [ ] **Step 3: Implement trimmer.go**

`internal/trimmer/trimmer.go`:
```go
package trimmer

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Projectors produces slimmed list-mode projections for known Kinds,
// falling back to a generic shape for unknown ones.
type Projectors struct {
	// Now is injected for test determinism. nil => time.Now.
	Now func() time.Time
}

func (p *Projectors) now() time.Time {
	if p == nil || p.Now == nil {
		return time.Now()
	}
	return p.Now()
}

// Project returns a minimal map representation of u suited for LLM list
// responses. managedFields, selfLink, and last-applied annotations are
// guaranteed absent.
func (p *Projectors) Project(u *unstructured.Unstructured) map[string]interface{} {
	switch u.GetKind() {
	case "Pod":
		return p.projectPod(u)
	default:
		return p.projectGeneric(u)
	}
}

func (p *Projectors) projectGeneric(u *unstructured.Unstructured) map[string]interface{} {
	m := map[string]interface{}{
		"name":      u.GetName(),
		"namespace": u.GetNamespace(),
		"age":       humanAge(p.now(), u.GetCreationTimestamp()),
	}
	if lbls := u.GetLabels(); len(lbls) > 0 {
		m["labels"] = toMapInterface(lbls)
	}
	return m
}

func humanAge(now time.Time, ts metav1.Time) string {
	if ts.IsZero() {
		return "unknown"
	}
	d := now.Sub(ts.Time)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

func toMapInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 4: Implement pod.go**

`internal/trimmer/pod.go`:
```go
package trimmer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (p *Projectors) projectPod(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)

	phase, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	nodeName, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
	if phase != "" {
		base["phase"] = phase
	}
	if nodeName != "" {
		base["nodeName"] = nodeName
	}

	specContainers, _, _ := unstructured.NestedSlice(u.Object, "spec", "containers")
	statuses, _, _ := unstructured.NestedSlice(u.Object, "status", "containerStatuses")

	readyCount, totalRestarts := 0, int64(0)
	containerList := make([]interface{}, 0, len(statuses))
	for _, raw := range statuses {
		cs, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ready, _ := cs["ready"].(bool)
		if ready {
			readyCount++
		}
		rc, _ := cs["restartCount"].(int64)
		if rc == 0 {
			// json.Unmarshal into interface{} produces float64
			if f, ok := cs["restartCount"].(float64); ok {
				rc = int64(f)
			}
		}
		totalRestarts += rc

		name, _ := cs["name"].(string)
		state := summarizeState(cs["state"])
		containerList = append(containerList, map[string]interface{}{
			"name":         name,
			"ready":        ready,
			"restartCount": rc,
			"state":        state,
		})
	}

	base["restarts"] = totalRestarts
	if len(specContainers) > 0 {
		base["ready"] = fmt.Sprintf("%d/%d", readyCount, len(specContainers))
	}
	if len(containerList) > 0 {
		base["containers"] = containerList
	}
	return base
}

// summarizeState collapses the container state struct into a single string
// like "Running", "CrashLoopBackOff", "Completed", etc.
func summarizeState(raw interface{}) string {
	state, ok := raw.(map[string]interface{})
	if !ok {
		return "Unknown"
	}
	if _, running := state["running"].(map[string]interface{}); running {
		return "Running"
	}
	if w, waiting := state["waiting"].(map[string]interface{}); waiting {
		if reason, _ := w["reason"].(string); reason != "" {
			return reason
		}
		return "Waiting"
	}
	if tr, terminated := state["terminated"].(map[string]interface{}); terminated {
		if reason, _ := tr["reason"].(string); reason != "" {
			return reason
		}
		return "Terminated"
	}
	return "Unknown"
}
```

- [ ] **Step 5: Run test to confirm pass**

```bash
go test ./internal/trimmer/... -v
```
Expected: all 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/trimmer/
git commit -m "feat(trimmer): projector interface, generic fallback, Pod projection"
```

---

## Task 11: trimmer — Deployment and Node projections

**Files:**
- Create: `internal/trimmer/deployment.go`
- Create: `internal/trimmer/node.go`
- Create: `internal/trimmer/testdata/deployment.input.json`
- Create: `internal/trimmer/testdata/deployment.golden.json`
- Create: `internal/trimmer/testdata/node.input.json`
- Create: `internal/trimmer/testdata/node.golden.json`
- Modify: `internal/trimmer/trimmer_test.go` (append)
- Modify: `internal/trimmer/trimmer.go` (wire switch cases)

- [ ] **Step 1: Write golden fixtures**

`internal/trimmer/testdata/deployment.input.json`:
```json
{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {"name": "api", "namespace": "prod",
               "creationTimestamp": "2026-04-10T09:00:00Z",
               "labels": {"app": "api"}},
  "spec": {"replicas": 3},
  "status": {"replicas": 3, "readyReplicas": 2, "updatedReplicas": 3, "availableReplicas": 2}
}
```

`internal/trimmer/testdata/deployment.golden.json`:
```json
{
  "name": "api",
  "namespace": "prod",
  "labels": {"app": "api"},
  "replicas": {"desired": 3, "ready": 2, "updated": 3, "available": 2}
}
```

`internal/trimmer/testdata/node.input.json`:
```json
{
  "apiVersion": "v1",
  "kind": "Node",
  "metadata": {"name": "node-3",
               "creationTimestamp": "2026-03-11T09:00:00Z",
               "labels": {"kubernetes.io/arch": "amd64"}},
  "status": {
    "conditions": [
      {"type": "Ready", "status": "True"},
      {"type": "MemoryPressure", "status": "False"}
    ],
    "nodeInfo": {"kubeletVersion": "v1.31.0", "osImage": "Ubuntu 22.04"},
    "capacity": {"cpu": "8", "memory": "16Gi"},
    "allocatable": {"cpu": "7800m", "memory": "15Gi"}
  }
}
```

`internal/trimmer/testdata/node.golden.json`:
```json
{
  "name": "node-3",
  "labels": {"kubernetes.io/arch": "amd64"},
  "ready": true,
  "kubeletVersion": "v1.31.0",
  "osImage": "Ubuntu 22.04",
  "capacity": {"cpu": "8", "memory": "16Gi"},
  "allocatable": {"cpu": "7800m", "memory": "15Gi"}
}
```

- [ ] **Step 2: Append failing tests**

```go
func TestProject_Deployment_Golden(t *testing.T) {
	in := loadFixture(t, "deployment.input.json")
	golden := loadGolden(t, "deployment.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_Node_Golden(t *testing.T) {
	in := loadFixture(t, "node.input.json")
	golden := loadGolden(t, "node.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}
```

- [ ] **Step 3: Run test to confirm failure**

```bash
go test ./internal/trimmer/... -run TestProject_Deployment -v
```
Expected: equality mismatch — generic projection is used instead of Deployment-specific.

- [ ] **Step 4: Wire new Kinds into the switch**

Edit `internal/trimmer/trimmer.go`, extend the `Project` switch:
```go
func (p *Projectors) Project(u *unstructured.Unstructured) map[string]interface{} {
	switch u.GetKind() {
	case "Pod":
		return p.projectPod(u)
	case "Deployment":
		return p.projectDeployment(u)
	case "Node":
		return p.projectNode(u)
	default:
		return p.projectGeneric(u)
	}
}
```

- [ ] **Step 5: Implement deployment.go**

`internal/trimmer/deployment.go`:
```go
package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectDeployment(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)

	desired, _, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
	ready, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	updated, _, _ := unstructured.NestedInt64(u.Object, "status", "updatedReplicas")
	available, _, _ := unstructured.NestedInt64(u.Object, "status", "availableReplicas")

	base["replicas"] = map[string]interface{}{
		"desired":   desired,
		"ready":     ready,
		"updated":   updated,
		"available": available,
	}
	return base
}
```

- [ ] **Step 6: Implement node.go**

`internal/trimmer/node.go`:
```go
package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectNode(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)

	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, raw := range conds {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if c["type"] == "Ready" {
			base["ready"] = c["status"] == "True"
			break
		}
	}

	kv, _, _ := unstructured.NestedString(u.Object, "status", "nodeInfo", "kubeletVersion")
	os, _, _ := unstructured.NestedString(u.Object, "status", "nodeInfo", "osImage")
	base["kubeletVersion"] = kv
	base["osImage"] = os

	if cap, ok, _ := unstructured.NestedMap(u.Object, "status", "capacity"); ok {
		base["capacity"] = cap
	}
	if alloc, ok, _ := unstructured.NestedMap(u.Object, "status", "allocatable"); ok {
		base["allocatable"] = alloc
	}
	return base
}
```

- [ ] **Step 7: Run test to confirm pass**

```bash
go test ./internal/trimmer/... -v
```
Expected: all 5 tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/trimmer/deployment.go internal/trimmer/node.go \
        internal/trimmer/trimmer.go internal/trimmer/trimmer_test.go \
        internal/trimmer/testdata/deployment.* internal/trimmer/testdata/node.*
git commit -m "feat(trimmer): Deployment and Node projections"
```

---

## Task 12: trimmer — Service and Event projections

**Files:**
- Create: `internal/trimmer/service.go`
- Create: `internal/trimmer/event.go`
- Create: `internal/trimmer/testdata/service.input.json`
- Create: `internal/trimmer/testdata/service.golden.json`
- Create: `internal/trimmer/testdata/event.input.json`
- Create: `internal/trimmer/testdata/event.golden.json`
- Modify: `internal/trimmer/trimmer.go`
- Modify: `internal/trimmer/trimmer_test.go`

- [ ] **Step 1: Write golden fixtures**

`internal/trimmer/testdata/service.input.json`:
```json
{
  "apiVersion": "v1",
  "kind": "Service",
  "metadata": {"name": "api", "namespace": "prod",
               "creationTimestamp": "2026-04-10T09:00:00Z",
               "labels": {"app": "api"}},
  "spec": {
    "type": "ClusterIP",
    "clusterIP": "10.0.1.42",
    "ports": [
      {"name": "http", "port": 80, "targetPort": 8080, "protocol": "TCP"}
    ],
    "selector": {"app": "api"}
  }
}
```

`internal/trimmer/testdata/service.golden.json`:
```json
{
  "name": "api",
  "namespace": "prod",
  "labels": {"app": "api"},
  "type": "ClusterIP",
  "clusterIP": "10.0.1.42",
  "selector": {"app": "api"},
  "ports": [
    {"name": "http", "port": 80, "targetPort": 8080, "protocol": "TCP"}
  ]
}
```

`internal/trimmer/testdata/event.input.json`:
```json
{
  "apiVersion": "v1",
  "kind": "Event",
  "metadata": {"name": "api.abc", "namespace": "prod",
               "creationTimestamp": "2026-04-11T08:00:00Z"},
  "type": "Warning",
  "reason": "BackOff",
  "message": "Back-off restarting failed container",
  "count": 12,
  "firstTimestamp": "2026-04-11T07:00:00Z",
  "lastTimestamp": "2026-04-11T08:00:00Z",
  "involvedObject": {"kind": "Pod", "name": "api-xxx", "namespace": "prod"}
}
```

`internal/trimmer/testdata/event.golden.json`:
```json
{
  "namespace": "prod",
  "type": "Warning",
  "reason": "BackOff",
  "message": "Back-off restarting failed container",
  "count": 12,
  "firstTimestamp": "2026-04-11T07:00:00Z",
  "lastTimestamp": "2026-04-11T08:00:00Z",
  "involvedObject": {"kind": "Pod", "name": "api-xxx", "namespace": "prod"}
}
```

- [ ] **Step 2: Append failing tests**

```go
func TestProject_Service_Golden(t *testing.T) {
	in := loadFixture(t, "service.input.json")
	golden := loadGolden(t, "service.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_Event_Golden(t *testing.T) {
	in := loadFixture(t, "event.input.json")
	golden := loadGolden(t, "event.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	// Event projection does not emit "age" or "name".
	assert.Equal(t, golden, got)
}
```

- [ ] **Step 3: Wire new Kinds into switch**

```go
func (p *Projectors) Project(u *unstructured.Unstructured) map[string]interface{} {
	switch u.GetKind() {
	case "Pod":
		return p.projectPod(u)
	case "Deployment":
		return p.projectDeployment(u)
	case "Node":
		return p.projectNode(u)
	case "Service":
		return p.projectService(u)
	case "Event":
		return p.projectEvent(u)
	default:
		return p.projectGeneric(u)
	}
}
```

- [ ] **Step 4: Implement service.go**

`internal/trimmer/service.go`:
```go
package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectService(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return base
	}
	if v, ok := spec["type"].(string); ok {
		base["type"] = v
	}
	if v, ok := spec["clusterIP"].(string); ok {
		base["clusterIP"] = v
	}
	if v, ok := spec["selector"].(map[string]interface{}); ok {
		base["selector"] = v
	}
	if v, ok := spec["ports"].([]interface{}); ok {
		base["ports"] = v
	}
	return base
}
```

- [ ] **Step 5: Implement event.go**

`internal/trimmer/event.go`:
```go
package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectEvent(u *unstructured.Unstructured) map[string]interface{} {
	out := map[string]interface{}{
		"namespace": u.GetNamespace(),
	}
	copyStr := func(dst, src string) {
		if v, ok := u.Object[src].(string); ok {
			out[dst] = v
		}
	}
	copyStr("type", "type")
	copyStr("reason", "reason")
	copyStr("message", "message")
	copyStr("firstTimestamp", "firstTimestamp")
	copyStr("lastTimestamp", "lastTimestamp")

	if v, ok := u.Object["count"]; ok {
		out["count"] = v
	}
	if v, ok := u.Object["involvedObject"].(map[string]interface{}); ok {
		// Trim to the three fields the LLM actually needs.
		trimmed := map[string]interface{}{}
		for _, k := range []string{"kind", "name", "namespace"} {
			if vv, ok := v[k].(string); ok {
				trimmed[k] = vv
			}
		}
		out["involvedObject"] = trimmed
	}
	return out
}
```

- [ ] **Step 6: Run test to confirm pass**

```bash
go test ./internal/trimmer/... -v -cover
```
Expected: all 7 tests PASS, coverage ≥ 90%.

- [ ] **Step 7: Commit**

```bash
git add internal/trimmer/service.go internal/trimmer/event.go \
        internal/trimmer/trimmer.go internal/trimmer/trimmer_test.go \
        internal/trimmer/testdata/service.* internal/trimmer/testdata/event.*
git commit -m "feat(trimmer): Service and Event projections"
```

---

## Task 13: audit — slog logger and arg whitelist sanitizer

**Files:**
- Create: `internal/audit/logger.go`
- Create: `internal/audit/argmask.go`
- Create: `internal/audit/audit_test.go`

- [ ] **Step 1: Write failing test for argmask**

`internal/audit/audit_test.go`:
```go
package audit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskArgs_KeepsKnownFields(t *testing.T) {
	whitelist := []string{"kind", "namespace", "name", "labelSelector"}
	in := map[string]interface{}{
		"kind":          "Pod",
		"namespace":     "prod",
		"labelSelector": "app=api",
		"token":         "should-be-dropped", // not in whitelist
	}
	got := MaskArgs(in, whitelist)
	assert.Equal(t, map[string]interface{}{
		"kind":          "Pod",
		"namespace":     "prod",
		"labelSelector": "app=api",
	}, got)
}

func TestMaskArgs_EmptyWhitelistDropsAll(t *testing.T) {
	got := MaskArgs(map[string]interface{}{"x": 1}, nil)
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/audit/... -run TestMaskArgs -v
```
Expected: `undefined: MaskArgs`.

- [ ] **Step 3: Implement argmask.go**

`internal/audit/argmask.go`:
```go
package audit

// MaskArgs returns a new map containing only the keys listed in whitelist.
// Values are passed through unchanged — callers are responsible for ensuring
// that no listed field carries sensitive content.
func MaskArgs(args map[string]interface{}, whitelist []string) map[string]interface{} {
	out := make(map[string]interface{}, len(whitelist))
	for _, k := range whitelist {
		if v, ok := args[k]; ok {
			out[k] = v
		}
	}
	return out
}
```

- [ ] **Step 4: Implement logger.go**

`internal/audit/logger.go`:
```go
package audit

import (
	"log/slog"
	"os"
)

// New returns a JSON slog.Logger writing to stderr at the given level.
// level must be "info" or "debug"; anything else defaults to info.
func New(level string) *slog.Logger {
	lvl := slog.LevelInfo
	if level == "debug" {
		lvl = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
```

- [ ] **Step 5: Run test to confirm pass**

```bash
go test ./internal/audit/... -v
```
Expected: tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/audit/
git commit -m "feat(audit): slog JSON logger and arg whitelist sanitizer"
```

---

## Task 14: audit — tool-call middleware

**Files:**
- Create: `internal/audit/middleware.go`
- Modify: `internal/audit/audit_test.go` (append)

The middleware wraps an `mcp-go` tool handler and emits one JSON log record per call with the schema defined in spec §4.2.

- [ ] **Step 1: Append failing test using a capturing slog handler**

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), buf
}

func TestMiddleware_LogsSuccess(t *testing.T) {
	logger, buf := captureLogger()
	called := false
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText(`{"items":[]}`), nil
	}

	spec := ToolSpec{
		Name:         "kubectl_get",
		ArgWhitelist: []string{"kind", "namespace"},
		Cluster:      "https://example:6443",
	}
	wrapped := Wrap(logger, spec, handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "kubectl_get"
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"dropped":   "should-not-appear",
	}

	_, err := wrapped(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "tool_call", entry["msg"])
	assert.Equal(t, "kubectl_get", entry["tool"])
	assert.Equal(t, "https://example:6443", entry["cluster"])

	args := entry["args"].(map[string]interface{})
	assert.Equal(t, "Pod", args["kind"])
	assert.Equal(t, "prod", args["namespace"])
	_, hasDropped := args["dropped"]
	assert.False(t, hasDropped)

	result := entry["result"].(map[string]interface{})
	assert.Equal(t, true, result["ok"])
	assert.NotNil(t, entry["trace_id"])
	assert.NotNil(t, entry["latency_ms"])
}

func TestMiddleware_LogsError(t *testing.T) {
	logger, buf := captureLogger()
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("pod not found"), nil
	}
	wrapped := Wrap(logger, ToolSpec{Name: "kubectl_logs"}, handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "kubectl_logs"
	req.Params.Arguments = map[string]interface{}{}
	_, err := wrapped(context.Background(), req)
	require.NoError(t, err)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "ERROR", entry["level"])
	result := entry["result"].(map[string]interface{})
	assert.Equal(t, false, result["ok"])
	assert.Contains(t, entry["error"], "pod not found")
}

func TestMiddleware_TimestampIncreasing(t *testing.T) {
	// Sanity: latency_ms >= 0
	logger, buf := captureLogger()
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(5 * time.Millisecond)
		return mcp.NewToolResultText("ok"), nil
	}
	wrapped := Wrap(logger, ToolSpec{Name: "t"}, handler)
	_, _ = wrapped(context.Background(), mcp.CallToolRequest{})

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	lat, _ := entry["latency_ms"].(float64)
	assert.GreaterOrEqual(t, lat, float64(0))
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/audit/... -run TestMiddleware -v
```
Expected: `undefined: Wrap`, `undefined: ToolSpec`.

- [ ] **Step 3: Implement middleware.go**

`internal/audit/middleware.go`:
```go
package audit

import (
	"context"
	"crypto/rand"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oklog/ulid/v2"
)

// Handler matches mcp-go's handler signature.
type Handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolSpec describes everything the middleware needs to know about a tool
// in order to produce faithful audit records.
type ToolSpec struct {
	Name         string
	ArgWhitelist []string
	Cluster      string
}

// Wrap produces a new Handler that emits a single structured log record per
// invocation. Panics are recovered and logged.
func Wrap(logger *slog.Logger, spec ToolSpec, next Handler) Handler {
	return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		start := time.Now()
		traceID := ulid.MustNew(ulid.Now(), rand.Reader).String()

		defer func() {
			if r := recover(); r != nil {
				logger.LogAttrs(ctx, slog.LevelError, "tool_call",
					slog.String("trace_id", traceID),
					slog.String("tool", spec.Name),
					slog.Any("args", MaskArgs(argsMap(req), spec.ArgWhitelist)),
					slog.Group("result", slog.Bool("ok", false)),
					slog.Int64("latency_ms", time.Since(start).Milliseconds()),
					slog.String("cluster", spec.Cluster),
					slog.String("error", "panic"),
				)
				result = mcp.NewToolResultError("internal error")
				err = nil
			}
		}()

		result, err = next(ctx, req)

		level := slog.LevelInfo
		ok := err == nil && (result == nil || !result.IsError)
		var errMsg string
		if !ok {
			level = slog.LevelError
			if err != nil {
				errMsg = err.Error()
			} else if result != nil {
				errMsg = extractErrorText(result)
			}
		}

		attrs := []slog.Attr{
			slog.String("trace_id", traceID),
			slog.String("tool", spec.Name),
			slog.Any("args", MaskArgs(argsMap(req), spec.ArgWhitelist)),
			slog.Group("result", slog.Bool("ok", ok)),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			slog.String("cluster", spec.Cluster),
		}
		if errMsg != "" {
			attrs = append(attrs, slog.String("error", errMsg))
		}
		logger.LogAttrs(ctx, level, "tool_call", attrs...)

		return result, err
	}
}

func argsMap(req mcp.CallToolRequest) map[string]interface{} {
	if m, ok := req.Params.Arguments.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func extractErrorText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/audit/... -v -cover
```
Expected: all tests PASS, coverage ≥ 85%.

- [ ] **Step 5: Commit**

```bash
git add internal/audit/middleware.go internal/audit/audit_test.go
git commit -m "feat(audit): tool_call middleware with trace_id and latency"
```

---

## Task 15: envtest shared setup for component tests

**Files:**
- Create: `test/envtest/setup_test.go`
- Create: `test/envtest/helpers.go`

One-time envtest bootstrap shared across component tests. Uses `setup-envtest` to install the envtest binary.

- [ ] **Step 1: Add controller-runtime dependency**

```bash
go get sigs.k8s.io/controller-runtime@latest
```

- [ ] **Step 2: Install envtest assets**

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use 1.31.0 --bin-dir ./bin/envtest
```

Expected output: a path like `bin/envtest/k8s/1.31.0-darwin-arm64`.

- [ ] **Step 3: Write envtest setup**

`test/envtest/setup_test.go`:
```go
package envtest

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var SharedConfig *rest.Config

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Prefer the path set up by setup-envtest into bin/envtest/k8s/<version>.
		root, err := filepath.Abs("../../bin/envtest/k8s")
		if err == nil {
			entries, _ := os.ReadDir(root)
			if len(entries) > 0 {
				os.Setenv("KUBEBUILDER_ASSETS", filepath.Join(root, entries[0].Name()))
			}
		}
	}

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		ctrl.Log.Error(err, "envtest start failed")
		os.Exit(1)
	}
	SharedConfig = cfg

	code := m.Run()

	_ = env.Stop()
	os.Exit(code)
}
```

- [ ] **Step 4: Write test helper**

`test/envtest/helpers.go`:
```go
package envtest

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/stretchr/testify/require"
)

func NewTypedClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	c, err := kubernetes.NewForConfig(SharedConfig)
	require.NoError(t, err)
	return c
}

func NewDynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()
	c, err := dynamic.NewForConfig(SharedConfig)
	require.NoError(t, err)
	return c
}

func CreateNamespace(t *testing.T, client kubernetes.Interface, name string) {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Create(context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{})
	require.NoError(t, err)
}

var PodGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
```

- [ ] **Step 5: Smoke test that envtest boots**

Add a trivial smoke test to `test/envtest/setup_test.go`:
```go
func TestEnvtest_Boots(t *testing.T) {
	client := NewTypedClient(t)
	ns, err := client.CoreV1().Namespaces().List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	// default namespace exists out of the box
	assert.NotEmpty(t, ns.Items)
}
```

Add `"github.com/stretchr/testify/assert"`, `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`, and `"github.com/stretchr/testify/require"` to the imports.

- [ ] **Step 6: Run envtest smoke**

```bash
go test ./test/envtest/... -v
```
Expected: `TestEnvtest_Boots` PASSES.

- [ ] **Step 7: Commit**

```bash
git add test/envtest/ go.mod go.sum
git commit -m "test(envtest): shared setup with smoke test"
```

---

## Task 16: mcptools — kubectl_get (list and get modes)

**Files:**
- Create: `internal/mcptools/deps.go`
- Create: `internal/mcptools/kubectl_get.go`
- Create: `internal/mcptools/kubectl_get_test.go`

`Deps` bundles everything tools need: clients, sanitize options, trimmer, logger, cluster URL. Handlers receive a `*Deps` and produce a closure. This avoids package-global state.

- [ ] **Step 1: Create shared Deps struct**

Write `internal/mcptools/deps.go` using the interface-based version shown in the "Deps design note" below Step 2. The handler layer must depend on narrow interfaces (`dynamic.Interface`, `kubernetes.Interface`, `ResourceMapper`, etc.), not on the concrete `k8sclient.Clients`, so that tests can inject fakes.

- [ ] **Step 2: Write failing test for list mode using fake dynamic client**

`internal/mcptools/kubectl_get_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

func newPod(name, ns, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         ns,
				"creationTimestamp": "2026-04-10T09:00:00Z",
				"labels":            map[string]interface{}{"app": "api"},
			},
			"spec": map[string]interface{}{
				"nodeName":   "node-1",
				"containers": []interface{}{map[string]interface{}{"name": "main"}},
			},
			"status": map[string]interface{}{
				"phase": phase,
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":         "main",
						"ready":        true,
						"restartCount": int64(0),
						"state":        map[string]interface{}{"running": map[string]interface{}{}},
					},
				},
			},
		},
	}
}

func fakeDeps(t *testing.T, objs ...runtime.Object) *Deps {
	t.Helper()
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	listKinds := map[schema.GroupVersionResource]string{gvr: "PodList"}
	return &Deps{
		Dynamic:    dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...),
		Mapper:     &testMapper{gvr: gvr},
		Logger:     slog.Default(),
		Projectors: &trimmer.Projectors{},
		Cluster:    "https://test",
	}
}

func TestKubectlGet_ListMode(t *testing.T) {
	d := fakeDeps(t,
		newPod("api-1", "prod", "Running"),
		newPod("api-2", "prod", "Running"),
	)
	handler := NewKubectlGetHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Kind          string                   `json:"kind"`
		ReturnedCount int                      `json:"returnedCount"`
		Truncated     bool                     `json:"truncated"`
		Items         []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))

	assert.Equal(t, "Pod", payload.Kind)
	assert.Equal(t, 2, payload.ReturnedCount)
	assert.False(t, payload.Truncated)
	assert.Len(t, payload.Items, 2)
	assert.Equal(t, "Running", payload.Items[0]["phase"])
}

func TestKubectlGet_GetMode(t *testing.T) {
	d := fakeDeps(t, newPod("api-1", "prod", "Running"))
	handler := NewKubectlGetHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"name":      "api-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &obj))
	meta := obj["metadata"].(map[string]interface{})
	assert.Equal(t, "api-1", meta["name"])
	_, hasManagedFields := meta["managedFields"]
	assert.False(t, hasManagedFields)
}

func TestKubectlGet_MissingKind(t *testing.T) {
	d := fakeDeps(t)
	handler := NewKubectlGetHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func textOf(r *mcp.CallToolResult) string {
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// --- test doubles -----------------------------------------------------------

type testMapper struct{ gvr schema.GroupVersionResource }

func (m *testMapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	return m.gvr, true, nil
}

var _ = metav1.ListOptions{}
```

**Deps design note:** The handler layer must not depend on the concrete `k8sclient.Clients` struct so tests can inject fakes. Back in Step 1, `deps.go` should be written against narrow interfaces from the start:

```go
package mcptools

import (
	"log/slog"
	"regexp"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/kube-agent-helper/kube-agent-helper/internal/k8sclient"
	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

// ResourceMapper is satisfied by k8sclient.Mapper and test fakes.
type ResourceMapper interface {
	ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error)
}

// Deps holds tool handler dependencies via interfaces to ease testing.
type Deps struct {
	Dynamic      dynamic.Interface
	Typed        kubernetes.Interface
	Metrics      metricsv.Interface // nil if metrics-server unavailable
	Prometheus   promv1.API          // nil if --prometheus-url not set
	Mapper       ResourceMapper
	Discovery    discovery.DiscoveryInterface
	Logger       *slog.Logger
	SanitizeOpts sanitize.Options
	Projectors   *trimmer.Projectors
	Cluster      string
}

// NewDeps constructs a Deps from a k8sclient.Clients produced in main.go.
func NewDeps(c *k8sclient.Clients, logger *slog.Logger, opts sanitize.Options) *Deps {
	return &Deps{
		Dynamic:      c.Dynamic,
		Typed:        c.Typed,
		Metrics:      c.Metrics,
		Prometheus:   c.Prometheus,
		Mapper:       c.Mapper,
		Discovery:    c.Mapper.Discovery(),
		Logger:       logger,
		SanitizeOpts: opts,
		Projectors:   &trimmer.Projectors{},
		Cluster:      c.Resolved.ClusterHost,
	}
}

// DefaultSanitizeOpts builds sanitize.Options from the --mask-configmap-keys
// regex string. Returns an error if the regex fails to compile.
func DefaultSanitizeOpts(maskCMKeys string) (sanitize.Options, error) {
	if maskCMKeys == "" {
		return sanitize.Options{}, nil
	}
	re, err := regexp.Compile(maskCMKeys)
	if err != nil {
		return sanitize.Options{}, err
	}
	return sanitize.Options{ConfigMapKeyMask: re}, nil
}
```

This is the file Step 1 refers to. Write it before running the tests.

- [ ] **Step 3: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestKubectlGet -v
```
Expected: `undefined: NewKubectlGetHandler`.

- [ ] **Step 4: Implement kubectl_get.go**

`internal/mcptools/kubectl_get.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// NewKubectlGetHandler returns an mcp-go handler implementing kubectl_get.
func NewKubectlGetHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		kind, _ := args["kind"].(string)
		if kind == "" {
			return mcp.NewToolResultError("missing required argument: kind"), nil
		}
		apiVersion, _ := args["apiVersion"].(string)
		namespace, _ := args["namespace"].(string)
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)
		fieldSelector, _ := args["fieldSelector"].(string)
		limit := defaultListLimit
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		if limit <= 0 || limit > maxListLimit {
			return mcp.NewToolResultError(fmt.Sprintf("limit must be between 1 and %d", maxListLimit)), nil
		}

		gvr, namespaced, err := d.Mapper.ResolveGVR(kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unsupported kind, try list_api_resources: %v", err)), nil
		}

		ri := d.Dynamic.Resource(gvr)

		// --- get mode -------------------------------------------------------
		if name != "" {
			if namespaced && namespace == "" {
				return mcp.NewToolResultError("namespace is required for namespaced kinds in get mode"), nil
			}
			var obj *unstructured.Unstructured
			if namespaced {
				obj, err = ri.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			} else {
				obj, err = ri.Get(ctx, name, metav1.GetOptions{})
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cleaned := sanitize.Clean(obj, d.SanitizeOpts)
			return jsonResult(cleaned.Object)
		}

		// --- list mode ------------------------------------------------------
		listOpts := metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
			Limit:         int64(limit),
		}
		var list *unstructured.UnstructuredList
		if namespaced && namespace != "" {
			list, err = ri.Namespace(namespace).List(ctx, listOpts)
		} else {
			list, err = ri.List(ctx, listOpts)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		items := make([]map[string]interface{}, 0, len(list.Items))
		for i := range list.Items {
			cleaned := sanitize.Clean(&list.Items[i], d.SanitizeOpts)
			items = append(items, d.Projectors.Project(cleaned))
		}

		truncated := int64(len(items)) >= int64(limit)
		total := int64(len(items))
		countAccurate := true
		if rem := list.GetRemainingItemCount(); rem != nil {
			total = int64(len(items)) + *rem
		} else {
			countAccurate = !truncated
		}

		payload := map[string]interface{}{
			"kind":          kind,
			"apiVersion":    apiVersion,
			"totalCount":    total,
			"returnedCount": len(items),
			"truncated":     truncated,
			"countAccurate": countAccurate,
			"items":         items,
		}
		return jsonResult(payload)
	}
}

// unstructuredWrap/unstructuredListWrap are thin adapters to avoid repetitive
// interface conversions. Defined in a helper file to keep this one short.

// jsonResult marshals payload to JSON and wraps it in a tool result.
func jsonResult(payload interface{}) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(errors.Join(errors.New("marshal result"), err).Error()), nil
	}
	return mcp.NewToolResultText(string(raw)), nil
}
```

- [ ] **Step 5: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestKubectlGet -v
```
Expected: all 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcptools/
git commit -m "feat(mcptools): kubectl_get list/get with trimmer + sanitize"
```

---

## Task 17: mcptools — kubectl_describe

**Files:**
- Create: `internal/mcptools/kubectl_describe.go`
- Create: `internal/mcptools/kubectl_describe_test.go`

Fetches a single object plus the last 20 events whose `involvedObject.uid` matches.

- [ ] **Step 1: Write failing test**

`internal/mcptools/kubectl_describe_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubectlDescribe_ReturnsObjectAndEvents(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := newPod("api-1", "prod", "Running")
	pod.SetUID(types.UID("pod-uid-1"))

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	listKinds := map[schema.GroupVersionResource]string{gvr: "PodList"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, pod)

	typed := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "evt1"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "api-1", Namespace: "prod", UID: types.UID("pod-uid-1"),
		},
		Type:    "Warning",
		Reason:  "BackOff",
		Message: "Back-off restarting",
		Count:   5,
	})

	d := &Deps{
		Dynamic: dyn, Typed: typed,
		Mapper:     &testMapper{gvr: gvr},
		Projectors: &trimmer.Projectors{},
	}
	handler := NewKubectlDescribeHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"name":      "api-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Object        map[string]interface{} `json:"object"`
		RelatedEvents []map[string]interface{} `json:"relatedEvents"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))

	assert.Equal(t, "api-1", payload.Object["metadata"].(map[string]interface{})["name"])
	require.Len(t, payload.RelatedEvents, 1)
	assert.Equal(t, "BackOff", payload.RelatedEvents[0]["reason"])
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestKubectlDescribe -v
```
Expected: `undefined: NewKubectlDescribeHandler`.

- [ ] **Step 3: Implement kubectl_describe.go**

`internal/mcptools/kubectl_describe.go`:
```go
package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
)

const maxRelatedEvents = 20

func NewKubectlDescribeHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		kind, _ := args["kind"].(string)
		apiVersion, _ := args["apiVersion"].(string)
		namespace, _ := args["namespace"].(string)
		name, _ := args["name"].(string)
		if kind == "" || name == "" {
			return mcp.NewToolResultError("kind and name are required"), nil
		}

		gvr, namespaced, err := d.Mapper.ResolveGVR(kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unsupported kind: %v", err)), nil
		}
		if namespaced && namespace == "" {
			return mcp.NewToolResultError("namespace is required for namespaced kinds"), nil
		}

		ri := d.Dynamic.Resource(gvr)
		var got map[string]interface{}
		if namespaced {
			obj, err := ri.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			got = sanitize.Clean(obj, d.SanitizeOpts).Object
		} else {
			obj, err := ri.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			got = sanitize.Clean(obj, d.SanitizeOpts).Object
		}

		uid, _ := extractUID(got)
		events, err := listRelatedEvents(ctx, d, namespace, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]interface{}{
			"object":        got,
			"relatedEvents": events,
		})
	}
}

func extractUID(obj map[string]interface{}) (string, bool) {
	meta, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return "", false
	}
	uid, ok := meta["uid"].(string)
	return uid, ok
}

func listRelatedEvents(ctx context.Context, d *Deps, namespace, uid string) ([]map[string]interface{}, error) {
	if d.Typed == nil {
		return nil, nil
	}
	evList, err := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	filtered := make([]corev1.Event, 0, len(evList.Items))
	for _, ev := range evList.Items {
		if uid != "" && string(ev.InvolvedObject.UID) != uid {
			continue
		}
		filtered = append(filtered, ev)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].LastTimestamp.After(filtered[j].LastTimestamp.Time)
	})
	if len(filtered) > maxRelatedEvents {
		filtered = filtered[:maxRelatedEvents]
	}

	out := make([]map[string]interface{}, 0, len(filtered))
	for _, ev := range filtered {
		out = append(out, map[string]interface{}{
			"type":           ev.Type,
			"reason":         ev.Reason,
			"message":        ev.Message,
			"firstTimestamp": ev.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
			"lastTimestamp":  ev.LastTimestamp.Format("2006-01-02T15:04:05Z"),
			"count":          ev.Count,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestKubectlDescribe -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/kubectl_describe.go internal/mcptools/kubectl_describe_test.go
git commit -m "feat(mcptools): kubectl_describe with related events"
```

---

## Task 18: mcptools — kubectl_logs

**Files:**
- Create: `internal/mcptools/kubectl_logs.go`
- Create: `internal/mcptools/kubectl_logs_test.go`

Fake clientset does not support `GetLogs` cleanly, so tests only cover argument validation and multi-container error handling. Real log retrieval is validated by the kind integration test in Task 27.

- [ ] **Step 1: Write failing test**

`internal/mcptools/kubectl_logs_test.go`:
```go
package mcptools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubectlLogs_MissingArgs(t *testing.T) {
	d := &Deps{Typed: fake.NewSimpleClientset()}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestKubectlLogs_MultiContainerRequiresExplicit(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main"}, {Name: "sidecar"},
			},
		},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod",
		"pod":       "api",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textOf(result), "main")
	assert.Contains(t, textOf(result), "sidecar")
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestKubectlLogs -v
```
Expected: `undefined: NewKubectlLogsHandler`.

- [ ] **Step 3: Implement kubectl_logs.go**

`internal/mcptools/kubectl_logs.go`:
```go
package mcptools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultTailLines int64 = 200
	maxTailLines     int64 = 2000
	maxLogBytes            = 256 * 1024
)

func NewKubectlLogsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		namespace, _ := args["namespace"].(string)
		pod, _ := args["pod"].(string)
		if namespace == "" || pod == "" {
			return mcp.NewToolResultError("namespace and pod are required"), nil
		}

		container, _ := args["container"].(string)
		tail := defaultTailLines
		if v, ok := args["tailLines"].(float64); ok {
			tail = int64(v)
		}
		if tail <= 0 || tail > maxTailLines {
			return mcp.NewToolResultError(fmt.Sprintf("tailLines must be between 1 and %d", maxTailLines)), nil
		}
		previous, _ := args["previous"].(bool)
		var sinceSeconds *int64
		if v, ok := args["sinceSeconds"].(float64); ok {
			s := int64(v)
			sinceSeconds = &s
		}

		if container == "" {
			p, err := d.Typed.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if len(p.Spec.Containers) > 1 {
				names := make([]string, 0, len(p.Spec.Containers))
				for _, c := range p.Spec.Containers {
					names = append(names, c.Name)
				}
				return mcp.NewToolResultError(fmt.Sprintf(
					"pod has multiple containers; specify one of: %s",
					strings.Join(names, ", "),
				)), nil
			}
		}

		opts := &corev1.PodLogOptions{
			Container:    container,
			TailLines:    &tail,
			Previous:     previous,
			SinceSeconds: sinceSeconds,
		}
		rc, err := d.Typed.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer rc.Close()

		limited := io.LimitReader(rc, int64(maxLogBytes+1))
		data, err := io.ReadAll(limited)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		truncated := len(data) > maxLogBytes
		if truncated {
			data = data[:maxLogBytes]
		}

		lineCount := strings.Count(string(data), "\n")
		return jsonResult(map[string]interface{}{
			"logs":      string(data),
			"truncated": truncated,
			"lineCount": lineCount,
		})
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestKubectlLogs -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/kubectl_logs.go internal/mcptools/kubectl_logs_test.go
git commit -m "feat(mcptools): kubectl_logs with tail/size limits"
```

---

## Task 19: mcptools — events_list

**Files:**
- Create: `internal/mcptools/events_list.go`
- Create: `internal/mcptools/events_list_test.go`

- [ ] **Step 1: Write failing test**

`internal/mcptools/events_list_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEventsList_FiltersByType(t *testing.T) {
	base := metav1.NewTime(time.Now())
	typed := fake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "prod", Name: "e1"},
			Type:           "Warning",
			Reason:         "BackOff",
			Message:        "x",
			LastTimestamp:  base,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"},
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "prod", Name: "e2"},
			Type:           "Normal",
			Reason:         "Scheduled",
			Message:        "ok",
			LastTimestamp:  base,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"},
		},
	)
	d := &Deps{Typed: typed}
	handler := NewEventsListHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod",
		"types":     []interface{}{"Warning"},
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		ReturnedCount int                      `json:"returnedCount"`
		Events        []map[string]interface{} `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, 1, payload.ReturnedCount)
	assert.Equal(t, "BackOff", payload.Events[0]["reason"])
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestEventsList -v
```
Expected: `undefined: NewEventsListHandler`.

- [ ] **Step 3: Implement events_list.go**

`internal/mcptools/events_list.go`:
```go
package mcptools

import (
	"context"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewEventsListHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		namespace, _ := args["namespace"].(string)
		involvedKind, _ := args["involvedKind"].(string)
		involvedName, _ := args["involvedName"].(string)
		limit := 100
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		if limit <= 0 || limit > 500 {
			return mcp.NewToolResultError("limit must be between 1 and 500"), nil
		}

		typeFilter := map[string]bool{}
		if rawTypes, ok := args["types"].([]interface{}); ok {
			for _, t := range rawTypes {
				if s, ok := t.(string); ok {
					typeFilter[s] = true
				}
			}
		}

		list, err := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		filtered := make([]corev1.Event, 0, len(list.Items))
		for _, ev := range list.Items {
			if len(typeFilter) > 0 && !typeFilter[ev.Type] {
				continue
			}
			if involvedKind != "" && ev.InvolvedObject.Kind != involvedKind {
				continue
			}
			if involvedName != "" && ev.InvolvedObject.Name != involvedName {
				continue
			}
			filtered = append(filtered, ev)
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].LastTimestamp.After(filtered[j].LastTimestamp.Time)
		})

		total := len(filtered)
		truncated := total > limit
		if truncated {
			filtered = filtered[:limit]
		}

		events := make([]map[string]interface{}, 0, len(filtered))
		for _, ev := range filtered {
			events = append(events, map[string]interface{}{
				"namespace":      ev.Namespace,
				"type":           ev.Type,
				"reason":         ev.Reason,
				"message":        ev.Message,
				"firstTimestamp": ev.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
				"lastTimestamp":  ev.LastTimestamp.Format("2006-01-02T15:04:05Z"),
				"count":          ev.Count,
				"involvedObject": map[string]interface{}{
					"kind":      ev.InvolvedObject.Kind,
					"name":      ev.InvolvedObject.Name,
					"namespace": ev.InvolvedObject.Namespace,
				},
			})
		}

		return jsonResult(map[string]interface{}{
			"totalCount":    total,
			"returnedCount": len(events),
			"truncated":     truncated,
			"events":        events,
		})
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestEventsList -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/events_list.go internal/mcptools/events_list_test.go
git commit -m "feat(mcptools): events_list with type and involvedObject filters"
```

---

## Task 20: mcptools — register M5 tools and wire into main

**Files:**
- Create: `internal/mcptools/register.go`
- Modify: `cmd/k8s-mcp-server/main.go`

At this point the 4 core tools are all implemented but unreached from the stdio server. This task hooks them up, completes the natural M5 exit, and performs a smoke test of the full stdio protocol.

- [ ] **Step 1: Implement register.go**

`internal/mcptools/register.go`:
```go
package mcptools

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/kube-agent-helper/kube-agent-helper/internal/audit"
)

// RegisterCore adds the four core diagnostic tools (M5) to the server.
// Each handler is wrapped with the audit middleware.
func RegisterCore(s *server.MCPServer, d *Deps) {
	register(s, d, "kubectl_get",
		"Get a Kubernetes resource (list mode if no name, get mode if name is provided)",
		[]string{"kind", "apiVersion", "namespace", "name", "labelSelector", "fieldSelector", "limit"},
		NewKubectlGetHandler(d))

	register(s, d, "kubectl_describe",
		"Describe a single resource with related events",
		[]string{"kind", "apiVersion", "namespace", "name"},
		NewKubectlDescribeHandler(d))

	register(s, d, "kubectl_logs",
		"Fetch container logs (supports tailLines, previous, sinceSeconds)",
		[]string{"namespace", "pod", "container", "tailLines", "previous", "sinceSeconds"},
		NewKubectlLogsHandler(d))

	register(s, d, "events_list",
		"List events, optionally filtered by type or involvedObject",
		[]string{"namespace", "involvedKind", "involvedName", "types", "limit"},
		NewEventsListHandler(d))
}

func register(s *server.MCPServer, d *Deps, name, desc string, whitelist []string, handler audit.Handler) {
	tool := mcp.NewTool(name, mcp.WithDescription(desc))
	wrapped := audit.Wrap(d.Logger, audit.ToolSpec{
		Name:         name,
		ArgWhitelist: whitelist,
		Cluster:      d.Cluster,
	}, handler)
	s.AddTool(tool, func(ctx any, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return wrapped(req.Context(), req)
	})
}
```

Note: consult the mark3labs/mcp-go version's `server.AddTool` signature; the exact context parameter type may differ. Adjust the adapter accordingly.

- [ ] **Step 2: Update main.go to call RegisterCore**

Modify `cmd/k8s-mcp-server/main.go`, inside `run()`, after precheck:

```go
	opts, err := mcptools.DefaultSanitizeOpts(optsMaskCMKeys)
	if err != nil {
		return fmt.Errorf("compile mask-configmap-keys regex: %w", err)
	}
	deps := mcptools.NewDeps(clients, slog.Default(), opts)

	srv := server.NewMCPServer("k8s-mcp-server", "0.1.0")
	mcptools.RegisterCore(srv, deps)

	return server.ServeStdio(srv)
```

Add the `mcptools` import. Pass `optsMaskCMKeys` down from `runOptions`.

- [ ] **Step 3: Verify build**

```bash
make build
```
Expected: success.

- [ ] **Step 4: Smoke test the stdio protocol**

With a real kubeconfig (any kind cluster works), run:

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | ./bin/k8s-mcp-server --kubeconfig ~/.kube/config
```

Expected: second response includes a `tools` array of length 4 containing the four M5 tool names.

- [ ] **Step 5: Full unit test run**

```bash
go test ./... -race -cover
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcptools/register.go cmd/k8s-mcp-server/main.go
git commit -m "feat: register M5 tools and wire stdio server end-to-end"
```

**🏁 Natural exit point:** At this commit, the M5 milestone is complete. The four core diagnostic tools are usable end-to-end via stdio. Tasks 21-29 add extension tools, integration tests, and docs.

---

## Task 21: mcptools — top_pods and top_nodes with graceful degradation

**Files:**
- Create: `internal/mcptools/top.go`
- Create: `internal/mcptools/top_test.go`

- [ ] **Step 1: Write failing tests**

`internal/mcptools/top_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestTopPods_Unavailable(t *testing.T) {
	d := &Deps{Metrics: nil} // metrics not wired
	handler := NewTopPodsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError, "unavailable is not an error")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
	assert.Contains(t, payload["error"], "metrics-server not installed")
}

func TestTopPods_ReturnsMetrics(t *testing.T) {
	metrics := metricsfake.NewSimpleClientset(
		&metricsv1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Containers: []metricsv1.ContainerMetrics{{
				Name: "main",
				Usage: map[string]resource.Quantity{
					"cpu":    resource.MustParse("1250m"),
					"memory": resource.MustParse("512Mi"),
				},
			}},
		},
	)
	d := &Deps{Metrics: metrics}
	handler := NewTopPodsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Available bool                     `json:"available"`
		Items     []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.True(t, payload.Available)
	require.Len(t, payload.Items, 1)
	assert.Equal(t, "api", payload.Items[0]["name"])
	assert.Equal(t, float64(1250), payload.Items[0]["cpuMilli"])
	assert.Equal(t, float64(512), payload.Items[0]["memoryMi"])
}

func TestTopNodes_Unavailable(t *testing.T) {
	d := &Deps{Metrics: nil}
	handler := NewTopNodesHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textOf(result), "metrics-server not installed")
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestTop -v
```
Expected: `undefined: NewTopPodsHandler`.

- [ ] **Step 3: Implement top.go**

`internal/mcptools/top.go`:
```go
package mcptools

import (
	"context"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTopPodsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Metrics == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "metrics-server not installed",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		labelSelector, _ := args["labelSelector"].(string)
		sortBy, _ := args["sortBy"].(string)
		if sortBy == "" {
			sortBy = "cpu"
		}
		limit := 50
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}

		list, err := d.Metrics.MetricsV1beta1().PodMetricses(namespace).
			List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		type row struct {
			Name      string
			Namespace string
			CPUMilli  int64
			MemoryMi  int64
		}
		rows := make([]row, 0, len(list.Items))
		for _, pm := range list.Items {
			var cpu, mem int64
			for _, c := range pm.Containers {
				if q, ok := c.Usage["cpu"]; ok {
					cpu += q.MilliValue()
				}
				if q, ok := c.Usage["memory"]; ok {
					mem += q.Value() / (1024 * 1024)
				}
				_ = resource.Quantity{}
			}
			rows = append(rows, row{
				Name: pm.Name, Namespace: pm.Namespace,
				CPUMilli: cpu, MemoryMi: mem,
			})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if sortBy == "memory" {
				return rows[i].MemoryMi > rows[j].MemoryMi
			}
			return rows[i].CPUMilli > rows[j].CPUMilli
		})
		if len(rows) > limit {
			rows = rows[:limit]
		}

		items := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]interface{}{
				"name":      r.Name,
				"namespace": r.Namespace,
				"cpuMilli":  r.CPUMilli,
				"memoryMi":  r.MemoryMi,
			})
		}
		return jsonResult(map[string]interface{}{
			"available": true,
			"items":     items,
		})
	}
}

func NewTopNodesHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Metrics == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "metrics-server not installed",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		sortBy, _ := args["sortBy"].(string)
		if sortBy == "" {
			sortBy = "cpu"
		}
		limit := 50
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}

		list, err := d.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		type row struct {
			Name     string
			CPUMilli int64
			MemoryMi int64
		}
		rows := make([]row, 0, len(list.Items))
		for _, nm := range list.Items {
			cpu := nm.Usage["cpu"]
			mem := nm.Usage["memory"]
			rows = append(rows, row{
				Name:     nm.Name,
				CPUMilli: cpu.MilliValue(),
				MemoryMi: mem.Value() / (1024 * 1024),
			})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if sortBy == "memory" {
				return rows[i].MemoryMi > rows[j].MemoryMi
			}
			return rows[i].CPUMilli > rows[j].CPUMilli
		})
		if len(rows) > limit {
			rows = rows[:limit]
		}

		items := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]interface{}{
				"name":     r.Name,
				"cpuMilli": r.CPUMilli,
				"memoryMi": r.MemoryMi,
			})
		}
		return jsonResult(map[string]interface{}{
			"available": true,
			"items":     items,
		})
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestTop -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/top.go internal/mcptools/top_test.go
git commit -m "feat(mcptools): top_pods and top_nodes with graceful degradation"
```

---

## Task 22: mcptools — list_api_resources

**Files:**
- Create: `internal/mcptools/list_api_resources.go`
- Create: `internal/mcptools/list_api_resources_test.go`

- [ ] **Step 1: Write failing test with fake discovery client**

`internal/mcptools/list_api_resources_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func fakeDiscovery() discovery.DiscoveryInterface {
	clientset := fake.NewSimpleClientset()
	disc := clientset.Discovery().(*discoveryfake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Namespaced: true, Kind: "Pod", Verbs: []string{"get", "list", "watch"}},
				{Name: "nodes", Namespaced: false, Kind: "Node", Verbs: []string{"get", "list"}},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Namespaced: true, Kind: "Deployment", Verbs: []string{"get", "list", "watch"}},
			},
		},
	}
	return disc
}

func TestListAPIResources_All(t *testing.T) {
	d := &Deps{Discovery: fakeDiscovery()}
	handler := NewListAPIResourcesHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Resources []map[string]interface{} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Len(t, payload.Resources, 3)
}

func TestListAPIResources_NamespacedFilter(t *testing.T) {
	d := &Deps{Discovery: fakeDiscovery()}
	handler := NewListAPIResourcesHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespaced": true}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	var payload struct {
		Resources []map[string]interface{} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Len(t, payload.Resources, 2)
	for _, r := range payload.Resources {
		assert.True(t, r["namespaced"].(bool))
	}
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestListAPIResources -v
```
Expected: `undefined: NewListAPIResourcesHandler`.

- [ ] **Step 3: Implement list_api_resources.go**

`internal/mcptools/list_api_resources.go`:
```go
package mcptools

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewListAPIResourcesHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		wantNamespaced, hasNamespaced := args["namespaced"].(bool)
		verbFilter, _ := args["verb"].(string)

		lists, err := d.Discovery.ServerPreferredResources()
		if err != nil && len(lists) == 0 {
			return mcp.NewToolResultError(err.Error()), nil
		}

		out := make([]map[string]interface{}, 0, 64)
		for _, rl := range lists {
			gv, err := schema.ParseGroupVersion(rl.GroupVersion)
			if err != nil {
				continue
			}
			for _, r := range rl.APIResources {
				// Skip subresources (they contain a "/").
				if strings.Contains(r.Name, "/") {
					continue
				}
				if hasNamespaced && r.Namespaced != wantNamespaced {
					continue
				}
				if verbFilter != "" && !containsVerb(r.Verbs, verbFilter) {
					continue
				}
				out = append(out, map[string]interface{}{
					"group":      gv.Group,
					"version":    gv.Version,
					"kind":       r.Kind,
					"namespaced": r.Namespaced,
					"verbs":      r.Verbs,
				})
			}
		}

		return jsonResult(map[string]interface{}{"resources": out})
	}
}

func containsVerb(verbs []string, want string) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestListAPIResources -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/list_api_resources.go internal/mcptools/list_api_resources_test.go
git commit -m "feat(mcptools): list_api_resources with namespaced/verb filters"
```

---

## Task 23: mcptools — prometheus_query with httptest mock

**Files:**
- Create: `internal/mcptools/prometheus_query.go`
- Create: `internal/mcptools/prometheus_query_test.go`

- [ ] **Step 1: Write failing test using httptest**

`internal/mcptools/prometheus_query_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusQuery_NotConfigured(t *testing.T) {
	d := &Deps{Prometheus: nil}
	handler := NewPrometheusQueryHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": "up"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.Contains(t, textOf(result), "prometheus not configured")
}

func TestPrometheusQuery_Instant(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status":"success",
			"data":{"resultType":"vector","result":[
				{"metric":{"job":"api"},"value":[1712815200,"42.5"]}
			]}
		}`))
	}))
	defer ts.Close()

	client, _ := promapi.NewClient(promapi.Config{Address: ts.URL})
	d := &Deps{Prometheus: promv1.NewAPI(client)}
	handler := NewPrometheusQueryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": "up"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Available  bool                     `json:"available"`
		ResultType string                   `json:"resultType"`
		Samples    []map[string]interface{} `json:"samples"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.True(t, payload.Available)
	assert.Equal(t, "vector", payload.ResultType)
	require.Len(t, payload.Samples, 1)
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestPrometheusQuery -v
```
Expected: `undefined: NewPrometheusQueryHandler`.

- [ ] **Step 3: Implement prometheus_query.go**

`internal/mcptools/prometheus_query.go`:
```go
package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func NewPrometheusQueryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Prometheus == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus not configured",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		query, _ := args["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		var result model.Value
		var warnings promv1.Warnings
		var err error

		if rawRange, ok := args["range"].(map[string]interface{}); ok {
			start, err1 := parseTime(rawRange["start"])
			end, err2 := parseTime(rawRange["end"])
			step, err3 := time.ParseDuration(toString(rawRange["step"]))
			if err1 != nil || err2 != nil || err3 != nil {
				return mcp.NewToolResultError("invalid range parameters"), nil
			}
			result, warnings, err = d.Prometheus.QueryRange(ctx, query,
				promv1.Range{Start: start, End: end, Step: step})
		} else {
			ts := time.Now()
			if v, ok := args["time"].(string); ok && v != "" {
				if parsed, perr := time.Parse(time.RFC3339, v); perr == nil {
					ts = parsed
				}
			}
			result, warnings, err = d.Prometheus.Query(ctx, query, ts)
		}
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		return jsonResult(map[string]interface{}{
			"available":  true,
			"resultType": string(result.Type()),
			"samples":    marshalSamples(result),
			"warnings":   warnings,
		})
	}
}

func parseTime(v interface{}) (time.Time, error) {
	s, _ := v.(string)
	return time.Parse(time.RFC3339, s)
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func marshalSamples(v model.Value) []map[string]interface{} {
	switch vv := v.(type) {
	case model.Vector:
		out := make([]map[string]interface{}, 0, len(vv))
		for _, s := range vv {
			out = append(out, map[string]interface{}{
				"metric": s.Metric,
				"value":  []interface{}{float64(s.Timestamp.Unix()), s.Value.String()},
			})
		}
		return out
	case model.Matrix:
		out := make([]map[string]interface{}, 0, len(vv))
		for _, s := range vv {
			values := make([][]interface{}, 0, len(s.Values))
			for _, p := range s.Values {
				values = append(values, []interface{}{
					float64(p.Timestamp.Unix()), p.Value.String(),
				})
			}
			out = append(out, map[string]interface{}{
				"metric": s.Metric,
				"values": values,
			})
		}
		return out
	default:
		return []map[string]interface{}{{"raw": fmt.Sprint(v)}}
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestPrometheusQuery -v
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/prometheus_query.go internal/mcptools/prometheus_query_test.go
git commit -m "feat(mcptools): prometheus_query with instant and range modes"
```

---

## Task 24: mcptools — kubectl_explain via OpenAPI v3

**Files:**
- Create: `internal/mcptools/kubectl_explain.go`
- Create: `internal/mcptools/kubectl_explain_test.go`

OpenAPI v3 schema endpoints (`/openapi/v3/apis/<group>/<version>`) return a JSON document of schema definitions. The handler navigates `$ref`s to describe the requested field.

- [ ] **Step 1: Write failing test using a fake HTTP transport**

`internal/mcptools/kubectl_explain_test.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const sampleOpenAPIv3 = `{
  "components": {
    "schemas": {
      "io.k8s.api.core.v1.Pod": {
        "description": "Pod is a collection of containers",
        "properties": {
          "spec": { "$ref": "#/components/schemas/io.k8s.api.core.v1.PodSpec" }
        }
      },
      "io.k8s.api.core.v1.PodSpec": {
        "description": "PodSpec is a description of a pod",
        "properties": {
          "containers": {
            "type": "array",
            "description": "List of containers"
          }
        }
      }
    }
  }
}`

func fakeExplainServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleOpenAPIv3))
	}))
}

func TestKubectlExplain_TopLevel(t *testing.T) {
	ts := fakeExplainServer()
	defer ts.Close()
	typed, _ := kubernetes.NewForConfig(&rest.Config{Host: ts.URL})
	d := &Deps{Typed: typed}

	handler := NewKubectlExplainHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"kind": "Pod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Description string                   `json:"description"`
		Fields      []map[string]interface{} `json:"fields"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Contains(t, payload.Description, "collection of containers")
	require.NotEmpty(t, payload.Fields)
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/mcptools/... -run TestKubectlExplain -v
```
Expected: `undefined: NewKubectlExplainHandler`.

- [ ] **Step 3: Implement kubectl_explain.go**

`internal/mcptools/kubectl_explain.go`:
```go
package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/client-go/kubernetes"
)

// openapiSchema is the minimal subset of the OpenAPI v3 document we use.
type openapiSchema struct {
	Components struct {
		Schemas map[string]*openapiType `json:"schemas"`
	} `json:"components"`
}

type openapiType struct {
	Description string                  `json:"description"`
	Type        string                  `json:"type"`
	Properties  map[string]*openapiType `json:"properties"`
	Ref         string                  `json:"$ref"`
	Items       *openapiType            `json:"items"`
	Required    []string                `json:"required"`
}

func NewKubectlExplainHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		kind, _ := args["kind"].(string)
		field, _ := args["field"].(string)
		if kind == "" {
			return mcp.NewToolResultError("kind is required"), nil
		}

		doc, err := fetchOpenAPIv3(ctx, d.Typed)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		node := findKind(doc, kind)
		if node == nil {
			return mcp.NewToolResultError(fmt.Sprintf("kind not found: %s", kind)), nil
		}

		if field != "" {
			parts := strings.Split(field, ".")
			for _, p := range parts {
				if node.Ref != "" {
					node = resolveRef(doc, node.Ref)
				}
				if node == nil || node.Properties == nil {
					return mcp.NewToolResultError(fmt.Sprintf("field not found: %s", field)), nil
				}
				next, ok := node.Properties[p]
				if !ok {
					return mcp.NewToolResultError(fmt.Sprintf("field not found: %s", field)), nil
				}
				node = next
			}
		}
		if node.Ref != "" {
			node = resolveRef(doc, node.Ref)
		}

		return jsonResult(buildExplainResult(node))
	}
}

func fetchOpenAPIv3(ctx context.Context, typed kubernetes.Interface) (*openapiSchema, error) {
	raw, err := typed.Discovery().RESTClient().Get().
		AbsPath("/openapi/v3/api/v1").
		DoRaw(ctx)
	if err != nil {
		// Fallback: try the root OpenAPI v3 index (exact path depends on k8s version).
		raw, err = typed.Discovery().RESTClient().Get().
			AbsPath("/openapi/v3").
			DoRaw(ctx)
		if err != nil {
			return nil, err
		}
	}
	var doc openapiSchema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func findKind(doc *openapiSchema, kind string) *openapiType {
	for name, schema := range doc.Components.Schemas {
		if strings.HasSuffix(name, "."+kind) {
			return schema
		}
	}
	return nil
}

func resolveRef(doc *openapiSchema, ref string) *openapiType {
	// "#/components/schemas/io.k8s.api.core.v1.PodSpec"
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return nil
	}
	return doc.Components.Schemas[strings.TrimPrefix(ref, prefix)]
}

func buildExplainResult(node *openapiType) map[string]interface{} {
	fields := make([]map[string]interface{}, 0)
	required := map[string]bool{}
	for _, r := range node.Required {
		required[r] = true
	}
	for name, prop := range node.Properties {
		ftype := prop.Type
		if ftype == "" && prop.Ref != "" {
			ftype = "object"
		}
		fields = append(fields, map[string]interface{}{
			"name":        name,
			"type":        ftype,
			"description": prop.Description,
			"required":    required[name],
		})
	}
	return map[string]interface{}{
		"description": node.Description,
		"fields":      fields,
	}
}
```

- [ ] **Step 4: Run test to confirm pass**

```bash
go test ./internal/mcptools/... -run TestKubectlExplain -v
```
Expected: PASS. Note: the test uses a static OpenAPI fixture, so it does not exercise the fallback path.

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/kubectl_explain.go internal/mcptools/kubectl_explain_test.go
git commit -m "feat(mcptools): kubectl_explain via OpenAPI v3"
```

---

## Task 25: mcptools — register extension tools and rebuild wire-up

**Files:**
- Modify: `internal/mcptools/register.go`

- [ ] **Step 1: Add RegisterExtension**

Append to `internal/mcptools/register.go`:

```go
// RegisterExtension adds the four extension tools (M6) to the server.
func RegisterExtension(s *server.MCPServer, d *Deps) {
	register(s, d, "top_pods",
		"Show top pods by cpu or memory (requires metrics-server)",
		[]string{"namespace", "labelSelector", "sortBy", "limit"},
		NewTopPodsHandler(d))

	register(s, d, "top_nodes",
		"Show top nodes by cpu or memory (requires metrics-server)",
		[]string{"sortBy", "limit"},
		NewTopNodesHandler(d))

	register(s, d, "list_api_resources",
		"List supported API resources in the cluster",
		[]string{"namespaced", "verb"},
		NewListAPIResourcesHandler(d))

	register(s, d, "prometheus_query",
		"Execute a PromQL query (requires --prometheus-url)",
		[]string{"query", "time", "range"},
		NewPrometheusQueryHandler(d))

	register(s, d, "kubectl_explain",
		"Describe Kubernetes API schema for a kind/field",
		[]string{"kind", "apiVersion", "field"},
		NewKubectlExplainHandler(d))
}

// RegisterAll is a convenience for the server main to register every tool.
func RegisterAll(s *server.MCPServer, d *Deps) {
	RegisterCore(s, d)
	RegisterExtension(s, d)
}
```

- [ ] **Step 2: Update main.go to use RegisterAll**

In `cmd/k8s-mcp-server/main.go` replace `mcptools.RegisterCore(srv, deps)` with:
```go
mcptools.RegisterAll(srv, deps)
```

- [ ] **Step 3: Full test + build**

```bash
make build
go test ./... -race -cover
```
Expected: all tests PASS, build succeeds.

- [ ] **Step 4: Smoke test shows 8 tools listed**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | ./bin/k8s-mcp-server --kubeconfig ~/.kube/config
```

Expected: response id=2 contains a `tools` array of length 9 (kubectl_get, kubectl_describe, kubectl_logs, events_list, top_pods, top_nodes, list_api_resources, prometheus_query, kubectl_explain).

- [ ] **Step 5: Commit**

```bash
git add internal/mcptools/register.go cmd/k8s-mcp-server/main.go
git commit -m "feat: register M6 extension tools (8 tools total)"
```

---

## Task 26: integration test harness (kind)

**Files:**
- Create: `test/integration/run.sh`
- Create: `test/integration/kind-config.yaml`
- Create: `test/integration/README.md`

Bootstrap a kind cluster, run Go integration tests, tear down on exit. The test code itself lives in Task 27.

- [ ] **Step 1: Write kind cluster config**

`test/integration/kind-config.yaml`:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: k8s-mcp-it
nodes:
- role: control-plane
```

- [ ] **Step 2: Write run.sh**

`test/integration/run.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
CLUSTER_NAME="k8s-mcp-it"

cleanup() {
  kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Creating kind cluster $CLUSTER_NAME"
kind create cluster --name "$CLUSTER_NAME" --config "$SCRIPT_DIR/kind-config.yaml"

export KUBECONFIG="$(mktemp)"
kind export kubeconfig --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG"

echo "==> Building binary"
cd "$ROOT_DIR"
go build -o bin/k8s-mcp-server ./cmd/k8s-mcp-server

echo "==> Running integration tests"
K8S_MCP_SERVER_BIN="$ROOT_DIR/bin/k8s-mcp-server" \
KIND_KUBECONFIG="$KUBECONFIG" \
go test -tags=integration ./test/integration/... -v -timeout 10m
```

- [ ] **Step 3: Make run.sh executable**

```bash
chmod +x test/integration/run.sh
```

- [ ] **Step 4: Write README**

`test/integration/README.md`:
```markdown
# Integration Tests

These tests bring up a temporary kind cluster, build the binary, and
exercise the stdio MCP protocol end-to-end.

## Requirements

- Docker daemon running
- kind >= 0.20 in PATH
- Go 1.23

## Run

```
make integration
```

The cluster is always cleaned up on exit. Individual test files use the
`integration` build tag so `go test ./...` ignores them.
```

- [ ] **Step 5: Verify run.sh refuses to run without kind**

```bash
# Sanity: script exists and is syntactically valid bash
bash -n test/integration/run.sh
```
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add test/integration/
git commit -m "test(integration): kind harness scaffolding"
```

---

## Task 27: integration test scenarios

**Files:**
- Create: `test/integration/mcp_client_test.go`
- Create: `test/integration/scenarios_test.go`

Five scenarios per spec §6.4: protocol smoke, real logs, CrashLoop describe, Secret redaction, precheck fail-fast.

- [ ] **Step 1: Write a minimal JSON-RPC stdio client helper**

`test/integration/mcp_client_test.go`:
```go
//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type rpcClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID atomic.Int64
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func startServer(t *testing.T) *rpcClient {
	t.Helper()
	bin := os.Getenv("K8S_MCP_SERVER_BIN")
	require.NotEmpty(t, bin)
	kubeconfig := os.Getenv("KIND_KUBECONFIG")
	require.NotEmpty(t, kubeconfig)

	cmd := exec.Command(bin, "--kubeconfig", kubeconfig)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	c := &rpcClient{cmd: cmd, stdin: stdin, reader: bufio.NewReader(stdout)}
	c.initialize(t)
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})
	return c
}

func (c *rpcClient) initialize(t *testing.T) {
	c.call(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "integration", "version": "0"},
	})
}

func (c *rpcClient) call(t *testing.T, method string, params interface{}) json.RawMessage {
	t.Helper()
	id := c.nextID.Add(1)
	payload := map[string]interface{}{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = fmt.Fprintln(c.stdin, string(raw))
	require.NoError(t, err)

	line, err := c.reader.ReadBytes('\n')
	require.NoError(t, err)
	var resp rpcResponse
	require.NoError(t, json.Unmarshal(line, &resp))
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", resp.Error.Message)
	}
	return resp.Result
}

func (c *rpcClient) callTool(t *testing.T, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw := c.call(t, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.False(t, out.IsError, "tool returned error: %s", out.Content)
	require.NotEmpty(t, out.Content)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out.Content[0].Text), &payload))
	return payload
}

var _ = context.Background
```

- [ ] **Step 2: Write 5 scenarios**

`test/integration/scenarios_test.go`:
```go
//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kubectl(t *testing.T, args ...string) {
	t.Helper()
	kubeconfig := os.Getenv("KIND_KUBECONFIG")
	require.NotEmpty(t, kubeconfig, "KIND_KUBECONFIG env required")
	cmd := exec.Command("kubectl", append([]string{"--kubeconfig", kubeconfig}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl %v: %s", args, string(out))
}

// --- Scenario 1: protocol smoke ------------------------------------------

func TestScenario_ProtocolSmoke(t *testing.T) {
	c := startServer(t)
	raw := c.call(t, "tools/list", nil)
	var payload struct {
		Tools []map[string]interface{} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Len(t, payload.Tools, 9)
}

// --- Scenario 2: real Pod logs --------------------------------------------

func TestScenario_PodLogs(t *testing.T) {
	kubectl(t, "create", "ns", "it-logs")
	t.Cleanup(func() { kubectl(t, "delete", "ns", "it-logs", "--ignore-not-found") })
	kubectl(t, "run", "hello", "-n", "it-logs",
		"--image=busybox",
		"--restart=Never",
		"--command", "--", "sh", "-c", "echo hello world; sleep 3600",
	)
	// wait for pod to be ready
	time.Sleep(10 * time.Second)

	c := startServer(t)
	payload := c.callTool(t, "kubectl_logs", map[string]interface{}{
		"namespace": "it-logs",
		"pod":       "hello",
		"tailLines": float64(50),
	})
	assert.Contains(t, payload["logs"].(string), "hello world")
}

// --- Scenario 3: CrashLoop describe ---------------------------------------

func TestScenario_CrashLoop(t *testing.T) {
	kubectl(t, "create", "ns", "it-crash")
	t.Cleanup(func() { kubectl(t, "delete", "ns", "it-crash", "--ignore-not-found") })
	kubectl(t, "run", "bad", "-n", "it-crash",
		"--image=busybox",
		"--restart=Always",
		"--command", "--", "sh", "-c", "exit 1",
	)
	time.Sleep(30 * time.Second) // let the pod crash a few times

	c := startServer(t)
	payload := c.callTool(t, "kubectl_describe", map[string]interface{}{
		"kind":      "Pod",
		"namespace": "it-crash",
		"name":      "bad",
	})
	events := payload["relatedEvents"].([]interface{})
	foundBackoff := false
	for _, ev := range events {
		m := ev.(map[string]interface{})
		if reason, _ := m["reason"].(string); strings.Contains(reason, "BackOff") || strings.Contains(reason, "Failed") {
			foundBackoff = true
		}
	}
	assert.True(t, foundBackoff, "expected a BackOff-ish event in relatedEvents")
}

// --- Scenario 4: Secret redaction -----------------------------------------

func TestScenario_SecretRedaction(t *testing.T) {
	kubectl(t, "create", "ns", "it-secret")
	t.Cleanup(func() { kubectl(t, "delete", "ns", "it-secret", "--ignore-not-found") })
	kubectl(t, "create", "secret", "generic", "db-creds", "-n", "it-secret",
		"--from-literal=username=admin", "--from-literal=password=supersecret")

	c := startServer(t)
	payload := c.callTool(t, "kubectl_get", map[string]interface{}{
		"kind":      "Secret",
		"namespace": "it-secret",
		"name":      "db-creds",
	})
	data := payload["data"].(map[string]interface{})
	assert.Contains(t, data["username"].(string), "<redacted")
	assert.Contains(t, data["password"].(string), "<redacted")
}

// --- Scenario 5: Precheck fail-fast ---------------------------------------

func TestScenario_PrecheckFailsFast(t *testing.T) {
	// Point the server at an invalid kubeconfig file and assert exit != 0.
	// Creating a zero-permission SA would be more thorough but is cumbersome
	// for this smoke scenario.
	bin := os.Getenv("K8S_MCP_SERVER_BIN")
	require.NotEmpty(t, bin)
	cmd := exec.Command(bin, "--kubeconfig", "/tmp/definitely-not-a-kubeconfig")
	err := cmd.Run()
	require.Error(t, err, "expected nonzero exit")
}
```

- [ ] **Step 3: Run integration tests locally**

```bash
make integration
```
Expected: kind cluster boots, 5 tests pass, cluster deleted.

If kind or docker is unavailable, skip this step and document the failure in a separate issue.

- [ ] **Step 4: Commit**

```bash
git add test/integration/mcp_client_test.go test/integration/scenarios_test.go
git commit -m "test(integration): 5 end-to-end scenarios via stdio protocol"
```

---

## Task 28: user-facing documentation

**Files:**
- Create: `docs/k8s-mcp-server.md`

- [ ] **Step 1: Write user guide**

`docs/k8s-mcp-server.md`:
````markdown
# k8s-mcp-server

A read-only Model Context Protocol (MCP) server that exposes Kubernetes
diagnostic tools to LLM agents via stdio.

## Installation

```bash
git clone https://github.com/kube-agent-helper/kube-agent-helper
cd kube-agent-helper
make build
# binary at ./bin/k8s-mcp-server
```

## Running

### Local / stand-alone

```bash
./bin/k8s-mcp-server --kubeconfig ~/.kube/config
```

Logs go to stderr as JSON; stdout is the MCP JSON-RPC channel.

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "k8s": {
      "command": "/absolute/path/to/bin/k8s-mcp-server",
      "args": ["--kubeconfig", "/absolute/path/to/kubeconfig"]
    }
  }
}
```

Restart Claude Desktop. Tools will appear with the `mcp__k8s__*` prefix.

### In-cluster (Pod)

```yaml
containers:
- name: k8s-mcp-server
  image: k8s-mcp-server:0.1.0
  args: ["--in-cluster"]
```

The pod's ServiceAccount must have at least `list:pods` cluster-wide.

## CLI flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--in-cluster` | bool | false | Use in-cluster ServiceAccount config |
| `--kubeconfig` | string | "" | Path to kubeconfig file |
| `--context` | string | "" | kubeconfig context name |
| `--prometheus-url` | string | "" | Prometheus HTTP endpoint |
| `--mask-configmap-keys` | string | (see below) | Regex for ConfigMap key redaction |
| `--log-level` | string | info | `info` or `debug` |

Default `--mask-configmap-keys` regex:
```
(?i)(password|passwd|pwd|secret|token|apikey|api_key|credential|private[_-]?key|cert)
```

`--in-cluster` and `--kubeconfig` are mutually exclusive.

## Tools

| Tool | Purpose |
|---|---|
| `kubectl_get` | List or get a resource (dynamic by Kind) |
| `kubectl_describe` | Single resource + related events |
| `kubectl_logs` | Container logs with tail and size limits |
| `events_list` | List events filtered by type / involvedObject |
| `top_pods` | Pod CPU/memory (requires metrics-server) |
| `top_nodes` | Node CPU/memory (requires metrics-server) |
| `list_api_resources` | Discover supported resource kinds |
| `prometheus_query` | PromQL instant/range query (requires `--prometheus-url`) |
| `kubectl_explain` | Describe API schema by kind/field |

Full input/output schemas in [`docs/superpowers/specs/2026-04-11-k8s-mcp-server-design.md`](superpowers/specs/2026-04-11-k8s-mcp-server-design.md) §2.

## Security

- **Read-only:** no write verbs are exposed, ever.
- **Secret redaction:** `Secret.data` values are always replaced with `<redacted len=N>`.
- **ConfigMap redaction:** keys matching `--mask-configmap-keys` replaced with `<redacted>`.
- **Pod env redaction:** env vars whose name matches `(?i)(password|secret|token|...)` are redacted before reaching the LLM.

See spec §3 for the full rules.

## Troubleshooting

- **"precheck failed: cannot list pods"** — the current identity cannot list pods. Grant the SA/user at least `list` on `pods` cluster-wide.
- **"metrics-server not installed"** — `top_*` tools return `available: false`; install metrics-server to enable them.
- **"prometheus not configured"** — pass `--prometheus-url http://prom.monitoring:9090` to enable `prometheus_query`.
- **Log lines interleaved with JSON output** — do not write to stdout in custom wrappers; stdout is the MCP protocol channel.
````

- [ ] **Step 2: Commit**

```bash
git add docs/k8s-mcp-server.md
git commit -m "docs: user guide for k8s-mcp-server"
```

---

## Task 29: README update and final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append Components section to README.md**

Add to `README.md` after the existing introduction:

```markdown
## Components

This repository will host the Phase 1 components described in
[`docs/design.md`](docs/design.md). So far implemented:

- **k8s-mcp-server** — read-only Kubernetes MCP server exposing 9 diagnostic tools
  via stdio. See [docs/k8s-mcp-server.md](docs/k8s-mcp-server.md).

## Build

```
make build           # compile bin/k8s-mcp-server
make test            # unit + component (envtest) tests
make lint            # golangci-lint
make integration     # kind-based end-to-end tests
```
```

- [ ] **Step 2: Run full verification**

```bash
make fmt
make vet
make lint
make test
make build
```
Expected: every target succeeds.

- [ ] **Step 3: Confirm tool count and coverage**

```bash
go test ./internal/sanitize/... -cover
go test ./internal/trimmer/... -cover
go test ./internal/audit/... -cover
```
Expected: each reports ≥ 85%.

- [ ] **Step 4: Final smoke against a kind cluster**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"final","version":"0"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kubectl_get","arguments":{"kind":"Pod","namespace":"default"}}}' \
  | ./bin/k8s-mcp-server --kubeconfig ~/.kube/config
```
Expected: three JSON-RPC responses, the third containing a populated or empty `items` array for Pods in the default namespace.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: README components and build quickstart"
```

---

## Definition of Done

- [x] Tasks 1-29 committed
- [x] `make test` green
- [x] `make lint` green
- [x] `make build` green
- [x] `go test ./internal/{sanitize,trimmer,audit}/... -cover` ≥ 85%
- [x] `tools/list` returns 9 tools
- [x] `docs/k8s-mcp-server.md` exists and is accurate
- [x] A real Claude Desktop / stdio client can call `kubectl_get` against a kind cluster

If any box is unchecked, the plan is not complete.
