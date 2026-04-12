package mcptools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultTailLines int64 = 200
	maxTailLines     int64 = 2000
	maxLogBytes            = 256 * 1024
)

func NewKubectlLogsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		namespace, _ := args["namespace"].(string)
		pod, _ := args["pod"].(string)
		if namespace == "" || pod == "" {
			return mcp.NewToolResultError("namespace and pod are required"), nil
		}

		container, _ := args["container"].(string)
		tail := defaultTailLines
		if v, ok := args["tailLines"].(float64); ok {
			tail = int64(v)
		}
		if tail <= 0 || tail > maxTailLines {
			return mcp.NewToolResultError(fmt.Sprintf("tailLines must be between 1 and %d", maxTailLines)), nil
		}
		previous, _ := args["previous"].(bool)
		var sinceSeconds *int64
		if v, ok := args["sinceSeconds"].(float64); ok {
			s := int64(v)
			sinceSeconds = &s
		}

		if container == "" {
			p, err := d.Typed.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if len(p.Spec.Containers) > 1 {
				names := make([]string, 0, len(p.Spec.Containers))
				for _, c := range p.Spec.Containers {
					names = append(names, c.Name)
				}
				return mcp.NewToolResultError(fmt.Sprintf(
					"pod has multiple containers; specify one of: %s",
					strings.Join(names, ", "),
				)), nil
			}
		}

		opts := &corev1.PodLogOptions{
			Container:    container,
			TailLines:    &tail,
			Previous:     previous,
			SinceSeconds: sinceSeconds,
		}
		rc, err := d.Typed.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer rc.Close()

		limited := io.LimitReader(rc, int64(maxLogBytes+1))
		data, err := io.ReadAll(limited)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		truncated := len(data) > maxLogBytes
		if truncated {
			data = data[:maxLogBytes]
		}

		lineCount := strings.Count(string(data), "\n")
		return jsonResult(map[string]interface{}{
			"logs":      string(data),
			"truncated": truncated,
			"lineCount": lineCount,
		})
	}
}
