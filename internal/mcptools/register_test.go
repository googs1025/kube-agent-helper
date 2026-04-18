package mcptools

import (
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

// minimalDeps returns a *Deps with only fields needed by registerTool (Logger, Cluster).
// Handler constructors are not called during registration, so nil clients are fine.
func minimalDeps() *Deps {
	return &Deps{
		Logger:     slog.Default(),
		Cluster:    "https://test",
		Projectors: &trimmer.Projectors{},
	}
}

func newTestServer() *server.MCPServer {
	return server.NewMCPServer("test", "0.0.0")
}

func TestRegisterCore_ToolCount(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterCore(s, d)

	tools := s.ListTools()
	assert.Len(t, tools, 4, "RegisterCore should register exactly 4 core tools")
}

func TestRegisterExtension_ToolCount(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterExtension(s, d)

	tools := s.ListTools()
	assert.Len(t, tools, 10, "RegisterExtension should register exactly 10 extension tools")
}

func TestRegisterAll_TotalCount(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterAll(s, d)

	tools := s.ListTools()
	assert.Len(t, tools, 14, "RegisterAll should register all 14 tools (4 core + 10 extension)")
}

func TestRegisterAll_NoDuplicateNames(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterAll(s, d)

	tools := s.ListTools()
	// ListTools returns a map keyed by name, so duplicates would collapse.
	// We verify the count still equals 14, confirming every name is unique.
	require.Len(t, tools, 14, "each tool name must be unique — duplicates would reduce count")

	// Also explicitly collect names and verify uniqueness as documentation.
	seen := make(map[string]struct{}, len(tools))
	for name := range tools {
		_, exists := seen[name]
		assert.False(t, exists, "duplicate tool name detected: %s", name)
		seen[name] = struct{}{}
	}
}

func TestRegisteredTool_HasDescription(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterAll(s, d)

	tools := s.ListTools()
	require.NotEmpty(t, tools)

	for name, st := range tools {
		assert.NotEmpty(t, st.Tool.Description,
			"tool %q must have a non-empty description", name)
	}
}

func TestRegisteredTool_HasInputSchema(t *testing.T) {
	s := newTestServer()
	d := minimalDeps()

	RegisterAll(s, d)

	tools := s.ListTools()
	require.NotEmpty(t, tools)

	for name, st := range tools {
		// Each tool must declare an object-type input schema.
		assert.Equal(t, "object", st.Tool.InputSchema.Type,
			"tool %q inputSchema.type must be \"object\"", name)
	}
}
