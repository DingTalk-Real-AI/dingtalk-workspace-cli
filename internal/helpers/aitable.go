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
	"bufio"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return aitableHandler{}
	})
}

type aitableHandler struct{}

func (aitableHandler) Name() string {
	return "aitable"
}

func (aitableHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "aitable",
		Short:             i18n.T("AITable 多维表格管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	base := &cobra.Command{
		Use:               "base",
		Short:             i18n.T("AI 表格 Base 管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	base.AddCommand(
		newAitableBaseListCommand(runner),
		newAitableBaseSearchCommand(runner),
		newAitableBaseGetCommand(runner),
		newAitableBaseCreateCommand(runner),
		newAitableBaseUpdateCommand(runner),
		newAitableBaseDeleteCommand(runner),
		newAitableBaseGetPrimaryDocIdCommand(runner),
		newAitableBaseCopyCommand(runner),
	)

	table := &cobra.Command{
		Use:               "table",
		Short:             i18n.T("数据表管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	table.AddCommand(
		newAitableTableGetCommand(runner),
		newAitableTableCreateCommand(runner),
		newAitableTableUpdateCommand(runner),
		newAitableTableDeleteCommand(runner),
		newAitableTableListAlias(runner),
	)

	field := &cobra.Command{
		Use:               "field",
		Short:             i18n.T("字段管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	field.AddCommand(
		newAitableFieldGetCommand(runner),
		newAitableFieldCreateCommand(runner),
		newAitableFieldUpdateCommand(runner),
		newAitableFieldDeleteCommand(runner),
		newAitableFieldListAlias(runner),
	)

	record := &cobra.Command{
		Use:               "record",
		Short:             i18n.T("记录管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	record.AddCommand(
		newAitableRecordQueryCommand(runner),
		newAitableRecordGetCommand(runner),
		newAitableRecordCreateCommand(runner),
		newAitableRecordUpdateCommand(runner),
		newAitableRecordBatchUpdateCommand(runner),
		newAitableRecordDeleteCommand(runner),
		newAitableRecordListAlias(runner),
	)

	template := &cobra.Command{
		Use:               "template",
		Short:             i18n.T("模板搜索"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	template.AddCommand(newAitableTemplateSearchCommand(runner))

	attachment := &cobra.Command{
		Use:               "attachment",
		Short:             i18n.T("附件工作流"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	attachment.AddCommand(
		newAITableAttachmentUploadCommand(runner),
		newAITableUploadFileCommand(runner),
	)

	// export / import group：覆盖 mse 默认行为，提供同步轮询 + 自动 IO
	export := &cobra.Command{
		Use:               "export",
		Short:             i18n.T("AI 表格数据导出（异步任务）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	export.AddCommand(newAitableExportDataCommand(runner))

	importCmd := &cobra.Command{
		Use:               "import",
		Short:             i18n.T("AI 表格数据导入（异步任务）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	importCmd.AddCommand(
		newAitableImportUploadCommand(runner),
		newAitableImportDataCommand(runner),
	)

	chart := &cobra.Command{
		Use:               "chart",
		Short:             i18n.T("图表管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	chartShare := &cobra.Command{
		Use:               "share",
		Short:             i18n.T("图表分享管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	chartShare.AddCommand(
		newAitableChartShareGetCommand(runner),
		newAitableChartShareUpdateCommand(runner),
	)
	chart.AddCommand(
		newAitableChartGetCommand(runner),
		newAitableChartCreateCommand(runner),
		newAitableChartUpdateCommand(runner),
		newAitableChartDeleteCommand(runner),
		newAitableChartWidgetsExampleCommand(runner),
		chartShare,
	)

	dashboard := &cobra.Command{
		Use:               "dashboard",
		Short:             i18n.T("仪表盘管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	dashboardShare := &cobra.Command{
		Use:               "share",
		Short:             i18n.T("仪表盘分享管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	dashboardShare.AddCommand(
		newAitableDashboardShareGetCommand(runner),
		newAitableDashboardShareUpdateCommand(runner),
	)
	dashboard.AddCommand(
		newAitableDashboardGetCommand(runner),
		newAitableDashboardCreateCommand(runner),
		newAitableDashboardUpdateCommand(runner),
		newAitableDashboardDeleteCommand(runner),
		newAitableDashboardConfigExampleCommand(runner),
		dashboardShare,
	)

	view := &cobra.Command{
		Use:               "view",
		Short:             i18n.T("视图管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	view.AddCommand(
		newAitableViewGetCommand(runner),
		newAitableViewListCommand(runner),
		newAitableViewCreateCommand(runner),
		newAitableViewUpdateCommand(runner),
		newAitableViewDeleteCommand(runner),
	)

	root.AddCommand(base, table, field, record, newAitableFormCommand(runner), template, attachment, export, importCmd, dashboard, chart, view)

	// 顶层别名：dws aitable search/list/create/info → base search/list/create/get
	// 每个 alias 复用现有 constructor，独立 cobra.Command 实例（避免与 base.* 共享 flag 指针）
	root.AddCommand(
		newAitableSearchAlias(runner),
		newAitableListAlias(runner),
		newAitableCreateAlias(runner),
		newAitableInfoAlias(runner),
	)
	return root
}

func newAitableSearchAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableBaseSearchCommand(runner)
	cmd.Use = "search"
	cmd.Short = i18n.T("搜索 AI 表格（dws aitable base search 的别名）")
	cmd.Example = "  dws aitable search --query 项目管理"
	return cmd
}

func newAitableListAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableBaseListCommand(runner)
	cmd.Use = "list"
	cmd.Short = i18n.T("获取 AI 表格列表（dws aitable base list 的别名）")
	cmd.Example = "  dws aitable list\n  dws aitable list --limit 5"
	return cmd
}

func newAitableCreateAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableBaseCreateCommand(runner)
	cmd.Use = "create"
	cmd.Short = i18n.T("创建 AI 表格（dws aitable base create 的别名）")
	cmd.Example = "  dws aitable create --name 项目跟踪"
	return cmd
}

func newAitableInfoAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableBaseGetCommand(runner)
	cmd.Use = "info"
	cmd.Short = i18n.T("获取 AI 表格信息（dws aitable base get 的别名）")
	cmd.Example = "  dws aitable info --base-id BASE_ID"
	return cmd
}

// TRANSITIONAL: 等 mse 把 get_tables / get_fields / query_records 三条
// toolOverride 加上 `cliAliases: ["list"]` 字段后，下面 3 个 helper 可整体
// 删除——CLI discovery 会自动把 list 注册为对应命令的 cobra alias。
// 工单：plan/mse-yuyuan-patch.md 改动 1.2。

func newAitableTableListAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableTableGetCommand(runner)
	cmd.Use = "list"
	cmd.Short = i18n.T("获取数据表信息（dws aitable table get 的别名）")
	cmd.Example = "  dws aitable table list --base-id BASE_ID\n  dws aitable table list --base-id BASE_ID --table-ids tbl1,tbl2"
	return cmd
}

func newAitableFieldListAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableFieldGetCommand(runner)
	cmd.Use = "list"
	cmd.Short = i18n.T("获取字段列表（dws aitable field get 的别名）")
	cmd.Example = "  dws aitable field list --base-id BASE_ID --table-id TABLE_ID"
	return cmd
}

func newAitableRecordListAlias(runner executor.Runner) *cobra.Command {
	cmd := newAitableRecordQueryCommand(runner)
	cmd.Use = "list"
	cmd.Short = i18n.T("获取记录列表（dws aitable record query 的别名）")
	cmd.Example = "  dws aitable record list --base-id BASE_ID --table-id TABLE_ID"
	return cmd
}

// TRANSITIONAL: 等 mse 把 get_base_primary_doc_id 加入 aitable toolOverrides
// 后，本 helper 可整体删除——CLI discovery 会自动生成等价命令。
// 工单：plan/mse-yuyuan-patch.md 改动 1。
func newAitableBaseGetPrimaryDocIdCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-primary-doc-id",
		Short: i18n.T("获取主键文档 ID"),
		Long: i18n.T(`根据 baseId / tableId / recordId 获取主键文档对应的 dentryUuid。
当 AI 表格使用文档类型作为主键字段时，可凭此 uuid 进一步获取文档内容或执行其它操作。`),
		Example:           "  dws aitable base get-primary-doc-id --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			recordID, err := aitableRequiredFlag(cmd, "record-id")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_base_primary_doc_id", map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"recordId": recordID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("record-id", "", i18n.T("Record ID (必填)"))
	return cmd
}

// ── base delete ────────────────────────────────────────────

func newAitableBaseDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除 AI 表格"),
		Long:              i18n.T("删除指定 Base（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable base delete --base-id BASE_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, _ := cmd.Flags().GetString("base-id")
			if strings.TrimSpace(baseID) == "" {
				return apperrors.NewValidation("--base-id is required")
			}
			if !confirmDeletePrompt(cmd, i18n.T("AI 表格 Base"), baseID) {
				return nil
			}
			params := map[string]any{"baseId": baseID}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				params["reason"] = v
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "aitable", "delete_base", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "aitable", "delete_base", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("reason", "", i18n.T("删除原因（可选）"))

	return cmd
}

// ── table delete ───────────────────────────────────────────

func newAitableTableDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除数据表"),
		Long:              i18n.T("删除指定数据表（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable table delete --base-id BASE_ID --table-id TABLE_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, _ := cmd.Flags().GetString("base-id")
			tableID, _ := cmd.Flags().GetString("table-id")
			if strings.TrimSpace(baseID) == "" {
				return apperrors.NewValidation("--base-id is required")
			}
			if strings.TrimSpace(tableID) == "" {
				return apperrors.NewValidation("--table-id is required")
			}
			if !confirmDeletePrompt(cmd, i18n.T("数据表"), tableID) {
				return nil
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				params["reason"] = v
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "aitable", "delete_table", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "aitable", "delete_table", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("数据表 ID (必填)"))
	cmd.Flags().String("reason", "", i18n.T("删除原因（可选）"))

	return cmd
}

// ── field delete ───────────────────────────────────────────

func newAitableFieldDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除字段"),
		Long:              i18n.T("删除指定字段（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable field delete --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID := aitableFlagOrFallback(cmd, "base-id", "base")
			tableID, _ := cmd.Flags().GetString("table-id")
			fieldID, _ := cmd.Flags().GetString("field-id")
			if strings.TrimSpace(baseID) == "" {
				return apperrors.NewValidation("--base-id is required")
			}
			if strings.TrimSpace(tableID) == "" {
				return apperrors.NewValidation("--table-id is required")
			}
			if strings.TrimSpace(fieldID) == "" {
				return apperrors.NewValidation("--field-id is required")
			}
			if !confirmDeletePrompt(cmd, i18n.T("字段"), fieldID) {
				return nil
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"fieldId": fieldID,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "aitable", "delete_field", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "aitable", "delete_field", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("table-id", "", i18n.T("数据表 ID (必填)"))
	cmd.Flags().String("field-id", "", i18n.T("字段 ID (必填)"))

	return cmd
}

// ── record delete ──────────────────────────────────────────

func newAitableRecordDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除行记录"),
		Long:              i18n.T("批量删除记录（高风险、不可逆），单次最多 100 条。使用 --yes 跳过确认。"),
		Example:           "  dws aitable record delete --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2 --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, _ := cmd.Flags().GetString("base-id")
			tableID, _ := cmd.Flags().GetString("table-id")
			recordIDsStr, _ := cmd.Flags().GetString("record-ids")
			if strings.TrimSpace(baseID) == "" {
				return apperrors.NewValidation("--base-id is required")
			}
			if strings.TrimSpace(tableID) == "" {
				return apperrors.NewValidation("--table-id is required")
			}
			if strings.TrimSpace(recordIDsStr) == "" {
				return apperrors.NewValidation("--record-ids is required")
			}
			if !confirmDeletePrompt(cmd, i18n.T("记录"), recordIDsStr) {
				return nil
			}
			recordIDs := parseAitableCSVValues(recordIDsStr)
			params := map[string]any{
				"baseId":    baseID,
				"tableId":   tableID,
				"recordIds": recordIDs,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "aitable", "delete_records", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "aitable", "delete_records", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("数据表 ID (必填)"))
	cmd.Flags().String("record-ids", "", i18n.T("记录 ID 列表，逗号分隔 (必填)"))

	return cmd
}

// confirmDeletePrompt asks for interactive confirmation before destructive operations.
// Returns true if --yes flag is set or user answers "yes"/"y".
func confirmDeletePrompt(cmd *cobra.Command, resourceType, resourceName string) bool {
	yes, _ := cmd.Flags().GetBool("yes")
	if yes {
		return true
	}
	if commandDryRun(cmd) {
		return true
	}

	fmt.Fprintf(cmd.ErrOrStderr(), i18n.T("⚠️  即将删除 %s: %s\\n"), resourceType, resourceName)
	fmt.Fprint(cmd.ErrOrStderr(), i18n.T("确认删除? (yes/no): "))

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "yes" || answer == "y" {
		return true
	}

	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("已取消操作"))
	return false
}

// ── attachment upload-file ─────────────────────────────────

func newAITableUploadFileCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload-file",
		Short: i18n.T("本地文件一键上传到 AITable 附件字段 (3 步自动合一: prepare + PUT + 返回 fileToken)"),
		Long: `本地文件一键上传到 AITable 附件字段, 一行命令完成 3 步:
  1. prepare_attachment_upload → 获取 OSS 上传地址 uploadUrl + fileToken
  2. HTTP PUT 文件二进制 → OSS
  3. 返回 fileToken (可直接用于 dws aitable record create/update 的 attachment 字段)

推荐 AI Agent 优先使用此命令上传单个附件, 比手动调用 attachment upload (只 prepare)
之后再自己 PUT 文件二进制要可靠得多.`,
		Example:           "  dws aitable attachment upload-file --base-id <BASE_ID> --file ./report.pdf",
		Args:              cobra.NoArgs,
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseId, _ := cmd.Flags().GetString("base-id")
			filePath, _ := cmd.Flags().GetString("file")

			if baseId == "" {
				return apperrors.NewValidation("--base-id is required")
			}
			if filePath == "" {
				return apperrors.NewValidation("--file is required")
			}

			// Validate file
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				return apperrors.NewValidation(i18n.T("无法解析文件路径: ") + err.Error())
			}
			info, err := os.Stat(absPath)
			if err != nil {
				return apperrors.NewValidation(i18n.T("文件不存在: ") + absPath)
			}
			if info.IsDir() {
				return apperrors.NewValidation(i18n.T("不是文件: ") + absPath)
			}
			fileSize := info.Size()
			if fileSize <= 0 {
				return apperrors.NewValidation(i18n.T("文件为空"))
			}
			maxFileSize := config.MaxUploadFileSize
			if fileSize > maxFileSize {
				return apperrors.NewValidation(fmt.Sprintf(i18n.T("文件过大 (%d 字节，限制 %d 字节)"), fileSize, maxFileSize))
			}

			fileName := filepath.Base(absPath)
			mimeType := detectMIME(fileName)

			// Step 1: prepare_attachment_upload
			fmt.Fprintf(os.Stderr, i18n.T("步骤 1/3: 准备上传 %s (%d 字节, %s)...\\n"), fileName, fileSize, mimeType)
			prepareParams := map[string]any{
				"baseId":   baseId,
				"fileName": fileName,
				"size":     fileSize,
				"mimeType": mimeType,
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "aitable", "prepare_attachment_upload", prepareParams,
				))
			}

			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "aitable", "prepare_attachment_upload", prepareParams,
			))
			if err != nil {
				return fmt.Errorf(i18n.T("准备上传失败: %w"), err)
			}

			resultMap := result.Response
			if content, ok := resultMap["content"].(map[string]any); ok && len(content) > 0 {
				resultMap = content
			}
			if resultMap == nil {
				return apperrors.NewValidation(i18n.T("prepare_attachment_upload 返回格式异常"))
			}
			data, _ := resultMap["data"].(map[string]any)
			if data == nil {
				data = resultMap
			}
			uploadURL, _ := data["uploadUrl"].(string)
			fileToken, _ := data["fileToken"].(string)
			if uploadURL == "" || fileToken == "" {
				return apperrors.NewValidation(i18n.T("返回数据缺少 uploadUrl 或 fileToken"))
			}

			// Step 2: HTTP PUT to OSS
			fmt.Fprintln(os.Stderr, i18n.T("步骤 2/3: 上传文件到 OSS..."))
			f, err := os.Open(absPath)
			if err != nil {
				return fmt.Errorf(i18n.T("无法打开文件: %w"), err)
			}
			defer f.Close()

			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPut, uploadURL, f)
			if err != nil {
				return fmt.Errorf(i18n.T("构建上传请求失败: %w"), err)
			}
			req.ContentLength = fileSize
			req.Header.Set("Content-Type", mimeType)

			uploadClient := &http.Client{Timeout: 5 * time.Minute}
			resp, err := uploadClient.Do(req)
			if err != nil {
				return fmt.Errorf(i18n.T("上传失败: %w"), err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				return fmt.Errorf(i18n.T("OSS 上传失败 HTTP %d: %s"), resp.StatusCode, string(body))
			}

			// Step 3: Return fileToken
			fmt.Fprintln(os.Stderr, i18n.T("步骤 3/3: 上传完成！"))
			output := map[string]any{
				"fileToken": fileToken,
				"fileName":  fileName,
				"size":      fileSize,
				"mimeType":  mimeType,
			}
			return writeCommandPayload(cmd, output)
		},
	}
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("file", "", i18n.T("本地文件路径 (必填)"))
	preferLegacyLeaf(cmd)
	return cmd
}

func detectMIME(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}
