package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectNode(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)
	delete(base, "namespace")

	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, raw := range conds {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if c["type"] == "Ready" {
			base["ready"] = c["status"] == "True"
			break
		}
	}

	kv, _, _ := unstructured.NestedString(u.Object, "status", "nodeInfo", "kubeletVersion")
	osImg, _, _ := unstructured.NestedString(u.Object, "status", "nodeInfo", "osImage")
	base["kubeletVersion"] = kv
	base["osImage"] = osImg

	if cap, ok, _ := unstructured.NestedMap(u.Object, "status", "capacity"); ok {
		base["capacity"] = cap
	}
	if alloc, ok, _ := unstructured.NestedMap(u.Object, "status", "allocatable"); ok {
		base["allocatable"] = alloc
	}
	return base
}
