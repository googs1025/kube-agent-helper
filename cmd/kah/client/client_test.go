package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/runs" {
			t.Errorf("expected /api/runs, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Run{
			{ID: "abc123", Name: "test-run", Status: "Succeeded"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	var runs []Run
	err := c.Get(context.Background(), "/api/runs", &runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "abc123" {
		t.Errorf("expected ID abc123, got %s", runs[0].ID)
	}
}

func TestGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	var result map[string]interface{}
	err := c.Get(context.Background(), "/api/runs/missing", &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "API error 404: not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["namespace"] != "default" {
			t.Errorf("expected namespace=default, got %s", body["namespace"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-123"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	var result map[string]string
	err := c.Post(context.Background(), "/api/runs", map[string]string{"namespace": "default"}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "new-123" {
		t.Errorf("expected id new-123, got %s", result["id"])
	}
}

func TestPatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/fixes/fix-1/approve" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["approvedBy"] != "admin" {
			t.Errorf("expected approvedBy=admin, got %s", body["approvedBy"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Patch(context.Background(), "/api/fixes/fix-1/approve", map[string]string{"approvedBy": "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Delete(context.Background(), "/api/runs/abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetWithTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Cluster{{Name: "local", Phase: "Connected"}})
	}))
	defer srv.Close()

	// BaseURL with trailing slash should still work
	c := New(srv.URL + "/")
	var clusters []Cluster
	err := c.Get(context.Background(), "/api/clusters", &clusters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
}
