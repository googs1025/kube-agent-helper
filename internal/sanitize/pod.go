package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var podEnvMask = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key|credential|auth)`)

func cleanPod(u *unstructured.Unstructured) {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return
	}
	redactContainers(spec, "containers")
	redactContainers(spec, "initContainers")
}

func redactContainers(spec map[string]interface{}, key string) {
	raw, ok := spec[key].([]interface{})
	if !ok {
		return
	}
	for i := range raw {
		c, ok := raw[i].(map[string]interface{})
		if !ok {
			continue
		}
		envs, ok := c["env"].([]interface{})
		if !ok {
			continue
		}
		for j := range envs {
			e, ok := envs[j].(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := e["name"].(string)
			if _, hasValue := e["value"]; hasValue && podEnvMask.MatchString(name) {
				e["value"] = "<redacted>"
			}
		}
	}
}
