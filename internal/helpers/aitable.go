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
		newAitableRecordCreateCommand(runner),
		newAitableRecordUpdateCommand(runner),
		newAitableRecordDeleteCommand(runner),
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

	root.AddCommand(base, table, field, record, template, attachment)
	return root
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
			baseID, _ := cmd.Flags().GetString("base-id")
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
			var recordIDs []any
			for _, id := range strings.Split(recordIDsStr, ",") {
				if s := strings.TrimSpace(id); s != "" {
					recordIDs = append(recordIDs, s)
				}
			}
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
		Use:    "upload-file",
		Short:  i18n.T("本地文件一键上传到 AITable 附件字段"),
		Hidden: true,
		Long: `完整流程 (自动执行 3 步):
  1. dws aitable attachment upload → 获取 uploadUrl + fileToken
  2. HTTP PUT 上传文件到 OSS
  3. 返回 fileToken，可直接用于 record create/update`,
		Example:           "  dws aitable attachment upload-file --base-id <BASE_ID> --file ./report.pdf",
		Args:              cobra.NoArgs,
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
