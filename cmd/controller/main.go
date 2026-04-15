package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

var (
	dbPath           string
	httpAddr         string
	agentImage       string
	controllerURL    string
	skillsDir        string
	anthropicBaseURL string
	model            string
)

func main() {
	flag.StringVar(&dbPath, "db", "/data/kube-agent-helper.db", "SQLite database path")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP server listen address")
	flag.StringVar(&agentImage, "agent-image", "ghcr.io/kube-agent-helper/agent-runtime:latest", "Agent Pod image")
	flag.StringVar(&controllerURL, "controller-url", "http://controller.kube-agent-helper.svc:8080", "Controller URL for Agent callbacks")
	flag.StringVar(&skillsDir, "skills-dir", "/skills", "Directory containing built-in SKILL.md files")
	flag.StringVar(&anthropicBaseURL, "anthropic-base-url", "", "Anthropic API base URL (empty = default)")
	flag.StringVar(&model, "model", "", "LLM model name (empty = agent default)")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		slog.Error("add clientgo scheme", "error", err)
		os.Exit(1)
	}
	if err := k8saiV1.AddToScheme(scheme); err != nil {
		slog.Error("add k8sai scheme", "error", err)
		os.Exit(1)
	}

	// Open DB
	st, err := sqlitestore.New(dbPath)
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Load built-in skills from the skills directory into DB
	if err := loadBuiltinSkills(context.Background(), st, skillsDir); err != nil {
		slog.Error("load builtin skills", "error", err)
		os.Exit(1)
	}

	// Manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		Metrics: metricsserver.Options{BindAddress: ":9090"},
	})
	if err != nil {
		slog.Error("new manager", "error", err)
		os.Exit(1)
	}

	// Create skill registry (reads from store on every call — hot-reload)
	reg := registry.New(st)

	tr := translator.New(translator.Config{
		AgentImage:       agentImage,
		ControllerURL:    controllerURL,
		AnthropicBaseURL: anthropicBaseURL,
		Model:            model,
	}, reg)

	if err := (&reconciler.DiagnosticRunReconciler{
		Client:     mgr.GetClient(),
		Store:      st,
		Translator: tr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup reconciler", "error", err)
		os.Exit(1)
	}

	if err := (&reconciler.DiagnosticSkillReconciler{
		Client: mgr.GetClient(),
		Store:  st,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup skill reconciler", "error", err)
		os.Exit(1)
	}

	if err := (&reconciler.ModelConfigReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup modelconfig reconciler", "error", err)
		os.Exit(1)
	}

	if err := (&reconciler.DiagnosticFixReconciler{
		Client: mgr.GetClient(),
		Store:  st,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup fix reconciler", "error", err)
		os.Exit(1)
	}

	// HTTP server as manager Runnable
	httpSrv := httpserver.New(st, mgr.GetClient())
	if err := mgr.Add(&runnableHTTP{srv: httpSrv, addr: httpAddr}); err != nil {
		slog.Error("add http server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("controller starting", "http", httpAddr)
	if err := mgr.Start(ctx); err != nil {
		slog.Error("manager stopped", "error", err)
		os.Exit(1)
	}
}

type runnableHTTP struct {
	srv  *httpserver.Server
	addr string
}

func (r *runnableHTTP) Start(ctx context.Context) error {
	return r.srv.Start(ctx, r.addr)
}

func (r *runnableHTTP) NeedLeaderElection() bool { return false }

func loadBuiltinSkills(ctx context.Context, st store.Store, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("skills directory not found, no builtin skills loaded", "dir", dir)
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			slog.Warn("skip skill file", "file", e.Name(), "error", err)
			continue
		}
		sk := parseSkillMD(string(data))
		if sk == nil {
			continue
		}
		if err := st.UpsertSkill(ctx, sk); err != nil {
			return err
		}
	}
	return nil
}

func parseSkillMD(content string) *store.Skill {
	// Extract frontmatter between --- markers
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil
	}
	// Simple key: value parsing
	meta := map[string]string{}
	for _, line := range strings.Split(parts[1], "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(kv) == 2 {
			meta[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	name := meta["name"]
	if name == "" {
		return nil
	}
	toolsJSON := meta["tools"]
	if toolsJSON == "" {
		toolsJSON = "[]"
	}
	requiresJSON := meta["requires_data"]
	if requiresJSON == "" {
		requiresJSON = "[]"
	}
	return &store.Skill{
		Name:             name,
		Dimension:        meta["dimension"],
		Prompt:           strings.TrimSpace(parts[2]),
		ToolsJSON:        toolsJSON,
		RequiresDataJSON: requiresJSON,
		Source:           "builtin",
		Enabled:          true,
		Priority:         100,
	}
}
