package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Options configures the sanitizer.
type Options struct {
	// ConfigMapKeyMask matches ConfigMap data keys to redact. If nil, no
	// ConfigMap redaction is performed beyond the generic rules.
	ConfigMapKeyMask *regexp.Regexp
}

// Clean returns a deep-copied, sanitized copy of obj. The original is never
// mutated. The function is idempotent: Clean(Clean(x)) == Clean(x).
func Clean(obj *unstructured.Unstructured, opts Options) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}
	out := obj.DeepCopy()

	stripGenericMetadata(out)

	switch out.GetKind() {
	case "Secret":
		cleanSecret(out)
	case "ConfigMap":
		cleanConfigMap(out, opts.ConfigMapKeyMask)
	case "Pod":
		cleanPod(out)
	case "PodList":
		// handled by trimmer layer; nothing to do here
	}

	return out
}

// stripGenericMetadata removes fields that every resource should lose,
// regardless of Kind: managedFields, selfLink, and the last-applied
// annotation.
func stripGenericMetadata(u *unstructured.Unstructured) {
	meta, ok := u.Object["metadata"].(map[string]interface{})
	if !ok {
		return
	}
	delete(meta, "managedFields")
	delete(meta, "selfLink")

	if annos, ok := meta["annotations"].(map[string]interface{}); ok {
		delete(annos, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annos) == 0 {
			delete(meta, "annotations")
		}
	}
}
