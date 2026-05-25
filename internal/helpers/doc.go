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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return docHandler{}
	})
}

type docHandler struct{}

func (docHandler) Name() string {
	return "doc"
}

func (docHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "doc",
		Short:             i18n.T("钉钉文档操作"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	media := &cobra.Command{
		Use:               "media",
		Short:             i18n.T("文档媒体 / 附件管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	media.AddCommand(newDocMediaDownloadCommand(runner))
	media.AddCommand(newDocMediaInsertCommand(runner))

	permission := &cobra.Command{
		Use:               "permission",
		Short:             i18n.T("文档权限协作管理（节点级）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	permission.AddCommand(
		newDocPermissionAddCommand(runner),
		newDocPermissionUpdateCommand(runner),
		newDocPermissionListCommand(runner),
	)

	root.AddCommand(media)
	root.AddCommand(permission)
	root.AddCommand(newDocDeleteCommand(runner))
	return root
}

// TRANSITIONAL: 等 mse 把 add_permission / update_permission /
// list_permission 加入 doc toolOverrides（group: "permission"）后，
// 本节 3 个 helper 可整体删除。工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 与 wiki member add 的关键区别：
//   - wiki member add —— 知识库（workspace）容器级授权
//   - doc permission —— 节点（document/file/folder）级授权

var docPermissionRoles = map[string]bool{
	"MANAGER":    true,
	"EDITOR":     true,
	"DOWNLOADER": true,
	"READER":     true,
}

// normalizeDocPermissionRole 把用户输入的 role 转为悟空兼容的大写形式。
// 返回 (规范化后的 role, 是否合法)。OWNER 不允许通过本接口设置。
func normalizeDocPermissionRole(raw string) (string, bool) {
	r := strings.ToUpper(strings.TrimSpace(raw))
	if r == "" {
		return "", false
	}
	if r == "OWNER" {
		// OWNER 不可通过 add/update 接口添加，统一拒绝
		return r, false
	}
	return r, docPermissionRoles[r]
}

// parseDocPermissionUsers 把逗号分隔的 userId 列表拆成数组，单次最多 30 个。
func parseDocPermissionUsers(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	users := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			users = append(users, p)
		}
	}
	if len(users) == 0 {
		return nil, apperrors.NewValidation("--user must contain at least 1 userId")
	}
	if len(users) > 30 {
		return nil, apperrors.NewValidation(fmt.Sprintf("--user supports at most 30 ids per call (got %d)", len(users)))
	}
	return users, nil
}

func newDocPermissionAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: i18n.T("添加文档协作者"),
		Long: i18n.T(`为指定节点（文档/文件夹/文件）添加一个或多个协作成员，并授予指定角色。

支持角色（--role 大小写不敏感，内部规范化为大写）：
  MANAGER     管理员，可读写、管理成员
  EDITOR      编辑者，可查看、编辑、上传内容
  DOWNLOADER  查看下载者，可查看并下载内容
  READER      仅可查看者

注意：
  - OWNER 角色不可通过本命令添加
  - 单次 --user 最多 30 个 id；超过请分批调用
  - 本命令是节点级授权，跟 dws wiki member add（容器级授权）不同`),
		Example: `  dws doc permission add --node DOC_ID --user uid1,uid2 --role READER
  dws doc permission add --node DOC_ID --user uid1 --role MANAGER`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocPermissionMutation(cmd, runner, "add_permission")
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标节点 nodeId / URL (必填)"))
	cmd.Flags().String("user", "", i18n.T("被授权用户 userId 列表，逗号分隔，单次最多 30 (必填)"))
	cmd.Flags().String("role", "", i18n.T("权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填，辅助构造返回的 docUrl）"))
	return cmd
}

func newDocPermissionUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新文档协作者权限"),
		Long: i18n.T(`更新指定节点已有协作者的权限角色（仅支持 USER 类型成员）。

支持角色与限制同 dws doc permission add。

仅可更新已存在协作关系的用户；新增协作者请使用 add。`),
		Example: `  dws doc permission update --node DOC_ID --user uid1 --role EDITOR
  dws doc permission update --node DOC_ID --user uid1,uid2 --role READER`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocPermissionMutation(cmd, runner, "update_permission")
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标节点 nodeId / URL (必填)"))
	cmd.Flags().String("user", "", i18n.T("被更新用户 userId 列表，逗号分隔，单次最多 30 (必填)"))
	cmd.Flags().String("role", "", i18n.T("新权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填)"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填）"))
	return cmd
}

func newDocPermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("查询文档协作者列表"),
		Long: i18n.T(`查询指定节点的协作者列表，返回每位成员的 userId、姓名、角色等信息。

底层不支持游标分页；--max-results 仅控制单次返回最大条数（默认 30，最大 200）。
若 truncated=true，可通过 --filter-role 收窄查询。`),
		Example: `  dws doc permission list --node DOC_ID
  dws doc permission list --node DOC_ID --max-results 100
  dws doc permission list --node DOC_ID --filter-role MANAGER,EDITOR`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, _ := cmd.Flags().GetString("node")
			if strings.TrimSpace(nodeID) == "" {
				return apperrors.NewValidation("--node is required")
			}
			params := map[string]any{"nodeId": nodeID}
			if v, _ := cmd.Flags().GetInt("max-results"); cmd.Flags().Changed("max-results") {
				if v < 1 || v > 200 {
					return apperrors.NewValidation("--max-results must be between 1 and 200")
				}
				params["maxResults"] = v
			}
			if v, _ := cmd.Flags().GetString("filter-role"); v != "" {
				roles := make([]string, 0)
				for _, r := range strings.Split(v, ",") {
					norm, ok := normalizeDocPermissionRole(r)
					if !ok && strings.ToUpper(strings.TrimSpace(r)) != "OWNER" {
						return apperrors.NewValidation(fmt.Sprintf("--filter-role got invalid role: %s", r))
					}
					roles = append(roles, norm)
				}
				params["filterRoleIds"] = roles
			}
			if v, _ := cmd.Flags().GetString("workspace"); v != "" {
				params["workspaceId"] = v
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "list_permission", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "list_permission", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标节点 nodeId / URL (必填)"))
	cmd.Flags().Int("max-results", 30, i18n.T("返回成员数上限，默认 30，最大 200"))
	cmd.Flags().String("filter-role", "", i18n.T("按角色过滤（逗号分隔）: OWNER / MANAGER / EDITOR / DOWNLOADER / READER"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填）"))
	return cmd
}

// runDocPermissionMutation 是 add / update 两个命令共用的执行体：
// 校验 → 规范化 → 调对应 MCP tool。
func runDocPermissionMutation(cmd *cobra.Command, runner executor.Runner, mcpTool string) error {
	nodeID, _ := cmd.Flags().GetString("node")
	if strings.TrimSpace(nodeID) == "" {
		return apperrors.NewValidation("--node is required")
	}
	rawUsers, _ := cmd.Flags().GetString("user")
	if strings.TrimSpace(rawUsers) == "" {
		return apperrors.NewValidation("--user is required")
	}
	userIDs, err := parseDocPermissionUsers(rawUsers)
	if err != nil {
		return err
	}
	rawRole, _ := cmd.Flags().GetString("role")
	if strings.TrimSpace(rawRole) == "" {
		return apperrors.NewValidation("--role is required")
	}
	role, ok := normalizeDocPermissionRole(rawRole)
	if !ok {
		if role == "OWNER" {
			return apperrors.NewValidation("OWNER role cannot be set via permission add/update")
		}
		return apperrors.NewValidation(fmt.Sprintf("invalid --role: %s (expected MANAGER / EDITOR / DOWNLOADER / READER)", rawRole))
	}
	params := map[string]any{
		"nodeId":  nodeID,
		"roleId":  role,
		"userIds": userIDs,
	}
	if v, _ := cmd.Flags().GetString("workspace"); v != "" {
		params["workspaceId"] = v
	}
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", mcpTool, params,
		))
	}
	result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", mcpTool, params,
	))
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

// TRANSITIONAL: 等 mse 把 delete_document 加入 doc toolOverrides（含
// destructive_hint: true）后，本 helper 可删除——CLI discovery 会自动
// 生成等价命令。工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 命名注意：必须挂在 doc 顶层（不在 block group 下），与现有 mse 中
// `dws doc block delete`（删块，调 delete_document_block）做区分。
func newDocDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: i18n.T("删除整篇文档/文件到回收站"),
		Long: i18n.T(`将文档或文件移入回收站（高风险、不可逆操作）。

权限要求：对目标节点有"管理"权限。
执行前需要确认（交互式输入 yes），或传入 --yes 跳过确认。

与 dws doc block delete 的区别：
  - dws doc delete       —— 删除整篇文档/文件（本命令）
  - dws doc block delete —— 删除文档内部的某个块`),
		Example: `  dws doc delete --node DOC_ID --yes
  dws doc delete --node "https://alidocs.dingtalk.com/i/nodes/<UUID>" --yes
  dws doc delete --node DOC_ID    # 交互式确认后删除`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, _ := cmd.Flags().GetString("node")
			if strings.TrimSpace(nodeID) == "" {
				return apperrors.NewValidation("--node is required")
			}
			if !confirmDeletePrompt(cmd, i18n.T("文档节点"), nodeID) {
				return nil
			}
			params := map[string]any{"nodeId": nodeID}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "delete_document", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "delete_document", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标文档/文件的 nodeId 或 URL (必填)"))
	return cmd
}

// TRANSITIONAL: 等 mse 把 download_doc_attachment 加入 doc toolOverrides
// (cliName: "download", group: "media") 后，本 helper 可删除。
// 工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 单 MCP tool 包装，无本地文件 IO（只拿临时下载 URL，由调用方自行 GET）。
func newDocMediaDownloadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: i18n.T("获取文档附件的临时下载链接"),
		Long: i18n.T(`获取钉钉文档中指定附件的 OSS 临时下载链接，返回 downloadUrl 和过期时间。

resource-id 可通过 dws doc block list 获取：查询目标文档的块列表，找到
blockType 为 attachment 的元素，取其 resourceId。

本命令不下载文件到本地，仅返回 URL。如需落盘，调用方自行 GET 该 URL。`),
		Example: `  dws doc media download --node DOC_ID --resource-id RESOURCE_ID
  dws doc media download --node "https://alidocs.dingtalk.com/i/nodes/<UUID>" --resource-id <ID>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, _ := cmd.Flags().GetString("node")
			resourceID, _ := cmd.Flags().GetString("resource-id")
			if strings.TrimSpace(nodeID) == "" {
				return apperrors.NewValidation("--node is required")
			}
			if strings.TrimSpace(resourceID) == "" {
				return apperrors.NewValidation("--resource-id is required")
			}
			params := map[string]any{
				"nodeId":     nodeID,
				"resourceId": resourceID,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "download_doc_attachment", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "download_doc_attachment", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标文档 nodeId / URL (必填)"))
	cmd.Flags().String("resource-id", "", i18n.T("附件 resourceId，可通过 doc block list 获取 (必填)"))
	return cmd
}

// newDocMediaInsertCommand 把本地文件作为附件上传并插入文档，三步合一：
//  1. get_doc_attachment_upload_info → 获取 uploadUrl + resourceId
//  2. HTTP PUT 文件到 OSS
//  3. insert_document_block → 把附件块挂到文档
//
// 必须 helper 实现：第 2 步 HTTP PUT 是客户端文件 IO，无法用 mse toolOverrides 表达。
func newDocMediaInsertCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insert",
		Short: i18n.T("上传本地文件并作为附件插入文档（3 步合一：prepare + PUT + insert）"),
		Long: i18n.T(`将本地文件作为附件上传并插入到钉钉文档中（三步自动完成）。

流程：
  1. 获取附件上传凭证 (get_doc_attachment_upload_info)
  2. HTTP PUT 上传文件到 OSS
  3. 插入附件块到文档 (insert_document_block)

图片文件（image/*）小于 20MB 时会作为内联图片插入；其他文件作为附件块插入。
--mime-type 可选，不指定时根据文件扩展名自动推断。`),
		Example: `  # 插入 PDF 附件
  dws doc media insert --node DOC_ID --file ./report.pdf

  # 指定名称和 MIME 类型
  dws doc media insert --node DOC_ID --file ./data.bin --name "数据.dat" --mime-type application/octet-stream

  # 在指定块之前插入
  dws doc media insert --node DOC_ID --file ./image.png --ref-block BLOCK_ID --where before`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocMediaInsert(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", i18n.T("目标文档的 nodeId 或 URL (必填)"))
	cmd.Flags().String("file", "", i18n.T("本地文件路径 (必填)"))
	cmd.Flags().String("name", "", i18n.T("附件显示名称（默认使用文件名）"))
	cmd.Flags().String("mime-type", "", i18n.T("文件 MIME 类型（默认根据扩展名推断）"))
	cmd.Flags().Int("index", 0, i18n.T("插入位置索引"))
	cmd.Flags().String("where", "", i18n.T("相对位置: before / after（配合 --ref-block）"))
	cmd.Flags().String("ref-block", "", i18n.T("参考块 ID（配合 --where）"))
	return cmd
}

const docMaxInlineImageSize = 20 * 1024 * 1024 // 20MB

func runDocMediaInsert(cmd *cobra.Command, runner executor.Runner) error {
	nodeID, _ := cmd.Flags().GetString("node")
	filePath, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(nodeID) == "" {
		return apperrors.NewValidation("--node is required")
	}
	if strings.TrimSpace(filePath) == "" {
		return apperrors.NewValidation("--file is required")
	}

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
	if fileSize > config.MaxUploadFileSize {
		return apperrors.NewValidation(fmt.Sprintf(i18n.T("文件过大 (%d 字节，限制 %d 字节)"), fileSize, config.MaxUploadFileSize))
	}

	fileName, _ := cmd.Flags().GetString("name")
	if fileName == "" {
		fileName = filepath.Base(absPath)
	} else if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(absPath); ext != "" {
			fileName += ext
		}
	}

	mimeType, _ := cmd.Flags().GetString("mime-type")
	if mimeType == "" {
		mimeType = detectMIME(fileName)
	}

	// Step 1: 获取上传凭证
	fmt.Fprintf(os.Stderr, i18n.T("步骤 1/3: 获取附件上传凭证 (%s, %d 字节)...\n"), fileName, fileSize)
	step1Params := map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	}
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", "get_doc_attachment_upload_info", step1Params,
		))
	}
	credResult, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "get_doc_attachment_upload_info", step1Params,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("获取上传凭证失败: %w"), err)
	}

	uploadURL, resourceID, resourceURL, err := extractDocAttachmentUploadInfo(credResult.Response)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT
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

	httpClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(i18n.T("上传失败: %w"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf(i18n.T("OSS 上传失败 HTTP %d: %s"), resp.StatusCode, string(body))
	}

	// Step 3: 插入块到文档
	fmt.Fprintln(os.Stderr, i18n.T("步骤 3/3: 插入块到文档..."))
	element := buildDocAttachmentElement(mimeType, fileName, resourceID, resourceURL, fileSize)
	insertArgs := map[string]any{
		"nodeId":  nodeID,
		"element": element,
	}
	if cmd.Flags().Changed("index") {
		if v, _ := cmd.Flags().GetInt("index"); v >= 0 {
			insertArgs["index"] = v
		}
	}
	if v, _ := cmd.Flags().GetString("where"); v != "" {
		insertArgs["where"] = v
	}
	if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
		insertArgs["referenceBlockId"] = v
	}
	insertResult, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "insert_document_block", insertArgs,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("插入块失败: %w"), err)
	}
	return writeCommandPayload(cmd, insertResult)
}

// extractDocAttachmentUploadInfo 从 get_doc_attachment_upload_info 的返回中
// 抽出 uploadUrl / resourceId / resourceUrl 三项。返回结构兼容 content.data
// 和 data 两种包装层次（开源 runner 与 wukong 实测均见过）。
func extractDocAttachmentUploadInfo(resp map[string]any) (uploadURL, resourceID, resourceURL string, err error) {
	if resp == nil {
		err = apperrors.NewValidation(i18n.T("get_doc_attachment_upload_info 返回空"))
		return
	}
	src := resp
	if content, ok := src["content"].(map[string]any); ok && len(content) > 0 {
		src = content
	}
	data, _ := src["data"].(map[string]any)
	if data == nil {
		data = src
	}
	uploadURL, _ = data["uploadUrl"].(string)
	resourceID, _ = data["resourceId"].(string)
	resourceURL, _ = data["resourceUrl"].(string)
	if uploadURL == "" || resourceID == "" {
		err = apperrors.NewValidation(i18n.T("返回数据缺少 uploadUrl 或 resourceId"))
		return
	}
	return
}

// buildDocAttachmentElement 按文件类型生成 insert_document_block 需要的 element 结构。
// 图片 ≤ 20MB 走内联图片，否则走附件块。
func buildDocAttachmentElement(mimeType, fileName, resourceID, resourceURL string, fileSize int64) map[string]any {
	if strings.HasPrefix(mimeType, "image/") && resourceURL != "" && fileSize <= docMaxInlineImageSize {
		return map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{"text": ""},
			"children": []any{
				map[string]any{
					"elementType": "image",
					"properties":  map[string]any{"src": resourceURL},
				},
			},
		}
	}
	viewType := "preview"
	if mimeType == "text/markdown" {
		viewType = "summary"
	}
	return map[string]any{
		"blockType": "attachment",
		"attachment": map[string]any{
			"resourceId": resourceID,
			"type":       mimeType,
			"name":       fileName,
			"viewType":   viewType,
		},
	}
}
