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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
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
		Use:               "search [keyword]",
		Short:             "搜索开放平台文档",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := devdocFlagOrFallback(cmd, "query", "keyword")
			if keyword == "" && len(args) > 0 {
				keyword = strings.TrimSpace(args[0])
			}
			if keyword == "" {
				return apperrors.NewValidation("--query is required")
			}
			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				page = 1
			}
			size, _ := cmd.Flags().GetInt("size")
			if size < 1 {
				size = 10
			}
			return runDevdocTool(cmd, runner, "search_open_platform_docs_rag", map[string]any{
				"keyword": keyword,
				"page":    page,
				"size":    size,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	addDevdocHiddenStringFlag(cmd, "keyword", "--query 的悟空兼容别名")
	cmd.Flags().Int("page", 1, "分页页码 (从 1 开始，默认 1)")
	cmd.Flags().Int("size", 10, "分页大小 (默认 10)")
	return cmd
}

func newDevdocErrorDiagnoseCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "diagnose",
		Aliases:           []string{"troubleshoot"},
		Short:             "排查开放平台调用错误",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			requestID := devdocFlagOrFallback(cmd, "request-id", "trace-id")
			errorCode := devdocFlagOrFallback(cmd, "error-code")
			errorMessage := devdocFlagOrFallback(cmd, "error-message")
			contextValue := devdocFlagOrFallback(cmd, "context")
			query := devdocFlagOrFallback(cmd, "query")
			hasPrimaryInput := query != "" || requestID != "" || errorCode != "" || errorMessage != "" || contextValue != ""
			if !hasPrimaryInput {
				return apperrors.NewValidation("one of --query, --request-id, --error-code, --error-message, or --context is required")
			}
			combinedQuery := devdocJoinQueryParts(query, errorMessage, devdocFlagOrFallback(cmd, "api"), contextValue)
			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				page = 1
			}
			size, _ := cmd.Flags().GetInt("size")
			if size < 1 {
				size = 10
			}
			params := map[string]any{
				"page": page,
				"size": size,
			}
			devdocSetStringParam(params, "query", combinedQuery)
			devdocSetStringParam(params, "requestId", requestID)
			devdocSetStringParam(params, "errorCode", errorCode)
			return runDevdocTool(cmd, runner, "search_open_error_code_rag", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "原始排查问题")
	cmd.Flags().String("request-id", "", "开放平台 requestId")
	addDevdocHiddenStringFlag(cmd, "trace-id", "--request-id 的兼容别名")
	cmd.Flags().String("error-code", "", "错误码")
	cmd.Flags().String("error-message", "", "错误描述，会合并进原始问题")
	cmd.Flags().String("api", "", "API 名称，会合并进原始问题作为补充检索词")
	cmd.Flags().String("context", "", "额外排查上下文，会合并进原始问题")
	cmd.Flags().Int("page", 1, "分页页码 (从 1 开始，默认 1)")
	cmd.Flags().Int("size", 10, "分页大小 (默认 10)")
	return cmd
}

func runDevdocTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
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
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
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
