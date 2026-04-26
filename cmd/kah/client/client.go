package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client wraps net/http for talking to the kube-agent-helper API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a Client with a 30-second timeout.
func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs a GET request and decodes the JSON response into result.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Post performs a POST request with a JSON body and decodes the response.
func (c *Client) Post(ctx context.Context, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Patch performs a PATCH request with a JSON body.
func (c *Client) Patch(ctx context.Context, path string, body interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// --- DTOs (local copies; do NOT import internal packages) ---

// Run mirrors store.DiagnosticRun for JSON decoding.
type Run struct {
	ID          string  `json:"ID"`
	Name        string  `json:"Name"`
	ClusterName string  `json:"ClusterName"`
	TargetJSON  string  `json:"TargetJSON"`
	SkillsJSON  string  `json:"SkillsJSON"`
	Status      string  `json:"Status"`
	Message     string  `json:"Message"`
	StartedAt   *string `json:"StartedAt,omitempty"`
	CompletedAt *string `json:"CompletedAt,omitempty"`
	CreatedAt   string  `json:"CreatedAt"`
}

// Finding mirrors store.Finding.
type Finding struct {
	ID                string `json:"ID"`
	RunID             string `json:"RunID"`
	ClusterName       string `json:"ClusterName"`
	Dimension         string `json:"Dimension"`
	Severity          string `json:"Severity"`
	Title             string `json:"Title"`
	Description       string `json:"Description"`
	ResourceKind      string `json:"ResourceKind"`
	ResourceNamespace string `json:"ResourceNamespace"`
	ResourceName      string `json:"ResourceName"`
	Suggestion        string `json:"Suggestion"`
	CreatedAt         string `json:"CreatedAt"`
}

// Fix mirrors store.Fix.
type Fix struct {
	ID               string `json:"ID"`
	Name             string `json:"Name"`
	ClusterName      string `json:"ClusterName"`
	RunID            string `json:"RunID"`
	FindingTitle     string `json:"FindingTitle"`
	TargetKind       string `json:"TargetKind"`
	TargetNamespace  string `json:"TargetNamespace"`
	TargetName       string `json:"TargetName"`
	Strategy         string `json:"Strategy"`
	ApprovalRequired bool   `json:"ApprovalRequired"`
	PatchType        string `json:"PatchType"`
	PatchContent     string `json:"PatchContent"`
	Phase            string `json:"Phase"`
	ApprovedBy       string `json:"ApprovedBy"`
	Message          string `json:"Message"`
	FindingID        string `json:"FindingID"`
	BeforeSnapshot   string `json:"BeforeSnapshot"`
	CreatedAt        string `json:"CreatedAt"`
	UpdatedAt        string `json:"UpdatedAt"`
}

// Skill mirrors store.Skill.
type Skill struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Source   string `json:"Source"`
	Enabled  bool   `json:"Enabled"`
	Priority int    `json:"Priority"`
}

// Cluster represents a cluster entry from /api/clusters.
type Cluster struct {
	Name          string `json:"name"`
	Phase         string `json:"phase"`
	PrometheusURL string `json:"prometheusURL,omitempty"`
	Description   string `json:"description,omitempty"`
}
