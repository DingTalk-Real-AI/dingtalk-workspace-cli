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
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/i18n"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/spf13/cobra"
)

const todoListPageSizeMax = 20

func init() {
	RegisterPublic(func() Handler {
		return todoHandler{}
	})
}

type todoHandler struct{}

func (todoHandler) Name() string {
	return "todo"
}

func (todoHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "todo",
		Short:             "Todo helper overrides",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	task := &cobra.Command{
		Use:               "task",
		Short:             i18n.T("创建 / 查询 / 更新 / 删除待办"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	task.AddCommand(newTodoTaskListCommand(runner))
	root.AddCommand(task)
	return root
}

func newTodoTaskListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("查询待办列表"),
		Example:           `  dws todo task list --page 1 --size 20 --status false`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetString("page")
			sizeRaw, _ := cmd.Flags().GetString("size")
			status, _ := cmd.Flags().GetString("status")

			page = normalizePage(page)
			size := normalizeSize(sizeRaw)
			summaryParams := todoListRequestParams(page, strconv.Itoa(size), status)
			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"get_user_todos_in_current_org",
				summaryParams,
			)

			if size <= todoListPageSizeMax {
				result, err := runner.Run(cmd.Context(), invocation)
				if err != nil {
					return err
				}
				return writeCommandPayload(cmd, result)
			}

			if commandDryRun(cmd) {
				invocation.DryRun = true
				return writeCommandPayload(cmd, todoListPreviewResult(invocation, size, "automatic pagination preview"))
			}

			startPage, _ := strconv.Atoi(page)
			if startPage < 1 {
				startPage = 1
			}
			merged := make([]any, 0, size)
			for pageNum := startPage; len(merged) < size; pageNum++ {
				pageParams := todoListRequestParams(strconv.Itoa(pageNum), strconv.Itoa(todoListPageSizeMax), status)
				pageInvocation := executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd),
					"todo",
					"get_user_todos_in_current_org",
					pageParams,
				)
				pageResult, err := runner.Run(cmd.Context(), pageInvocation)
				if err != nil {
					return err
				}
				if !pageResult.Invocation.Implemented && len(helperResponseContent(pageResult)) == 0 {
					invocation.DryRun = pageResult.Invocation.DryRun
					return writeCommandPayload(cmd, todoListPreviewResult(invocation, size, "automatic pagination requires runtime execution"))
				}

				cards := todoCardsFromResult(pageResult)
				if len(cards) == 0 {
					break
				}
				for _, card := range cards {
					merged = append(merged, card)
					if len(merged) >= size {
						break
					}
				}
				if len(cards) < todoListPageSizeMax {
					break
				}
			}

			invocation.Implemented = true
			return writeCommandPayload(cmd, executor.Result{
				Invocation: invocation,
				Response: map[string]any{
					"content": map[string]any{
						"result": map[string]any{
							"todoCards": merged,
						},
					},
				},
			})
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("page", "1", i18n.T("页码 (必填)"))
	cmd.Flags().String("size", "20", i18n.T("获取数量，超过 20 自动分页 (默认 20)"))
	cmd.Flags().String("status", "", i18n.T("true=已完成, false=未完成"))
	return cmd
}

func normalizePage(raw string) string {
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		return trimmed
	}
	return "1"
}

func normalizeSize(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return todoListPageSizeMax
	}
	return value
}

func estimateTodoListRequests(size int) int {
	return (size + todoListPageSizeMax - 1) / todoListPageSizeMax
}

func todoListPreviewResult(invocation executor.Invocation, size int, note string) executor.Result {
	response := map[string]any{
		"estimated_requests": estimateTodoListRequests(size),
		"page_size_limit":    todoListPageSizeMax,
		"note":               note,
	}
	if invocation.DryRun {
		response["dry_run"] = true
	}
	return executor.Result{
		Invocation: invocation,
		Response:   response,
	}
}

func todoListRequestParams(page, pageSize, status string) map[string]any {
	pageSize = strings.TrimSpace(pageSize)
	if pageSize == "" {
		pageSize = strconv.Itoa(todoListPageSizeMax)
	}
	params := map[string]any{
		"pageNum":  normalizePage(page),
		"pageSize": pageSize,
	}
	status = strings.TrimSpace(status)
	if status != "" {
		params["isDone"] = status
		params["todoStatus"] = status
	}
	return params
}

func todoCardsFromResult(result executor.Result) []any {
	content := helperResponseContent(result)
	if len(content) == 0 {
		return nil
	}
	if payload, ok := content["result"].(map[string]any); ok {
		if cards, ok := payload["todoCards"].([]any); ok {
			return cards
		}
	}
	if cards, ok := content["todoCards"].([]any); ok {
		return cards
	}
	return nil
}
