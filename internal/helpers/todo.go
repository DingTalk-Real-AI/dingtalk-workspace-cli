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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws todo — 待办
// MCP 工具名与入参以 tools/list 为准；待办详情 query_todo_detail、删除 delete_todo，入参 taskId。
// ──────────────────────────────────────────────────────────

const todoListPageSizeMax = 20

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
				return callMCPTool("get_user_todos_in_current_org", toolArgs)
			}
			return todoListAutoPage(cmd, pageStr, size)
		},
	}

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

	// todoTaskGetCmd 对应 MCP 待办详情；入参以 tools/list 为准，当前为 taskId。
	todoTaskGetCmd := &cobra.Command{
		Use:   "get",
		Short: "待办详情",
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
			if !confirmDelete("待办", taskId) {
				return nil
			}
			return callMCPTool("delete_todo", map[string]any{
				"taskId": taskId,
			})
		},
	}

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

	todoTaskAddReminderCmd := &cobra.Command{
		Use:   "add-reminder",
		Short: "添加待办提醒",
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

	todoTaskUpdateReminderCmd := &cobra.Command{
		Use:   "reset-reminder",
		Short: "重置待办提醒",
		Example: `  dws todo task reset-reminder --task-id <taskId>
			dws todo task reset-reminder --task-id <taskId> --reminder-rules <reminderRules>
  # 查询 taskId: dws todo task list
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			var reminderRules []any
			if v, _ := cmd.Flags().GetString("reminder-rules"); v != "" {
				var att []any
				if err := json.Unmarshal([]byte(v), &att); err == nil && len(att) > 0 {
					for i, item := range att {
						m, ok := item.(map[string]any)
						if !ok {
							continue
						}
						if m["baseTime"] == "customTime" {
							if ts, ok := m["reminderTimeStamp"].(string); ok && ts != "" {
								ms, err := parseISOTimeToMillis("reminderTimeStamp", ts)
								if err != nil {
									return err
								}
								m["reminderTimeStamp"] = ms
								att[i] = m
							}
						}
					}
					reminderRules = att
				}
			}
			return callMCPTool("reset_todo_reminder", map[string]any{
				"todoReminderUpdateRequest": map[string]any{
					"taskId":        mustGetFlag(cmd, "task-id"),
					"reminderRules": reminderRules,
				},
			})
		},
	}

	todoTaskAddAttachment := &cobra.Command{
		Use:   "add-attachment",
		Short: "上传待办附件",
		Long: `上传待办附件
⚠️ 重要：该接口会上传文件到附件，不可用于测试或试探性调用。调用前必须确认待办存在。`,
		Example: `  dws todo task add-attachment --task-id <taskId> --file-path <filePath>
  # 查询 taskId: dws todo task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "file-path"); err != nil {
				return err
			}

			filePath, _ := cmd.Flags().GetString("file-path")
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
	todoTaskUpdateReminderCmd.Flags().String("reminder-rules", "", "提醒规则 JSON 数组 (可选，为空则清除提醒)")
	todoTaskListSubCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskListAttachmentCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddAttachment.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoTaskAddAttachment.Flags().String("file-path", "", "本地文件路径（必填）")
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
			if !confirmDelete("待办评论", commentId) {
				return nil
			}
			return callMCPTool("delete_todo_comment", map[string]any{
				"taskId":    mustGetFlag(cmd, "task-id"),
				"commentId": commentId,
			})
		},
	}

	todoCommentAddCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentAddCmd.Flags().String("content", "", "评论内容 (必填)")
	todoCommentListCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentListCmd.Flags().String("page", "1", "页码 (默认 1)")
	todoCommentListCmd.Flags().String("size", "20", "每页数量 (默认 20)")
	todoCommentDeleteCmd.Flags().String("task-id", "", "待办任务 ID (必填)")
	todoCommentDeleteCmd.Flags().String("comment-id", "", "评论 ID (必填)")

	todoCommentCmd.AddCommand(todoCommentAddCmd, todoCommentListCmd, todoCommentDeleteCmd)
	todoCmd.AddCommand(todoCommentCmd)

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
		Code:       CodeMissingParam,
		Message:    fmt.Sprintf("todo 当前不支持独立 reminder 参数: %s", strings.Join(unsupported, ", ")),
		Suggestion: "请使用 --due 表示截止时间；如果用户要的是精确提醒时间，请明确说明该能力当前不支持，而不是把 --due 当作 reminder。",
		Operation:  "todo.task.reminder",
	}
}

// todoListAutoPage 当 size > 20 时自动分页请求并合并结果。pageStr 为起始页码，wantSize 为期望条数。
func todoListAutoPage(cmd *cobra.Command, pageStr string, wantSize int) error {
	ctx := context.Background()
	if deps.Caller.DryRun() {
		numPages := (wantSize + todoListPageSizeMax - 1) / todoListPageSizeMax
		bold := color.New(color.FgYellow, color.Bold)
		bold.Println("[DRY-RUN] 自动分页待办列表:")
		deps.Out.PrintKeyValue("Tool", "get_user_todos_in_current_org")
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
		text, err := callMCPToolReturnText(ctx, "get_user_todos_in_current_org", toolArgs)
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
		md5Value, err = fileMD5Hex(filePath)
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
