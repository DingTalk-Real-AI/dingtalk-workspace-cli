package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers/doc"
	jsonrepair "github.com/RealAlexandreAI/json-repair"
	"github.com/spf13/cobra"
)

var (
	repairJSONML        = jsonrepair.RepairJSON
	diagnoseJSONMLError = doc.DiagnoseJSONError
)

// isInputDangerousUnicode matches the dangerous-unicode set defined by the
// server-side RejectControlChars validator (pkg/validate/input.go).
func isInputDangerousUnicode(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200D: // zero-width space/non-joiner/joiner
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0x202A && r <= 0x202E: // Bidi: LRE/RLE/PDF/LRO/RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // line/paragraph separator
		return true
	case r >= 0x2066 && r <= 0x2069: // Bidi isolates: LRI/RLI/FSI/PDI
		return true
	}
	return false
}

// stripInputUnsafeChars removes characters that would be rejected by the
// server-side RejectControlChars validator:
//  1. C0 control characters (except \t and \n) and DEL (0x7F)
//  2. Dangerous Unicode characters (zero-width, Bidi, line/paragraph separators)
//
// This is applied at the write boundary so content passes server validation.
func stripInputUnsafeChars(s string) string {
	return strings.Map(func(r rune) rune {
		// C0 controls (except tab and newline) and DEL
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return -1
		}
		if isInputDangerousUnicode(r) {
			return -1
		}
		return r
	}, s)
}

// ──────────────────────────────────────────────────────────
// Shared JSONML preprocessing pipeline used by:
//
//	dws doc create   --content-format jsonml --content        (body, root 校验)
//	dws doc update   --content-format jsonml --content        (body, root 校验)
//	dws doc block insert --content-format jsonml --element    (single node, 不校验 root)
//	dws doc block update --content-format jsonml --element    (single node, 不校验 root)
//
// Pipeline:
//
//	[optional JSON repair] → coerce → parse → validate → marshal
//
// Flags:
//
//	--fix-jsonml       启用 JSON 语法 repair（括号/逗号补全），推荐 agent 调用时使用
//	(缺省)             严格模式：仅做协议形状包装 + 校验，不做内容修复
// ──────────────────────────────────────────────────────────

// addJsonMLFlags registers --fix-jsonml on a cobra command.
// Idempotent — safe to call once per command.
func addJsonMLFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("fix-jsonml") == nil {
		cmd.Flags().Bool("fix-jsonml", false,
			"启用 JSON 语法修复（括号/逗号补全），推荐 agent 调用时使用")
	}
}

// coerceJsonMLBodyShape normalizes the body input shape before parse.
//
// Accepted inputs:
//   - {"jsonml": [...]}  → passthrough (returned as-is)
//   - [...]              → wrapped to {"jsonml": [...]} silently (protocol-only coercion)
//   - anything else      → passthrough (let downstream parse emit the original error)
//
// Empty string is passthrough. The bare-array wrap is protocol plumbing
// invisible to callers; no fix note is emitted.
func coerceJsonMLBodyShape(raw string) (string, []string, error) {
	if raw == "" {
		return raw, nil, nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return raw, nil, nil
	}
	if _, ok := probe.([]any); ok {
		wrapped, _ := json.Marshal(map[string]any{"jsonml": probe})
		return string(wrapped), nil, nil
	}
	return raw, nil, nil
}

// coerceJsonMLNodeShape is the block-command mirror of coerceJsonMLBodyShape.
//
// Accepted inputs:
//   - [tag, attrs, ...]          → passthrough
//   - {"jsonml": [node]}         → unwrap to bare node + fix note
//   - {"jsonml": []}             → error (empty wrapper)
//   - {"jsonml": [n1, n2, ...]}  → error (block ops accept exactly one node)
//   - anything else              → passthrough (downstream emits original error)
func coerceJsonMLNodeShape(raw string) (string, []string, error) {
	if raw == "" {
		return raw, nil, nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return raw, nil, nil
	}
	if _, ok := probe.([]any); ok {
		return raw, nil, nil
	}
	wrapper, ok := probe.(map[string]any)
	if !ok {
		return raw, nil, nil
	}
	inner, hasKey := wrapper["jsonml"]
	if !hasKey {
		return raw, nil, nil
	}
	arr, ok := inner.([]any)
	if !ok {
		return raw, nil, nil
	}
	switch len(arr) {
	case 0:
		return "", nil, fmt.Errorf(`--content-format jsonml 输入 {"jsonml":[]}: wrapper 中 jsonml 数组为空`)
	case 1:
		out, _ := json.Marshal(arr[0])
		note := `输入为 {"jsonml":[node]} body 形态，已自动解包为单节点以符合 block 命令协议`
		return string(out), []string{note}, nil
	default:
		return "", nil, fmt.Errorf(`block insert/update 一次只能处理一个 JSONML 节点，输入 {"jsonml":[...]} 包含 %d 个节点。请分多次调用，或使用 doc update --content-format jsonml 整篇覆盖`, len(arr))
	}
}

// prepareJsonMLBody processes the body extracted from a `doc create/update
// --content` payload of shape `{"jsonml": [...]}`.
//
// Pipeline: [JSON repair] → coerce → parse → root 校验 → validate → marshal.
//
// Root 校验：doc create/update 要求 body 以 ["root", {attrs?}, ...] 形态传入。
// 缺少 root 报错而非自动包装——调用者应显式提供完整结构。
// block insert/update 走 prepareJsonMLNode，不做 root 校验。
//
// Returns the JSON-stringified body to be sent to the server.
func prepareJsonMLBody(cmd *cobra.Command, raw string) (string, error) {
	fixJSON, _ := cmd.Flags().GetBool("fix-jsonml")

	// Preserve original source for position-accurate schema diagnostics.
	originalSrc := raw
	repaired := false

	// Step 1: JSON repair — fix broken syntax BEFORE any shape detection.
	// jsonrepair only fixes brackets/commas; it does not alter semantics.
	if fixJSON {
		var probe any
		if err := json.Unmarshal([]byte(raw), &probe); err != nil {
			result, repairErr := repairJSONML(raw)
			if repairErr != nil {
				return "", fmt.Errorf("JSON 语法错误且自动修复失败: %w\n原始错误: %v", repairErr, err)
			}
			fmt.Fprintf(os.Stderr, "[FIX] JSON 语法已自动修复（括号/逗号等结构性错误）\n")
			raw = result
			repaired = true
		}
	}

	// Step 2: Coerce shape — bare array → {"jsonml": [...]} wrapper.
	coerced, coerceNotes, _ := coerceJsonMLBodyShape(raw)
	emitFixNotes(coerceNotes)
	raw = coerced

	// Step 3: Parse into wrapper map.
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		if !fixJSON {
			// Use parser for detailed positional diagnostics
			detail := diagnoseJSONMLError([]byte(raw))
			if detail != "" {
				return "", fmt.Errorf("JSON 语法错误: %w\n\n%s\n如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err, detail)
			}
			return "", fmt.Errorf("JSON 语法错误: %w\n输入不是有效的 JSON（可能缺少括号或逗号）。如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err)
		}
		// fixJSON was true but repair+coerce still couldn't produce a valid wrapper
		return "", fmt.Errorf("JSON 修复后仍无法解析为 {\"jsonml\": [...]}: %w", err)
	}

	bodyAny, ok := wrapper["jsonml"]
	if !ok {
		return "", fmt.Errorf(`--content-format jsonml 输入 JSON 必须包含 "jsonml" 字段，格式: {"jsonml": [...]}`)
	}
	bodyArr, ok := bodyAny.([]any)
	if !ok {
		return "", fmt.Errorf(`--content-format jsonml 字段 "jsonml" 必须是数组`)
	}

	// Root 校验：doc create/update 要求以 ["root", {attrs?}, ...] 为根。
	if len(bodyArr) == 0 {
		return "", fmt.Errorf(`--content-format jsonml body 为空数组，期望 ["root", {attrs?}, ...blocks]`)
	}
	if tag, ok := bodyArr[0].(string); !ok || tag != "root" {
		return "", fmt.Errorf(`--content-format jsonml body 必须以 "root" 为根节点，格式: ["root", {attrs?}, ...blocks]。当前首元素: %v`, bodyArr[0])
	}

	// Validate: always ON.
	// Use ValidateJsonMLSource with original bytes for accurate line/col positions
	// when the original input is a bare JSONML array AND was not repaired.
	// If repair happened, original source is broken — validate parsed data instead.
	var vr *doc.JsonMLValidationResult
	if !repaired && strings.HasPrefix(strings.TrimSpace(originalSrc), "[") {
		vr = doc.ValidateJsonMLSource([]byte(originalSrc))
	} else {
		vr = doc.ValidateJsonMLBodyV2(bodyArr)
	}
	if vr.HasErrors() {
		return "", fmt.Errorf("JSONML 格式校验失败:\n%s",
			vr.Summary())
	}
	if summary := vr.Summary(); summary != "" {
		fmt.Fprintf(os.Stderr, "[WARN] %s\n", summary)
	}

	out, _ := json.Marshal(bodyArr)
	cleaned := stripInputUnsafeChars(string(out))
	return cleaned, nil
}

// prepareJsonMLNode processes the JSONML array passed to `doc block
// insert/update --element` (a single JSONML block node, not a body).
//
// Pipeline: [JSON repair] → coerce → parse → validate → marshal.
// No root 校验——block 级操作不要求 root 根节点。
func prepareJsonMLNode(cmd *cobra.Command, rawElement string) (string, error) {
	if rawElement == "" {
		return "", fmt.Errorf("--content-format jsonml 要求通过 --element 提供 JSONML 数组")
	}
	fixJSON, _ := cmd.Flags().GetBool("fix-jsonml")

	// Preserve original source for position-accurate schema diagnostics.
	originalSrc := rawElement
	repaired := false

	// Step 1: JSON repair — fix broken syntax BEFORE shape detection.
	if fixJSON {
		var probe any
		if err := json.Unmarshal([]byte(rawElement), &probe); err != nil {
			result, repairErr := repairJSONML(rawElement)
			if repairErr != nil {
				return "", fmt.Errorf("JSON 语法错误且自动修复失败: %w\n原始错误: %v", repairErr, err)
			}
			fmt.Fprintf(os.Stderr, "[FIX] JSON 语法已自动修复（括号/逗号等结构性错误）\n")
			rawElement = result
			repaired = true
		}
	}

	// Step 2: Coerce shape — unwrap {"jsonml":[node]} → bare node.
	coerced, coerceNotes, err := coerceJsonMLNodeShape(rawElement)
	if err != nil {
		return "", err
	}
	emitFixNotes(coerceNotes)
	rawElement = coerced

	// Step 3: Parse.
	var node any
	if err := json.Unmarshal([]byte(rawElement), &node); err != nil {
		if !fixJSON {
			// Use parser for detailed positional diagnostics
			detail := diagnoseJSONMLError([]byte(rawElement))
			if detail != "" {
				return "", fmt.Errorf("JSON 语法错误: %w\n\n%s\n如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err, detail)
			}
			return "", fmt.Errorf("JSON 语法错误: %w\n输入不是有效的 JSON（可能缺少括号或逗号）。如果输入来自 LLM 生成，可通过 --fix-jsonml 尝试自动修复", err)
		}
		return "", fmt.Errorf("JSON 语法错误: %w", err)
	}
	if _, ok := node.([]any); !ok {
		return "", fmt.Errorf("--content-format jsonml 要求 --element 为 JSON 数组，实际类型: %T", node)
	}

	// Validate: always ON.
	// Use ValidateJsonMLSource with original bytes for accurate line/col positions
	// when the original input is a bare JSONML node array AND was not repaired.
	var vr *doc.JsonMLValidationResult
	if !repaired && strings.HasPrefix(strings.TrimSpace(originalSrc), "[") {
		vr = doc.ValidateJsonMLSource([]byte(originalSrc))
	} else {
		vr = doc.ValidateJsonMLNodeV2(node)
	}
	if vr.HasErrors() {
		return "", fmt.Errorf("JSONML 格式校验失败:\n%s",
			vr.Summary())
	}
	if summary := vr.Summary(); summary != "" {
		fmt.Fprintf(os.Stderr, "[WARN] %s\n", summary)
	}

	out, _ := json.Marshal(node)
	return stripInputUnsafeChars(string(out)), nil
}

func emitFixNotes(notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "[FIX] JSONML 自动修复（%d 项）:\n", len(notes))
	for i, n := range notes {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, n)
	}
}
