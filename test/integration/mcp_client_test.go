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
	"strings"
	"testing"
	"time"
)

// mcpRequest is a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// mcpResponse is a JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// MCPClient drives the k8s-mcp-server binary over stdio.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Scanner
	nextID int
}

func newMCPClient(t *testing.T) *MCPClient {
	t.Helper()
	binary := os.Getenv("MCP_SERVER_BINARY")
	if binary == "" {
		t.Skip("MCP_SERVER_BINARY not set; skipping integration test")
	}
	kubeconfig := os.Getenv("MCP_TEST_KUBECONFIG")

	args := []string{}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	return &MCPClient{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewScanner(stdout),
		nextID: 1,
	}
}

// initialize performs the MCP handshake.
func (c *MCPClient) initialize(t *testing.T) {
	t.Helper()
	resp := c.call(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]interface{}{"name": "test", "version": "0.0.1"},
		"capabilities":    map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	// Send initialized notification
	notif, _ := json.Marshal(mcpRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	fmt.Fprintln(c.stdin, string(notif))
}

func (c *MCPClient) call(t *testing.T, method string, params interface{}) mcpResponse {
	t.Helper()
	req := mcpRequest{JSONRPC: "2.0", ID: c.nextID, Method: method, Params: params}
	c.nextID++
	data, _ := json.Marshal(req)
	fmt.Fprintln(c.stdin, string(data))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan mcpResponse, 1)
	go func() {
		for c.reader.Scan() {
			line := strings.TrimSpace(c.reader.Text())
			if line == "" {
				continue
			}
			var resp mcpResponse
			if err := json.Unmarshal([]byte(line), &resp); err == nil && resp.ID == req.ID {
				done <- resp
				return
			}
		}
	}()
	select {
	case resp := <-done:
		return resp
	case <-ctx.Done():
		t.Fatalf("timeout waiting for response to %s", method)
		return mcpResponse{}
	}
}

func (c *MCPClient) callTool(t *testing.T, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := c.call(t, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if resp.Error != nil {
		t.Fatalf("tools/call %s error: %s", name, resp.Error.Message)
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v\nraw: %s", err, resp.Result)
	}
	if result.IsError {
		t.Logf("tool %s returned isError=true: %s", name, result.Content[0].Text)
	}
	var payload map[string]interface{}
	if len(result.Content) > 0 {
		_ = json.Unmarshal([]byte(result.Content[0].Text), &payload)
	}
	return payload
}
