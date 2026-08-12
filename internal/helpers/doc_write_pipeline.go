package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const (
	// initialChunkSize is the first attempted chunk size (rune count).
	// Server-side OSS delta resolution is now fixed, so large chunks are safe.
	initialChunkSize = 10000

	// longContentWarningThreshold triggers a hint to use --content-file.
	longContentWarningThreshold = 2048
)

// contentInputSource 标记内容的输入来源。
type contentInputSource int

const (
	sourceContentFlag contentInputSource = iota // --content "literal"
	sourceContentFile                           // --content-file path
	sourceStdin                                 // --content -
)

// detectContentSource 根据 cobra.Command 的 flags 判断内容输入来源。
func detectContentSource(cmd *cobra.Command) contentInputSource {
	if filePath := flagOrFallback(cmd, "content-file", "content-path"); filePath != "" {
		return sourceContentFile
	}
	if raw, _ := cmd.Flags().GetString("content"); raw == "-" {
		return sourceStdin
	}
	if raw, _ := cmd.Flags().GetString("content"); raw == "" {
		if md, _ := cmd.Flags().GetString("markdown"); md == "-" {
			return sourceStdin
		}
	}
	return sourceContentFlag
}

// DocWriteResult is the structured output of the write pipeline.
type DocWriteResult struct {
	Success        bool            `json:"success"`
	NodeID         string          `json:"nodeId"`
	ChunksWritten  int             `json:"chunksWritten"`
	ServerResponse json.RawMessage `json:"serverResponse,omitempty"`
}

// docWritePipeline is the unified entry point for doc create/update with
// automatic chunking.
//
// Phases:
//
//  0. Pre-check: warn if --content literal is long
//  1. Strategy: single write (≤initialChunkSize) or chunked
//  2. Write: single call or adaptive chunked writes
//  3. Output: JSON result
func docWritePipeline(cmd *cobra.Command, toolName string, toolArgs map[string]any,
	markdown string, operation string) error {

	// Phase 0: pre-check — guide long --content literals toward --content-file
	if markdown != "" && detectContentSource(cmd) == sourceContentFlag &&
		len(markdown) > longContentWarningThreshold {
		deps.Out.PrintInfo("[WARN] 内容较长 (>2KB)，建议使用 --content-file 传入以避免 shell escape 问题")
	}

	// Strip control characters and dangerous Unicode that the server rejects.
	markdown = stripInputUnsafeChars(markdown)
	if _, hasKey := toolArgs["markdown"]; hasKey {
		toolArgs["markdown"] = markdown
	}

	runeCount := utf8.RuneCountInString(markdown)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Phase 1+2: strategy selection and write
	var nodeID string
	var chunksWritten int
	var lastResponse string
	var writeErr error

	if markdown == "" || runeCount <= initialChunkSize {
		// Single write path
		nodeID, lastResponse, writeErr = singleWrite(ctx, toolName, toolArgs)
		chunksWritten = 1
		if writeErr != nil && isTimeoutError(writeErr.Error()) {
			// The server may have committed the write before the client observed the
			// timeout. Replaying create/append here can duplicate a document or
			// content, so fail closed and require inspection before any retry.
			writeErr = docWriteUnknownStateError(operation, nodeID, "single_write", 0, 1, writeErr)
		}
	} else {
		// Chunked write path
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 内容较长 (%d 字符)，自动分片写入...", runeCount))
		nodeID, chunksWritten, lastResponse, writeErr = chunkedWrite(ctx, toolName, toolArgs, markdown, operation, initialChunkSize)
	}

	if writeErr != nil {
		return writeErr
	}

	// Phase 3: output
	result := DocWriteResult{
		Success:       true,
		NodeID:        nodeID,
		ChunksWritten: chunksWritten,
	}
	if json.Valid([]byte(lastResponse)) {
		result.ServerResponse = json.RawMessage(lastResponse)
	}
	return deps.Out.PrintJSON(result)
}

// singleWrite performs a single MCP tool call and returns nodeId + raw server response.
func singleWrite(ctx context.Context, toolName string, toolArgs map[string]any) (string, string, error) {
	resultText, err := callMCPToolReturnText(ctx, toolName, toolArgs)
	if err != nil {
		return "", resultText, err
	}
	nodeID := extractNodeIDFromResult(resultText)
	return nodeID, resultText, nil
}

// chunkedWrite performs adaptive chunked writing.
// For doc create: first chunk creates the document directly (with content), rest append.
// For doc update with overwrite: first chunk uses overwrite, rest use append.
// Returns nodeID, chunks written, last server response text, and error.
func chunkedWrite(ctx context.Context, toolName string, toolArgs map[string]any,
	markdown string, operation string, startChunkSize int) (string, int, string, error) {

	var nodeID string
	var lastResponse string
	chunkSize := startChunkSize
	chunks := splitMarkdownSafe(markdown, chunkSize)
	writtenCount := 0

	// --- Write first chunk ---
	if toolName == "create_document" {
		createArgs := make(map[string]any)
		for k, v := range toolArgs {
			createArgs[k] = v
		}
		createArgs["markdown"] = chunks[0]
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 写入分片 (1/%d)，%d 字符 (create)...",
			len(chunks), utf8.RuneCountInString(chunks[0])))
		resultText, err := callMCPToolReturnText(ctx, "create_document", createArgs)
		if err != nil {
			if isTimeoutError(err.Error()) {
				return "", 0, resultText, docWriteUnknownStateError(operation, "", "chunk_1", 0, len(chunks), err)
			}
			return "", 0, resultText, fmt.Errorf("创建文档失败: %w", err)
		}
		nodeID = extractNodeIDFromResult(resultText)
		if nodeID == "" {
			return "", 0, resultText, fmt.Errorf("创建文档成功但无法提取 nodeId")
		}
		lastResponse = resultText
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 文档已创建 (nodeId=%s)", nodeID))
	} else {
		if id, ok := toolArgs["nodeId"].(string); ok {
			nodeID = id
		}
		firstMode := "append"
		if m, ok := toolArgs["mode"].(string); ok {
			firstMode = m
		}
		updateArgs := map[string]any{
			"nodeId":   nodeID,
			"markdown": chunks[0],
			"mode":     firstMode,
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 写入分片 (1/%d)，%d 字符 (%s)...",
			len(chunks), utf8.RuneCountInString(chunks[0]), firstMode))
		resultText, err := callMCPToolReturnText(ctx, "update_document", updateArgs)
		if err != nil {
			if isTimeoutError(err.Error()) {
				return nodeID, 0, resultText, docWriteUnknownStateError(operation, nodeID, "chunk_1", 0, len(chunks), err)
			}
			return nodeID, 0, resultText, fmt.Errorf("第 1 片写入失败: %w", err)
		}
		lastResponse = resultText
	}
	writtenCount = 1

	// --- Write remaining chunks with append ---
	for i := 1; i < len(chunks); i++ {
		if ctx.Err() != nil {
			return nodeID, writtenCount, lastResponse, fmt.Errorf("写入被中断，已完成 %d/%d 片", writtenCount, len(chunks))
		}

		chunk := chunks[i]
		preview := chunk
		if len(preview) > 80 {
			preview = preview[:80]
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 写入分片 (%d/%d)，%d 字符, preview=[%s]...",
			i+1, len(chunks), utf8.RuneCountInString(chunk), preview))

		updateArgs := map[string]any{
			"nodeId":   nodeID,
			"markdown": chunk,
			"mode":     "append",
		}
		resultText, err := callMCPToolReturnText(ctx, "update_document", updateArgs)
		if err != nil {
			if isTimeoutError(err.Error()) {
				return nodeID, writtenCount, resultText, docWriteUnknownStateError(
					operation, nodeID, fmt.Sprintf("chunk_%d", i+1), writtenCount, len(chunks), err,
				)
			}
			return nodeID, writtenCount, resultText, fmt.Errorf("分片 %d 写入失败: %w", writtenCount+1, err)
		}
		lastResponse = resultText
		writtenCount++
	}

	deps.Out.PrintInfo(fmt.Sprintf("[INFO] 全部 %d 个分片写入完成", writtenCount))
	return nodeID, writtenCount, lastResponse, nil
}

func docWriteUnknownStateError(operation, nodeID, stage string, written, total int, cause error) error {
	details := map[string]any{
		"status":        "unknown",
		"nodeId":        nodeID,
		"chunksWritten": written,
		"chunksTotal":   total,
		"failedStage":   stage,
	}
	return apperrors.NewAPI(
		"文档写入响应超时，服务端提交状态未知；为避免重复创建或重复追加，已停止自动重试",
		apperrors.WithOperation(operation),
		apperrors.WithReason("doc_write_commit_unknown"),
		apperrors.WithFailureStage(stage),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithActions("先读取目标文档确认实际写入状态", "仅在确认服务端未提交后重新执行"),
		apperrors.WithDetails(details),
		apperrors.WithCause(cause),
	)
}

// isTimeoutError checks if an error message indicates a server-side timeout.
func isTimeoutError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "hsftimeoutexception")
}

// extractNodeIDFromResult 从 MCP 工具返回的 JSON 文本中提取 nodeId 字段。
func extractNodeIDFromResult(resultText string) string {
	var result map[string]any
	if err := json.Unmarshal([]byte(resultText), &result); err != nil {
		return ""
	}
	if nodeID, ok := result["nodeId"].(string); ok && nodeID != "" {
		return nodeID
	}
	return ""
}
