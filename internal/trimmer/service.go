package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectService(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return base
	}
	if v, ok := spec["type"].(string); ok {
		base["type"] = v
	}
	if v, ok := spec["clusterIP"].(string); ok {
		base["clusterIP"] = v
	}
	if v, ok := spec["selector"].(map[string]interface{}); ok {
		base["selector"] = v
	}
	if v, ok := spec["ports"].([]interface{}); ok {
		base["ports"] = v
	}
	return base
}
