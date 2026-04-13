package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/kube-agent-helper/kube-agent-helper/internal/k8sclient"
	"github.com/kube-agent-helper/kube-agent-helper/internal/mcptools"
)

func main() {
	var (
		inCluster      bool
		kubeconfigPath string
		contextName    string
		prometheusURL  string
		maskCMKeys     string
		logLevel       string
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

	sanitizeOpts, err := mcptools.DefaultSanitizeOpts(opts.MaskCMKeys)
	if err != nil {
		return fmt.Errorf("compile mask-configmap-keys regex: %w", err)
	}
	deps := mcptools.NewDeps(clients, slog.Default(), sanitizeOpts)

	srv := server.NewMCPServer("k8s-mcp-server", "0.1.0")
	mcptools.RegisterAll(srv, deps)

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