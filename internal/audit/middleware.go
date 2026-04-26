// Package audit 给 MCP 工具调用包一层"审计中间件"。
//
// 包装方式：
//
//	audit.Wrap(toolSpec, originalHandler) → newHandler
//
// 每次调用时：
//   1. 生成 ULID 作为 invocation ID
//   2. 用 argmask 把 Secret 名/值等敏感参数 redact 掉
//   3. 记录开始日志（tool, args, ulid, ts）
//   4. 执行原始 handler，捕获结果或错误
//   5. 记录结束日志（ulid, status, duration_ms）
//
// 输出：结构化 JSON 行 → controller stderr → 接入 ELK / Loki 即可查询。
package audit

import (
	"context"
	"crypto/rand"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oklog/ulid/v2"
)

// Handler matches mcp-go's handler signature.
type Handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolSpec describes everything the middleware needs to know about a tool
// in order to produce faithful audit records.
type ToolSpec struct {
	Name         string
	ArgWhitelist []string
	Cluster      string
}

// Wrap produces a new Handler that emits a single structured log record per
// invocation. Panics are recovered and logged.
func Wrap(logger *slog.Logger, spec ToolSpec, next Handler) Handler {
	return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		start := time.Now()
		traceID := ulid.MustNew(ulid.Now(), rand.Reader).String()

		defer func() {
			if r := recover(); r != nil {
				logger.LogAttrs(ctx, slog.LevelError, "tool_call",
					slog.String("trace_id", traceID),
					slog.String("tool", spec.Name),
					slog.Any("args", MaskArgs(argsMap(req), spec.ArgWhitelist)),
					slog.Group("result", slog.Bool("ok", false)),
					slog.Int64("latency_ms", time.Since(start).Milliseconds()),
					slog.String("cluster", spec.Cluster),
					slog.String("error", "panic"),
				)
				result = mcp.NewToolResultError("internal error")
				err = nil
			}
		}()

		result, err = next(ctx, req)

		level := slog.LevelInfo
		ok := err == nil && (result == nil || !result.IsError)
		var errMsg string
		if !ok {
			level = slog.LevelError
			if err != nil {
				errMsg = err.Error()
			} else if result != nil {
				errMsg = extractErrorText(result)
			}
		}

		attrs := []slog.Attr{
			slog.String("trace_id", traceID),
			slog.String("tool", spec.Name),
			slog.Any("args", MaskArgs(argsMap(req), spec.ArgWhitelist)),
			slog.Group("result", slog.Bool("ok", ok)),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			slog.String("cluster", spec.Cluster),
		}
		if errMsg != "" {
			attrs = append(attrs, slog.String("error", errMsg))
		}
		logger.LogAttrs(ctx, level, "tool_call", attrs...)

		return result, err
	}
}

func argsMap(req mcp.CallToolRequest) map[string]interface{} {
	if m, ok := req.Params.Arguments.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func extractErrorText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
