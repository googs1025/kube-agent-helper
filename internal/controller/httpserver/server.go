package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type Server struct {
	store        store.Store
	k8sClient    client.Client
	fixGenerator *translator.FixGenerator
	mux          *http.ServeMux
}

func New(s store.Store, k8sClient client.Client, fg *translator.FixGenerator) *Server {
	srv := &Server{store: s, k8sClient: k8sClient, fixGenerator: fg, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/internal/fixes", srv.handleInternalFixCallback)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
	srv.mux.HandleFunc("/api/fixes", srv.handleAPIFixes)
	srv.mux.HandleFunc("/api/fixes/", srv.handleAPIFixDetail)
	srv.mux.HandleFunc("/api/findings/", srv.handleAPIFindingAction)
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
			Strategy:         "dry-run",
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
		runs, err := s.store.ListRuns(r.Context(), store.ListOpts{Limit: 50})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if runs == nil {
			runs = make([]*store.DiagnosticRun, 0)
		}
		writeJSON(w, runs)
	case http.MethodPost:
		s.handleAPIRunsPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, run)
}

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
	fixes, err := s.store.ListFixes(r.Context(), store.ListOpts{Limit: 50})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fixes == nil {
		fixes = make([]*store.Fix, 0)
	}
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
			if err := s.store.UpdateFixApproval(r.Context(), fixID, body.ApprovedBy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		case action == "reject" && r.Method == http.MethodPatch:
			if err := s.store.UpdateFixPhase(r.Context(), fixID, store.FixPhaseFailed, "rejected by user"); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
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

	// Validate target kind is in the Fix CRD's allowed set
	supportedKinds := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true,
		"Service": true, "ConfigMap": true,
	}
	if !supportedKinds[finding.ResourceKind] {
		http.Error(w, fmt.Sprintf("unsupported target kind %q for fix generation (supported: Deployment, StatefulSet, DaemonSet, Service, ConfigMap)", finding.ResourceKind), http.StatusBadRequest)
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

func randSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
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
