package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

var (
	dbPath        string
	httpAddr      string
	agentImage    string
	controllerURL string
	skillsDir     string
)

func main() {
	flag.StringVar(&dbPath, "db", "/data/kube-agent-helper.db", "SQLite database path")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP server listen address")
	flag.StringVar(&agentImage, "agent-image", "ghcr.io/kube-agent-helper/agent-runtime:latest", "Agent Pod image")
	flag.StringVar(&controllerURL, "controller-url", "http://controller.kube-agent-helper.svc:8080", "Controller URL for Agent callbacks")
	flag.StringVar(&skillsDir, "skills-dir", "/skills", "Directory containing built-in SKILL.md files")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = k8saiV1.AddToScheme(scheme)

	// Open DB
	st, err := sqlitestore.New(dbPath)
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Load built-in skills into DB (stub for Phase 0; replaced in Task 9)
	if err := loadBuiltinSkills(context.Background(), st, skillsDir); err != nil {
		slog.Warn("load builtin skills", "error", err)
	}

	// Manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		slog.Error("new manager", "error", err)
		os.Exit(1)
	}

	// Load skills for translator
	skills, _ := st.ListSkills(context.Background())

	tr := translator.New(translator.Config{
		AgentImage:    agentImage,
		ControllerURL: controllerURL,
	}, skills)

	if err := (&reconciler.DiagnosticRunReconciler{
		Client:     mgr.GetClient(),
		Store:      st,
		Translator: tr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup reconciler", "error", err)
		os.Exit(1)
	}

	// HTTP server as manager Runnable
	httpSrv := httpserver.New(st)
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

// loadBuiltinSkills is a Phase 0 stub. Task 9 replaces this with a SKILL.md file scanner.
func loadBuiltinSkills(ctx context.Context, st store.Store, _ string) error {
	return st.UpsertSkill(ctx, &store.Skill{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a Kubernetes pod health specialist. See SKILL.md for full prompt.",
		ToolsJSON: `["kubectl_get","kubectl_describe","kubectl_logs","events_list"]`,
		Source:    "builtin",
		Enabled:   true,
		Priority:  100,
	})
}
