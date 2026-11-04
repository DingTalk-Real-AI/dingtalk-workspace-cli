package helpers

import (
	"os"
	"strings"
)

// docEditExtraInfo captures the source once for a document edit. A caller that
// already captured it (including a chunked write) keeps that same object.
func docEditExtraInfo(args map[string]any) map[string]any {
	if extraInfo, ok := args["extraInfo"].(map[string]any); ok {
		return extraInfo
	}
	agentName := strings.TrimSpace(os.Getenv("DINGTALK_AGENT"))
	if agentName == "" {
		agentName = "unknown"
	}
	return map[string]any{"agentName": agentName}
}

// withDocEditExtraInfo covers the ordinary commands and composite shortcuts at
// their shared outgoing argument boundary, restricted to these four doc tools.
func withDocEditExtraInfo(serverID, toolName string, args map[string]any) map[string]any {
	if serverID != "doc" {
		return args
	}
	switch toolName {
	case "update_document", "update_document_block", "delete_document_block", "insert_document_block":
	default:
		return args
	}
	if _, ok := args["extraInfo"].(map[string]any); ok {
		return args
	}
	enriched := make(map[string]any, len(args)+1)
	for key, value := range args {
		enriched[key] = value
	}
	enriched["extraInfo"] = docEditExtraInfo(args)
	return enriched
}
