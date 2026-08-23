package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws todo — 待办
// MCP 工具名与入参以 tools/list 为准；待办详情 query_todo_detail、删除 delete_todo，入参 taskId。
// ──────────────────────────────────────────────────────────

const todoListPageSizeMax = 20

var todoFileMD5Hex = fileMD5Hex

func ensureTodoTaskExists(ctx context.Context, taskID string) error {
	text, err := callMCPToolReturnTextOnServer(ctx, "todo", "get_todo_detail", map[string]any{
		"taskId": taskID,
	})
	if err != nil {
		return fmt.Errorf("待办任务 %q 不存在或不可访问: %w", taskID, err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return fmt.Errorf("校验待办任务 %q 时无法解析详情响应: %w", taskID, err)
	}
	result, ok := body["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("待办任务 %q 不存在或详情响应缺少 result", taskID)
	}
	detail, ok := result["todoDetailModel"].(map[string]any)
	if !ok {
		return fmt.Errorf("待办任务 %q 不存在或详情响应缺少 todoDetailModel", taskID)
	}
	returnedTaskID := stringFromJSONScalar(detail["taskId"])
	if returnedTaskID == "" || returnedTaskID != taskID {
		return fmt.Errorf("待办任务 %q 不存在或详情响应中的 taskId 不匹配", taskID)
	}
	return nil
}

func newTodoCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/todo.json
	// products.todo). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "todo",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理待办任务、标签、子任务、执行人、参与人、评论、附件与提醒",
			UseWhen: []string{
				"查询、创建或更新个人待办及其协作信息时",
			},
			AvoidWhen: []string{
				"不要用于 OA 审批流转、工作日志提交或日历日程管理",
			},
		},
	})
	todoCmd := &cobra.Command{
		Use:   "todo",
		Short: "待办任务管理",
		Long:  `管理钉钉个人待办：创建、查询列表、查看详情、修改、标记完成、删除。`,
		RunE:  groupRunE,
	}

	todoTaskCmd := &cobra.Command{Use: "task", Short: "创建 / 查询 / 更新 / 删除待办", RunE: groupRunE}

	todoTaskCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建待办",
		Example: `  dws todo task create --title "修复线上Bug" --executors userId1,userId2 --priority 40
  dws todo task create --title "提交报告" --executors userId1 --due "2026-03-10T18:00:00+08:00"

  # 查询 userId: dws contact user search --keyword "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectUnsupportedTodoReminderFlags(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "title", "subject", "content"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "executors"); err != nil {
				return err
			}
			executorsStr := mustGetFlag(cmd, "executors")
			executorIds := parseExecutorIds(executorsStr)
			toolArgs := map[string]any{
				"PersonalTodoCreateVO": map[string]any{
					"subject":     flagOrFallback(cmd, "title", "subject", "content"),
					"executorIds": executorIds,
				},
			}
			if v, _ := cmd.Flags().GetString("due"); v != "" {
				ms, err := parseISOTimeToMillis("due", v)
				if err != nil {
					return err
				}
				toolArgs["PersonalTodoCreateVO"].(map[string]any)["dueTime"] = ms
			}
			if v, _ := cmd.Flags().GetString("priority"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					toolArgs["PersonalTodoCreateVO"].(map[string]any)["priority"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("recurrence"); v != "" {
				toolArgs["PersonalTodoCreateVO"].(map[string]any)["recurrence"] = v
			}
			return callMCPTool("create_personal_todo", toolArgs)
		},
	}
	DeclareLeafMetadata(todoTaskCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "create_personal_todo",
				CanonicalPath:  "todo.create_personal_todo",
				CLIPath:        "todo task create",
				PrimaryCLIPath: "todo task create",
			},
			Description: "创建个人待办",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "create_personal_todo"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建个人待办",
				UseWhen:      []string{"需要在当前组织创建个人待办（标题与执行人必填；可选截止时间、优先级、按天循环）时"},
				AvoidWhen: []string{
					"只需查询已有待办时改用 dws todo task list",
					"要创建子待办时改用 dws todo task create-sub；标题或执行人未确认时不要创建",
				},
				Examples: []string{
					"dws todo task create --title \"修复线上Bug\" --executors <USER_ID_1>,<USER_ID_2> --priority 40",
					"dws todo task create --title \"每日站会\" --executors <USER_ID> --due \"2026-03-20T10:00:00+08:00\" --recurrence \"DTSTART:20260320T020000Z\\nRRULE:FREQ=DAILY;INTERVAL=1\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "due", Property: "PersonalTodoCreateVO.dueTime"},
				{Name: "executors", Property: "PersonalTodoCreateVO.executorIds"},
				{Name: "priority", Property: "PersonalTodoCreateVO.priority"},
				{Name: "recurrence", Property: "PersonalTodoCreateVO.recurrence"},
				{Name: "title", Property: "PersonalTodoCreateVO.subject"},
			},
		},
	})

	todoTaskCreateSubCmd := &cobra.Command{
		Use:   "create-sub",
		Short: "创建子待办",
		Example: `  dws todo task create-sub --parent-id <parentId> --title "子任务标题" --executors userId1,userId2 --priority 40
  dws todo task create-sub --parent-id <parentId> --title "子任务标题" --executors userId1 --due "2026-03-10T18:00:00+08:00"

  # 查询 parentId: dws todo task list
  # 查询 userId: dws contact user search --keyword "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectUnsupportedTodoReminderFlags(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "title", "subject", "content"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "executors", "parent-id"); err != nil {
				return err
			}
			parentId := mustGetFlag(cmd, "parent-id")
			if _, err := strconv.ParseInt(parentId, 10, 64); err != nil {
				return &CLIError{
					Code:       CodeMissingParam,
					Message:    fmt.Sprintf("父待办 ID 必须是纯数字，当前值: %s", parentId),
					Suggestion: "请通过 'dws todo task list' 获取正确的父待办任务 ID。",
					Operation:  "todo.task.create-sub.parent-id",
				}
			}
			executorsStr := mustGetFlag(cmd, "executors")
			executorIds := parseExecutorIds(executorsStr)
			toolArgs := map[string]any{
				"PersonalTodoCreateVO": map[string]any{
					"subject":     flagOrFallback(cmd, "title", "subject", "content"),
					"executorIds": executorIds,
					"parentId":    mustGetFlag(cmd, "parent-id"),
				},
			}
			if v, _ := cmd.Flags().GetString("due"); v != "" {
				ms, err := parseISOTimeToMillis("due", v)
				if err != nil {
					return err
				}
				toolArgs["PersonalTodoCreateVO"].(map[string]any)["dueTime"] = ms
			}
			if v, _ := cmd.Flags().GetString("priority"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					toolArgs["PersonalTodoCreateVO"].(map[string]any)["priority"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("recurrence"); v != "" {
				toolArgs["PersonalTodoCreateVO"].(map[string]any)["recurrence"] = v
			}
			return callMCPTool("create_personal_sub_todo", toolArgs)
		},
	}
	DeclareLeafMetadata(todoTaskCreateSubCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "create_personal_sub_todo",
				CanonicalPath:  "todo.create_personal_sub_todo",
				CLIPath:        "todo task create-sub",
				PrimaryCLIPath: "todo task create-sub",
			},
			Description: "创建个人子待办",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "create_personal_sub_todo"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建个人子待办",
				UseWhen:      []string{"父待办由本人创建且已确认 parentId，需要创建子待办（标题、执行人、可选截止时间/优先级）时"},
				AvoidWhen: []string{
					"只需创建顶层待办时改用 dws todo task create",
					"父待办不是本人创建或 parentId 未确认时不要创建；子待办不支持独立循环规则",
				},
				Examples: []string{
					"dws todo task create-sub --parent-id <PARENT_TASK_ID> --title \"子任务标题\" --executors <USER_ID_1>,<USER_ID_2> --priority 40",
					"dws todo task create-sub --parent-id <PARENT_TASK_ID> --title \"子任务标题\" --executors <USER_ID> --due \"2026-03-20T10:00:00+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "due", Property: "PersonalTodoCreateVO.dueTime"},
				{Name: "executors", Property: "PersonalTodoCreateVO.executorIds"},
				{Name: "parent-id", Property: "PersonalTodoCreateVO.parentId"},
				{Name: "priority", Property: "PersonalTodoCreateVO.priority"},
				{Name: "recurrence", Property: "PersonalTodoCreateVO.recurrence"},
				{Name: "title", Property: "PersonalTodoCreateVO.subject"},
			},
		},
	})

	todoTaskListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询待办列表",
		Example: `  dws todo task list --page 1 --size 20 --status false --priority 40,30,10 --role-types creator,executor,participant
--plan-finish-date-start 2026-03-01T00:00:00+08:00 --plan-finish-date-end 2026-03-10T18:00:00+08:00 `,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageStr := mustGetFlag(cmd, "page")
			sizeStr := mustGetFlag(cmd, "size")
			size, err := strconv.Atoi(sizeStr)
			if err != nil || size < 1 {
				size = 20
			}

			if size <= todoListPageSizeMax {
				toolArgs := map[string]any{
					"pageNum":  pageStr,
					"pageSize": sizeStr,
				}
				err := buildListTodoTaskArgs(cmd, toolArgs)
				if err != nil {
					return err
				}
				toolName := "get_user_todos_in_current_org"
				if queryAll, _ := cmd.Flags().GetBool("query-all"); queryAll {
					toolName = "get_user_todos"
				}
				return callMCPTool(toolName, toolArgs)
			}
			return todoListAutoPage(cmd, pageStr, size)
		},
	}
	DeclareLeafMetadata(todoTaskListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "get_user_todos_in_current_org",
				CanonicalPath:  "todo.get_user_todos_in_current_org",
				CLIPath:        "todo task list",
				PrimaryCLIPath: "todo task list",
			},
			Description: "查询当前组织待办，或通过 --query-all 跨组织查询全部待办",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "get_user_todos_in_current_org"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前组织待办，或通过 --query-all 跨组织查询全部待办",
				UseWhen:      []string{"需要按完成状态、优先级、角色或截止日期范围查询当前用户待办列表时；跨组织范围时显式使用 --query-all"},
				AvoidWhen:    []string{"已知 taskId 需要完整单条详情时改用 dws todo task get"},
				Examples: []string{
					"dws todo task list --page 1 --size 20 --status false",
					"dws todo task list --page 1 --size 20 --status false --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query-all", Required: boolPtr(false), InterfaceType: "boolean", Description: "为 true 时跨组织查询全部待办；默认仅查询当前组织待办"},
				{Name: "role-types", Property: "roleTypes", Required: boolPtr(false), Description: "角色类型列表；省略时运行时默认使用 executor"},
				{Name: "page", Property: "pageNum"},
				{Name: "priority", Property: "priorityList"},
				{Name: "size", Property: "pageSize"},
				{Name: "status", Property: "todoStatus"},
			},
		},
	})

	todoTaskListSubCmd := &cobra.Command{
		Use:   "list-sub",
		Short: "查询子待办列表",
		Example: `  dws todo task list-sub --task-id <taskId>
  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskId := mustGetFlag(cmd, "task-id")
			toolArgs := map[string]any{
				"taskId": taskId,
			}
			return callMCPTool("list_sub_tasks", map[string]any{
				"todoSubTaskListRequest": toolArgs,
			})
		},
	}

	todoTaskUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "修改待办任务",
		Example: `  dws todo task update --task-id <taskId> --title "新标题"
  dws todo task update --task-id <taskId> --priority 40 --due "2026-03-10T18:00:00+08:00"
  dws todo task update --task-id <taskId> --done true

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectUnsupportedTodoReminderFlags(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			inner := map[string]any{
				"taskId": mustGetFlag(cmd, "task-id"),
			}
			if v, _ := cmd.Flags().GetString("title"); v != "" {
				inner["subject"] = v
			}
			if v, _ := cmd.Flags().GetString("due"); v != "" {
				ms, err := parseISOTimeToMillis("due", v)
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
			return callMCPTool("update_todo_task", map[string]any{
				"TodoUpdateRequest": inner,
			})
		},
	}
	DeclareLeafMetadata(todoTaskUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "update_todo_task",
				CanonicalPath:  "todo.update_todo_task",
				CLIPath:        "todo task update",
				PrimaryCLIPath: "todo task update",
			},
			Description: "修改整个待办任务",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "update_todo_task"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改整个待办任务",
				UseWhen:      []string{"已知 taskId，需要修改待办标题、截止时间、优先级或完成标记等字段时"},
				AvoidWhen: []string{
					"只需改执行者完成状态时优先 dws todo task done",
					"目标任务与待改字段未确认时不要更新",
				},
				Examples: []string{
					"dws todo task update --task-id <taskId> --title \"新标题\"",
					"dws todo task update --task-id <taskId> --priority 40 --due \"2026-03-10T18:00:00+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "done", Property: "TodoUpdateRequest.isDone"},
				{Name: "due", Property: "TodoUpdateRequest.dueTime"},
				{Name: "priority", Property: "TodoUpdateRequest.priority"},
				{Name: "task-id", Property: "TodoUpdateRequest.taskId"},
				{Name: "title", Property: "TodoUpdateRequest.subject"},
			},
		},
	})

	todoTaskDoneCmd := &cobra.Command{
		Use:   "done",
		Short: "修改执行者的待办完成状态",
		Example: `  dws todo task done --task-id <taskId> --status true
  dws todo task done --task-id <taskId> --status false

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "status"); err != nil {
				return err
			}
			taskID := mustGetFlag(cmd, "task-id")
			if !deps.Caller.DryRun() {
				if err := ensureTodoTaskExists(cmd.Context(), taskID); err != nil {
					return err
				}
			}
			return callMCPTool("update_todo_done_status", map[string]any{
				"taskId": taskID,
				"isDone": mustGetFlag(cmd, "status"),
			})
		},
	}
	DeclareLeafMetadata(todoTaskDoneCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "update_todo_done_status",
				CanonicalPath:  "todo.update_todo_done_status",
				CLIPath:        "todo task done",
				PrimaryCLIPath: "todo task done",
			},
			Description: "修改执行者的待办完成状态",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "update_todo_done_status"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改执行者的待办完成状态",
				UseWhen:      []string{"已知 taskId，需要把执行者侧完成状态设为已完成或未完成时"},
				AvoidWhen: []string{
					"要改标题/截止时间/优先级时改用 dws todo task update",
					"执行人、目标待办或期望状态未确认时不要修改",
				},
				Examples: []string{
					"dws todo task done --task-id <taskId> --status true",
					"dws todo task done --task-id <taskId> --status false",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "status", Property: "isDone"},
			},
		},
	})

	// todoTaskGetCmd 对应 MCP 待办详情；入参以 tools/list 为准，当前为 taskId。
	todoTaskGetCmd := &cobra.Command{
		Use:   "get",
		Short: "待办详情",
		Long: `查询单条待办的详情。

当前上游详情接口不返回 reminderRules；本命令不能读取或验证 add-reminder /
reset-reminder 写入的提醒规则。提醒写命令的成功响应只能作为写入回执。`,
		Example: `  dws todo task get --task-id <taskId>

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			raw, err := callMCPToolReturnText(context.Background(), "get_todo_detail", map[string]any{
				"taskId": mustGetFlag(cmd, "task-id"),
			})
			if err != nil {
				return err
			}
			if raw == "" {
				return nil
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				deps.Out.PrintRaw(raw)
				return nil
			}
			// Transform detailUrl appUrl and pcUrl to dingtalk protocol links
			if r, ok := parsed["result"].(map[string]any); ok {
				if model, ok := r["todoDetailModel"].(map[string]any); ok {
					if detailUrl, ok := model["detailUrl"].(map[string]any); ok {
						const prefix = "dingtalk://dingtalkclient/page/link?pc_slide=true&url="
						if appUrl, ok := detailUrl["appUrl"].(string); ok && appUrl != "" {
							detailUrl["appUrl"] = prefix + url.QueryEscape(appUrl)
						}
						if pcUrl, ok := detailUrl["pcUrl"].(string); ok && pcUrl != "" {
							detailUrl["pcUrl"] = prefix + url.QueryEscape(pcUrl)
						}
					}
				}
			}
			return deps.Out.PrintJSON(parsed)
		},
	}
	DeclareLeafMetadata(todoTaskGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "get_todo_detail",
				CanonicalPath:  "todo.get_todo_detail",
				CLIPath:        "todo task get",
				PrimaryCLIPath: "todo task get",
			},
			Description: "查询待办详情",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "get_todo_detail"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询待办详情",
				UseWhen:      []string{"已知 taskId，且调用者是创建者或执行者，需要查看单条待办详情时"},
				AvoidWhen: []string{
					"需要按条件查多个待办时改用 dws todo task list",
					"需要修改待办时改用 update/done/delete 等写命令",
					"需要读取或验证提醒规则时不要使用；当前详情接口不返回 reminderRules",
				},
				Examples: []string{
					"dws todo task get --task-id <taskId>",
					"dws todo task get --task-id <taskId> --format json",
				},
			},
		},
	})

	// todoTaskDeleteCmd 对应 MCP 删除待办；入参以 tools/list 为准，当前为 taskId。
	todoTaskDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除待办",
		Example: `  dws todo task delete --task-id <taskId>
  dws todo task delete --task-id <taskId> --yes

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskId := mustGetFlag(cmd, "task-id")
			return callMCPTool("delete_todo", map[string]any{
				"taskId": taskId,
			})
		},
	}
	DeclareLeafMetadata(todoTaskDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "delete_todo",
				CanonicalPath:  "todo.delete_todo",
				CLIPath:        "todo task delete",
				PrimaryCLIPath: "todo task delete",
			},
			Description: "删除待办",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "delete_todo"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除待办",
				UseWhen:      []string{"用户明确要求删除指定待办（所有执行者侧一并删除）时"},
				AvoidWhen: []string{
					"用户未明确删除、目标 taskId 不确定或仍需保留任务记录时不要执行",
					"只需标记完成时改用 dws todo task done",
				},
				Examples: []string{"dws todo task delete --task-id <taskId>"},
			},
		},
	})

	todoTaskAddExecutorCmd := &cobra.Command{
		Use:   "add-executor",
		Short: "添加待办执行人",
		Example: `  dws todo task add-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "executors"); err != nil {
				return err
			}
			executorsStr := mustGetFlag(cmd, "executors")
			executorIds := parseExecutorIds(executorsStr)
			return callMCPTool("add_task_executors", map[string]any{
				"todoExecutorsAddRequest": map[string]any{
					"taskId":      mustGetFlag(cmd, "task-id"),
					"executorIds": executorIds,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskAddExecutorCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "add_task_executors",
				CanonicalPath:  "todo.add_task_executors",
				CLIPath:        "todo task add-executor",
				PrimaryCLIPath: "todo task add-executor",
			},
			Description: "添加待办执行人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "add_task_executors"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "添加待办执行人",
				UseWhen:      []string{"已知 taskId，需要为待办追加执行人（userId 列表）时"},
				AvoidWhen: []string{
					"要添加参与人时改用 dws todo task add-participant",
					"要上传附件时改用 dws todo task add-attachment",
					"待办或执行人未确认时不要添加",
				},
				Examples: []string{
					"dws todo task add-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>",
					"dws todo task add-executor --task-id <taskId> --executors userId1,userId2 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "executors", Property: "todoExecutorsAddRequest.executorIds"},
				{Name: "task-id", Property: "todoExecutorsAddRequest.taskId"},
			},
		},
	})
	todoTaskRemoveExecutorCmd := &cobra.Command{
		Use:   "remove-executor",
		Short: "移除待办执行人",
		Example: `  dws todo task remove-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "executors"); err != nil {
				return err
			}
			executorsStr := mustGetFlag(cmd, "executors")
			executorIds := parseExecutorIds(executorsStr)
			return callMCPTool("remove_task_executors", map[string]any{
				"todoExecutorsRemoveRequest": map[string]any{
					"taskId":      mustGetFlag(cmd, "task-id"),
					"executorIds": executorIds,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskRemoveExecutorCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "remove_task_executors",
				CanonicalPath:  "todo.remove_task_executors",
				CLIPath:        "todo task remove-executor",
				PrimaryCLIPath: "todo task remove-executor",
			},
			Description: "移除待办执行人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "remove_task_executors"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除待办执行人",
				UseWhen:      []string{"用户明确要求从待办移除指定执行人时"},
				AvoidWhen: []string{
					"要添加执行人时改用 dws todo task add-executor",
					"待办或执行人未确认时不要移除",
				},
				Examples: []string{
					"dws todo task remove-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>",
					"dws todo task remove-executor --task-id <taskId> --executors userId1 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "executors", Property: "todoExecutorsRemoveRequest.executorIds"},
				{Name: "task-id", Property: "todoExecutorsRemoveRequest.taskId"},
			},
		},
	})
	todoTaskAddParticipantCmd := &cobra.Command{
		Use:   "add-participant",
		Short: "添加待办参与人",
		Example: `  dws todo task add-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "participants"); err != nil {
				return err
			}
			participantsStr := mustGetFlag(cmd, "participants")
			participantIds := parseExecutorIds(participantsStr)
			return callMCPTool("add_task_participants", map[string]any{
				"todoParticipantsAddRequest": map[string]any{
					"taskId":         mustGetFlag(cmd, "task-id"),
					"participantIds": participantIds,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskAddParticipantCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "add_task_participants",
				CanonicalPath:  "todo.add_task_participants",
				CLIPath:        "todo task add-participant",
				PrimaryCLIPath: "todo task add-participant",
			},
			Description: "添加待办参与人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "add_task_participants"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "添加待办参与人",
				UseWhen:      []string{"已知 taskId，需要为待办追加参与人（userId 列表）时"},
				AvoidWhen: []string{
					"要添加执行人时改用 dws todo task add-executor",
					"要上传附件时改用 dws todo task add-attachment",
					"待办或参与人未确认时不要添加",
				},
				Examples: []string{
					"dws todo task add-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>",
					"dws todo task add-participant --task-id <taskId> --participants userId1,userId2 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "participants", Property: "todoParticipantsAddRequest.participantIds"},
				{Name: "task-id", Property: "todoParticipantsAddRequest.taskId"},
			},
		},
	})
	todoTaskRemoveParticipantCmd := &cobra.Command{
		Use:   "remove-participant",
		Short: "移除待办参与人",
		Example: `  dws todo task remove-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "participants"); err != nil {
				return err
			}
			participantsStr := mustGetFlag(cmd, "participants")
			participantIds := parseExecutorIds(participantsStr)
			return callMCPTool("remove_task_participants", map[string]any{
				"todoParticipantsRemoveRequest": map[string]any{
					"taskId":         mustGetFlag(cmd, "task-id"),
					"participantIds": participantIds,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskRemoveParticipantCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "remove_task_participants",
				CanonicalPath:  "todo.remove_task_participants",
				CLIPath:        "todo task remove-participant",
				PrimaryCLIPath: "todo task remove-participant",
			},
			Description: "移除待办参与人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "remove_task_participants"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除待办参与人",
				UseWhen:      []string{"用户明确要求从待办移除指定参与人时"},
				AvoidWhen: []string{
					"要添加参与人时改用 dws todo task add-participant",
					"待办或参与人未确认时不要移除",
				},
				Examples: []string{
					"dws todo task remove-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>",
					"dws todo task remove-participant --task-id <taskId> --participants userId1 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "participants", Property: "todoParticipantsRemoveRequest.participantIds"},
				{Name: "task-id", Property: "todoParticipantsRemoveRequest.taskId"},
			},
		},
	})

	todoTaskAddReminderCmd := &cobra.Command{
		Use:   "add-reminder",
		Short: "添加待办提醒",
		Long: `为已有待办写入一条提醒规则。

当前上游没有提醒规则查询接口，task get/list 也不返回 reminderRules；
成功响应是写入回执，不代表 CLI 能再次读取并核验该规则。`,
		Example: `  dws todo task add-reminder --task-id <taskId> --base-time dueTime --due-date-offset -30
			dws todo task add-reminder --task-id <taskId> --base-time customTime --reminder-time-stamp 2026-03-10T18:00:00+08:00
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "base-time"); err != nil {
				return err
			}
			baseTime := strings.TrimSpace(mustGetFlag(cmd, "base-time"))
			dueDateOffset := any(nil)
			reminderTimeStamp := any(nil)
			switch baseTime {
			case "dueTime":
				value := strings.TrimSpace(mustGetFlag(cmd, "due-date-offset"))
				if value == "" {
					return fmt.Errorf("--due-date-offset is required when --base-time=dueTime")
				}
				dueDateOffset = value
			case "customTime":
				value, _ := cmd.Flags().GetString("reminder-time-stamp")
				value = strings.TrimSpace(value)
				if value == "" {
					return fmt.Errorf("--reminder-time-stamp is required when --base-time=customTime")
				}
				ms, err := parseISOTimeToMillis("reminder-time-stamp", value)
				if err != nil {
					return err
				}
				reminderTimeStamp = ms
			default:
				return fmt.Errorf("--base-time must be one of dueTime or customTime, got %q", baseTime)
			}
			return callMCPTool("add_todo_reminder", map[string]any{
				"todoReminderAddRequest": map[string]any{
					"taskId":            mustGetFlag(cmd, "task-id"),
					"baseTime":          baseTime,
					"dueDateOffset":     dueDateOffset,
					"reminderTimeStamp": reminderTimeStamp,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskAddReminderCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "add_todo_reminder",
				CanonicalPath:  "todo.add_todo_reminder",
				CLIPath:        "todo task add-reminder",
				PrimaryCLIPath: "todo task add-reminder",
			},
			Description: "写入一条待办提醒规则（上游不支持规则读回）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "add_todo_reminder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "写入一条待办提醒规则（上游不支持规则读回）",
				UseWhen:      []string{"已知 taskId，需要按截止时间偏移或自定义时间戳添加提醒时（dueTime 模式要求待办已有截止时间）；成功响应仅作为写入回执"},
				AvoidWhen: []string{
					"要重置/清除提醒规则时改用 dws todo task reset-reminder",
					"提醒时间与目标待办未确认时不要添加",
					"业务要求写后读取并核验 reminderRules 时不要声称可验证；当前 task get/list 均不返回提醒规则",
				},
				Examples: []string{
					"dws todo task add-reminder --task-id <taskId> --base-time dueTime --due-date-offset -30",
					"dws todo task add-reminder --task-id <taskId> --base-time customTime --reminder-time-stamp \"2026-03-10T18:00:00+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "base-time", Property: "todoReminderAddRequest.baseTime"},
				{Name: "due-date-offset", Property: "todoReminderAddRequest.dueDateOffset"},
				{Name: "reminder-time-stamp", Property: "todoReminderAddRequest.reminderTimeStamp"},
				{Name: "task-id", Property: "todoReminderAddRequest.taskId"},
			},
		},
	})

	todoTaskUpdateReminderCmd := &cobra.Command{
		Use:   "reset-reminder",
		Short: "重置待办提醒",
		Long: `整体替换待办提醒规则；不传 --reminder-rules 时清除提醒。显式传值必须是合法 JSON 数组，
每条规则必须按 baseTime 提供 dueDateOffset 或 reminderTimeStamp；非法输入会在远端调用前失败。

当前上游没有提醒规则查询接口，task get/list 也不返回 reminderRules；
成功响应是写入回执，不代表 CLI 能再次读取并核验最终规则。`,
		Example: `  dws todo task reset-reminder --task-id <taskId>
			dws todo task reset-reminder --task-id <taskId> --reminder-rules <reminderRules>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			var reminderRules []map[string]any
			if cmd.Flags().Changed("reminder-rules") {
				value, _ := cmd.Flags().GetString("reminder-rules")
				parsedRules, err := parseTodoReminderRules(value)
				if err != nil {
					return err
				}
				reminderRules = parsedRules
			}
			return callMCPTool("reset_todo_reminder", map[string]any{
				"todoReminderUpdateRequest": map[string]any{
					"taskId":        mustGetFlag(cmd, "task-id"),
					"reminderRules": reminderRules,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskUpdateReminderCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "reset_todo_reminder",
				CanonicalPath:  "todo.reset_todo_reminder",
				CLIPath:        "todo task reset-reminder",
				PrimaryCLIPath: "todo task reset-reminder",
			},
			Description: "整体替换或清除待办提醒规则（上游不支持规则读回）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "reset_todo_reminder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "整体替换或清除待办提醒规则（上游不支持规则读回）",
				UseWhen:      []string{"需要清除或整体替换待办提醒规则时（不传 reminder-rules 或传 [] 可清除）；非空规则会在远端调用前严格校验，成功响应仅作为写入回执"},
				AvoidWhen: []string{
					"只需追加一条提醒时改用 dws todo task add-reminder",
					"新规则与目标待办未确认时不要重置",
					"业务要求写后读取并核验 reminderRules 时不要声称可验证；当前 task get/list 均不返回提醒规则",
				},
				Examples: []string{
					"dws todo task reset-reminder --task-id <taskId>",
					"dws todo task reset-reminder --task-id <taskId> --reminder-rules '[{\"dueDateOffset\":-30,\"baseTime\":\"dueTime\"},{\"reminderTimeStamp\":\"2026-03-10T18:00:00+08:00\",\"baseTime\":\"customTime\"}]'",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "reminder-rules", Property: "todoReminderUpdateRequest.reminderRules", Description: "提醒规则 JSON 数组；不传表示清除，显式传值必须为对象数组，且每条按 baseTime 提供整数 dueDateOffset 或 ISO8601 reminderTimeStamp"},
				{Name: "task-id", Property: "todoReminderUpdateRequest.taskId"},
			},
		},
	})

	todoTaskAddAttachment := &cobra.Command{
		Use:   "add-attachment",
		Short: "上传待办附件",
		Long: `上传待办附件
⚠️ 重要：该接口会上传文件到附件，不可用于测试或试探性调用。调用前必须确认待办存在。`,
		Example: `  dws todo task add-attachment --task-id <taskId> --file <filePath>
  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := flagOrFallback(cmd, "file-path", "file")
			taskID := mustGetFlag(cmd, "task-id")
			if taskID == "" && filePath == "" {
				return validateRequiredFlags(cmd, "task-id", "file")
			}
			if taskID == "" {
				return validateRequiredFlags(cmd, "task-id")
			}
			if filePath == "" {
				return validateRequiredFlags(cmd, "file")
			}
			meta, err := buildTodoLocalFileMeta(filePath, "", "")
			if err != nil {
				return err
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "上传本地文件并添加为待办附件")
				deps.Out.PrintKeyValue("文件", meta.LocalPath)
				deps.Out.PrintKeyValue("名称", meta.FileName)
				deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", meta.FileSize))
				return nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
			defer cancel()
			commitText, err := uploadTodoLocalFile(ctx, meta)
			if err != nil {
				return err
			}

			dentryId, spaceId, err := parseTodoFileSendIDs(commitText)
			if err != nil {
				return err
			}
			var attachments []any
			attachments = append(attachments, map[string]any{
				"fileId":   strconv.FormatInt(dentryId, 10),
				"fileName": meta.FileName,
				"fileSize": meta.FileSize,
				"spaceId":  strconv.FormatInt(spaceId, 10),
				"fileType": meta.FileType,
			})
			return callMCPTool("add_todo_attachment", map[string]any{
				"todoAttachmentAddRequest": map[string]any{
					"taskId":         mustGetFlag(cmd, "task-id"),
					"attachmentList": attachments,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskAddAttachment, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "add_todo_attachment",
				CanonicalPath:  "todo.add_todo_attachment",
				CLIPath:        "todo task add-attachment",
				PrimaryCLIPath: "todo task add-attachment",
			},
			Description: "上传待办附件",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "add_todo_attachment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "上传待办附件",
				UseWhen:      []string{"已知 taskId 且待办已确认存在，需要上传本地文件为待办附件时"},
				AvoidWhen: []string{
					"待办尚未确认存在或仅试探调用时不要上传（会真实写附件）",
					"要添加执行人/参与人/提醒时改用对应 add 命令",
				},
				Examples: []string{
					"dws todo task add-attachment --task-id <taskId> --file /path/to/file.pdf",
					"dws todo task add-attachment --task-id <taskId> --file /path/to/file.pdf --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "todoAttachmentAddRequest.taskId"},
			},
		},
	})

	todoTaskListAttachmentCmd := &cobra.Command{
		Use:   "list-attachment",
		Short: "查询待办任务的附件列表",
		Example: `  dws todo task list-attachment --task-id <taskId>
  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskID := mustGetFlag(cmd, "task-id")
			if !deps.Caller.DryRun() {
				if err := ensureTodoTaskExists(cmd.Context(), taskID); err != nil {
					return err
				}
			}
			return callMCPTool("list_todo_attachment", map[string]any{
				"todoAttachmentListRequest": map[string]any{
					"taskId": taskID,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTaskListAttachmentCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "list_todo_attachment",
				CanonicalPath:  "todo.list_todo_attachment",
				CLIPath:        "todo task list-attachment",
				PrimaryCLIPath: "todo task list-attachment",
			},
			Description: "查询待办附件列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询待办附件列表",
				UseWhen:      []string{"已知 taskId，需要查看该待办当前附件及其 attachmentId、文件名和大小时"},
				AvoidWhen: []string{
					"要上传附件时改用 dws todo task add-attachment",
					"要删除附件时先用本命令确认 attachmentId，再改用 dws todo task remove-attachment",
				},
				Examples: []string{
					"dws todo task list-attachment --task-id <taskId>",
					"dws todo task list-attachment --task-id <taskId> --format json",
				},
			},
		},
	})

	todoTaskRemoveAttachmentCmd := &cobra.Command{
		Use:   "remove-attachment",
		Short: "删除待办任务的附件",
		Example: `  dws todo task remove-attachment --task-id <taskId> --attachment-id <attachmentId>
  # 查询 taskId: dws todo task list
  # 查询 attachmentId: dws todo task list-attachment --task-id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskId := mustGetFlag(cmd, "task-id")
			attachmentId := mustGetFlag(cmd, "attachment-id")
			if !commandDryRun(cmd) && !confirmDelete("附件", attachmentId) {
				return nil
			}
			return callMCPTool("remove_todo_attachment", map[string]any{
				"todoAttachmentRemoveRequest": map[string]any{
					"taskId":       taskId,
					"attachmentId": attachmentId,
				},
			})
		},
	}

	todoTaskCreateCmd.Flags().String("title", "", "待办标题 (必填)")
	todoTaskCreateCmd.Flags().String("executors", "", "执行者 userId 列表 (必填)")
	todoTaskCreateCmd.Flags().String("due", "", "截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)")
	todoTaskCreateCmd.Flags().String("priority", "", "优先级: 10低/20普通/30较高/40紧急")
	todoTaskCreateCmd.Flags().String("recurrence", "", "循环待办 (需先设置 --due); 格式: DTSTART:20260320T100000Z\\nRRULE:FREQ=DAILY;INTERVAL=1")

	todoTaskCreateSubCmd.Flags().String("parent-id", "", "父待办任务 ID (必填)")
	todoTaskCreateSubCmd.Flags().String("title", "", "子待办标题 (必填)")
	todoTaskCreateSubCmd.Flags().String("executors", "", "执行者 userId 列表 (必填)")
	todoTaskCreateSubCmd.Flags().String("due", "", "截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)")
	todoTaskCreateSubCmd.Flags().String("priority", "", "优先级: 10低/20普通/30较高/40紧急")
	todoTaskCreateSubCmd.Flags().String("recurrence", "", "循环待办 (需先设置 --due); 格式: DTSTART:20260320T100000Z\\nRRULE:FREQ=DAILY;INTERVAL=1")

	todoTaskListCmd.Flags().String("page", "1", "页码（默认 1）")
	todoTaskListCmd.Flags().String("size", "20", "获取数量，超过 20 自动分页 (默认 20)")
	todoTaskListCmd.Flags().String("status", "", "true=已完成, false=未完成")
	todoTaskListCmd.Flags().String("priority", "", "优先级: 10 低/20 普通/30 较高/40 紧急")
	todoTaskListCmd.Flags().String("role-types", "", "角色类型: creator/executor/participant")
	todoTaskListCmd.Flags().String("plan-finish-date-start", "", "截止时间范围查询开始 ISO-8601 (如 2026-03-10T18:00:00+08:00)")
	todoTaskListCmd.Flags().String("plan-finish-date-end", "", "截止时间范围查询结束 ISO-8601 (如 2026-03-10T18:00:00+08:00)")
	todoTaskListCmd.Flags().Bool("query-all", false, "查询所有待办，而不是仅查询当前组织待办")

	todoTaskUpdateCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskUpdateCmd.Flags().String("title", "", "新标题")
	todoTaskUpdateCmd.Flags().String("due", "", "截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)")
	todoTaskUpdateCmd.Flags().String("priority", "", "优先级: 10低/20普通/30较高/40紧急")
	todoTaskUpdateCmd.Flags().String("done", "", "完成状态: true/false")
	todoTaskDoneCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskDoneCmd.Flags().String("status", "", "完成状态: true=已完成, false=未完成 (必填)")

	todoTaskGetCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskDeleteCmd.Flags().String("task-id", "", "待办任务 ID (必填)")

	todoTaskCreateCmd.Flags().String("subject", "", "--title 的别名")
	todoTaskCreateCmd.Flags().String("content", "", "--title 的别名")
	_ = todoTaskCreateCmd.Flags().MarkHidden("subject")
	_ = todoTaskCreateCmd.Flags().MarkHidden("content")

	todoTaskCreateSubCmd.Flags().String("subject", "", "--title 的别名")
	todoTaskCreateSubCmd.Flags().String("content", "", "--title 的别名")
	_ = todoTaskCreateSubCmd.Flags().MarkHidden("subject")
	_ = todoTaskCreateSubCmd.Flags().MarkHidden("content")

	todoTaskCmd.PersistentFlags().String("remind-at", "", "内部兼容标志")
	_ = todoTaskCmd.PersistentFlags().MarkHidden("remind-at")

	todoTaskAddExecutorCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddExecutorCmd.Flags().String("executors", "", "执行者 userId 列表 (必填)")
	todoTaskRemoveExecutorCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskRemoveExecutorCmd.Flags().String("executors", "", "执行者 userId 列表 (必填)")
	todoTaskAddParticipantCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddParticipantCmd.Flags().String("participants", "", "参与人 userId 列表 (必填)")
	todoTaskRemoveParticipantCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskRemoveParticipantCmd.Flags().String("participants", "", "参与人 userId 列表 (必填)")

	todoTaskAddReminderCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddReminderCmd.Flags().String("base-time", "", "提醒基准时间: dueTime/customTime (必填)")
	todoTaskAddReminderCmd.Flags().String("due-date-offset", "", "截止时间偏移量，为整数 (baseTime=dueTime 时必填)")
	todoTaskAddReminderCmd.Flags().String("reminder-time-stamp", "", "自定义提醒时间 ISO-8601 (如 2026-03-10T18:00:00+08:00，baseTime=customTime 时必填)")

	todoTaskUpdateReminderCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskUpdateReminderCmd.Flags().String("reminder-rules", "", "提醒规则 JSON 数组 (不传则清除；显式传值必须合法)")
	todoTaskListSubCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskListAttachmentCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddAttachment.Flags().String("task-id", "", "待办任务 ID (必填)")
	corecmd.RegisterFlags(todoTaskAddAttachment, []corecmd.FlagSpec{{
		Name:    "file",
		Usage:   "本地文件路径（必填）",
		Aliases: []string{"file-path"},
	}})
	todoTaskRemoveAttachmentCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskRemoveAttachmentCmd.Flags().String("attachment-id", "", "待办附件 ID（必填）")

	todoTaskCmd.AddCommand(todoTaskCreateCmd,
		todoTaskCreateSubCmd,
		todoTaskListCmd,
		todoTaskUpdateCmd,
		todoTaskDoneCmd,
		todoTaskGetCmd,
		todoTaskDeleteCmd,
		todoTaskAddExecutorCmd,
		todoTaskRemoveExecutorCmd,
		todoTaskAddParticipantCmd,
		todoTaskRemoveParticipantCmd,
		todoTaskAddReminderCmd,
		todoTaskUpdateReminderCmd,
		todoTaskListSubCmd,
		todoTaskListAttachmentCmd,
		todoTaskAddAttachment,
		todoTaskRemoveAttachmentCmd,
	)
	todoCmd.AddCommand(todoTaskCmd)

	// ──────────────────────────────────────────────────────────
	// dws todo comment — 待办评论
	// 对应 MCP：add_todo_comment / list_todo_comment / delete_todo_comment
	// ──────────────────────────────────────────────────────────
	todoCommentCmd := &cobra.Command{Use: "comment", Short: "待办评论：新增 / 列表 / 删除", RunE: groupRunE}

	todoCommentAddCmd := &cobra.Command{
		Use:   "add",
		Short: "新增待办评论",
		Example: `  dws todo comment add --task-id <taskId> --content "评论内容"

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "content"); err != nil {
				return err
			}
			return callMCPTool("add_todo_comment", map[string]any{
				"taskId":  mustGetFlag(cmd, "task-id"),
				"content": mustGetFlag(cmd, "content"),
			})
		},
	}
	DeclareLeafMetadata(todoCommentAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "add_todo_comment",
				CanonicalPath:  "todo.add_todo_comment",
				CLIPath:        "todo comment add",
				PrimaryCLIPath: "todo comment add",
			},
			Description: "给待办添加评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "add_todo_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "给待办添加评论",
				UseWhen:      []string{"已知 taskId，需要给待办新增一条评论文本时"},
				AvoidWhen: []string{
					"要删除评论时改用 dws todo comment delete",
					"要列出评论时改用 dws todo comment list",
				},
				Examples: []string{
					"dws todo comment add --task-id <taskId> --content \"评论内容\"",
					"dws todo comment add --task-id <taskId> --content \"已开始处理\" --format json",
				},
			},
		},
	})

	todoCommentListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询待办评论列表",
		Example: `  dws todo comment list --task-id <taskId>
  dws todo comment list --task-id <taskId> --page 1 --size 20

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			return callMCPTool("list_todo_comment", map[string]any{
				"taskId":   mustGetFlag(cmd, "task-id"),
				"page":     mustGetFlag(cmd, "page"),
				"pageSize": mustGetFlag(cmd, "size"),
			})
		},
	}
	DeclareLeafMetadata(todoCommentListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "list_todo_comment",
				CanonicalPath:  "todo.list_todo_comment",
				CLIPath:        "todo comment list",
				PrimaryCLIPath: "todo comment list",
			},
			Description: "获取待办评论列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "list_todo_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取待办评论列表",
				UseWhen:      []string{"已知 taskId，需要分页查看该待办评论时"},
				AvoidWhen: []string{
					"要新增评论时改用 dws todo comment add",
					"要删除评论时改用 dws todo comment delete",
				},
				Examples: []string{
					"dws todo comment list --task-id <taskId>",
					"dws todo comment list --task-id <taskId> --page 1 --size 20",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "size", Property: "pageSize"},
			},
		},
	})

	todoCommentDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除待办评论",
		Example: `  dws todo comment delete --task-id <taskId> --comment-id <commentId>
  dws todo comment delete --task-id <taskId> --comment-id <commentId> --yes

  # 查询 commentId: dws todo comment list --task-id <taskId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "comment-id"); err != nil {
				return err
			}
			commentId := mustGetFlag(cmd, "comment-id")
			return callMCPTool("delete_todo_comment", map[string]any{
				"taskId":    mustGetFlag(cmd, "task-id"),
				"commentId": commentId,
			})
		},
	}
	DeclareLeafMetadata(todoCommentDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "delete_todo_comment",
				CanonicalPath:  "todo.delete_todo_comment",
				CLIPath:        "todo comment delete",
				PrimaryCLIPath: "todo comment delete",
			},
			Description: "删除待办评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "todo", RPCName: "delete_todo_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除待办评论",
				UseWhen:      []string{"用户明确要求删除指定待办下的某条评论时"},
				AvoidWhen: []string{
					"用户未明确删除，或 taskId/commentId 不确定时不要执行",
					"要新增评论时改用 dws todo comment add；要列出评论时改用 dws todo comment list",
				},
				Examples: []string{"dws todo comment delete --task-id <taskId> --comment-id <commentId>"},
			},
		},
	})

	todoCommentAddCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentAddCmd.Flags().String("content", "", "评论内容 (必填)")
	todoCommentListCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentListCmd.Flags().String("page", "1", "页码 (默认 1)")
	todoCommentListCmd.Flags().String("size", "20", "每页数量 (默认 20)")
	todoCommentDeleteCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentDeleteCmd.Flags().String("comment-id", "", "评论 ID (必填)")

	todoCommentCmd.AddCommand(todoCommentAddCmd, todoCommentListCmd, todoCommentDeleteCmd)
	todoCmd.AddCommand(todoCommentCmd)

	// ──────────────────────────────────────────────────────────
	// dws todo tag — 待办标签
	// 对应 MCP：tag_todo / delete_todo_tag / update_todo_tag / list_todo_tags / create_todo_tag
	// ──────────────────────────────────────────────────────────
	todoTagCmd := &cobra.Command{Use: "tag", Short: "待办标签：打标 / 列表 / 创建 / 更新 / 删除", RunE: groupRunE}

	todoTagAddCmd := &cobra.Command{
		Use:   "add",
		Short: "给待办打标",
		Example: `  dws todo tag add --task-id <taskId> --tag-codes code1,code2

  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "tag-codes"); err != nil {
				return err
			}
			tagCodes := parseStringList(mustGetFlag(cmd, "tag-codes"))
			if len(tagCodes) == 0 {
				return apperrors.NewValidation("--tag-codes must contain at least one non-empty code")
			}
			return callMCPTool("tag_todo", map[string]any{
				"TodoTagRequest": map[string]any{
					"taskId":   mustGetFlag(cmd, "task-id"),
					"tagCodes": tagCodes,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTagAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "tag_todo",
				CanonicalPath:  "todo.tag_todo",
				CLIPath:        "todo tag add",
				PrimaryCLIPath: "todo tag add",
			},
			Description: "把一个或多个现有标签添加到指定待办",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI builds TodoTagRequest and calls todo/tag_todo, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把一个或多个现有标签添加到指定待办",
				UseWhen:      []string{"已有 taskId 和标签 code，需要给该待办打标"},
				AvoidWhen:    []string{"尚无标签 code 时先用 todo tag list 或 todo tag create；修改标签定义应使用 todo tag update"},
				Examples:     []string{"dws todo tag add --task-id <taskId> --tag-codes code1,code2"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "tag-codes", Property: "tagCodes", Required: boolPtr(true), InterfaceType: "array"},
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
			},
		},
	})

	todoTagDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除待办标签",
		Long:  `删除当前用户的待办标签。该操作不可逆；正式执行必须先获得用户确认并追加 --yes，可先使用 --dry-run 预览。`,
		Example: `  dws todo tag delete --tag-codes code1,code2 --yes
  # 查询 tag code: dws todo tag list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "tag-codes"); err != nil {
				return err
			}
			tagCodes := parseStringList(mustGetFlag(cmd, "tag-codes"))
			if len(tagCodes) == 0 {
				return apperrors.NewValidation("--tag-codes must contain at least one non-empty code")
			}
			return callMCPTool("delete_todo_tag", map[string]any{
				"UserTagDeleteRequest": map[string]any{
					"tagCodes": tagCodes,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTagDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "delete_todo_tag",
				CanonicalPath:  "todo.delete_todo_tag",
				CLIPath:        "todo tag delete",
				PrimaryCLIPath: "todo tag delete",
			},
			Description: "不可逆地删除当前用户的一个或多个待办标签",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI parses tag codes into UserTagDeleteRequest and calls todo/delete_todo_tag, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "不可逆地删除当前用户的一个或多个待办标签",
				UseWhen:      []string{"用户明确要求删除已有标签 code，且已确认标签编码及不可逆影响"},
				AvoidWhen:    []string{"只需从某个待办移除标签或重命名标签时不要删除标签定义；未确认 code 时先用 todo tag list"},
				Examples:     []string{"dws todo tag delete --tag-codes code1,code2"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "tag-codes", Property: "tagCodes", Required: boolPtr(true), InterfaceType: "array"},
			},
		},
	})

	todoTagUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新待办标签",
		Example: `  dws todo tag update --user-tags '[{"code":"code1","name":"新名称"}]'
  # 查询 code: dws todo tag list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "user-tags"); err != nil {
				return err
			}
			var userTags []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "user-tags")), &userTags); err != nil {
				return &CLIError{
					Code:       CodeMissingParam,
					Message:    fmt.Sprintf("--user-tags 必须是合法的 JSON 数组: %v", err),
					Suggestion: `示例: --user-tags '[{"code":"code1","name":"新名称"}]'`,
					Operation:  "todo.tag.update.user-tags",
				}
			}
			if userTags == nil {
				return apperrors.NewValidation("--user-tags must be a JSON array")
			}
			return callMCPTool("update_todo_tag", map[string]any{
				"UserTagAddRequest": map[string]any{
					"userTags": userTags,
				},
			})
		},
	}
	DeclareLeafMetadata(todoTagUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "update_todo_tag",
				CanonicalPath:  "todo.update_todo_tag",
				CLIPath:        "todo tag update",
				PrimaryCLIPath: "todo tag update",
			},
			Description: "批量更新已有待办标签的名称等定义",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI decodes user tag JSON into UserTagAddRequest and calls todo/update_todo_tag, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量更新已有待办标签的名称等定义",
				UseWhen:      []string{"已有标签 code，需要按 JSON 数组更新一个或多个标签定义"},
				AvoidWhen:    []string{"给待办打标签应使用 todo tag add；创建没有 code 的新标签应使用 todo tag create"},
				Examples:     []string{"dws todo tag update --user-tags '[{\"code\":\"code1\",\"name\":\"新名称\"}]'"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "user-tags", Property: "userTags", Required: boolPtr(true), InterfaceType: "array"},
			},
		},
	})

	todoTagListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查询待办标签列表",
		Example: `  dws todo tag list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_todo_tags", map[string]any{})
		},
	}
	DeclareLeafMetadata(todoTagListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "list_todo_tags",
				CanonicalPath:  "todo.list_todo_tags",
				CLIPath:        "todo tag list",
				PrimaryCLIPath: "todo tag list",
			},
			Description: "列出当前用户可用的待办标签及其 code",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI calls todo/list_todo_tags, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出当前用户可用的待办标签及其 code",
				UseWhen:      []string{"需要查看待办标签目录或先取得标签 code 供打标、更新、删除使用"},
				AvoidWhen:    []string{"查询待办任务列表应使用 todo task list；该命令只返回标签定义"},
				Examples:     []string{"dws todo tag list"},
			},
		},
	})

	todoTagCreateCmd := &cobra.Command{
		Use:     "create",
		Short:   "创建待办标签",
		Example: `  dws todo tag create --name "标签名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			name := strings.TrimSpace(mustGetFlag(cmd, "name"))
			if name == "" {
				return apperrors.NewValidation("--name must not be blank")
			}
			return callMCPTool("create_todo_tag", map[string]any{
				"UserTagAddRequest": map[string]any{
					"userTags": []map[string]any{{"name": name}},
				},
			})
		},
	}
	DeclareLeafMetadata(todoTagCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "todo",
				Name:           "create_todo_tag",
				CanonicalPath:  "todo.create_todo_tag",
				CLIPath:        "todo tag create",
				PrimaryCLIPath: "todo tag create",
			},
			Description: "为当前用户创建一个新的待办标签",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI wraps one trimmed name in UserTagAddRequest and calls todo/create_todo_tag, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为当前用户创建一个新的待办标签",
				UseWhen:      []string{"用户明确要求创建可复用的待办标签，且已给出非空标签名称"},
				AvoidWhen:    []string{"给已有待办打上现有标签应使用 todo tag add；重命名已有标签应使用 todo tag update"},
				Examples:     []string{"dws todo tag create --name \"项目标签\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "name", Required: boolPtr(true)},
			},
		},
	})

	todoTagAddCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTagAddCmd.Flags().String("tag-codes", "", "标签编码列表，逗号分隔 (必填)")
	todoTagDeleteCmd.Flags().String("tag-codes", "", "要删除的标签编码列表，逗号分隔 (必填)")
	todoTagUpdateCmd.Flags().String("user-tags", "", "标签列表 JSON 数组 (必填)")
	todoTagCreateCmd.Flags().String("name", "", "标签名称 (必填)")

	todoTagCmd.AddCommand(todoTagAddCmd, todoTagDeleteCmd, todoTagUpdateCmd, todoTagListCmd, todoTagCreateCmd)
	todoCmd.AddCommand(todoTagCmd)

	todoCmd.AddCommand(
		hintSubCmd("create", "use: dws todo task create"),
		hintSubCmd("list", "use: dws todo task list"),
		hintSubCmd("get", "use: dws todo task get"),
		hintSubCmd("delete", "use: dws todo task delete"),
	)

	return todoCmd
}

func rejectUnsupportedTodoReminderFlags(cmd *cobra.Command) error {
	var unsupported []string
	if v, _ := cmd.Flags().GetString("remind-at"); v != "" {
		unsupported = append(unsupported, "--remind-at")
	}
	if len(unsupported) == 0 {
		return nil
	}
	return &CLIError{
		Code:       CodeInvalidParam,
		Message:    fmt.Sprintf("todo 当前不支持独立 reminder 参数: %s", strings.Join(unsupported, ", ")),
		Suggestion: "需要截止时间请改用 --due；需要独立提醒请用 dws todo task add-reminder。dueTime 模式要求待办已有截止时间，customTime 模式直接传 --reminder-time-stamp，不必先设置 --due。当前上游不支持读取提醒规则，不能用 task get/list 验证写入结果。",
		Operation:  "todo.task.reminder",
	}
}

func parseTodoReminderRules(raw string) ([]map[string]any, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, invalidTodoReminderRules(CodeInvalidParam, "显式传入的 --reminder-rules 不能为空；清除提醒请省略该参数或传 []", nil)
	}
	if !json.Valid([]byte(value)) {
		return nil, invalidTodoReminderRules(CodeInvalidJSON, "--reminder-rules 必须是合法 JSON 数组", nil)
	}

	var rules []map[string]any
	if err := unmarshalJSONUseNumber(value, &rules); err != nil {
		return nil, invalidTodoReminderRules(CodeInvalidParam, "--reminder-rules 必须是由对象组成的 JSON 数组", err)
	}
	if rules == nil {
		return nil, invalidTodoReminderRules(CodeInvalidParam, "--reminder-rules 不能是 null；清除提醒请省略该参数或传 []", nil)
	}

	for i, rule := range rules {
		position := i + 1
		if rule == nil {
			return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条必须是对象", position), nil)
		}
		baseTime, ok := rule["baseTime"].(string)
		baseTime = strings.TrimSpace(baseTime)
		if !ok || baseTime == "" {
			return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条缺少字符串 baseTime", position), nil)
		}
		rule["baseTime"] = baseTime

		switch baseTime {
		case "dueTime":
			offset, ok := rule["dueDateOffset"].(json.Number)
			if !ok {
				return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条在 baseTime=dueTime 时必须提供整数 dueDateOffset", position), nil)
			}
			parsed, err := strconv.ParseInt(offset.String(), 10, 64)
			if err != nil {
				return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条的 dueDateOffset 必须是整数", position), err)
			}
			rule["dueDateOffset"] = parsed
		case "customTime":
			timestamp, ok := rule["reminderTimeStamp"].(string)
			timestamp = strings.TrimSpace(timestamp)
			if !ok || timestamp == "" {
				return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条在 baseTime=customTime 时必须提供 ISO8601 字符串 reminderTimeStamp", position), nil)
			}
			parsed, err := parseISOTimeToMillis("reminderTimeStamp", timestamp)
			if err != nil {
				return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条的 reminderTimeStamp 无效", position), err)
			}
			rule["reminderTimeStamp"] = parsed
		default:
			return nil, invalidTodoReminderRules(CodeInvalidParam, fmt.Sprintf("--reminder-rules 第 %d 条的 baseTime 必须是 dueTime 或 customTime", position), nil)
		}
	}
	return rules, nil
}

func invalidTodoReminderRules(code, message string, cause error) error {
	return &CLIError{
		Code:       code,
		Message:    message,
		Suggestion: "使用 dueTime + 整数 dueDateOffset，或 customTime + ISO8601 reminderTimeStamp；清除提醒请省略 --reminder-rules 或传 []。",
		Operation:  "todo.task.reset-reminder.reminder-rules",
		Cause:      cause,
	}
}

// todoListAutoPage 当 size > 20 时自动分页请求并合并结果。pageStr 为起始页码，wantSize 为期望条数。
func todoListAutoPage(cmd *cobra.Command, pageStr string, wantSize int) error {
	ctx := context.Background()
	toolName := "get_user_todos_in_current_org"
	if queryAll, _ := cmd.Flags().GetBool("query-all"); queryAll {
		toolName = "get_user_todos"
	}
	if deps.Caller.DryRun() {
		numPages := (wantSize + todoListPageSizeMax - 1) / todoListPageSizeMax
		bold := color.New(color.FgYellow, color.Bold)
		bold.Println("[DRY-RUN] 自动分页待办列表:")
		deps.Out.PrintKeyValue("Tool", toolName)
		deps.Out.PrintKeyValue("预计请求次数", fmt.Sprintf("%d (每页最多 %d 条)", numPages, todoListPageSizeMax))
		return nil
	}
	startPage, _ := strconv.Atoi(pageStr)
	if startPage < 1 {
		startPage = 1
	}
	var merged []any
	for pageNum := startPage; len(merged) < wantSize; pageNum++ {
		toolArgs := map[string]any{
			"pageNum":  fmt.Sprintf("%d", pageNum),
			"pageSize": fmt.Sprintf("%d", todoListPageSizeMax),
		}
		err := buildListTodoTaskArgs(cmd, toolArgs)
		if err != nil {
			return err
		}
		text, err := callMCPToolReturnText(ctx, toolName, toolArgs)
		if err != nil {
			return err
		}
		var resp struct {
			Result struct {
				TodoCards []any `json:"todoCards"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(text), &resp); err != nil {
			return fmt.Errorf("failed to parse todo response: %w", err)
		}
		cards := resp.Result.TodoCards
		if len(cards) == 0 {
			break
		}
		for _, c := range cards {
			merged = append(merged, c)
			if len(merged) >= wantSize {
				break
			}
		}
		if len(cards) < todoListPageSizeMax {
			break
		}
	}
	out := map[string]any{"result": map[string]any{"todoCards": merged}}
	flagFormat := deps.Caller.Format()
	if flagFormat == "json" {
		return deps.Out.PrintJSON(out)
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	deps.Out.PrintRaw(string(raw))
	return nil
}

// parseExecutorIds 将 "id1,id2" 转为 []string，供 MCP 数组类型 executorIds 使用
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

type todoLocalFileMeta struct {
	LocalPath   string
	FileName    string
	FileType    string
	ContentPath string
	FileSize    int64
	MD5         string
}

func parseTodoFileSendIDs(text string) (int64, int64, error) {
	var data any
	if err := unmarshalJSONUseNumber(text, &data); err != nil {
		return 0, 0, fmt.Errorf("failed to parse uploaded file response JSON: %w", err)
	}
	dentryID, _ := findInt64Field(data, "dentryId", "dentryID")
	spaceID, _ := findInt64Field(data, "spaceId", "spaceID")
	if dentryID == 0 || spaceID == 0 {
		return 0, 0, fmt.Errorf("uploaded file response missing dentryId or spaceId")
	}
	return dentryID, spaceID, nil
}

func buildTodoLocalFileMeta(filePath, fileName, md5Value string) (todoLocalFileMeta, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return todoLocalFileMeta{}, fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fi.IsDir() {
		return todoLocalFileMeta{}, fmt.Errorf("%s is a directory, not a file", filePath)
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	fileType := strings.TrimPrefix(filepath.Ext(fileName), ".")
	if md5Value == "" {
		md5Value, err = todoFileMD5Hex(filePath)
		if err != nil {
			return todoLocalFileMeta{}, err
		}
	}
	return todoLocalFileMeta{
		LocalPath:   filePath,
		FileName:    fileName,
		FileType:    fileType,
		ContentPath: "/" + fileName,
		FileSize:    fi.Size(),
		MD5:         md5Value,
	}, nil
}

func parseTodoFileUploadInfo(text string) (resourceURL, uploadKey string, headers map[string]string, err error) {
	var data map[string]any
	if err = unmarshalJSONUseNumber(text, &data); err != nil {
		return "", "", nil, fmt.Errorf("failed to parse upload credentials JSON: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	resourceURL = firstStringField(data, "resourceUrl", "resourceURL", "url")
	if resourceURL == "" {
		if values, ok := data["resourceUrls"].([]any); ok && len(values) > 0 {
			resourceURL = stringFromJSONScalar(values[0])
		}
	}
	uploadKey = firstStringField(data, "uploadKey", "key")
	if resourceURL == "" || uploadKey == "" {
		return "", "", nil, fmt.Errorf("incomplete upload credentials: resourceUrl=%q, uploadKey=%q", resourceURL, uploadKey)
	}

	headers = map[string]string{}
	for _, key := range []string{"headers", "ossHeaders"} {
		if h, ok := data[key].(map[string]any); ok {
			for name, value := range h {
				if s := stringFromJSONScalar(value); s != "" {
					headers[name] = s
				}
			}
		}
	}
	return resourceURL, uploadKey, headers, nil
}

func uploadTodoLocalFile(ctx context.Context, meta todoLocalFileMeta) (string, error) {
	initArgs := map[string]any{}
	initArgs["fileName"] = meta.FileName
	initArgs["fileSize"] = meta.FileSize
	initArgs["md5"] = meta.MD5
	initText, err := callMCPToolReturnTextOnServer(ctx, "todo", "init_todo_file_upload",
		map[string]any{"todoAttachmentInitUploadInfoRequest": initArgs})
	if err != nil {
		return "", err
	}
	resourceURL, uploadKey, headers, err := parseTodoFileUploadInfo(initText)
	if err != nil {
		return "", err
	}

	if err := httpPutFile(ctx, resourceURL, headers, meta.LocalPath, meta.FileSize); err != nil {
		return "", err
	}

	commitArgs := map[string]any{}
	commitArgs["uploadKey"] = uploadKey
	commitArgs["fileName"] = meta.FileName
	commitArgs["fileSize"] = meta.FileSize
	commitArgs["md5"] = meta.MD5
	return callMCPToolReturnTextOnServer(ctx, "todo", "commit_todo_file_upload",
		map[string]any{"todoAttachmentCommitUploadInfoRequest": commitArgs})
}

// parseIntList splits a comma-separated string into a slice of ints.
// Empty segments and non-numeric values are skipped. Returns nil if no valid ints found.
func parseIntList(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			result = append(result, n)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// parseStringList splits a comma-separated string into a slice of strings.
// Empty segments are skipped. Returns nil if no valid strings found.
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		result = append(result, p)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// parseRoleTypes parses a comma-separated role-types string and validates each value.
// Only "creator", "executor", "participant" are allowed. Returns error on invalid values.
func parseRoleTypes(s string) ([]string, error) {
	allowed := map[string]bool{"creator": true, "executor": true, "participant": true}
	parts := parseStringList(s)
	if parts == nil {
		return nil, nil
	}
	for _, p := range parts {
		if !allowed[p] {
			return nil, fmt.Errorf("invalid role-type %q, allowed values: creator, executor, participant", p)
		}
	}
	return parts, nil
}

func buildListTodoTaskArgs(cmd *cobra.Command, toolArgs map[string]any) error {
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		toolArgs["todoStatus"] = v
	}
	if v, _ := cmd.Flags().GetString("priority"); v != "" {
		if priorities := parseIntList(v); priorities != nil {
			toolArgs["priorityList"] = priorities
		}
	}
	toolArgs["roleTypes"] = []string{"executor"}
	if v, _ := cmd.Flags().GetString("role-types"); v != "" {
		roleTypes, err := parseRoleTypes(v)
		if err != nil {
			return err
		}
		if roleTypes != nil {
			toolArgs["roleTypes"] = roleTypes
		}
	}
	if v, _ := cmd.Flags().GetString("plan-finish-date-start"); v != "" {
		ms, err := parseISOTimeToMillis("plan-finish-date-start", v)
		if err != nil {
			return err
		}
		toolArgs["planFinishDateStart"] = ms
	}
	if v, _ := cmd.Flags().GetString("plan-finish-date-end"); v != "" {
		ms, err := parseISOTimeToMillis("plan-finish-date-end", v)
		if err != nil {
			return err
		}
		toolArgs["planFinishDateEnd"] = ms
	}

	return nil
}
