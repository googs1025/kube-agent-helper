package translator_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func TestFixGenerator_Compile_ProducesJob(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-abc", Namespace: "default", UID: "run-uid-1"},
		Spec: k8saiV1.DiagnosticRunSpec{
			ModelConfigRef: "creds",
			OutputLanguage: "zh",
		},
	}
	finding := &store.Finding{
		ID:                "finding-1",
		RunID:             "run-uid-1",
		Dimension:         "reliability",
		Severity:          "medium",
		Title:             "Dashboard Deployment has no health probes",
		Description:       "No readiness/liveness probes configured.",
		ResourceKind:      "Deployment",
		ResourceNamespace: "kube-agent-helper",
		ResourceName:      "kah-dashboard",
		Suggestion:        "Add readiness and liveness probes.",
	}

	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{
		AgentImage:    "kube-agent-helper/agent-runtime:dev",
		ControllerURL: "http://kah.kube-agent-helper.svc:8080",
	})

	job, err := fg.Compile(run, finding)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Job spec invariants
	assert.Equal(t, "fix-gen-finding-1", job.Name)
	assert.Equal(t, "default", job.Namespace)
	assert.Equal(t, int32(0), *job.Spec.BackoffLimit)
	assert.Equal(t, int64(120), *job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, "run-run-abc", job.Spec.Template.Spec.ServiceAccountName)

	// Container
	c := job.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "kube-agent-helper/agent-runtime:dev", c.Image)
	assert.Equal(t, []string{"python", "-m", "runtime.fix_main"}, c.Command)

	envs := map[string]string{}
	for _, e := range c.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, "http://kah.kube-agent-helper.svc:8080", envs["CONTROLLER_URL"])
	assert.Equal(t, "zh", envs["OUTPUT_LANGUAGE"])
	assert.Contains(t, envs, "FIX_INPUT_JSON")

	// FIX_INPUT_JSON body is a valid JSON with the right keys
	var input map[string]any
	err = json.Unmarshal([]byte(envs["FIX_INPUT_JSON"]), &input)
	assert.NoError(t, err)
	assert.Equal(t, "finding-1", input["findingID"])
	assert.Equal(t, "run-uid-1", input["runID"])
	assert.Equal(t, "Dashboard Deployment has no health probes", input["title"])
	target, _ := input["target"].(map[string]any)
	assert.Equal(t, "Deployment", target["kind"])
	assert.Equal(t, "kube-agent-helper", target["namespace"])
	assert.Equal(t, "kah-dashboard", target["name"])
}

func TestFixGenerator_Compile_DefaultsOutputLanguageToEn(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "u"},
		Spec:       k8saiV1.DiagnosticRunSpec{ModelConfigRef: "creds"},
	}
	finding := &store.Finding{ID: "f", RunID: "u", Title: "t", ResourceKind: "Pod", ResourceNamespace: "default", ResourceName: "p"}
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "img", ControllerURL: "http://x"})
	job, err := fg.Compile(run, finding)
	assert.NoError(t, err)
	var lang string
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "OUTPUT_LANGUAGE" {
			lang = e.Value
		}
	}
	assert.Equal(t, "en", lang, "default output language should be 'en'")
}

// Silence unused import warning if batchv1 isn't referenced directly elsewhere
var _ = batchv1.Job{}
