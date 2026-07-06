package helpers

import (
	"github.com/spf13/cobra"
)

func newTbCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "tb",
		Short: "Teambition 项目管理",
		Long: `管理 Teambition 项目：项目、任务、工时。

命令结构:
  dws tb project  [list|list-mine|create|update|...]   项目管理
  dws tb task     [create|get|search|update-*|...]     任务管理
  dws tb worktime [list|create|update]                 工时管理`,
		RunE: groupRunE,
	}

	// ── project ─────────────────────────────────────────────────

	projectCmd := &cobra.Command{Use: "project", Short: "项目管理", RunE: groupRunE}

	projectListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查询项目列表",
		Example: `  dws tb project list --name "Q1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			return callMCPTool("list_projects", toolArgs)
		},
	}

	projectListMineCmd := &cobra.Command{
		Use:     "list-mine",
		Short:   "查看我参与的项目",
		Example: `  dws tb project list-mine`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_user_projects", nil)
		},
	}

	projectCreateCmd := &cobra.Command{
		Use:     "create",
		Short:   "创建项目",
		Example: `  dws tb project create --name "Q1 产品迭代"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			return callMCPTool("create_project", map[string]any{
				"name": mustGetFlag(cmd, "name"),
			})
		},
	}

	projectUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新项目信息",
		Example: `  dws tb project update --id PID --name "新名称"
  # 查询 projectId: dws tb project list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{"projectId": mustGetFlag(cmd, "id")}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			return callMCPTool("update_project_info", toolArgs)
		},
	}

	projectListMembersCmd := &cobra.Command{
		Use:   "list-members",
		Short: "查看项目成员",
		Example: `  dws tb project list-members --id PID
  # 查询 projectId: dws tb project list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPTool("get_project_member_list", map[string]any{
				"projectId": mustGetFlag(cmd, "id"),
			})
		},
	}

	projectAddMemberCmd := &cobra.Command{
		Use:   "add-member",
		Short: "添加项目成员",
		Example: `  dws tb project add-member --id PID --users userId1,userId2
  # 查询 projectId: dws tb project list
  # 查询 userId: dws contact user search --keyword "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "users"); err != nil {
				return err
			}
			return callMCPTool("add_project_members", map[string]any{
				"projectId": mustGetFlag(cmd, "id"),
				"userIds":   mustGetFlag(cmd, "users"),
			})
		},
	}

	projectListTaskTypesCmd := &cobra.Command{
		Use:   "list-task-types",
		Short: "查看任务类型",
		Example: `  dws tb project list-task-types --id PID
  # 查询 projectId: dws tb project list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPTool("get_project_task_types", map[string]any{
				"projectId": mustGetFlag(cmd, "id"),
			})
		},
	}

	projectListWorkflowCmd := &cobra.Command{
		Use:   "list-workflow",
		Short: "查看任务状态列表",
		Example: `  dws tb project list-workflow --id PID --query "关键词"
  # 查询 projectId: dws tb project list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{"projectId": mustGetFlag(cmd, "id")}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			return callMCPTool("search_project_workflow_status", toolArgs)
		},
	}

	projectListPrioritiesCmd := &cobra.Command{
		Use:     "list-priorities",
		Short:   "查看企业优先级列表",
		Example: `  dws tb project list-priorities`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_org_task_priorities", nil)
		},
	}

	// ── task ─────────────────────────────────────────────────────

	taskCmd := &cobra.Command{Use: "task", Short: "任务管理", RunE: groupRunE}

	taskCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建任务",
		Example: `  dws tb task create --project PID --title "开发登录模块"
  # 查询 projectId: dws tb project list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "project", "title", "content"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"projectId": mustGetFlag(cmd, "project"),
				"title":     mustGetFlag(cmd, "title"),
				"content":   mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("executor"); v != "" {
				toolArgs["executorId"] = v
			}
			return callMCPTool("create_task", toolArgs)
		},
	}

	taskGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查看任务详情",
		Example: `  dws tb task get --id TID
  # 查询 taskId: dws tb task search --tql "..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPTool("get_task_detail", map[string]any{
				"taskId": mustGetFlag(cmd, "id"),
			})
		},
	}

	taskSearchCmd := &cobra.Command{
		Use:     "search",
		Short:   "TQL 搜索任务",
		Example: `  dws tb task search --tql "isDone = false ORDER BY priority DESC"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "tql"); err != nil {
				return err
			}
			return callMCPTool("search_task_ids_by_tql", map[string]any{
				"tql": mustGetFlag(cmd, "tql"),
			})
		},
	}

	taskUpdateTitleCmd := &cobra.Command{
		Use:   "update-title",
		Short: "修改任务标题",
		Example: `  dws tb task update-title --id TID --title "新标题"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "title"); err != nil {
				return err
			}
			return callMCPTool("update_task_title", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "content": mustGetFlag(cmd, "title"),
			})
		},
	}

	taskUpdateStatusCmd := &cobra.Command{
		Use:   "update-status",
		Short: "更新任务状态",
		Example: `  dws tb task update-status --id TID --status "已完成"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "status"); err != nil {
				return err
			}
			return callMCPTool("update_task_status", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "statusName": mustGetFlag(cmd, "status"),
			})
		},
	}

	taskUpdatePriorityCmd := &cobra.Command{
		Use:   "update-priority",
		Short: "更新任务优先级",
		Example: `  dws tb task update-priority --id TID --priority "紧急"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "priority"); err != nil {
				return err
			}
			return callMCPTool("update_task_priority", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "priorityName": mustGetFlag(cmd, "priority"),
			})
		},
	}

	taskUpdateRemarkCmd := &cobra.Command{
		Use:   "update-remark",
		Short: "更新任务备注",
		Example: `  dws tb task update-remark --id TID --note "## 进展\n已完成第一阶段"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "note"); err != nil {
				return err
			}
			return callMCPTool("update_task_remark", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "note": mustGetFlag(cmd, "note"),
			})
		},
	}

	taskAssignCmd := &cobra.Command{
		Use:   "assign",
		Short: "分配执行人",
		Example: `  dws tb task assign --id TID --executor userId1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "executor"); err != nil {
				return err
			}
			return callMCPTool("assign_task_assignees", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "executorId": mustGetFlag(cmd, "executor"),
			})
		},
	}

	taskUpdateDueCmd := &cobra.Command{
		Use:   "update-due",
		Short: "设置截止日期",
		Example: `  dws tb task update-due --id TID --date 2026-03-15`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "date"); err != nil {
				return err
			}
			return callMCPTool("set_task_due_date", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "dueDate": mustGetFlag(cmd, "date"),
			})
		},
	}

	taskUpdateStartCmd := &cobra.Command{
		Use:   "update-start",
		Short: "设置开始时间",
		Example: `  dws tb task update-start --id TID --date 2026-03-01`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "date"); err != nil {
				return err
			}
			return callMCPTool("set_task_start_time", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "startDate": mustGetFlag(cmd, "date"),
			})
		},
	}

	taskCommentCmd := &cobra.Command{
		Use:   "comment",
		Short: "添加评论",
		Example: `  dws tb task comment --id TID --content "进展顺利"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "content"); err != nil {
				return err
			}
			return callMCPTool("add_task_comment", map[string]any{
				"taskId": mustGetFlag(cmd, "id"), "content": mustGetFlag(cmd, "content"),
			})
		},
	}

	taskAddProgressCmd := &cobra.Command{
		Use:   "add-progress",
		Short: "创建任务进展",
		Example: `  dws tb task add-progress --id TID --title "周进展" --content "已完成60%"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "title", "content"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"taskId":  mustGetFlag(cmd, "id"),
				"title":   mustGetFlag(cmd, "title"),
				"content": mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("status"); v != "" {
				toolArgs["status"] = v
			}
			return callMCPTool("create_task_progress", toolArgs)
		},
	}

	taskGetProgressCmd := &cobra.Command{
		Use:   "get-progress",
		Short: "获取任务进展",
		Example: `  dws tb task get-progress --id TID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPTool("get_task_progress", map[string]any{
				"taskId": mustGetFlag(cmd, "id"),
			})
		},
	}

	// ── worktime ────────────────────────────────────────────────

	worktimeCmd := &cobra.Command{Use: "worktime", Short: "工时管理", RunE: groupRunE}

	worktimeListCmd := &cobra.Command{
		Use:   "list",
		Short: "查看任务工时",
		Example: `  dws tb worktime list --task TID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task"); err != nil {
				return err
			}
			return callMCPTool("get_actual_work_hours_by_task_id", map[string]any{
				"taskId": mustGetFlag(cmd, "task"),
			})
		},
	}

	worktimeCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建工时记录",
		Example: `  dws tb worktime create --task TID --executor userId1 --start 2026-03-01 --end 2026-03-01 --hours 28800000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "task", "executor", "start", "end", "hours"); err != nil {
				return err
			}
			return callMCPTool("create_actual_work_hour_record", map[string]any{
				"taskId":     mustGetFlag(cmd, "task"),
				"executorId": mustGetFlag(cmd, "executor"),
				"startDate":  mustGetFlag(cmd, "start"),
				"endDate":    mustGetFlag(cmd, "end"),
				"actualHour": mustGetFlag(cmd, "hours"),
			})
		},
	}

	worktimeUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新工时记录",
		Example: `  dws tb worktime update --id WID --executor userId1 --date 2026-03-09`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "executor", "date"); err != nil {
				return err
			}
			return callMCPTool("update_actual_work_hour_record", map[string]any{
				"workHourId": mustGetFlag(cmd, "id"),
				"executorId": mustGetFlag(cmd, "executor"),
				"date":       mustGetFlag(cmd, "date"),
			})
		},
	}

	// project flags
	projectListCmd.Flags().String("name", "", "项目名称筛选")
	projectCreateCmd.Flags().String("name", "", "项目名称 (必填)")
	projectUpdateCmd.Flags().String("id", "", "项目 ID (必填)")
	projectUpdateCmd.Flags().String("name", "", "新项目名称")
	projectListMembersCmd.Flags().String("id", "", "项目 ID (必填)")
	projectAddMemberCmd.Flags().String("id", "", "项目 ID (必填)")
	projectAddMemberCmd.Flags().String("users", "", "用户 ID 列表 (必填)")
	projectListTaskTypesCmd.Flags().String("id", "", "项目 ID (必填)")
	projectListWorkflowCmd.Flags().String("id", "", "项目 ID (必填)")
	projectListWorkflowCmd.Flags().String("query", "", "关键词筛选")
	projectListWorkflowCmd.Flags().String("keyword", "", "关键词筛选 (--query 的别名)")
	_ = projectListWorkflowCmd.Flags().MarkHidden("keyword")
	projectCmd.AddCommand(
		projectListCmd, projectListMineCmd, projectCreateCmd,
		projectUpdateCmd, projectListMembersCmd, projectAddMemberCmd,
		projectListTaskTypesCmd, projectListWorkflowCmd, projectListPrioritiesCmd,
	)

	// task flags
	taskCmd.PersistentFlags().String("id", "", "任务 ID")
	taskCreateCmd.Flags().String("project", "", "项目 ID (必填)")
	taskCreateCmd.Flags().String("title", "", "任务标题 (必填)")
	taskCreateCmd.Flags().String("content", "", "任务描述/正文 (必填)")
	taskCreateCmd.Flags().String("executor", "", "执行人 userId")
	taskSearchCmd.Flags().String("tql", "", "TQL 查询语句 (必填)")
	taskUpdateTitleCmd.Flags().String("title", "", "新标题 (必填)")
	taskUpdateStatusCmd.Flags().String("status", "", "状态名 (必填)")
	taskUpdatePriorityCmd.Flags().String("priority", "", "优先级名 (必填)")
	taskUpdateRemarkCmd.Flags().String("note", "", "备注 Markdown (必填)")
	taskAssignCmd.Flags().String("executor", "", "执行人 userId (必填)")
	taskUpdateDueCmd.Flags().String("date", "", "截止日期 ISO-8601，如 2026-03-15 (必填)")
	taskUpdateStartCmd.Flags().String("date", "", "开始日期 ISO-8601，如 2026-03-01 (必填)")
	taskCommentCmd.Flags().String("content", "", "评论内容 (必填)")
	taskAddProgressCmd.Flags().String("title", "", "进展标题 (必填)")
	taskAddProgressCmd.Flags().String("content", "", "进展内容 (必填)")
	taskAddProgressCmd.Flags().String("status", "", "状态: 1=正常 2=风险 3=逾期")
	taskCmd.AddCommand(
		taskCreateCmd, taskGetCmd, taskSearchCmd,
		taskUpdateTitleCmd, taskUpdateStatusCmd, taskUpdatePriorityCmd,
		taskUpdateRemarkCmd, taskAssignCmd, taskUpdateDueCmd,
		taskUpdateStartCmd, taskCommentCmd, taskAddProgressCmd, taskGetProgressCmd,
	)

	// worktime flags
	worktimeListCmd.Flags().String("task", "", "任务 ID (必填)")
	worktimeCreateCmd.Flags().String("task", "", "任务 ID (必填)")
	worktimeCreateCmd.Flags().String("executor", "", "执行人 userId (必填)")
	worktimeCreateCmd.Flags().String("start", "", "开始日期 ISO-8601，如 2026-03-01 (必填)")
	worktimeCreateCmd.Flags().String("end", "", "结束日期 ISO-8601，如 2026-03-01 (必填)")
	worktimeCreateCmd.Flags().String("hours", "", "工时 (毫秒, 必填)")
	worktimeUpdateCmd.Flags().String("id", "", "工时记录 ID (必填)")
	worktimeUpdateCmd.Flags().String("executor", "", "执行人 userId (必填)")
	worktimeUpdateCmd.Flags().String("date", "", "日期 ISO-8601，如 2026-03-09 (必填)")
	worktimeCmd.AddCommand(worktimeListCmd, worktimeCreateCmd, worktimeUpdateCmd)

	root.AddCommand(projectCmd, taskCmd, worktimeCmd)
	return root
}
