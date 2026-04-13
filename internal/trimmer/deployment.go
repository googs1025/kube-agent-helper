package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func nestedFloat64(obj map[string]interface{}, fields ...string) float64 {
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	}
	return 0
}

func (p *Projectors) projectDeployment(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)

	desired := nestedFloat64(u.Object, "spec", "replicas")
	ready := nestedFloat64(u.Object, "status", "readyReplicas")
	updated := nestedFloat64(u.Object, "status", "updatedReplicas")
	available := nestedFloat64(u.Object, "status", "availableReplicas")

	base["replicas"] = map[string]interface{}{
		"desired":   desired,
		"ready":     ready,
		"updated":   updated,
		"available": available,
	}
	return base
}
