package trimmer

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Projectors produces slimmed list-mode projections for known Kinds,
// falling back to a generic shape for unknown ones.
type Projectors struct {
	// Now is injected for test determinism. nil => time.Now.
	Now func() time.Time
}

func (p *Projectors) now() time.Time {
	if p == nil || p.Now == nil {
		return time.Now()
	}
	return p.Now()
}

// Project returns a minimal map representation of u suited for LLM list
// responses. managedFields, selfLink, and last-applied annotations are
// guaranteed absent.
func (p *Projectors) Project(u *unstructured.Unstructured) map[string]interface{} {
	switch u.GetKind() {
	case "Pod":
		return p.projectPod(u)
	case "Deployment":
		return p.projectDeployment(u)
	case "Node":
		return p.projectNode(u)
	case "Service":
		return p.projectService(u)
	case "Event":
		return p.projectEvent(u)
	default:
		return p.projectGeneric(u)
	}
}

func (p *Projectors) projectGeneric(u *unstructured.Unstructured) map[string]interface{} {
	m := map[string]interface{}{
		"name":      u.GetName(),
		"namespace": u.GetNamespace(),
		"age":       humanAge(p.now(), u.GetCreationTimestamp()),
	}
	if lbls := u.GetLabels(); len(lbls) > 0 {
		m["labels"] = toMapInterface(lbls)
	}
	return m
}

func humanAge(now time.Time, ts metav1.Time) string {
	if ts.IsZero() {
		return "unknown"
	}
	d := now.Sub(ts.Time)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

func toMapInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}