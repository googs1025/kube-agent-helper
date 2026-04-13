package trimmer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func loadFixture(t *testing.T, name string) *unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &data))
	return &unstructured.Unstructured{Object: data}
}

func loadGolden(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &data))
	return data
}

// Freeze "now" for age calculation reproducibility.
func frozenNow() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-04-11T09:00:00Z")
	return t
}

func TestProject_Pod_Golden(t *testing.T) {
	in := loadFixture(t, "pod.input.json")
	golden := loadGolden(t, "pod.golden.json")

	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_UnknownKind_Generic(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata": map[string]interface{}{
				"name":              "nightly",
				"namespace":         "ops",
				"labels":            map[string]interface{}{"team": "sre"},
				"creationTimestamp": "2026-04-10T00:00:00Z",
			},
		},
	}
	p := &Projectors{Now: frozenNow}
	got := p.Project(obj)
	assert.Equal(t, "nightly", got["name"])
	assert.Equal(t, "ops", got["namespace"])
	assert.Equal(t, map[string]interface{}{"team": "sre"}, got["labels"])
	assert.Contains(t, got, "age")
}

func TestProject_StripsManagedFields(t *testing.T) {
	in := loadFixture(t, "pod.input.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	_, hasManagedFields := got["managedFields"]
	assert.False(t, hasManagedFields, "trimmer must never leak managedFields")
}

func TestProject_Deployment_Golden(t *testing.T) {
	in := loadFixture(t, "deployment.input.json")
	golden := loadGolden(t, "deployment.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_Node_Golden(t *testing.T) {
	in := loadFixture(t, "node.input.json")
	golden := loadGolden(t, "node.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_Service_Golden(t *testing.T) {
	in := loadFixture(t, "service.input.json")
	golden := loadGolden(t, "service.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	delete(got, "age")
	assert.Equal(t, golden, got)
}

func TestProject_Event_Golden(t *testing.T) {
	in := loadFixture(t, "event.input.json")
	golden := loadGolden(t, "event.golden.json")
	p := &Projectors{Now: frozenNow}
	got := p.Project(in)
	// Event projection does not emit "age" or "name".
	assert.Equal(t, golden, got)
}