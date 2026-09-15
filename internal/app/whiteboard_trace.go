package app

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// Explicit diagnostic opt-in. Never include auth headers, token configuration,
// or endpoints. Files contain complete board responses and are owner-only.
func traceWhiteboardResponse(inv executor.Invocation, stage string, response any, raw json.RawMessage, callErr error) {
	dir := os.Getenv("DWS_WHITEBOARD_TRACE_DIR")
	if dir == "" || inv.CanonicalProduct != "whiteboard" {
		return
	}
	record := map[string]any{
		"recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"stage":      stage, "tool": inv.Tool, "arguments": inv.Params,
		"response": response,
	}
	if len(raw) > 0 {
		// Preserve the exact raw result as text, even if decoding failed.
		record["rawMcpResult"] = string(raw)
	}
	if callErr != nil {
		record["callError"] = callErr.Error()
	}
	if stage == "local_dry_run" {
		record["remoteCallIssued"] = false
	}
	if err := writeWhiteboardTrace(dir, record); err != nil {
		// A diagnostic write failure must not turn a completed remote mutation
		// into an apparent API failure that callers might retry.
		fmt.Fprintf(os.Stderr, "whiteboard trace recording failed: %v\n", err)
	}
}

func writeWhiteboardTrace(dir string, record any) error {
	file, err := os.CreateTemp(dir, "whiteboard-response-*.json")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(record)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func traceWhiteboardTransportResponse(inv executor.Invocation, result transport.ToolCallResult, err error) {
	traceWhiteboardResponse(inv, "transport_before_business_validation", map[string]any{
		"content": result.Content, "blocks": result.Blocks,
		"structuredContent": result.StructuredContent, "isError": result.IsError,
	}, result.RawResult, err)
}
