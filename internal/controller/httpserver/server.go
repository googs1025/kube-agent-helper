package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/metrics"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// Option is a functional option for configuring the Server.
type Option func(*Server)

// WithMetrics configures the server with Prometheus metrics.
func WithMetrics(m *metrics.Metrics) Option {
	return func(s *Server) {
		s.metrics = m
	}
}

type Server struct {
	store        store.Store
	k8sClient    client.Client
	fixGenerator *translator.FixGenerator
	metrics      *metrics.Metrics
	mux          *http.ServeMux
}

func New(s store.Store, k8sClient client.Client, fg *translator.FixGenerator, opts ...Option) *Server {
	srv := &Server{store: s, k8sClient: k8sClient, fixGenerator: fg, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(srv)
	}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/internal/fixes", srv.handleInternalFixCallback)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
	srv.mux.HandleFunc("/api/fixes", srv.handleAPIFixes)
	srv.mux.HandleFunc("/api/fixes/", srv.handleAPIFixDetail)
	srv.mux.HandleFunc("/api/findings/", srv.handleAPIFindingAction)
	srv.mux.HandleFunc("/api/events", srv.handleAPIEvents)
	srv.mux.HandleFunc("/api/modelconfigs", srv.handleAPIModelConfigs)
	srv.mux.HandleFunc("/api/k8s/resources", srv.handleAPIK8sResources)
	srv.mux.HandleFunc("/api/clusters", srv.handleAPIClusters)
	srv.mux.HandleFunc("/internal/llm-metrics", srv.handleLLMMetrics)
	if srv.metrics != nil {
		srv.mux.Handle("/metrics", promhttp.HandlerFor(srv.metrics.Registry(), promhttp.HandlerOpts{}))
	}
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv.ListenAndServe()
}

// POST /internal/runs/{id}/findings
func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["internal","runs","{id}","findings"]
	if len(parts) != 4 || parts[3] != "findings" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runID := parts[2]
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	f := &store.Finding{
		RunID:             runID,
		Dimension:         strVal(payload, "dimension"),
		Severity:          strVal(payload, "severity"),
		Title:             strVal(payload, "title"),
		Description:       strVal(payload, "description"),
		ResourceKind:      strVal(payload, "resource_kind"),
		ResourceNamespace: strVal(payload, "resource_namespace"),
		ResourceName:      strVal(payload, "resource_name"),
		Suggestion:        strVal(payload, "suggestion"),
	}
	if err := s.store.CreateFinding(r.Context(), f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.metrics != nil {
		s.metrics.RecordFinding(f.Severity, f.ResourceNamespace, "")
	}
	w.WriteHeader(http.StatusCreated)
}

// POST /internal/fixes — called by fix-generator Pod after producing a patch
func (s *Server) handleInternalFixCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FindingID        string `json:"findingID"`
		DiagnosticRunRef string `json:"diagnosticRunRef"`
		FindingTitle     string `json:"findingTitle"`
		Target           struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"target"`
		Patch struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"patch"`
		BeforeSnapshot string `json:"beforeSnapshot"`
		Explanation    string `json:"explanation"`
		Strategy       string `json:"strategy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.FindingID == "" || req.DiagnosticRunRef == "" ||
		req.Target.Kind == "" || req.Target.Namespace == "" || req.Target.Name == "" ||
		req.Patch.Content == "" {
		http.Error(w, "findingID, diagnosticRunRef, target{kind,namespace,name}, patch.content are required",
			http.StatusBadRequest)
		return
	}
	if req.Patch.Type == "" {
		req.Patch.Type = "strategic-merge"
	}
	strategy := req.Strategy
	if strategy == "" {
		strategy = "dry-run"
	}

	name := fmt.Sprintf("fix-%s", req.FindingID)
	cr := &v1alpha1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: req.Target.Namespace,
		},
		Spec: v1alpha1.DiagnosticFixSpec{
			DiagnosticRunRef: req.DiagnosticRunRef,
			FindingTitle:     req.FindingTitle,
			FindingID:        req.FindingID,
			Target: v1alpha1.FixTarget{
				Kind:      req.Target.Kind,
				Namespace: req.Target.Namespace,
				Name:      req.Target.Name,
			},
			Strategy:         strategy,
			ApprovalRequired: true,
			Patch: v1alpha1.FixPatch{
				Type:    req.Patch.Type,
				Content: req.Patch.Content,
			},
			Rollback: v1alpha1.RollbackConfig{
				Enabled:               true,
				SnapshotBefore:        true,
				AutoRollbackOnFailure: true,
			},
		},
	}
	if err := s.k8sClient.Create(r.Context(), cr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.store.CreateFix(r.Context(), &store.Fix{
		ID:               string(cr.UID),
		RunID:            req.DiagnosticRunRef,
		FindingID:        req.FindingID,
		FindingTitle:     req.FindingTitle,
		TargetKind:       req.Target.Kind,
		TargetNamespace:  req.Target.Namespace,
		TargetName:       req.Target.Name,
		Strategy:         "dry-run",
		ApprovalRequired: true,
		PatchType:        req.Patch.Type,
		PatchContent:     req.Patch.Content,
		Phase:            store.FixPhasePendingApproval,
		Message:          req.Explanation,
		BeforeSnapshot:   req.BeforeSnapshot,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cr)
}

// GET /api/runs
func (s *Server) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		opts := parsePagination(r)
		opts.ClusterName = r.URL.Query().Get("cluster")
		runs, err := s.store.ListRuns(r.Context(), opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if runs == nil {
			runs = make([]*store.DiagnosticRun, 0)
		}
		// Enrich SQLite runs with K8s CR name, then merge scheduled templates
		runs = s.enrichWithK8sNames(r.Context(), runs)
		runs = s.mergeScheduledTemplates(r.Context(), runs)
		writeJSON(w, runs)
	case http.MethodPost:
		s.handleAPIRunsPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// enrichFixesWithK8sNames sets the Name field on fixes by matching UIDs to DiagnosticFix CR names.
func (s *Server) enrichFixesWithK8sNames(ctx context.Context, fixes []*store.Fix) {
	if s.k8sClient == nil || len(fixes) == 0 {
		return
	}
	var list v1alpha1.DiagnosticFixList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return
	}
	uidToName := make(map[string]string, len(list.Items))
	for _, cr := range list.Items {
		uidToName[string(cr.UID)] = cr.Name
	}
	for _, f := range fixes {
		if name, ok := uidToName[f.ID]; ok {
			f.Name = name
		}
	}
}

// enrichWithK8sNames sets the Name field on SQLite runs by matching UIDs to K8s CR names.
func (s *Server) enrichWithK8sNames(ctx context.Context, runs []*store.DiagnosticRun) []*store.DiagnosticRun {
	if s.k8sClient == nil || len(runs) == 0 {
		return runs
	}
	var list v1alpha1.DiagnosticRunList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return runs
	}
	uidToName := make(map[string]string, len(list.Items))
	for _, cr := range list.Items {
		uidToName[string(cr.UID)] = cr.Name
	}
	for _, r := range runs {
		if name, ok := uidToName[r.ID]; ok {
			r.Name = name
		}
	}
	return runs
}

// mergeScheduledTemplates appends K8s scheduled-template runs (spec.schedule != "")
// that are not already present in the SQLite list (matched by UID).
func (s *Server) mergeScheduledTemplates(ctx context.Context, existing []*store.DiagnosticRun) []*store.DiagnosticRun {
	if s.k8sClient == nil {
		return existing
	}
	// Build set of known UIDs
	seen := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		seen[r.ID] = struct{}{}
	}
	var list v1alpha1.DiagnosticRunList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return existing
	}
	for i := range list.Items {
		cr := &list.Items[i]
		if cr.Spec.Schedule == "" {
			continue // only interested in scheduled templates
		}
		uid := string(cr.UID)
		if _, ok := seen[uid]; ok {
			continue // already in SQLite list
		}
		targetJSON, _ := json.Marshal(cr.Spec.Target)
		skillsJSON, _ := json.Marshal(cr.Spec.Skills)
		phase := store.Phase("Scheduled")
		r := &store.DiagnosticRun{
			ID:         uid,
			Name:       cr.Name,
			TargetJSON: string(targetJSON),
			SkillsJSON: string(skillsJSON),
			Status:     phase,
			Message:    cr.Status.Message,
			CreatedAt:  cr.CreationTimestamp.Time,
		}
		existing = append(existing, r)
	}
	return existing
}

// GET /api/runs/{id}  and  GET /api/runs/{id}/findings
func (s *Server) handleAPIRunDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","runs","{id}"] or ["api","runs","{id}","findings"]
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 4 && parts[3] == "crd" {
		s.handleAPIRunCRD(w, r, runID)
		return
	}

	if len(parts) == 4 && parts[3] == "findings" {
		findings, err := s.store.ListFindings(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if findings == nil {
			findings = make([]*store.Finding, 0)
		}
		// Join: findingID -> fixID (if any)
		fixes, _ := s.store.ListFixesByRun(r.Context(), runID)
		fixByFinding := make(map[string]string, len(fixes))
		for _, f := range fixes {
			if f.FindingID != "" {
				fixByFinding[f.FindingID] = f.ID
			}
		}

		type findingWithFix struct {
			*store.Finding
			FixID string
		}
		out := make([]findingWithFix, 0, len(findings))
		for _, f := range findings {
			out = append(out, findingWithFix{Finding: f, FixID: fixByFinding[f.ID]})
		}
		writeJSON(w, out)
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil && err != store.ErrNotFound {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		// Fallback: look up the K8s CR by UID (handles scheduled templates not in SQLite)
		run = s.runFromK8s(r.Context(), runID)
	}
	if run == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, run)
}

// GET /api/runs/{id}/crd — returns the raw DiagnosticRun K8s CR as YAML, looked up by UID
func (s *Server) handleAPIRunCRD(w http.ResponseWriter, r *http.Request, uid string) {
	list := &v1alpha1.DiagnosticRunList{}
	if err := s.k8sClient.List(r.Context(), list, &client.ListOptions{}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var found *v1alpha1.DiagnosticRun
	for i := range list.Items {
		if string(list.Items[i].UID) == uid {
			found = &list.Items[i]
			break
		}
	}
	if found == nil {
		// Fallback: synthesize YAML from the SQLite store record (CR was deleted)
		run, _ := s.store.GetRun(r.Context(), uid)
		if run == nil {
			http.NotFound(w, r)
			return
		}
		synthetic := s.syntheticRunYAML(run)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(synthetic))
		return
	}
	// Strip managed fields for readability
	found.ManagedFields = nil
	found.ResourceVersion = ""
	found.Generation = 0
	// Ensure TypeMeta is set
	found.APIVersion = "k8sai.io/v1alpha1"
	found.Kind = "DiagnosticRun"

	yamlBytes, err := sigsyaml.Marshal(found)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(yamlBytes)
}

// syntheticRunYAML builds a human-readable YAML representation of a run from SQLite data.
// Used when the original K8s CR has been deleted.
func (s *Server) syntheticRunYAML(run *store.DiagnosticRun) string {
	var sb strings.Builder
	sb.WriteString("# DiagnosticRun (synthesized from store — original CR was deleted)\n")
	sb.WriteString("apiVersion: k8sai.io/v1alpha1\n")
	sb.WriteString("kind: DiagnosticRun\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  uid: %s\n", run.ID))
	sb.WriteString(fmt.Sprintf("  creationTimestamp: %s\n", run.CreatedAt.Format(time.RFC3339)))
	sb.WriteString("spec:\n")
	if run.TargetJSON != "" {
		sb.WriteString(fmt.Sprintf("  target: %s\n", run.TargetJSON))
	}
	if run.SkillsJSON != "" {
		sb.WriteString(fmt.Sprintf("  skills: %s\n", run.SkillsJSON))
	}
	sb.WriteString("status:\n")
	sb.WriteString(fmt.Sprintf("  phase: %s\n", run.Status))
	if run.Message != "" {
		sb.WriteString(fmt.Sprintf("  message: %q\n", run.Message))
	}
	if run.StartedAt != nil {
		sb.WriteString(fmt.Sprintf("  startedAt: %s\n", run.StartedAt.Format(time.RFC3339)))
	}
	if run.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("  completedAt: %s\n", run.CompletedAt.Format(time.RFC3339)))
	}
	return sb.String()
}

// used to suppress unused import warning
var _ = types.UID("")

// GET|POST /api/skills
func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := s.store.ListSkills(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if skills == nil {
			skills = make([]*store.Skill, 0)
		}
		writeJSON(w, skills)
	case http.MethodPost:
		s.handleAPISkillsPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPISkillsPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		Namespace    string   `json:"namespace"`
		Dimension    string   `json:"dimension"`
		Description  string   `json:"description"`
		Prompt       string   `json:"prompt"`
		Tools        []string `json:"tools"`
		RequiresData []string `json:"requiresData"`
		Enabled      bool     `json:"enabled"`
		Priority     int      `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Dimension == "" || req.Prompt == "" || len(req.Tools) == 0 {
		http.Error(w, "name, dimension, prompt, and tools are required", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	priority := req.Priority
	if priority == 0 {
		priority = 100
	}

	cr := &v1alpha1.DiagnosticSkill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: v1alpha1.DiagnosticSkillSpec{
			Dimension:    req.Dimension,
			Description:  req.Description,
			Prompt:       req.Prompt,
			Tools:        req.Tools,
			RequiresData: req.RequiresData,
			Enabled:      req.Enabled,
			Priority:     &priority,
		},
	}

	if err := s.k8sClient.Create(r.Context(), cr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cr)
}

// GET /api/fixes
func (s *Server) handleAPIFixes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fixOpts := parsePagination(r)
	fixOpts.ClusterName = r.URL.Query().Get("cluster")
	fixes, err := s.store.ListFixes(r.Context(), fixOpts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fixes == nil {
		fixes = make([]*store.Fix, 0)
	}
	s.enrichFixesWithK8sNames(r.Context(), fixes)
	writeJSON(w, fixes)
}

// GET /api/fixes/{id}, PATCH /api/fixes/{id}/approve, PATCH /api/fixes/{id}/reject
func (s *Server) handleAPIFixDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	fixID := parts[2]

	if len(parts) == 3 && r.Method == http.MethodGet {
		fix, err := s.store.GetFix(r.Context(), fixID)
		if err != nil {
			if err == store.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.enrichFixesWithK8sNames(r.Context(), []*store.Fix{fix})
		writeJSON(w, fix)
		return
	}

	if len(parts) == 4 {
		action := parts[3]
		switch {
		case action == "approve" && r.Method == http.MethodPatch:
			var body struct {
				ApprovedBy string `json:"approvedBy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if body.ApprovedBy == "" {
				http.Error(w, "approvedBy is required", http.StatusBadRequest)
				return
			}
			// Update store
			if err := s.store.UpdateFixApproval(r.Context(), fixID, body.ApprovedBy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Also update the DiagnosticFix CR so the reconciler transitions
			// from DryRunComplete/PendingApproval → Approved → applies the patch.
			if s.k8sClient != nil {
				if fixCR, err := s.findFixCRByStoreID(r.Context(), fixID); err == nil && fixCR != nil {
					// Switch strategy from dry-run to auto so reconciler will apply
					if fixCR.Spec.Strategy == "dry-run" {
						fixCR.Spec.Strategy = "auto"
						_ = s.k8sClient.Update(r.Context(), fixCR)
					}
					now := metav1.Now()
					fixCR.Status.Phase = "Approved"
					fixCR.Status.ApprovedBy = body.ApprovedBy
					fixCR.Status.ApprovedAt = &now
					_ = s.k8sClient.Status().Update(r.Context(), fixCR)
				}
			}
			w.WriteHeader(http.StatusOK)
			return
		case action == "reject" && r.Method == http.MethodPatch:
			if err := s.store.UpdateFixPhase(r.Context(), fixID, store.FixPhaseFailed, "rejected by user"); err != nil {
				if err == store.ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Also update the CR
			if s.k8sClient != nil {
				if fixCR, err := s.findFixCRByStoreID(r.Context(), fixID); err == nil && fixCR != nil {
					now := metav1.Now()
					fixCR.Status.Phase = "Failed"
					fixCR.Status.Message = "rejected by user"
					fixCR.Status.CompletedAt = &now
					_ = s.k8sClient.Status().Update(r.Context(), fixCR)
				}
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	http.NotFound(w, r)
}

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
		OutputLanguage string   `json:"outputLanguage"`
		Schedule       string   `json:"schedule"`
		HistoryLimit   *int32   `json:"historyLimit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.ModelConfigRef == "" {
		http.Error(w, "modelConfigRef is required", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		req.Name = fmt.Sprintf("run-%s-%s", time.Now().Format("20060102"), randSuffix(4))
	}

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
			OutputLanguage: req.OutputLanguage,
			Schedule:       req.Schedule,
			HistoryLimit:   req.HistoryLimit,
		},
	}

	if err := s.k8sClient.Create(r.Context(), cr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cr)
}

// POST /api/findings/{findingID}/generate-fix
func (s *Server) handleAPIFindingAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// expected: ["api", "findings", "{id}", "generate-fix"]
	if len(parts) != 4 || parts[3] != "generate-fix" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	findingID := parts[2]
	if findingID == "" {
		http.Error(w, "missing finding id", http.StatusBadRequest)
		return
	}
	if s.fixGenerator == nil {
		http.Error(w, "fix generator not configured", http.StatusInternalServerError)
		return
	}

	// 1. Find the finding in the store
	finding, err := s.findFindingByID(r.Context(), findingID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if finding == nil {
		http.NotFound(w, r)
		return
	}

	// 2. Idempotency: if a fix already exists for this finding, return it.
	fixes, err := s.store.ListFixesByRun(r.Context(), finding.RunID)
	if err == nil {
		for _, f := range fixes {
			if f.FindingID == findingID {
				writeJSON(w, map[string]string{"fixID": f.ID})
				return
			}
		}
	}

	// 3. Fetch DiagnosticRun CR (need namespace + spec.modelConfigRef)
	var runCR v1alpha1.DiagnosticRun
	if err := s.findRunByUID(r.Context(), finding.RunID, &runCR); err != nil {
		http.Error(w, "run CR not found: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Compile Job
	job, err := s.fixGenerator.Compile(&runCR, finding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.k8sClient.Create(r.Context(), job); err != nil {
		if errors.IsAlreadyExists(err) {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "already-generating"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "generating"})
}

func (s *Server) findFindingByID(ctx context.Context, id string) (*store.Finding, error) {
	runs, err := s.store.ListRuns(ctx, store.ListOpts{Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		fs, err := s.store.ListFindings(ctx, run.ID)
		if err != nil {
			continue
		}
		for _, f := range fs {
			if f.ID == id {
				return f, nil
			}
		}
	}
	return nil, nil
}

// runFromK8s synthesizes a store.DiagnosticRun from a K8s CR looked up by UID.
// Used as fallback for scheduled-template runs that never enter the SQLite store.
func (s *Server) runFromK8s(ctx context.Context, uid string) *store.DiagnosticRun {
	var cr v1alpha1.DiagnosticRun
	if err := s.findRunByUID(ctx, uid, &cr); err != nil {
		return nil
	}
	phase := store.Phase(cr.Status.Phase)
	if phase == "" {
		if cr.Spec.Schedule != "" {
			phase = store.Phase("Scheduled")
		} else {
			phase = store.PhasePending
		}
	}
	targetJSON, _ := json.Marshal(cr.Spec.Target)
	skillsJSON, _ := json.Marshal(cr.Spec.Skills)
	r := &store.DiagnosticRun{
		ID:         uid,
		Name:       cr.Name,
		TargetJSON: string(targetJSON),
		SkillsJSON: string(skillsJSON),
		Status:     phase,
		Message:    cr.Status.Message,
		CreatedAt:  cr.CreationTimestamp.Time,
	}
	if cr.Status.StartedAt != nil {
		t := cr.Status.StartedAt.Time
		r.StartedAt = &t
	}
	if cr.Status.CompletedAt != nil {
		t := cr.Status.CompletedAt.Time
		r.CompletedAt = &t
	}
	return r
}

func (s *Server) findRunByUID(ctx context.Context, uid string, out *v1alpha1.DiagnosticRun) error {
	var list v1alpha1.DiagnosticRunList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return err
	}
	for _, r := range list.Items {
		if string(r.UID) == uid {
			*out = r
			return nil
		}
	}
	return fmt.Errorf("no DiagnosticRun with UID %s", uid)
}

func (s *Server) findFixCRByStoreID(ctx context.Context, storeID string) (*v1alpha1.DiagnosticFix, error) {
	// Store ID = CR UID. List all Fix CRs and match.
	var list v1alpha1.DiagnosticFixList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if string(list.Items[i].UID) == storeID {
			return &list.Items[i], nil
		}
	}
	// Fallback: match by findingID-based name pattern
	for i := range list.Items {
		if list.Items[i].Spec.FindingID != "" {
			fix, _ := s.store.GetFix(ctx, storeID)
			if fix != nil && list.Items[i].Spec.FindingID == fix.FindingID {
				return &list.Items[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no DiagnosticFix CR for store ID %s", storeID)
}

// GET /api/events?namespace=X&name=Y&since=60&limit=100
func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	namespace := q.Get("namespace")
	name := q.Get("name")

	since := 60
	if v := q.Get("since"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "since must be an integer (minutes)", http.StatusBadRequest)
			return
		}
		since = n
	}

	limit := 100
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "limit must be an integer", http.StatusBadRequest)
			return
		}
		limit = n
	}

	opts := store.ListEventsOpts{
		Namespace:    namespace,
		Name:         name,
		SinceMinutes: since,
		Limit:        limit,
		ClusterName:  q.Get("cluster"),
	}
	events, err := s.store.ListEvents(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = make([]*store.Event, 0)
	}
	writeJSON(w, events)
}

// GET /api/k8s/resources?kind=X&namespace=Y&name=Z
// GET|POST /api/modelconfigs
func (s *Server) handleAPIModelConfigs(w http.ResponseWriter, r *http.Request) {
	if s.k8sClient == nil {
		http.Error(w, "k8s client not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list v1alpha1.ModelConfigList
		if err := s.k8sClient.List(r.Context(), &list); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type modelConfigView struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			BaseURL   string `json:"baseURL,omitempty"`
			MaxTurns  *int   `json:"maxTurns,omitempty"`
			SecretRef string `json:"secretRef"`
			SecretKey string `json:"secretKey"`
			APIKey    string `json:"apiKey"` // always masked
		}
		views := make([]modelConfigView, 0, len(list.Items))
		for _, mc := range list.Items {
			views = append(views, modelConfigView{
				Name:      mc.Name,
				Namespace: mc.Namespace,
				Provider:  mc.Spec.Provider,
				Model:     mc.Spec.Model,
				BaseURL:   mc.Spec.BaseURL,
				MaxTurns:  mc.Spec.MaxTurns,
				SecretRef: mc.Spec.APIKeyRef.Name,
				SecretKey: mc.Spec.APIKeyRef.Key,
				APIKey:    "****",
			})
		}
		writeJSON(w, views)
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			BaseURL   string `json:"baseURL"`
			MaxTurns  *int   `json:"maxTurns"`
			SecretRef string `json:"secretRef"`
			SecretKey string `json:"secretKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Namespace == "" {
			http.Error(w, "name and namespace are required", http.StatusBadRequest)
			return
		}
		if req.SecretRef == "" {
			req.SecretRef = req.Name
		}
		if req.SecretKey == "" {
			req.SecretKey = "apiKey"
		}
		if req.Provider == "" {
			req.Provider = "anthropic"
		}
		if req.Model == "" {
			req.Model = "claude-sonnet-4-6"
		}
		mc := &v1alpha1.ModelConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
			Spec: v1alpha1.ModelConfigSpec{
				Provider: req.Provider,
				Model:    req.Model,
				BaseURL:  req.BaseURL,
				MaxTurns: req.MaxTurns,
				APIKeyRef: v1alpha1.SecretKeyRef{
					Name: req.SecretRef,
					Key:  req.SecretKey,
				},
			},
		}
		if err := s.k8sClient.Create(r.Context(), mc); err != nil {
			if errors.IsAlreadyExists(err) {
				http.Error(w, "modelconfig already exists", http.StatusConflict)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"name": mc.Name, "namespace": mc.Namespace})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIK8sResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := r.URL.Query().Get("kind")
	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")

	if kind == "" {
		http.Error(w, "kind query parameter is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	switch kind {
	case "Namespace":
		s.handleListNamespaces(ctx, w)
	case "Deployment":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.DeploymentList{}, &appsv1.Deployment{})
	case "Pod":
		s.handleK8sResource(ctx, w, namespace, name, &corev1.PodList{}, &corev1.Pod{})
	case "StatefulSet":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.StatefulSetList{}, &appsv1.StatefulSet{})
	case "DaemonSet":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.DaemonSetList{}, &appsv1.DaemonSet{})
	default:
		http.Error(w, "unsupported kind: "+kind, http.StatusBadRequest)
	}
}

func (s *Server) handleListNamespaces(ctx context.Context, w http.ResponseWriter) {
	var list corev1.NamespaceList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	items := make([]map[string]string, 0)
	for _, ns := range list.Items {
		if systemNS[ns.Name] {
			continue
		}
		items = append(items, map[string]string{"name": ns.Name})
	}
	writeJSON(w, items)
}

func (s *Server) handleK8sResource(ctx context.Context, w http.ResponseWriter, namespace, name string, listObj client.ObjectList, singleObj client.Object) {
	if namespace == "" {
		http.Error(w, "namespace is required for this kind", http.StatusBadRequest)
		return
	}

	if name != "" {
		key := client.ObjectKey{Namespace: namespace, Name: name}
		if err := s.k8sClient.Get(ctx, key, singleObj); err != nil {
			if errors.IsNotFound(err) {
				http.NotFound(w, nil)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, singleObj)
		return
	}

	if err := s.k8sClient.List(ctx, listObj, client.InNamespace(namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, _ := json.Marshal(listObj)
	var parsed struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		} `json:"items"`
	}
	_ = json.Unmarshal(raw, &parsed)
	items := make([]map[string]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		items = append(items, map[string]string{
			"name":      item.Metadata.Name,
			"namespace": item.Metadata.Namespace,
		})
	}
	writeJSON(w, items)
}

// GET|POST /api/clusters
func (s *Server) handleAPIClusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAPIClustersGet(w, r)
	case http.MethodPost:
		s.handleAPIClustersPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIClustersGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var list v1alpha1.ClusterConfigList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type clusterItem struct {
		Name          string `json:"name"`
		Phase         string `json:"phase"`
		PrometheusURL string `json:"prometheusURL,omitempty"`
		Description   string `json:"description,omitempty"`
	}

	items := []clusterItem{{Name: "local", Phase: "Connected", Description: "In-cluster (local)"}}
	for _, cc := range list.Items {
		items = append(items, clusterItem{
			Name:          cc.Name,
			Phase:         cc.Status.Phase,
			PrometheusURL: cc.Spec.PrometheusURL,
			Description:   cc.Spec.Description,
		})
	}
	writeJSON(w, items)
}

func (s *Server) handleAPIClustersPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		Namespace     string `json:"namespace"`
		SecretName    string `json:"secretName"`
		SecretKey     string `json:"secretKey"`
		PrometheusURL string `json:"prometheusURL"`
		Description   string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.SecretName == "" || body.SecretKey == "" {
		http.Error(w, "name, secretName, secretKey are required", http.StatusBadRequest)
		return
	}
	if body.Namespace == "" {
		body.Namespace = "kube-agent-helper"
	}

	cc := &v1alpha1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      body.Name,
			Namespace: body.Namespace,
		},
		Spec: v1alpha1.ClusterConfigSpec{
			KubeConfigRef: v1alpha1.SecretKeyRef{
				Name: body.SecretName,
				Key:  body.SecretKey,
			},
			PrometheusURL: body.PrometheusURL,
			Description:   body.Description,
		},
	}
	if err := s.k8sClient.Create(r.Context(), cc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"name": cc.Name, "namespace": cc.Namespace})
}

func randSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// parsePagination reads ?limit= and ?offset= from the request query string.
func parsePagination(r *http.Request) store.ListOpts {
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return store.ListOpts{Limit: limit, Offset: offset}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
