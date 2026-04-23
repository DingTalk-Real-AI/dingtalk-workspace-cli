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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
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
		Short:             i18n.T("待办任务管理"),
		Long:              i18n.T("管理钉钉个人待办：创建、查询列表、查看详情、修改、标记完成、删除。"),
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
	task.AddCommand(
		newTodoTaskCreateCommand(runner),
		newTodoTaskListCommand(runner),
		newTodoTaskUpdateCommand(runner),
		newTodoTaskDoneCommand(runner),
		newTodoTaskGetCommand(runner),
		newTodoTaskDeleteCommand(runner),
	)
	root.AddCommand(task)
	return root
}

// ── create ─────────────────────────────────────────────────

func newTodoTaskCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: i18n.T("创建待办"),
		Example: `  dws todo task create --title "修复线上Bug" --executors userId1,userId2 --priority 40
  dws todo task create --title "提交报告" --executors userId1 --due "2026-03-10T18:00:00+08:00"

  # 查询 userId: dws contact user search --keyword "姓名"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			title := cmdutil.FlagOrFallback(cmd, "title", "subject", "content")
			executorsStr, _ := cmd.Flags().GetString("executors")
			executorIds := parseExecutorIds(executorsStr)

			vo := map[string]any{
				"subject":     title,
				"executorIds": executorIds,
			}
			if v, _ := cmd.Flags().GetString("due"); v != "" {
				ms, err := cmdutil.ParseISOTimeToMillis("due", v)
				if err != nil {
					return err
				}
				vo["dueTime"] = ms
			}
			if v, _ := cmd.Flags().GetString("priority"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					vo["priority"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("recurrence"); v != "" {
				vo["recurrence"] = v
			}
			params := map[string]any{"PersonalTodoCreateVO": vo}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"create_personal_todo",
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("title", "", i18n.T("待办标题 (必填)"))
	_ = cmd.MarkFlagRequired("title")
	cmd.Flags().String("executors", "", i18n.T("执行者 userId 列表 (必填)"))
	_ = cmd.MarkFlagRequired("executors")
	cmd.Flags().String("due", "", i18n.T("截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)"))
	cmd.Flags().String("priority", "", i18n.T("优先级: 10低/20普通/30较高/40紧急"))
	cmd.Flags().String("recurrence", "", i18n.T("循环待办 (需先设置 --due); 格式: DTSTART:...\\nRRULE:FREQ=DAILY;INTERVAL=1"))

	cmd.Flags().String("subject", "", i18n.T("--title 的别名"))
	cmd.Flags().String("content", "", i18n.T("--title 的别名"))
	_ = cmd.Flags().MarkHidden("subject")
	_ = cmd.Flags().MarkHidden("content")

	return cmd
}

// ── list ───────────────────────────────────────────────────

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
				invocation.DryRun = commandDryRun(cmd)
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

// ── update ─────────────────────────────────────────────────

func newTodoTaskUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("修改待办任务"),
		Example: `  dws todo task update --task-id <taskId> --title "新标题"
  dws todo task update --task-id <taskId> --priority 40 --due "2026-03-10T18:00:00+08:00"
  dws todo task update --task-id <taskId> --done true

  # 查询 taskId: dws todo task list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task-id")
			inner := map[string]any{
				"taskId": taskID,
			}
			if v, _ := cmd.Flags().GetString("title"); v != "" {
				inner["subject"] = v
			}
			if v, _ := cmd.Flags().GetString("due"); v != "" {
				ms, err := cmdutil.ParseISOTimeToMillis("due", v)
				if err != nil {
					return err
				}
				inner["dueTime"] = ms
			}
			if v, _ := cmd.Flags().GetString("priority"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					inner["priority"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("done"); v != "" {
				inner["isDone"] = v == "true"
			}
			params := map[string]any{"TodoUpdateRequest": inner}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"update_todo_task",
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("task-id", "", i18n.T("待办任务 ID (必填)"))
	_ = cmd.MarkFlagRequired("task-id")
	cmd.Flags().String("title", "", i18n.T("新标题"))
	cmd.Flags().String("due", "", i18n.T("截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)"))
	cmd.Flags().String("priority", "", i18n.T("优先级: 10低/20普通/30较高/40紧急"))
	cmd.Flags().String("done", "", i18n.T("完成状态: true/false"))
	return cmd
}

// ── done ───────────────────────────────────────────────────

func newTodoTaskDoneCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "done",
		Short: i18n.T("修改执行者的待办完成状态"),
		Example: `  dws todo task done --task-id <taskId> --status true
  dws todo task done --task-id <taskId> --status false

  # 查询 taskId: dws todo task list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task-id")
			status, _ := cmd.Flags().GetString("status")
			params := map[string]any{
				"taskId": taskID,
				"isDone": status,
			}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"update_todo_done_status",
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("task-id", "", i18n.T("待办任务 ID (必填)"))
	_ = cmd.MarkFlagRequired("task-id")
	cmd.Flags().String("status", "", i18n.T("完成状态: true=已完成, false=未完成 (必填)"))
	_ = cmd.MarkFlagRequired("status")
	return cmd
}

// ── get ────────────────────────────────────────────────────

func newTodoTaskGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: i18n.T("待办详情"),
		Example: `  dws todo task get --task-id <taskId>

  # 查询 taskId: dws todo task list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task-id")
			params := map[string]any{
				"taskId": taskID,
			}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"query_todo_detail",
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("task-id", "", i18n.T("待办任务 ID (必填)"))
	_ = cmd.MarkFlagRequired("task-id")
	return cmd
}

// ── delete ─────────────────────────────────────────────────

func newTodoTaskDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: i18n.T("删除待办"),
		Example: `  dws todo task delete --task-id <taskId>
  dws todo task delete --task-id <taskId> --yes

  # 查询 taskId: dws todo task list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task-id")
			if !confirmDeletePrompt(cmd, i18n.T("待办"), taskID) {
				return nil
			}
			params := map[string]any{
				"taskId": taskID,
			}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"delete_todo",
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("task-id", "", i18n.T("待办任务 ID (必填)"))
	_ = cmd.MarkFlagRequired("task-id")
	cmd.Flags().Bool("yes", false, i18n.T("跳过确认直接删除"))
	return cmd
}

// ── helpers ────────────────────────────────────────────────

// parseExecutorIds splits "id1,id2" into []string for the MCP executorIds array.
func parseExecutorIds(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// ── list pagination helpers ────────────────────────────────

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
