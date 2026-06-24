// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	stderrors "errors"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

const (
	devdocArticleSearchTool       = "search_open_platform_docs_rag"
	devdocArticleSearchLegacyTool = "search_open_platform_docs"
	devdocErrorDiagnoseTool       = "search_open_error_code_rag"
)

func init() {
	RegisterPublic(func() Handler {
		return devdocHandler{}
	})
}

type devdocHandler struct{}

func (devdocHandler) Name() string {
	return "devdoc"
}

func (devdocHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "devdoc",
		Short:             "开放平台文档搜索",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	article := &cobra.Command{
		Use:               "article",
		Short:             "文档文章",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	article.AddCommand(newDevdocArticleSearchCommand(runner))
	errorCmd := &cobra.Command{
		Use:               "error",
		Short:             "错误排查",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	errorCmd.AddCommand(newDevdocErrorDiagnoseCommand(runner))
	root.AddCommand(article)
	root.AddCommand(errorCmd)
	return root
}

func newDevdocArticleSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "搜索开放平台文档",
		// Bind the devdoc MCP tool so `dws schema dev.doc.search` can fetch this
		// command's real parameters live from the devdoc server (mcp-source),
		// the same way dev app commands resolve against op-app.
		Annotations:       map[string]string{"mcp-tool": devdocArticleSearchTool, "mcp-source": "devdoc"},
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := devdocFlagOrFallback(cmd, "keyword", "query")
			if query == "" && len(args) > 0 {
				query = strings.TrimSpace(args[0])
			}
			if query == "" {
				return apperrors.NewValidation("--keyword is required")
			}
			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				page = 1
			}
			size := devdocPageSize(cmd)
			cursor := devdocFlagOrFallback(cmd, "cursor")
			return runDevdocToolWithFallback(cmd, runner, devdocArticleSearchTool,
				devdocSearchParams(query, page, size, cursor),
				devdocArticleSearchLegacyTool,
				devdocLegacySearchParams(query, page, size, cursor))
		},
	}
	preferLegacyLeaf(cmd)
	// The published devdoc RAG action accepts keyword. Keep --query as a hidden
	// compatibility alias for older scripts.
	cmd.Flags().String("keyword", "", "搜索关键词 (必填)")
	addDevdocHiddenStringFlag(cmd, "query", "--keyword 的兼容别名")
	cmd.Flags().Int("page", 1, "分页页码 (从 1 开始，默认 1)")
	cmd.Flags().String("cursor", "", "分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page")
	cmd.Flags().Int("size", 20, "单页条数 (默认 20)")
	cmd.Flags().Int("page-size", 0, "--size 的别名")
	_ = cmd.Flags().MarkHidden("page-size")
	return cmd
}

// devdocPageSize reads the page size, accepting --size (primary, == MCP param)
// with --page-size as a compatibility alias; falls back to 20 when neither set.
func devdocPageSize(cmd *cobra.Command) int {
	if cmd.Flags().Changed("size") {
		if v, _ := cmd.Flags().GetInt("size"); v > 0 {
			return v
		}
	}
	if cmd.Flags().Changed("page-size") {
		if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
			return v
		}
	}
	if v, _ := cmd.Flags().GetInt("size"); v > 0 {
		return v
	}
	return 20
}

func newDevdocErrorDiagnoseCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "diagnose",
		Aliases:           []string{"troubleshoot"},
		Short:             "排查开放平台调用错误",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			errorCode := devdocFlagOrFallback(cmd, "error-code")
			errorMessage := devdocFlagOrFallback(cmd, "error-message")
			api := devdocFlagOrFallback(cmd, "api")
			contextText := devdocFlagOrFallback(cmd, "context")
			requestID := devdocFlagOrFallback(cmd, "request-id", "trace-id")
			query := devdocFlagOrFallback(cmd, "query")

			// Validate primary troubleshooting input BEFORE merging api.
			hasPrimaryInput := query != "" || requestID != "" || errorCode != "" || errorMessage != "" || contextText != ""
			if !hasPrimaryInput {
				return apperrors.NewValidation("one of --query, --request-id, --error-code, --error-message, or --context is required")
			}

			query = devdocJoinQueryParts(query, errorMessage, api, contextText)
			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				page = 1
			}
			size, _ := cmd.Flags().GetInt("size")
			if size < 1 {
				size = 10
			}
			cursor := devdocFlagOrFallback(cmd, "cursor")

			params := map[string]any{
				"size": size,
			}
			if cursor != "" {
				params["cursor"] = cursor
			} else {
				params["page"] = page
			}
			if errorCode != "" {
				params["errorCode"] = errorCode
			}
			if query != "" {
				params["query"] = query
			}
			if requestID != "" {
				params["requestId"] = requestID
			}

			fallbackQuery := strings.TrimSpace(strings.Join([]string{errorCode, query, requestID}, " "))
			return runDevdocToolWithFallbacks(cmd, runner, devdocErrorDiagnoseTool, params, []devdocFallbackTool{
				{tool: devdocArticleSearchTool, params: devdocSearchParams(fallbackQuery, page, size, cursor)},
				{tool: devdocArticleSearchLegacyTool, params: devdocLegacySearchParams(fallbackQuery, page, size, cursor)},
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "原始排查问题")
	cmd.Flags().String("error-code", "", "错误码")
	cmd.Flags().String("error-message", "", "错误描述，会合并进原始问题")
	cmd.Flags().String("api", "", "API 名称，会合并进原始问题作为补充检索词")
	cmd.Flags().String("context", "", "额外排查上下文，会合并进原始问题")
	cmd.Flags().String("request-id", "", "开放平台 requestId")
	addDevdocHiddenStringFlag(cmd, "trace-id", "--request-id 的兼容别名")
	cmd.Flags().Int("page", 1, "分页页码 (从 1 开始，默认 1)")
	cmd.Flags().String("cursor", "", "分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page")
	cmd.Flags().Int("size", 10, "分页大小 (默认 10)")
	return cmd
}

func devdocSearchParams(query string, page int, size int, cursor string) map[string]any {
	params := map[string]any{
		"keyword": query,
		"size":    size,
	}
	if cursor != "" {
		params["cursor"] = cursor
	} else {
		params["page"] = page
	}
	return params
}

func devdocLegacySearchParams(keyword string, page int, size int, cursor string) map[string]any {
	return map[string]any{
		"keyword": keyword,
		"page":    devdocPageFromCursor(page, cursor),
		"size":    size,
	}
}

func devdocPageFromCursor(page int, cursor string) int {
	if cursor == "" {
		return page
	}
	cursorPage, err := strconv.Atoi(cursor)
	if err == nil && cursorPage > 0 {
		return cursorPage
	}
	return page
}

func runDevdocTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runDevdocToolWithFallback(cmd, runner, tool, params, "", nil)
}

type devdocFallbackTool struct {
	tool   string
	params map[string]any
}

func runDevdocToolWithFallback(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any, fallbackTool string, fallbackParams map[string]any) error {
	var fallbacks []devdocFallbackTool
	if fallbackTool != "" {
		fallbacks = append(fallbacks, devdocFallbackTool{tool: fallbackTool, params: fallbackParams})
	}
	return runDevdocToolWithFallbacks(cmd, runner, tool, params, fallbacks)
}

func runDevdocToolWithFallbacks(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any, fallbacks []devdocFallbackTool) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		"devdoc",
		tool,
		params,
	)
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, invocation)
	}
	result, err := runner.Run(cmd.Context(), invocation)
	currentTool := tool
	for _, fallback := range fallbacks {
		if err == nil && !shouldFallbackDevdocResult(currentTool, result) {
			break
		}
		if err != nil && !isDevdocToolNotFoundError(err) {
			break
		}
		if fallback.tool == "" {
			continue
		}
		fallbackParams := fallback.params
		if fallbackParams == nil {
			fallbackParams = params
		}
		fallbackInvocation := executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd),
			"devdoc",
			fallback.tool,
			fallbackParams,
		)
		result, err = runner.Run(cmd.Context(), fallbackInvocation)
		currentTool = fallback.tool
	}
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func shouldFallbackDevdocResult(tool string, result executor.Result) bool {
	if tool != devdocArticleSearchTool || !result.Invocation.Implemented {
		return false
	}
	content, ok := result.Response["content"].(map[string]any)
	if !ok {
		return false
	}
	return isEmptyDevdocArticleRAGContent(content)
}

func isEmptyDevdocArticleRAGContent(content map[string]any) bool {
	if hasDevdocArticleRAGPayload(content) {
		return false
	}
	for _, key := range []string{"materials", "references", "ragContext", "result", "success"} {
		if _, ok := content[key]; ok {
			return true
		}
	}
	return false
}

func hasDevdocArticleRAGPayload(content map[string]any) bool {
	if hasNonEmptyList(content["materials"]) || hasNonEmptyList(content["references"]) || hasNonEmptyString(content["ragContext"]) {
		return true
	}
	result, _ := content["result"].(map[string]any)
	return hasNonEmptyList(result["items"]) || hasNonEmptyList(result["materials"]) ||
		hasNonEmptyList(result["references"]) || hasNonEmptyString(result["ragContext"])
}

func hasNonEmptyList(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}

func hasNonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func isDevdocToolNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizedDevdocErrorText(err)
	return strings.Contains(msg, "tool not found") ||
		strings.Contains(msg, "unknown tool") ||
		strings.Contains(msg, "mcp_tool_not_found") ||
		strings.Contains(msg, "未找到指定工具")
}

func normalizedDevdocErrorText(err error) string {
	parts := []string{strings.ToLower(err.Error())}
	var typed *apperrors.Error
	if stderrors.As(err, &typed) && typed != nil {
		parts = append(parts,
			strings.ToLower(typed.Reason),
			strings.ToLower(typed.ServerDiag.ServerErrorCode),
			strings.ToLower(typed.ServerDiag.TechnicalDetail),
			strings.ToLower(typed.Hint),
		)
	}
	return strings.Join(parts, " ")
}

func devdocFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	names := append([]string{primary}, aliases...)
	for _, name := range names {
		value, err := cmd.Flags().GetString(name)
		if err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func devdocSetStringParam(params map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		params[key] = strings.TrimSpace(value)
	}
}

func devdocJoinQueryParts(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, " ")
}

func addDevdocHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}
