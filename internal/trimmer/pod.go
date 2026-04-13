package trimmer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (p *Projectors) projectPod(u *unstructured.Unstructured) map[string]interface{} {
	base := p.projectGeneric(u)

	phase, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	nodeName, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
	if phase != "" {
		base["phase"] = phase
	}
	if nodeName != "" {
		base["nodeName"] = nodeName
	}

	specContainers, _, _ := unstructured.NestedSlice(u.Object, "spec", "containers")
	statuses, _, _ := unstructured.NestedSlice(u.Object, "status", "containerStatuses")

	readyCount, totalRestarts := 0, int64(0)
	containerList := make([]interface{}, 0, len(statuses))
	for _, raw := range statuses {
		cs, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ready, _ := cs["ready"].(bool)
		if ready {
			readyCount++
		}
		rc, _ := cs["restartCount"].(int64)
		if rc == 0 {
			// json.Unmarshal into interface{} produces float64
			if f, ok := cs["restartCount"].(float64); ok {
				rc = int64(f)
			}
		}
		totalRestarts += rc

		name, _ := cs["name"].(string)
		state := summarizeState(cs["state"])
		containerList = append(containerList, map[string]interface{}{
			"name":         name,
			"ready":        ready,
			"restartCount": float64(rc), // match JSON number type
			"state":        state,
		})
	}

	base["restarts"] = float64(totalRestarts)
	if len(specContainers) > 0 {
		base["ready"] = fmt.Sprintf("%d/%d", readyCount, len(specContainers))
	}
	if len(containerList) > 0 {
		base["containers"] = containerList
	}
	return base
}

// summarizeState collapses the container state struct into a single string
// like "Running", "CrashLoopBackOff", "Completed", etc.
func summarizeState(raw interface{}) string {
	state, ok := raw.(map[string]interface{})
	if !ok {
		return "Unknown"
	}
	if _, running := state["running"].(map[string]interface{}); running {
		return "Running"
	}
	if w, waiting := state["waiting"].(map[string]interface{}); waiting {
		if reason, _ := w["reason"].(string); reason != "" {
			return reason
		}
		return "Waiting"
	}
	if tr, terminated := state["terminated"].(map[string]interface{}); terminated {
		if reason, _ := tr["reason"].(string); reason != "" {
			return reason
		}
		return "Terminated"
	}
	return "Unknown"
}