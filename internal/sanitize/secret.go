package sanitize

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cleanSecret(u *unstructured.Unstructured) {
	redactMap(u.Object, "data")
	redactMap(u.Object, "stringData")
}

func redactMap(obj map[string]interface{}, key string) {
	raw, ok := obj[key].(map[string]interface{})
	if !ok {
		return
	}
	for k, v := range raw {
		raw[k] = redactedLen(v)
	}
}

func redactedLen(v interface{}) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("<redacted len=%d>", len(s))
	}
	return "<redacted>"
}
