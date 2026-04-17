package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func NewNetworkPolicyCheckHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		podName, _ := args["podName"].(string)
		if namespace == "" || podName == "" {
			return mcp.NewToolResultError("namespace and podName are required"), nil
		}

		pod, err := d.Typed.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get pod: %v", err)), nil
		}

		npList, err := d.Typed.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list network policies: %v", err)), nil
		}

		var matching []map[string]interface{}
		defaultDeny := false

		for _, np := range npList.Items {
			sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
			if err != nil {
				continue
			}
			if !sel.Matches(labels.Set(pod.Labels)) {
				continue
			}

			if sel.Empty() && len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0 {
				defaultDeny = true
			}

			policyTypes := make([]string, len(np.Spec.PolicyTypes))
			for i, pt := range np.Spec.PolicyTypes {
				policyTypes[i] = string(pt)
			}

			ingressRules := make([]map[string]interface{}, 0)
			for _, rule := range np.Spec.Ingress {
				from := make([]string, 0)
				for _, peer := range rule.From {
					if peer.PodSelector != nil {
						from = append(from, fmt.Sprintf("pods(%v)", peer.PodSelector.MatchLabels))
					}
					if peer.NamespaceSelector != nil {
						from = append(from, fmt.Sprintf("namespaces(%v)", peer.NamespaceSelector.MatchLabels))
					}
					if peer.IPBlock != nil {
						from = append(from, fmt.Sprintf("cidr(%s)", peer.IPBlock.CIDR))
					}
				}
				ports := make([]string, 0)
				for _, p := range rule.Ports {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					if p.Port != nil {
						ports = append(ports, fmt.Sprintf("%s/%s", proto, p.Port.String()))
					}
				}
				ingressRules = append(ingressRules, map[string]interface{}{
					"from": from, "ports": ports,
				})
			}

			egressRules := make([]map[string]interface{}, 0)
			for _, rule := range np.Spec.Egress {
				to := make([]string, 0)
				for _, peer := range rule.To {
					if peer.PodSelector != nil {
						to = append(to, fmt.Sprintf("pods(%v)", peer.PodSelector.MatchLabels))
					}
					if peer.NamespaceSelector != nil {
						to = append(to, fmt.Sprintf("namespaces(%v)", peer.NamespaceSelector.MatchLabels))
					}
					if peer.IPBlock != nil {
						to = append(to, fmt.Sprintf("cidr(%s)", peer.IPBlock.CIDR))
					}
				}
				ports := make([]string, 0)
				for _, p := range rule.Ports {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					if p.Port != nil {
						ports = append(ports, fmt.Sprintf("%s/%s", proto, p.Port.String()))
					}
				}
				egressRules = append(egressRules, map[string]interface{}{
					"to": to, "ports": ports,
				})
			}

			matching = append(matching, map[string]interface{}{
				"name":         np.Name,
				"policyTypes":  policyTypes,
				"ingressRules": ingressRules,
				"egressRules":  egressRules,
			})
		}

		summary := fmt.Sprintf("%d NetworkPolicy(ies) match this pod.", len(matching))
		if len(matching) == 0 {
			summary = "No NetworkPolicies match this pod. All traffic is allowed by default."
		}
		if defaultDeny {
			summary += " A default-deny policy is in effect."
		}

		return jsonResult(map[string]interface{}{
			"podLabels":        pod.Labels,
			"matchingPolicies": matching,
			"defaultDeny":      defaultDeny,
			"summary":          summary,
		})
	}
}
