package registry_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "registry-test-*.db")
	require.NoError(t, err)
	f.Close()
	st, err := sqlitestore.New(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestListEnabled_ReturnsOnlyEnabled(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "enabled-skill", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 100,
	}))
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "disabled-skill", Dimension: "cost", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: false, Priority: 100,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "enabled-skill", skills[0].Name)
}

func TestListEnabled_CROverridesBuiltin(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "my-skill", Dimension: "health", Prompt: "builtin prompt",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 100,
	}))
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "my-skill", Dimension: "security", Prompt: "cr prompt",
		ToolsJSON: `["kubectl_get"]`, RequiresDataJSON: `[]`,
		Source: "cr", Enabled: true, Priority: 50,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "cr", skills[0].Source)
	assert.Equal(t, "cr prompt", skills[0].Prompt)
}

func TestListEnabled_OrderedByPriority(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "low-prio", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 200,
	}))
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "high-prio", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 10,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 2)
	assert.Equal(t, "high-prio", skills[0].Name)
	assert.Equal(t, "low-prio", skills[1].Name)
}
