package trimmer

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func (p *Projectors) projectEvent(u *unstructured.Unstructured) map[string]interface{} {
	out := map[string]interface{}{
		"namespace": u.GetNamespace(),
	}
	copyStr := func(dst, src string) {
		if v, ok := u.Object[src].(string); ok {
			out[dst] = v
		}
	}
	copyStr("type", "type")
	copyStr("reason", "reason")
	copyStr("message", "message")
	copyStr("firstTimestamp", "firstTimestamp")
	copyStr("lastTimestamp", "lastTimestamp")

	if v, ok := u.Object["count"]; ok {
		out["count"] = v
	}
	if v, ok := u.Object["involvedObject"].(map[string]interface{}); ok {
		// Trim to the three fields the LLM actually needs.
		trimmed := map[string]interface{}{}
		for _, k := range []string{"kind", "name", "namespace"} {
			if vv, ok := v[k].(string); ok {
				trimmed[k] = vv
			}
		}
		out["involvedObject"] = trimmed
	}
	return out
}
