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
	// DefaultMarkdownChunkRunes is the single source of truth for the markdown
	// append-mode chunk limit (rune count), shared by every write path that
	// chunks. The splitter budgets any injected repair (a re-emitted table
	// header, a reopened fence) against this limit, so a repaired chunk is still
	// guaranteed to be at most this many runes.
	DefaultMarkdownChunkRunes = 30000

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
	Success       bool   `json:"success"`
	NodeID        string `json:"nodeId"`
	ChunksWritten int    `json:"chunksWritten"`
	// Degradations lists the chunk boundaries that changed the rendered
	// structure. Empty means the document reads exactly as the input did.
	Degradations   []MarkdownDegradation `json:"degradations,omitempty"`
	ServerResponse json.RawMessage       `json:"serverResponse,omitempty"`
}

// chunkedWriteOutcome is what a chunked write reports back. It is a struct rather
// than another return value because the tuple was already four wide.
type chunkedWriteOutcome struct {
	nodeID       string
	written      int
	lastResponse string
	degradations []MarkdownDegradation
}

// docWritePipeline is the unified entry point for doc create/update with
// automatic chunking.
//
// Phases:
//
//  0. Pre-check: warn if --content literal is long
//  1. Strategy: single write (≤DefaultMarkdownChunkRunes) or chunked
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
	var outcome chunkedWriteOutcome
	var writeErr error

	if markdown == "" || runeCount <= DefaultMarkdownChunkRunes {
		// Single write path
		outcome.nodeID, outcome.lastResponse, writeErr = singleWrite(ctx, toolName, toolArgs)
		outcome.written = 1
		if writeErr != nil && isTimeoutError(writeErr.Error()) {
			// The server may have committed the write before the client observed the
			// timeout. Replaying create/append here can duplicate a document or
			// content, so fail closed and require inspection before any retry.
			writeErr = docWriteUnknownStateError(operation, outcome.nodeID, "single_write", 0, 1, writeErr, nil)
		}
	} else {
		// Chunked write path. --index cannot survive chunking: each chunk creates
		// an unpredictable number of blocks, so the insertion point for chunk 2
		// is unknowable. Fail closed rather than silently ignore the flag.
		if _, hasIndex := toolArgs["index"]; hasIndex {
			return apperrors.NewValidation(
				fmt.Sprintf("内容长度 %d 字符超过单次写入上限 %d，需要自动分片，而 --index 在分片写入下无法保证插入位置", runeCount, DefaultMarkdownChunkRunes),
				apperrors.WithOperation(operation),
				apperrors.WithReason("doc_write_index_with_chunking"),
				apperrors.WithRetryable(false),
				apperrors.WithActions(
					"去掉 --index 追加到文档末尾",
					"或把内容拆成小于上限的多段，各自带 --index 分别写入",
				),
			)
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 内容较长 (%d 字符)，自动分片写入...", runeCount))
		outcome, writeErr = chunkedWrite(ctx, toolName, toolArgs, markdown, operation, DefaultMarkdownChunkRunes)
	}

	if writeErr != nil {
		return writeErr
	}

	// Phase 3: output
	result := DocWriteResult{
		Success:       true,
		NodeID:        outcome.nodeID,
		ChunksWritten: outcome.written,
		Degradations:  outcome.degradations,
	}
	if json.Valid([]byte(outcome.lastResponse)) {
		result.ServerResponse = json.RawMessage(outcome.lastResponse)
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

// chunkedWrite writes markdown as a sequence of independently valid chunks.
// For doc create: the first chunk creates the document directly (with content),
// the rest append. For doc update with overwrite: the first chunk uses overwrite,
// the rest use append.
func chunkedWrite(ctx context.Context, toolName string, toolArgs map[string]any,
	markdown string, operation string, chunkSize int) (chunkedWriteOutcome, error) {

	plan := SplitMarkdownForAppend(markdown, chunkSize)
	chunks := plan.Chunks
	out := chunkedWriteOutcome{degradations: plan.Degradations}
	for _, warning := range plan.Warnings() {
		deps.Out.PrintInfo("[WARN] " + warning)
	}

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
		out.lastResponse = resultText
		if err != nil {
			if isTimeoutError(err.Error()) {
				return out, docWriteUnknownStateError(operation, "", "chunk_1", 0, len(chunks), err, plan.Degradations)
			}
			return out, fmt.Errorf("创建文档失败: %w", err)
		}
		out.nodeID = extractNodeIDFromResult(resultText)
		if out.nodeID == "" {
			return out, fmt.Errorf("创建文档成功但无法提取 nodeId")
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 文档已创建 (nodeId=%s)", out.nodeID))
	} else {
		if id, ok := toolArgs["nodeId"].(string); ok {
			out.nodeID = id
		}
		firstMode := "append"
		if m, ok := toolArgs["mode"].(string); ok {
			firstMode = m
		}
		updateArgs := map[string]any{
			"nodeId":   out.nodeID,
			"markdown": chunks[0],
			"mode":     firstMode,
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 写入分片 (1/%d)，%d 字符 (%s)...",
			len(chunks), utf8.RuneCountInString(chunks[0]), firstMode))
		resultText, err := callMCPToolReturnText(ctx, "update_document", updateArgs)
		out.lastResponse = resultText
		if err != nil {
			if isTimeoutError(err.Error()) {
				return out, docWriteUnknownStateError(operation, out.nodeID, "chunk_1", 0, len(chunks), err, plan.Degradations)
			}
			return out, fmt.Errorf("第 1 片写入失败: %w", err)
		}
	}
	out.written = 1

	// --- Write remaining chunks with append ---
	for i := 1; i < len(chunks); i++ {
		if ctx.Err() != nil {
			return out, fmt.Errorf("写入被中断，已完成 %d/%d 片", out.written, len(chunks))
		}

		chunk := chunks[i]
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] 写入分片 (%d/%d)，%d 字符, preview=[%s]...",
			i+1, len(chunks), utf8.RuneCountInString(chunk), previewRunes(chunk, 80)))

		updateArgs := map[string]any{
			"nodeId":   out.nodeID,
			"markdown": chunk,
			"mode":     "append",
		}
		resultText, err := callMCPToolReturnText(ctx, "update_document", updateArgs)
		out.lastResponse = resultText
		if err != nil {
			if isTimeoutError(err.Error()) {
				return out, docWriteUnknownStateError(
					operation, out.nodeID, fmt.Sprintf("chunk_%d", i+1), out.written, len(chunks), err, plan.Degradations,
				)
			}
			return out, fmt.Errorf("分片 %d 写入失败: %w", out.written+1, err)
		}
		out.written++
	}

	deps.Out.PrintInfo(fmt.Sprintf("[INFO] 全部 %d 个分片写入完成", out.written))
	return out, nil
}

// previewRunes truncates to at most n runes. Slicing by byte would cut a
// multi-byte character in half and put invalid UTF-8 into the log line.
func previewRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func docWriteUnknownStateError(operation, nodeID, stage string, written, total int,
	cause error, degradations []MarkdownDegradation) error {

	details := map[string]any{
		"status":        "unknown",
		"nodeId":        nodeID,
		"chunksWritten": written,
		"chunksTotal":   total,
		"failedStage":   stage,
	}
	if len(degradations) > 0 {
		// Resuming safely needs to know which boundaries carried injected repair
		// text, because a chunk that begins with a repeated table header is not
		// the same as the raw source at that offset.
		details["degradations"] = degradations
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
