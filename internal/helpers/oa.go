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
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return oaHandler{}
	})
}

type oaHandler struct{}

func (oaHandler) Name() string {
	return "oa"
}

func (oaHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "oa",
		Short:             "OA 审批 / 同意 / 拒绝 / 撤销",
		Long:              "管理钉钉 OA 审批：待办查询、审批详情、同意、拒绝、撤销、操作记录、已发起列表、表单列表。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	approval := &cobra.Command{
		Use:               "approval",
		Short:             "审批管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	approval.AddCommand(
		newOaApprovalListPendingCommand(runner),
		newOaApprovalDetailCommand(runner),
		newOaApprovalApproveCommand(runner),
		newOaApprovalRejectCommand(runner),
		newOaApprovalRevokeCommand(runner),
		newOaApprovalRecordsCommand(runner),
		newOaApprovalListInitiatedCommand(runner),
		newOaApprovalTasksCommand(runner),
		newOaApprovalListFormsCommand(runner),
	)
	root.AddCommand(approval)
	return root
}

// ── list-pending ────────────────────────────────────────────

func newOaApprovalListPendingCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-pending",
		Short:             "查询待我处理的审批",
		Example:           `  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			startMs, err := parseFlexTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			endMs, err := parseFlexTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			params := map[string]any{
				"starTime": float64(startMs),
				"endTime":  float64(endMs),
			}
			if v, _ := cmd.Flags().GetString("page"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					params["pageNum"] = n
				}
			}
			if v, _ := cmd.Flags().GetString("size"); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					params["pageSize"] = n
				}
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "list_pending_approvals", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	_ = cmd.MarkFlagRequired("start")
	cmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	_ = cmd.MarkFlagRequired("end")
	cmd.Flags().String("page", "", "分页页码 (可选)")
	cmd.Flags().String("size", "", "每页大小 (可选)")
	return cmd
}

// ── detail ──────────────────────────────────────────────────

func newOaApprovalDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "detail",
		Short:             "获取审批实例详情",
		Example:           `  dws oa approval detail --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "get_processInstance_detail",
				map[string]any{"processInstanceId": instanceID},
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	return cmd
}

// ── approve ─────────────────────────────────────────────────

func newOaApprovalApproveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "同意审批",
		Example: `  dws oa approval approve --instance-id <id> --task-id <taskId>  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks
  dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			taskIDStr, _ := cmd.Flags().GetString("task-id")
			taskIdNum, _ := strconv.ParseFloat(taskIDStr, 64)
			params := map[string]any{
				"processInstanceId": instanceID,
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				params["remark"] = v
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "approve_processInstance", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	cmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	_ = cmd.MarkFlagRequired("task-id")
	cmd.Flags().String("remark", "", "审批意见 (可选)")
	return cmd
}

// ── reject ──────────────────────────────────────────────────

func newOaApprovalRejectCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reject",
		Short:             "拒绝审批",
		Example:           `  dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不同意"  # 查询 instanceId: dws oa approval list-pending; taskId 来自 dws oa approval tasks`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			taskIDStr, _ := cmd.Flags().GetString("task-id")
			taskIdNum, _ := strconv.ParseFloat(taskIDStr, 64)
			params := map[string]any{
				"processInstanceId": instanceID,
				"taskId":            taskIdNum,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				params["remark"] = v
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "reject_processInstance", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	cmd.Flags().String("task-id", "", "审批任务 ID (必填)")
	_ = cmd.MarkFlagRequired("task-id")
	cmd.Flags().String("remark", "", "审批意见 (可选)")
	return cmd
}

// ── revoke ──────────────────────────────────────────────────

func newOaApprovalRevokeCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "撤销已发起的审批",
		Example: `  dws oa approval revoke --instance-id <id> --yes  # 查询 instanceId: dws oa approval list-pending
  dws oa approval revoke --instance-id <id> --remark "误发起" --yes`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			params := map[string]any{
				"processInstanceId": instanceID,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				params["remark"] = v
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "revoke_processInstance", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	cmd.Flags().String("remark", "", "撤销说明 (可选)")
	return cmd
}

// ── records ─────────────────────────────────────────────────

func newOaApprovalRecordsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "records",
		Short:             "获取审批操作记录",
		Example:           `  dws oa approval records --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "get_processInstance_records",
				map[string]any{"processInstanceId": instanceID},
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	return cmd
}

// ── list-initiated ──────────────────────────────────────────

func newOaApprovalListInitiatedCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-initiated",
		Short:             "查询已发起的审批实例列表",
		Example:           `  dws oa approval list-initiated --process-code <code> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --next-token 0 --max-results 20  # processCode 来自管理后台配置`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			processCode, _ := cmd.Flags().GetString("process-code")
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			startMs, err := parseFlexTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			endMs, err := parseFlexTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			nextToken, _ := strconv.ParseFloat(getStringFlagDefault(cmd, "next-token", "0"), 64)
			maxResults, _ := strconv.ParseFloat(getStringFlagDefault(cmd, "max-results", "20"), 64)
			params := map[string]any{
				"processCode": processCode,
				"startTime":   float64(startMs),
				"endTime":     float64(endMs),
				"nextToken":   nextToken,
				"maxResults":  maxResults,
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "list_initiated_instances", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("process-code", "", "表单 processCode (必填)")
	_ = cmd.MarkFlagRequired("process-code")
	cmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	_ = cmd.MarkFlagRequired("start")
	cmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	_ = cmd.MarkFlagRequired("end")
	cmd.Flags().String("next-token", "0", "分页游标，首次传 0")
	cmd.Flags().String("max-results", "20", "每页大小，最大 20")
	return cmd
}

// ── tasks ───────────────────────────────────────────────────

func newOaApprovalTasksCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "tasks",
		Short:             "查询待我审批的任务 ID",
		Example:           `  dws oa approval tasks --instance-id <processInstanceId>  # 查询 instanceId: dws oa approval list-pending`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, _ := cmd.Flags().GetString("instance-id")
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "list_pending_tasks",
				map[string]any{"processInstanceId": instanceID},
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("instance-id", "", "审批实例 ID (必填)")
	_ = cmd.MarkFlagRequired("instance-id")
	return cmd
}

// ── list-forms ──────────────────────────────────────────────

func newOaApprovalListFormsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-forms",
		Short:             "获取当前用户可见的审批表单列表",
		Example:           `  dws oa approval list-forms --cursor 0 --size 100`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cursorStr, _ := cmd.Flags().GetString("cursor")
			sizeStr, _ := cmd.Flags().GetString("size")
			cursor, _ := strconv.ParseFloat(cursorStr, 64)
			pageSize, _ := strconv.ParseFloat(sizeStr, 64)
			params := map[string]any{
				"cursor":   cursor,
				"pageSize": pageSize,
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "oa", "list_user_visible_process", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("cursor", "0", "分页游标，首次传 0")
	cmd.Flags().String("size", "100", "每页大小，最大 100")
	return cmd
}

// ── helpers ─────────────────────────────────────────────────

func getStringFlagDefault(cmd *cobra.Command, name, defaultVal string) string {
	v, _ := cmd.Flags().GetString(name)
	if strings.TrimSpace(v) == "" {
		return defaultVal
	}
	return v
}
