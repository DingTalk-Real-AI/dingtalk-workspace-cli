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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

  # 查询 userId: dws contact user search --query "姓名"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			title := cmdutil.FlagOrFallback(cmd, "title", "subject", "content")
			if strings.TrimSpace(title) == "" {
				return apperrors.NewValidation("--title is required")
			}
			executorsStr, _ := cmd.Flags().GetString("executors")
			if strings.TrimSpace(executorsStr) == "" {
				return apperrors.NewValidation("--executors is required")
			}
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
	cmd.Flags().String("executors", "", i18n.T("执行者 userId 列表，逗号分隔 (必填)。注意: 此处是通讯录 userId，可通过 dws contact user search --query 姓名 查询"))
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
		Use:   "list",
		Short: i18n.T("查询待办列表"),
		Long: i18n.T(`查询当前用户在当前企业的待办列表。

覆盖范围:
  返回当前用户作为"执行者"(executor) 的待办。
  仅参与但不执行的待办、自己创建但交给他人执行的待办不在返回范围内。

  当前列表能力面向"个人待办"，即钉钉待办模块中展示的待办任务，
  不包含 OA 审批流待办、Teambition 项目任务等其他业务线的待办。

分页:
  默认每页 20 条。--size 超过 20 时，CLI 会自动进行多次 API 调用
  并合并结果（自动分页），无需手动翻页。`),
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
			if strings.TrimSpace(taskID) == "" {
				return apperrors.NewValidation("--task-id is required")
			}
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
			if strings.TrimSpace(taskID) == "" {
				return apperrors.NewValidation("--task-id is required")
			}
			status, _ := cmd.Flags().GetString("status")
			if strings.TrimSpace(status) == "" {
				return apperrors.NewValidation("--status is required")
			}
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
	cmd.Flags().String("status", "", i18n.T("完成状态: true=已完成, false=未完成 (必填)"))
	return cmd
}

// ── get ────────────────────────────────────────────────────

func newTodoTaskGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: i18n.T("待办详情"),
		Long: i18n.T(`查看待办任务详情。

返回字段说明:
  creatorId / executorIds / participantIds / modifierId
    待办系统内部人员标识（短数字 ID，如 6380165826），
    不是通讯录 userId（如 035551044606950179）或 unionid。
    这些 ID 在待办系统内对同一用户稳定，但无法直接用于通讯录 API 查询。
    如需获取人员姓名，可参考返回中的 creatorInfo / executorInfos /
    participantInfos 字段（包含 name 属性）。

  bizTag / source
    底层待办引擎的实现标识。即使是在钉钉客户端直接创建的普通个人待办，
    也会返回 "teambition"，这是内核实现细节，不代表来自 Teambition 产品。

  tenantId / tenantType
    待办所属的租户标识，非企业 corpId。tenantType 为 "user" 时
    tenantId 是用户维度标识；为 "org" 时是组织维度标识。`),
		Example: `  dws todo task get --task-id <taskId>

  # 查询 taskId: dws todo task list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task-id")
			if strings.TrimSpace(taskID) == "" {
				return apperrors.NewValidation("--task-id is required")
			}
			params := map[string]any{
				"taskId": taskID,
			}

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"todo",
				"get_todo_detail",
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
			if strings.TrimSpace(taskID) == "" {
				return apperrors.NewValidation("--task-id is required")
			}
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
