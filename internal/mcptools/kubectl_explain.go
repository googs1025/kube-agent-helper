package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func NewKubectlExplainHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		resource, _ := args["resource"].(string)
		if resource == "" {
			return jsonResult(map[string]interface{}{"error": "resource is required"})
		}
		// resource may be "Pod", "Pod.spec", "Pod.spec.containers"
		parts := strings.SplitN(resource, ".", 2)
		kind := parts[0]
		fieldPath := ""
		if len(parts) == 2 {
			fieldPath = parts[1]
		}

		// Resolve GVR to get the group/version
		gvr, _, err := d.Mapper.ResolveGVR(kind, "")
		if err != nil {
			return jsonResult(map[string]interface{}{"error": fmt.Sprintf("unknown resource %q: %s", kind, err)})
		}

		schema, err := fetchOpenAPIv3(d, gvr.Group, gvr.Version, kind)
		if err != nil {
			return jsonResult(map[string]interface{}{"error": fmt.Sprintf("openapi schema unavailable: %s", err)})
		}

		result := schema
		if fieldPath != "" {
			result, err = navigateSchema(schema, strings.Split(fieldPath, "."))
			if err != nil {
				return jsonResult(map[string]interface{}{"error": err.Error()})
			}
		}
		return jsonResult(result)
	}
}

// fetchOpenAPIv3 fetches the OpenAPI v3 schema for a given group/version/kind.
func fetchOpenAPIv3(d *Deps, group, version, kind string) (map[string]interface{}, error) {
	gvString := version
	if group != "" {
		gvString = group + "/" + version
	}
	doc, err := d.Discovery.OpenAPIV3().Paths()
	if err != nil {
		return fetchOpenAPIv2(d, kind)
	}
	path := "api/" + gvString
	if group != "" {
		path = "apis/" + gvString
	}
	gvSpec, ok := doc[path]
	if !ok {
		return fetchOpenAPIv2(d, kind)
	}
	raw, err := gvSpec.Schema("application/json")
	if err != nil {
		return fetchOpenAPIv2(d, kind)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fetchOpenAPIv2(d, kind)
	}
	return findKind(spec, kind)
}

// fetchOpenAPIv2 falls back to the v2/swagger endpoint.
func fetchOpenAPIv2(d *Deps, kind string) (map[string]interface{}, error) {
	raw, err := d.Discovery.OpenAPISchema()
	if err != nil {
		return nil, fmt.Errorf("openapi v2: %w", err)
	}
	// raw is *openapi_v2.Document; convert to JSON for uniform handling
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal openapi v2: %w", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, err
	}
	return findKind(doc, kind)
}

// findKind extracts the schema for a kind from a parsed OpenAPI document.
func findKind(doc map[string]interface{}, kind string) (map[string]interface{}, error) {
	// Try OpenAPI v3 components/schemas
	if components, ok := doc["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			for key, v := range schemas {
				// key typically ends with the kind name, e.g. "io.k8s.api.core.v1.Pod"
				if schema, ok := v.(map[string]interface{}); ok {
					if x, _ := schema["x-kubernetes-group-version-kind"].([]interface{}); len(x) > 0 {
						if m, ok := x[0].(map[string]interface{}); ok {
							if k, _ := m["kind"].(string); k == kind {
								schema["_key"] = key
								return schema, nil
							}
						}
					}
				}
			}
			// fallback: suffix match
			kindLower := strings.ToLower(kind)
			for key, v := range schemas {
				if strings.HasSuffix(strings.ToLower(key), "."+kindLower) {
					if schema, ok := v.(map[string]interface{}); ok {
						schema["_key"] = key
						return schema, nil
					}
				}
			}
		}
	}
	// Try OpenAPI v2 definitions
	if defs, ok := doc["definitions"].(map[string]interface{}); ok {
		kindLower := strings.ToLower(kind)
		for key, v := range defs {
			if strings.HasSuffix(strings.ToLower(key), "."+kindLower) {
				if schema, ok := v.(map[string]interface{}); ok {
					schema["_key"] = key
					return schema, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("kind %q not found in OpenAPI schema", kind)
}

// navigateSchema walks the schema following a dotted path of field names.
func navigateSchema(schema map[string]interface{}, path []string) (map[string]interface{}, error) {
	if len(path) == 0 {
		return schema, nil
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no properties at current level")
	}
	field := path[0]
	fieldSchema, ok := props[field].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("field %q not found", field)
	}
	// If array, descend into items
	if t, _ := fieldSchema["type"].(string); t == "array" {
		if items, ok := fieldSchema["items"].(map[string]interface{}); ok {
			fieldSchema = items
		}
	}
	return navigateSchema(fieldSchema, path[1:])
}
