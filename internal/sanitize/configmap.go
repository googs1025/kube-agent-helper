package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cleanConfigMap(u *unstructured.Unstructured, mask *regexp.Regexp) {
	if mask == nil {
		return
	}
	data, ok := u.Object["data"].(map[string]interface{})
	if !ok {
		return
	}
	for k := range data {
		if mask.MatchString(k) {
			data[k] = "<redacted>"
		}
	}
}
