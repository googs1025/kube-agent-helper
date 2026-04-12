package sanitize

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func podWithManagedFields() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"managedFields": []interface{}{
					map[string]interface{}{"manager": "kubectl"},
				},
				"selfLink": "/api/v1/namespaces/prod/pods/api",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/last-applied-configuration": "{}",
					"keep-me": "yes",
				},
			},
		},
	}
}

func TestClean_StripsManagedFields(t *testing.T) {
	obj := podWithManagedFields()
	got := Clean(obj, Options{})

	meta := got.Object["metadata"].(map[string]interface{})
	_, has := meta["managedFields"]
	assert.False(t, has, "managedFields must be removed")
}

func TestClean_StripsSelfLink(t *testing.T) {
	got := Clean(podWithManagedFields(), Options{})
	meta := got.Object["metadata"].(map[string]interface{})
	_, has := meta["selfLink"]
	assert.False(t, has)
}

func TestClean_StripsLastAppliedAnnotation(t *testing.T) {
	got := Clean(podWithManagedFields(), Options{})
	meta := got.Object["metadata"].(map[string]interface{})
	annos := meta["annotations"].(map[string]interface{})
	_, hasLastApplied := annos["kubectl.kubernetes.io/last-applied-configuration"]
	assert.False(t, hasLastApplied)
	assert.Equal(t, "yes", annos["keep-me"])
}

func TestClean_Idempotent(t *testing.T) {
	once := Clean(podWithManagedFields(), Options{})
	twice := Clean(once, Options{})
	assert.Equal(t, once.Object, twice.Object)
}

func TestClean_DoesNotMutateInput(t *testing.T) {
	input := podWithManagedFields()
	snapshot := input.DeepCopy()
	_ = Clean(input, Options{})
	require.True(t, reflect.DeepEqual(snapshot.Object, input.Object),
		"input was mutated")
}

func secretObj(data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      "db-creds",
				"namespace": "prod",
			},
			"data": data,
		},
	}
}

func TestClean_SecretDataRedacted(t *testing.T) {
	obj := secretObj(map[string]interface{}{
		"username": "YWRtaW4=",     // base64 "admin" (8 chars decoded-length signal)
		"password": "c2VjcmV0MTIz", // base64 "secret123"
	})
	got := Clean(obj, Options{})

	data := got.Object["data"].(map[string]interface{})
	assert.Regexp(t, `^<redacted len=\d+>$`, data["username"])
	assert.Regexp(t, `^<redacted len=\d+>$`, data["password"])
	assert.Equal(t, "Opaque", got.Object["type"])
}

func TestClean_SecretStringDataRedacted(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"stringData": map[string]interface{}{
				"token": "plaintext-value",
			},
		},
	}
	got := Clean(obj, Options{})
	sd := got.Object["stringData"].(map[string]interface{})
	assert.Equal(t, "<redacted len=15>", sd["token"])
}

func cmObj(data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "app-config",
				"namespace": "prod",
			},
			"data": data,
		},
	}
}

func TestClean_ConfigMapKeyMasked(t *testing.T) {
	mask := regexp.MustCompile(`(?i)(password|token|secret)`)
	obj := cmObj(map[string]interface{}{
		"LOG_LEVEL":    "debug",
		"DB_PASSWORD":  "supersecret",
		"API_TOKEN":    "abc123",
		"INNOCENT_KEY": "hello",
	})
	got := Clean(obj, Options{ConfigMapKeyMask: mask})
	data := got.Object["data"].(map[string]interface{})

	assert.Equal(t, "debug", data["LOG_LEVEL"])
	assert.Equal(t, "<redacted>", data["DB_PASSWORD"])
	assert.Equal(t, "<redacted>", data["API_TOKEN"])
	assert.Equal(t, "hello", data["INNOCENT_KEY"])
}

func TestClean_ConfigMapNoMask(t *testing.T) {
	obj := cmObj(map[string]interface{}{"LOG_LEVEL": "debug", "PASSWORD": "x"})
	got := Clean(obj, Options{}) // no mask configured
	data := got.Object["data"].(map[string]interface{})
	assert.Equal(t, "debug", data["LOG_LEVEL"])
	assert.Equal(t, "x", data["PASSWORD"])
}

func podWithEnv(containers []interface{}, initContainers []interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{"containers": containers}
	if initContainers != nil {
		spec["initContainers"] = initContainers
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "api", "namespace": "prod"},
			"spec":       spec,
		},
	}
}

func TestClean_PodEnvRedacted(t *testing.T) {
	obj := podWithEnv(
		[]interface{}{
			map[string]interface{}{
				"name": "main",
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"},
					map[string]interface{}{"name": "DB_PASSWORD", "value": "super"},
					map[string]interface{}{"name": "API_TOKEN", "value": "abc"},
					// valueFrom should be preserved verbatim
					map[string]interface{}{
						"name": "DATABASE_SECRET",
						"valueFrom": map[string]interface{}{
							"secretKeyRef": map[string]interface{}{
								"name": "db-creds",
								"key":  "password",
							},
						},
					},
				},
			},
		},
		[]interface{}{
			map[string]interface{}{
				"name": "init",
				"env": []interface{}{
					map[string]interface{}{"name": "INIT_PASSWORD", "value": "init-secret"},
				},
			},
		},
	)

	got := Clean(obj, Options{})

	containers := got.Object["spec"].(map[string]interface{})["containers"].([]interface{})
	envs := containers[0].(map[string]interface{})["env"].([]interface{})
	assert.Equal(t, "debug", envs[0].(map[string]interface{})["value"])
	assert.Equal(t, "<redacted>", envs[1].(map[string]interface{})["value"])
	assert.Equal(t, "<redacted>", envs[2].(map[string]interface{})["value"])
	// valueFrom still present on entry 3 (no "value" field)
	_, hasValueFrom := envs[3].(map[string]interface{})["valueFrom"]
	assert.True(t, hasValueFrom)

	initContainers := got.Object["spec"].(map[string]interface{})["initContainers"].([]interface{})
	initEnv := initContainers[0].(map[string]interface{})["env"].([]interface{})
	assert.Equal(t, "<redacted>", initEnv[0].(map[string]interface{})["value"])
}

func TestClean_NilInput(t *testing.T) {
	got := Clean(nil, Options{})
	assert.Nil(t, got)
}

func TestClean_MissingMetadata(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
		},
	}
	got := Clean(obj, Options{})
	assert.NotNil(t, got)
	assert.Equal(t, "Pod", got.GetKind())
}

func TestClean_NilSecretData(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
		},
	}
	got := Clean(obj, Options{})
	assert.NotNil(t, got)
}

func TestClean_PodWithoutSpec(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}
	got := Clean(obj, Options{})
	assert.NotNil(t, got)
	assert.Equal(t, "Pod", got.GetKind())
}

func TestClean_PodWithoutEnv(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "test"},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "main"},
				},
			},
		},
	}
	got := Clean(obj, Options{})
	assert.NotNil(t, got)
}

func TestClean_EmptyAnnotations(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":        "test",
				"annotations": map[string]interface{}{},
			},
		},
	}
	got := Clean(obj, Options{})
	meta := got.Object["metadata"].(map[string]interface{})
	_, hasAnnos := meta["annotations"]
	assert.False(t, hasAnnos, "empty annotations should be removed")
}
