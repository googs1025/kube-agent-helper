package mcptools

import (
	"context"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewListAPIResourcesHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		verb, _ := args["verb"].(string)
		namespaced, hasNamespaced := args["namespaced"].(bool)

		_, lists, err := d.Discovery.ServerGroupsAndResources()
		if err != nil && lists == nil {
			return jsonResult(map[string]interface{}{"error": err.Error()})
		}

		type entry struct {
			Group      string   `json:"group"`
			Version    string   `json:"version"`
			Kind       string   `json:"kind"`
			Name       string   `json:"name"`
			Namespaced bool     `json:"namespaced"`
			Verbs      []string `json:"verbs"`
		}
		var resources []entry
		for _, list := range lists {
			gv, _ := schema.ParseGroupVersion(list.GroupVersion)
			for _, r := range list.APIResources {
				// Skip subresources (contain /)
				if containsRune(r.Name, '/') {
					continue
				}
				if hasNamespaced && r.Namespaced != namespaced {
					continue
				}
				verbs := []string(r.Verbs)
				if verb != "" && !containsVerb(verbs, verb) {
					continue
				}
				resources = append(resources, entry{
					Group:      gv.Group,
					Version:    gv.Version,
					Kind:       r.Kind,
					Name:       r.Name,
					Namespaced: r.Namespaced,
					Verbs:      verbs,
				})
			}
		}
		sort.SliceStable(resources, func(i, j int) bool {
			return resources[i].Kind < resources[j].Kind
		})
		return jsonResult(map[string]interface{}{
			"count":     len(resources),
			"resources": resources,
		})
	}
}

func containsVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}