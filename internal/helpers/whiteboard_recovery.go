// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

// WithWhiteboardRecovery adds domain guidance at a known whiteboard invocation
// boundary. Never call it from generic MCP parsing: these server codes are not
// globally unique. Auth, transport and already-guided failures pass unchanged.
func WithWhiteboardRecovery(err error) error {
	failure, ok := err.(*CLIError)
	if !ok || failure == nil || failure.Code != CodeMCPToolError || failure.Suggestion != "" {
		return err
	}
	hint := whiteboardRecoverySuggestion(failure.ServerCode)
	if hint == "" {
		return err
	}
	// Do not mutate a caller-owned error. Preserve its metadata and retain the
	// original instance in the cause chain, including any underlying cancellation.
	projected := *failure
	projected.Suggestion = hint
	projected.Cause = err
	return &projected
}

func whiteboardRecoverySuggestion(code string) string {
	switch code {
	case "SCHEMA_INVALID", "OVERWRITE_UNSAFE":
		return "根据服务端字段路径修正 OpenNodes；重新 render/+diff 并确认后提交，不要原样重试"
	case "REVISION_MISMATCH":
		return "白板已有新版本：重新 query 获取 revision，重新生成差异并确认；不要只替换 expectedRevision 重提旧请求"
	case "STATE_TIMEOUT", "COMMIT_FAILED", "CREATE_RESULT_UNKNOWN":
		return "白板写入结果需要核实：先查询最终内容和 revision，保留原 requestId；不要自动重发或更换 requestId"
	case "CHECKPOINT_INVALID", "CHECKPOINT_DECODE_FAILED", "CHECKPOINT_HYDRATION_FAILED":
		return "白板 Checkpoint 读取失败；保留 traceId 和服务端诊断联系维护者，不要改用 overwrite 或清空白板"
	default:
		return ""
	}
}
