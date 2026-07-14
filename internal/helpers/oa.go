package helpers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws oa — OA 审批
// MCP tools（tools/list）: list_pending_approvals, get_processInstance_detail,
// approve_processInstance, reject_processInstance, revoke_processInstance,
// get_processInstance_records, list_initiated_instances, list_pending_tasks,
// list_user_visible_process, append_task, search_form, oa_ding_user, revert_task,
// get_inst_revert_activities
// ──────────────────────────────────────────────────────────

func newOaCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "oa",
		Short: "OA 审批 / 同意 / 拒绝 / 撤销",
		Long:  `管理钉钉 OA 审批：待办查询、审批详情、同意、拒绝、撤销、操作记录、已发起列表、表单列表。`,
		RunE:  groupRunE,
	}

	approvalCmd := &cobra.Command{Use: "approval", Short: "审批管理", RunE: groupRunE}

	approvalListPendingCmd := &cobra.Command{
		Use:     "list-pending",
		Short:   "查询待我处理的审批",
		Example: `  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			argsMap := map[string]any{
				"starTime": float64(startMs),
				"endTime":  float64(endMs),
			}
			if v, _ := cmd.Flags().GetString("page"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					argsMap["pageNum"] = n
				}
			}
			if v := flagOrFallback(cmd, "limit", "size"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					argsMap["pageSize"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("list_pending_approvals", argsMap)
		},
	}

	approvalDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取审批实例详情",
		Example: `  dws oa approval detail --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("get_processInstance_detail", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}

	approvalApproveCmd := &cobra.Command{
		Use:   "approve",
		Short: "同意审批",
		Example: `  dws oa approval approve --instance-id <id> --task-id <taskId>  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks
  dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("approve_processInstance", argsMap)
		},
	}

	approvalRejectCmd := &cobra.Command{
		Use:     "reject",
		Short:   "拒绝审批",
		Example: `  dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不同意"  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("reject_processInstance", argsMap)
		},
	}

	approvalRevokeCmd := &cobra.Command{
		Use:   "revoke",
		Short: "撤销已发起的审批",
		Example: `  dws oa approval revoke --instance-id <id> --yes  # 查询 instanceId: dws oa approval list-pending
  dws oa approval revoke --instance-id <id> --remark "误发起" --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			instanceId := mustGetFlag(cmd, "instance-id")
			if !confirmDelete("审批实例", instanceId) {
				return nil
			}
			argsMap := map[string]any{
				"processInstanceId": instanceId,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("revoke_processInstance", argsMap)
		},
	}

	approvalRecordsCmd := &cobra.Command{
		Use:     "records",
		Short:   "获取审批操作记录",
		Example: `  dws oa approval records --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("get_processInstance_records", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}

	approvalListInitiatedCmd := &cobra.Command{
		Use:     "list-initiated",
		Short:   "查询审批模板下已发起的审批记录",
		Example: `  dws oa approval list-initiated --process-code <code> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --limit 20  # processCode 来自管理后台配置`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "process-code"); err != nil {
				return err
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			nextToken, _ := strconv.ParseFloat(flagOrFallback(cmd, "cursor", "next-token"), 64)
			maxResults, _ := strconv.ParseFloat(flagOrFallback(cmd, "limit", "max-results"), 64)
			return callMCPTool("list_initiated_instances", map[string]any{
				"processCode": mustGetFlag(cmd, "process-code"),
				"startTime":   float64(startMs),
				"endTime":     float64(endMs),
				"nextToken":   nextToken,
				"maxResults":  maxResults,
			})
		},
	}

	approvalTasksCmd := &cobra.Command{
		Use:     "tasks",
		Short:   "查询待我审批的任务 ID",
		Example: `  dws oa approval tasks --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("list_pending_tasks", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}

	approvalListFormsCmd := &cobra.Command{
		Use:     "list-forms",
		Short:   "获取当前用户可见的审批表单列表",
		Example: `  dws oa approval list-forms --cursor 0 --limit 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cursor, _ := strconv.ParseFloat(mustGetFlag(cmd, "cursor"), 64)
			pageSize, _ := strconv.ParseFloat(flagOrFallback(cmd, "limit", "size"), 64)
			return callMCPTool("list_user_visible_process", map[string]any{
				"cursor":   cursor,
				"pageSize": pageSize,
			})
		},
	}

	// 模糊搜索表单（按 processCode 或 name 关键字）
	approvalSearchFormsCmd := &cobra.Command{
		Use:   "search-forms",
		Short: "按关键字模糊搜索当前用户可见的审批表单",
		Example: `  dws oa approval search-forms --query AI
  dws oa approval search-forms --query 报销`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "query"); err != nil {
				return err
			}
			return callMCPTool("search_form", map[string]any{
				"query": mustGetFlag(cmd, "query"),
			})
		},
	}

	// 获取审批任务的被催办人 userId
	// 仅返回 userId；发 DING 的 robotCode 由 $DINGTALK_DING_ROBOT_CODE / --robot-code 提供，content 由 agent 撰写。
	approvalDingInfoCmd := &cobra.Command{
		Use:   "ding-info",
		Short: "获取审批任务的被催办人 userId（需与 ding message send 串联使用）",
		Example: `  dws oa approval ding-info --task-id <taskId>
  # 返回的 userId 作为 --users 传入 dws ding message send：
  # dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			return callMCPTool("oa_ding_user", map[string]any{
				"taskId": mustGetFlag(cmd, "task-id"),
			})
		},
	}

	// 已经审批过的
	approvalExecutedListCmd := &cobra.Command{
		Use:     "list-executed",
		Short:   "获取当前用户已经处理过的审批单列表",
		Example: `  dws oa approval list-executed  --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_done_tasks", argsMap)
		},
	}
	// 已发起
	approvalSubmittedListCmd := &cobra.Command{
		Use:     "list-submitted",
		Short:   "获取当前用户已发起的审批单列表",
		Example: `  dws oa approval list-submitted --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_submitted_instances", argsMap)
		},
	}
	// 抄送
	approvalCcListCmd := &cobra.Command{
		Use:     "list-cc",
		Short:   "获取抄送当前用户的审批单列表",
		Example: `  dws oa approval list-cc --limit 20 --page 1 --query 关键词`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := strconv.ParseFloat(mustGetFlag(cmd, "limit"), 64)
			pageNumber, _ := strconv.ParseFloat(mustGetFlag(cmd, "page"), 64)
			argsMap := map[string]any{
				"pageNumber": pageNumber,
				"pageSize":   pageSize,
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				argsMap["query"] = v
			}
			return callMCPTool("get_noticed_instances", argsMap)
		},
	}

	// 转交任务
	approvalTransferCmd := &cobra.Command{
		Use:   "redirect-task",
		Short: "转交审批任务给其他人",
		Example: `  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId>
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id", "to-actioner-id"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"taskId":       mustGetFlag(cmd, "task-id"),
				"toActionerId": mustGetFlag(cmd, "to-actioner-id"),
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			return callMCPTool("redirect_task", argsMap)
		},
	}

	// 评论审批实例
	approvalCommentCmd := &cobra.Command{
		Use:     "oa-comments",
		Short:   "对审批实例添加评论",
		Example: `  dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			commentText := flagOrFallback(cmd, "content", "text")
			if commentText == "" {
				return fmt.Errorf("--content is required")
			}
			return callMCPTool("dingflow_comments", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"text":              commentText,
			})
		},
	}

	// 审批抄送
	approvalCcCmd := &cobra.Command{
		Use:   "oa-cc-noticer",
		Short: "对审批实例进行抄送",
		Example: `  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816"
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2" --operator-id "123123"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			userListStr := flagOrFallback(cmd, "users", "user-list")
			if userListStr == "" {
				return fmt.Errorf("--users is required")
			}
			userList := strings.Split(userListStr, ",")
			argsMap := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"userList":          userList,
			}
			return callMCPTool("oa_cc_noticer", argsMap)
		},
	}

	// 加签任务
	approvalAppendTaskCmd := &cobra.Command{
		Use:   "append-task",
		Short: "对审批任务进行加签",
		Example: `  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type before --appender-user-ids "userId1,userId2" --activate-type ALL --agree-all true
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type Parallel --appender-user-ids "userId1" --activate-type ONE_BY_ONE --agree-all false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id", "type", "appender-user-ids", "activate-type", "agree-all"); err != nil {
				return err
			}
			typeVal := mustGetFlag(cmd, "type")
			if typeVal != "before" && typeVal != "after" && typeVal != "Parallel" {
				return fmt.Errorf("--type must be one of: before, after, Parallel, got: %s", typeVal)
			}
			activateTypeVal := mustGetFlag(cmd, "activate-type")
			if activateTypeVal != "ALL" && activateTypeVal != "ONE_BY_ONE" {
				return fmt.Errorf("--activate-type must be one of: ALL, ONE_BY_ONE, got: %s", activateTypeVal)
			}
			appenderUserIdsStr := mustGetFlag(cmd, "appender-user-ids")
			appenderUserIds := strings.Split(appenderUserIdsStr, ",")
			agreeAll, err := strconv.ParseBool(mustGetFlag(cmd, "agree-all"))
			if err != nil {
				return fmt.Errorf("--agree-all must be 'true' or 'false', got: %s", mustGetFlag(cmd, "agree-all"))
			}
			return callMCPTool("append_task", map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            mustGetFlag(cmd, "task-id"),
				"type":              typeVal,
				"appenderUserIds":   appenderUserIds,
				"activateType":      activateTypeVal,
				"agreeAll":          agreeAll,
			})
		},
	}

	// 获取任务可回退的节点信息
	approvalRevertActivitiesCmd := &cobra.Command{
		Use:   "revert-activities",
		Short: "获取审批任务可回退的节点信息（退回前必须调用，获取可回退节点列表）",
		Example: `  dws oa approval revert-activities --task-id <taskId>
  # 返回可回退节点列表，从中选择 targetActivityId 和 action`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task-id"); err != nil {
				return err
			}
			taskIdNum, err := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			if err != nil {
				return fmt.Errorf("--task-id must be a number, got: %s", mustGetFlag(cmd, "task-id"))
			}
			return callMCPTool("get_inst_revert_activities", map[string]any{
				"taskId": taskIdNum,
			})
		},
	}

	// 退回任务（退回到审批人或发起人）
	approvalRevertTaskCmd := &cobra.Command{
		Use:   "revert-task",
		Short: "退回审批任务到指定节点（审批人或发起人）",
		Example: `  # 退回到发起人（targetActivityId 固定传 sid-startevent）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id sid-startevent --action REVERT_FOR_RESUBMIT --remark "补充说明后重提"
  # 退回到某个审批节点（targetActivityId 从审批流程节点信息中获取 activityId）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id <activityId> --action REVERT_FOR_APPROVAL --remark "重新审批"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id", "task-id", "target-activity-id", "action"); err != nil {
				return err
			}
			action := mustGetFlag(cmd, "action")
			if action != "REVERT_FOR_APPROVAL" && action != "REVERT_FOR_RESUBMIT" {
				return fmt.Errorf("--action must be one of: REVERT_FOR_APPROVAL, REVERT_FOR_RESUBMIT, got: %s", action)
			}
			targetActivityId := mustGetFlag(cmd, "target-activity-id")
			// 退回发起人时，targetActivityId 固定为 sid-startevent
			if action == "REVERT_FOR_RESUBMIT" && targetActivityId != "sid-startevent" {
				return fmt.Errorf("--action=REVERT_FOR_RESUBMIT 时 --target-activity-id 必须为 sid-startevent，got: %s", targetActivityId)
			}
			taskIdNum, err := strconv.ParseFloat(mustGetFlag(cmd, "task-id"), 64)
			if err != nil {
				return fmt.Errorf("--task-id must be a number, got: %s", mustGetFlag(cmd, "task-id"))
			}
			inner := map[string]any{
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"taskId":            taskIdNum,
				"targetActivityId":  targetActivityId,
				"revertAction":      action,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				inner["remark"] = v
			}
			return callMCPTool("revert_task", map[string]any{
				"RevertTaskRequest": inner,
			})
		},
	}

	approvalListPendingCmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	approvalListPendingCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	approvalListPendingCmd.Flags().String("page", "", "分页页码 (可选)")
	approvalListPendingCmd.Flags().String("limit", "", "每页大小 (可选)")
	approvalListPendingCmd.Flags().String("size", "", "每页大小 (可选)")
	approvalListPendingCmd.Flags().Lookup("size").Hidden = true
	approvalListPendingCmd.Flags().String("query", "", "关键字搜索（可选）")

	approvalDetailCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalApproveCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalApproveCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalApproveCmd.Flags().String("remark", "", "审批意见 (可选)")
	approvalRejectCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRejectCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalRejectCmd.Flags().String("remark", "", "审批意见 (可选)")
	approvalRevokeCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRevokeCmd.Flags().String("remark", "", "撤销说明 (可选)")
	approvalRecordsCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalListInitiatedCmd.Flags().String("process-code", "", "表单 processCode (必填)")
	approvalListInitiatedCmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	approvalListInitiatedCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	approvalListInitiatedCmd.Flags().String("cursor", "0", "分页游标，首次传 0")
	approvalListInitiatedCmd.Flags().String("next-token", "0", "分页游标，首次传 0")
	approvalListInitiatedCmd.Flags().Lookup("next-token").Hidden = true
	approvalListInitiatedCmd.Flags().String("limit", "20", "每页大小，最大 20")
	approvalListInitiatedCmd.Flags().String("max-results", "20", "每页大小，最大 20")
	approvalListInitiatedCmd.Flags().Lookup("max-results").Hidden = true
	approvalTasksCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalListFormsCmd.Flags().String("cursor", "0", "分页游标（默认 0，翻页传返回的 cursor）")
	approvalListFormsCmd.Flags().String("limit", "100", "每页大小（默认 100，最大 100）")
	approvalListFormsCmd.Flags().String("size", "100", "每页大小（默认 100，最大 100）")
	approvalListFormsCmd.Flags().Lookup("size").Hidden = true
	approvalSearchFormsCmd.Flags().String("query", "", "关键字（匹配 processCode 或表单名称）(必填)")
	approvalDingInfoCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalExecutedListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalExecutedListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalExecutedListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalSubmittedListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalSubmittedListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalSubmittedListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalCcListCmd.Flags().String("page", "1", "分页页码（可选）")
	approvalCcListCmd.Flags().String("limit", "20", "每页大小（可选）")
	approvalCcListCmd.Flags().String("query", "", "关键字搜索（可选）")
	approvalTransferCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalTransferCmd.Flags().String("to-actioner-id", "", "转交目标用户 ID (必填)")
	approvalTransferCmd.Flags().String("remark", "", "转交说明 (可选)")
	approvalCommentCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalCommentCmd.Flags().String("content", "", "评论内容 (必填)")
	approvalCommentCmd.Flags().String("text", "", "评论内容 (必填)")
	approvalCommentCmd.Flags().Lookup("text").Hidden = true
	approvalCcCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalCcCmd.Flags().String("users", "", "抄送用户 ID 列表，多个用逗号分隔 (必填)")
	approvalCcCmd.Flags().String("user-list", "", "抄送用户 ID 列表，多个用逗号分隔 (必填)")
	approvalCcCmd.Flags().Lookup("user-list").Hidden = true
	approvalCcCmd.Flags().String("operator-id", "", "操作人 ID (可选)")

	approvalAppendTaskCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalAppendTaskCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalAppendTaskCmd.Flags().String("type", "", "加签类型：before（前加签），after（后加签），Parallel（并加签）(必填)")
	approvalAppendTaskCmd.Flags().String("appender-user-ids", "", "被加签用户 ID 列表，多个用逗号分隔 (必填)")
	approvalAppendTaskCmd.Flags().String("activate-type", "", "任务激活类型：ALL（或签），ONE_BY_ONE（依次审批）(必填)")
	approvalAppendTaskCmd.Flags().String("agree-all", "", "是否需要全部同意，true 或 false (必填)")

	approvalRevertActivitiesCmd.Flags().String("task-id", "", "审批任务 ID (必填)")

	approvalRevertTaskCmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	approvalRevertTaskCmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	approvalRevertTaskCmd.Flags().String("target-activity-id", "", "退回到的节点 ID（退回发起人固定传 sid-startevent）(必填)")
	approvalRevertTaskCmd.Flags().String("action", "", "退回方式：REVERT_FOR_APPROVAL（退回到审批人）/ REVERT_FOR_RESUBMIT（退回到发起人）(必填)")
	approvalRevertTaskCmd.Flags().String("remark", "", "退回说明 (可选)")

	approvalCmd.AddCommand(
		approvalListPendingCmd,
		approvalDetailCmd,
		approvalApproveCmd,
		approvalRejectCmd,
		approvalRevokeCmd,
		approvalRecordsCmd,
		approvalListInitiatedCmd,
		approvalTasksCmd,
		approvalListFormsCmd,
		approvalSearchFormsCmd,
		approvalDingInfoCmd,
		approvalExecutedListCmd,
		approvalSubmittedListCmd,
		approvalCcListCmd,
		approvalTransferCmd,
		approvalCommentCmd,
		approvalCcCmd,
		approvalAppendTaskCmd,
		approvalRevertActivitiesCmd,
		approvalRevertTaskCmd,
	)
	root.AddCommand(approvalCmd)

	return root
}
