package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func New(s store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}

// POST /internal/runs/{id}/findings
func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["internal","runs","{id}","findings"]
	if len(parts) != 4 || parts[3] != "findings" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]

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

// GET /api/runs
func (s *Server) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runs, err := s.store.ListRuns(r.Context(), store.ListOpts{Limit: 50})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, runs)
}

// GET /api/runs/{id}  and  GET /api/runs/{id}/findings
func (s *Server) handleAPIRunDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","runs","{id}"] or ["api","runs","{id}","findings"]
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]

	if len(parts) == 4 && parts[3] == "findings" {
		findings, err := s.store.ListFindings(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, findings)
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
