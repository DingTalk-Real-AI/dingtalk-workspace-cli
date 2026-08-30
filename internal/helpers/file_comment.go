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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	fileCommentServer           = "doc-comment"
	listFileCommentsTool        = "list_file_comments"
	createFileCommentTool       = "create_file_comment"
	fileCommentMaxPageSize      = 200
	fileCommentMaxAutoPages     = 10
	fileCommentMaxContentLength = 2099
)

type fileCommentPage struct {
	nodeID     string
	total      any
	hasMore    bool
	nextCursor string
	comments   []map[string]any
}

func fileCommentNodeFlag() LeafFlag {
	return LeafFlag{Name: "node", Usage: "文件 ID (dentryUuid)、数字 dentry ID 或钉盘文件 URL", Required: true, Aliases: []string{"url", "id", "node-id", "file-id"}, Bind: "fileId", Trim: true}
}

func fileCommentSpaceIDFlag() LeafFlag {
	return LeafFlag{Name: "space-id", Usage: "钉盘空间 ID；仅数字 dentry ID 必填", Bind: "spaceId", OmitEmpty: true, Trim: true, RequiredWhen: "--node is a numeric dentry ID"}
}

// driveLegacyDeprecationWriter keeps Cobra's deprecation notice out of the
// machine-readable stdout contract while retaining ordinary help output.
type driveLegacyDeprecationWriter struct {
	cmd *cobra.Command
}

func (w *driveLegacyDeprecationWriter) Write(p []byte) (int, error) {
	if bytes.HasPrefix(p, []byte("Command \"")) && bytes.Contains(p, []byte("\" is deprecated, ")) {
		return w.cmd.Root().ErrOrStderr().Write(p)
	}
	return w.cmd.Root().OutOrStdout().Write(p)
}

func markDriveLegacyCommentDeprecated(cmd *cobra.Command, replacement string) {
	cmd.Deprecated = "请改用 " + replacement + " 访问新评论体系"
	cmd.SetOut(&driveLegacyDeprecationWriter{cmd: cmd})
}

// newDriveFileCommentCmd follows the existing resource-first comment surface:
// doc comment, sheet comment, and drive comment. The public command belongs to
// Drive, while the implementation routes to the shared doc-comment MCP server.
func newDriveFileCommentCmd() *cobra.Command {
	commentCmd := newGroupCommand(&cobra.Command{
		Use:   "comment",
		Short: "普通文件评论管理",
		Long: `管理钉盘普通预览文件评论。旧 list/create 保持原评论服务契约但已废弃；
新评论体系使用 list-v2/create-v2 及其余生命周期命令。`,
		RunE: groupRunE,
	})

	listCmd := NewLeafCommand(LeafSpec{
		Use:   "list",
		Short: "查询普通文件评论列表",
		Long: `查询钉盘普通预览文件的评论。支持 dentryUuid、钉盘文件 URL，以及配合
--space-id 使用的数字 dentry ID。支持的文件类型由服务端判定。

默认返回一页；--all 固定按每页 200 条自动翻页，最多 10 页。--scope 在 CLI
侧按服务端返回的统一 anchor 过滤；total 始终表示文件的全部有效评论数，count
表示本次输出且符合 scope 的评论数。`,
		Example: `  dws drive comment list --node <dentryUuid> --format json
  dws drive comment list --node <dentryUuid> --all --format json`,
		Tool: listFileCommentsTool,
		Flags: []LeafFlag{
			fileCommentNodeFlag(),
			fileCommentSpaceIDFlag(),
			{Name: "limit", Usage: "每页评论数，范围 1-200", Kind: LeafInt, Default: "200", Aliases: []string{"page-size"}, Bind: "maxResults"},
			{Name: "cursor", Usage: "分页游标，取自上页 nextCursor", Bind: "nextToken", OmitEmpty: true, Trim: true},
			{Name: "all", Usage: "自动拉取全部评论，最多 10 页 / 2000 条", Kind: LeafBool, Bind: "all"},
			{Name: "scope", Usage: "评论范围: all(全部) / whole(全文) / partial(历史局部)", Default: "all", Bind: "scope", Trim: true, Enum: []string{"all", "whole", "partial"}},
		},
		Constraints: []LeafConstraint{
			{Kind: LeafMutuallyExclusive, Flags: []string{"all", "cursor"}, Description: "--all 与 --cursor 互斥"},
			{Kind: LeafMutuallyExclusive, Flags: []string{"all", "limit"}, Description: "--all 与显式 --limit/--page-size 互斥"},
			{Kind: "custom", Flags: []string{"limit"}, Description: "--limit/--page-size 必须在 1-200 之间"},
			{Kind: "custom", Flags: []string{"cursor"}, Description: "--cursor 必须是服务端返回的非负数字游标"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "list_file_comments",
				CanonicalPath:  "drive.list_file_comments",
				CLIPath:        "drive comment list",
				PrimaryCLIPath: "drive comment list",
			},
			Description: "查询钉盘普通预览文件评论，支持安全分页聚合和全文/局部范围过滤",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The CLI wraps doc-comment/list_file_comments with bounded auto-pagination, cursor anomaly guards, local anchor-scope filtering, and a stable output projection, so no single direct MCP interface represents the complete command contract.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询 PDF、DOCX、XLSX 等钉盘普通预览文件上的评论",
				UseWhen: []string{
					"用户要查看普通文件上的全文评论或历史高亮/矩形评论时",
					"需要从评论列表取得 commentId、作者、时间和统一 anchor，或用 --all 拉取完整列表时",
				},
				AvoidWhen: []string{
					"在线文字文档评论使用 dws doc comment list",
					"在线表格单元格评论使用 dws sheet comment list",
				},
				Examples: []string{
					"dws drive comment list --node <dentryUuid> --format json",
					"dws drive comment list --node <dentryUuid> --all --format json",
				},
			},
		},
		Validate: validateFileCommentList,
		Call:     runFileCommentList,
	})

	createCmd := NewLeafCommand(LeafSpec{
		Use:   "create",
		Short: "创建普通文件全文评论",
		Long: `在钉盘普通预览文件上创建一条全文纯文本评论。当前不支持 @人、通知选项或
局部锚点；content 按服务端规则使用 UTF-16 长度计数，最多 2099。`,
		Example: `  dws drive comment create --node <dentryUuid> --content "请补充最终结论" --format json`,
		Tool:    createFileCommentTool,
		Flags: []LeafFlag{
			fileCommentNodeFlag(),
			fileCommentSpaceIDFlag(),
			{Name: "content", Usage: "全文评论内容，纯文本且 UTF-16 长度不超过 2099", Required: true, Bind: "content"},
		},
		Constraints: []LeafConstraint{
			{Kind: "custom", Flags: []string{"content"}, Description: "--content 去除首尾空白后必须非空，且 UTF-16 长度不超过 2099"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "create_file_comment",
				CanonicalPath:  "drive.create_file_comment",
				CLIPath:        "drive comment create",
				PrimaryCLIPath: "drive comment create",
			},
			Description: "在钉盘普通预览文件上创建一条全文纯文本评论",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The CLI wraps doc-comment/create_file_comment with exact local content validation, runtime confirmation, and a stable flattened output projection, so no single direct MCP interface represents the complete command contract.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在 PDF、DOCX、XLSX 等钉盘普通预览文件上创建全文纯文本评论",
				UseWhen: []string{
					"用户明确要求在普通文件上留下不绑定具体位置的评论时",
				},
				AvoidWhen: []string{
					"在线文字文档评论使用 dws doc comment create",
					"在线表格单元格评论使用 dws sheet comment create；当前普通文件评论不支持 @人或局部锚点",
				},
				Examples: []string{
					"dws drive comment create --node <dentryUuid> --content \"请补充最终结论\" --format json",
				},
			},
		},
		Validate: validateFileCommentCreate,
		Call:     runFileCommentCreate,
	})

	markDriveLegacyCommentDeprecated(listCmd, "dws drive comment list-v2")
	markDriveLegacyCommentDeprecated(createCmd, "dws drive comment create-v2")
	commentCmd.AddCommand(listCmd, createCmd)
	commentCmd.AddCommand(newDriveCommentLifecycleCommands()...)
	return commentCmd
}

func validateFileCommentList(cmd *cobra.Command, _ []string) error {
	if err := validateFileCommentNodeSpace(cmd); err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	if cmd.Flags().Changed("page-size") {
		limit, _ = cmd.Flags().GetInt("page-size")
	}
	if limit < 1 || limit > fileCommentMaxPageSize {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--limit/--page-size 必须在 1-%d 之间", fileCommentMaxPageSize),
		}
	}
	if cursor, _ := cmd.Flags().GetString("cursor"); strings.TrimSpace(cursor) != "" {
		cursor = strings.TrimSpace(cursor)
		if !allASCIIDigits(cursor) {
			return &CLIError{Code: CodeInvalidParam, Message: "--cursor 必须是服务端返回的非负数字游标"}
		}
		if _, err := strconv.ParseInt(cursor, 10, 64); err != nil {
			return &CLIError{Code: CodeInvalidParam, Message: "--cursor 超出 64 位整数范围"}
		}
	}
	return nil
}

func validateFileCommentCreate(cmd *cobra.Command, _ []string) error {
	if err := validateFileCommentNodeSpace(cmd); err != nil {
		return err
	}
	content, _ := cmd.Flags().GetString("content")
	if strings.TrimSpace(content) == "" {
		return &CLIError{Code: CodeInvalidParam, Message: "--content 去除首尾空白后不能为空"}
	}
	length := fileCommentUTF16Length(content)
	if length > fileCommentMaxContentLength {
		return &CLIError{
			Code:    CodeInputTooLarge,
			Message: fmt.Sprintf("--content 最多 %d 个 UTF-16 代码单元，当前为 %d", fileCommentMaxContentLength, length),
		}
	}
	return nil
}

func validateFileCommentNodeSpace(cmd *cobra.Command) error {
	node := corecmd.EffectiveValue(cmd, fileCommentNodeFlag())
	if !allASCIIDigits(node) {
		return nil
	}
	if spaceID := corecmd.EffectiveValue(cmd, fileCommentSpaceIDFlag()); spaceID == "" {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: "--node 为数字 dentry ID 时必须同时提供 --space-id",
		}
	}
	return nil
}

func fileCommentUTF16Length(value string) int {
	length := 0
	for _, r := range value {
		length++
		if r > 0xffff {
			length++
		}
	}
	return length
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func runFileCommentList(cmd *cobra.Command, tool string, args map[string]any) error {
	fetchAll, _ := args["all"].(bool)
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "all"
	}
	request := fileCommentMCPArgs(args)
	if fetchAll {
		request["maxResults"] = fileCommentMaxPageSize
		delete(request, "nextToken")
	}
	if deps != nil && deps.Caller != nil && deps.Caller.DryRun() {
		return callMCPToolOnServer(fileCommentServer, tool, request)
	}

	if !fetchAll {
		page, err := fetchFileCommentPage(tool, request)
		if err != nil {
			return err
		}
		if err := validateFileCommentNextCursor(page, stringArg(request, "nextToken")); err != nil {
			return err
		}
		page.comments = filterFileCommentsByScope(page.comments, scope)
		return output.WriteCommandPayload(cmd, fileCommentListPayload(page, scope), output.FormatJSON)
	}

	var aggregate fileCommentPage
	aggregate.comments = make([]map[string]any, 0)
	seenCursors := map[string]bool{}
	currentCursor := ""
	for pageNumber := 0; pageNumber < fileCommentMaxAutoPages; pageNumber++ {
		if currentCursor == "" {
			delete(request, "nextToken")
		} else {
			request["nextToken"] = currentCursor
		}
		page, err := fetchFileCommentPage(tool, request)
		if err != nil {
			return err
		}
		if pageNumber == 0 {
			aggregate.nodeID = page.nodeID
			aggregate.total = page.total
		}
		aggregate.comments = append(aggregate.comments, filterFileCommentsByScope(page.comments, scope)...)
		if !page.hasMore {
			aggregate.hasMore = false
			aggregate.nextCursor = ""
			return output.WriteCommandPayload(cmd, fileCommentListPayload(aggregate, scope), output.FormatJSON)
		}
		if err := validateFileCommentNextCursor(page, currentCursor); err != nil {
			return err
		}
		if seenCursors[page.nextCursor] {
			return fileCommentPaginationError("分页游标发生循环，结果可能不完整")
		}
		seenCursors[page.nextCursor] = true
		currentCursor = page.nextCursor
	}
	return fileCommentPaginationError(fmt.Sprintf("自动翻页达到 %d 页上限但服务端仍返回 hasMore=true，结果可能不完整", fileCommentMaxAutoPages))
}

func runFileCommentCreate(cmd *cobra.Command, tool string, args map[string]any) error {
	request := fileCommentMCPArgs(args)
	if deps != nil && deps.Caller != nil && deps.Caller.DryRun() {
		return callMCPToolOnServer(fileCommentServer, tool, request)
	}
	raw, err := callMCPToolReturnTextOnServer(context.Background(), fileCommentServer, tool, request)
	if err != nil {
		return err
	}
	payload, err := decodeFileCommentPayload(tool, raw)
	if err != nil {
		return err
	}
	nodeID, ok := nonEmptyStringField(payload, "fileId")
	if !ok {
		return invalidFileCommentResponse(tool, "缺少 fileId", nil)
	}
	comment, ok := payload["comment"].(map[string]any)
	if !ok {
		return invalidFileCommentResponse(tool, "缺少 comment 对象", nil)
	}
	if err := validateFileCommentItem(tool, "comment", comment); err != nil {
		return err
	}
	out := map[string]any{"nodeId": nodeID}
	for key, value := range projectFileComment(comment) {
		out[key] = value
	}
	return output.WriteCommandPayload(cmd, out, output.FormatJSON)
}

func fileCommentMCPArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		switch key {
		case "all", "scope":
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func fetchFileCommentPage(tool string, request map[string]any) (fileCommentPage, error) {
	raw, err := callMCPToolReturnTextOnServer(context.Background(), fileCommentServer, tool, request)
	if err != nil {
		return fileCommentPage{}, err
	}
	payload, err := decodeFileCommentPayload(tool, raw)
	if err != nil {
		return fileCommentPage{}, err
	}
	nodeID, ok := nonEmptyStringField(payload, "fileId")
	if !ok {
		return fileCommentPage{}, invalidFileCommentResponse(tool, "缺少 fileId", nil)
	}
	total, ok := payload["total"]
	if !ok {
		return fileCommentPage{}, invalidFileCommentResponse(tool, "缺少 total", nil)
	}
	hasMore, ok := payload["hasMore"].(bool)
	if !ok {
		return fileCommentPage{}, invalidFileCommentResponse(tool, "缺少布尔字段 hasMore", nil)
	}
	nextCursor := ""
	if value, exists := payload["nextToken"]; exists && value != nil {
		var stringValue bool
		nextCursor, stringValue = value.(string)
		if !stringValue {
			return fileCommentPage{}, invalidFileCommentResponse(tool, "nextToken 不是字符串", nil)
		}
	}
	rawItems, exists := payload["items"]
	if !exists {
		return fileCommentPage{}, invalidFileCommentResponse(tool, "缺少 items", nil)
	}
	items, ok := rawItems.([]any)
	if rawItems == nil {
		items = []any{}
		ok = true
	}
	if !ok {
		return fileCommentPage{}, invalidFileCommentResponse(tool, "items 不是数组", nil)
	}
	comments := make([]map[string]any, 0, len(items))
	for index, item := range items {
		comment, ok := item.(map[string]any)
		if !ok {
			return fileCommentPage{}, invalidFileCommentResponse(tool, fmt.Sprintf("items[%d] 不是对象", index), nil)
		}
		if err := validateFileCommentItem(tool, fmt.Sprintf("items[%d]", index), comment); err != nil {
			return fileCommentPage{}, err
		}
		comments = append(comments, projectFileComment(comment))
	}
	return fileCommentPage{
		nodeID: nodeID, total: total, hasMore: hasMore,
		nextCursor: strings.TrimSpace(nextCursor), comments: comments,
	}, nil
}

func decodeFileCommentPayload(tool, raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, invalidFileCommentResponse(tool, "返回为空", nil)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, invalidFileCommentResponse(tool, "返回不是有效 JSON", err)
	}
	for depth := 0; depth < 2; depth++ {
		if _, ok := payload["fileId"]; ok {
			break
		}
		var nested map[string]any
		for _, key := range []string{"result", "data"} {
			if value, ok := payload[key].(map[string]any); ok {
				nested = value
				break
			}
		}
		if nested == nil {
			break
		}
		payload = nested
	}
	return payload, nil
}

func projectFileComment(comment map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"commentId", "parentCommentId", "content", "createdAt", "updatedAt", "options", "anchor"} {
		if value, ok := comment[key]; ok {
			out[key] = value
		}
	}
	if value, ok := comment["commentCustomType"]; ok {
		out["customType"] = value
	}
	creator := map[string]any{}
	for source, target := range map[string]string{
		"creatorId": "userId", "creatorName": "name", "creatorAvatar": "avatar",
	} {
		if value, ok := comment[source]; ok {
			creator[target] = value
		}
	}
	if len(creator) > 0 {
		out["creator"] = creator
	}
	return out
}

func validateFileCommentItem(tool, path string, comment map[string]any) error {
	if _, ok := nonEmptyStringField(comment, "commentId"); !ok {
		return invalidFileCommentResponse(tool, path+" 缺少 commentId", nil)
	}
	if _, ok := comment["anchor"].(map[string]any); !ok {
		return invalidFileCommentResponse(tool, path+" 缺少 anchor 对象", nil)
	}
	return nil
}

func filterFileCommentsByScope(comments []map[string]any, scope string) []map[string]any {
	if scope == "" || scope == "all" {
		return comments
	}
	out := make([]map[string]any, 0, len(comments))
	for _, comment := range comments {
		anchor, _ := comment["anchor"].(map[string]any)
		commentScope, _ := anchor["scope"].(string)
		if commentScope == scope {
			out = append(out, comment)
		}
	}
	return out
}

func fileCommentListPayload(page fileCommentPage, scope string) map[string]any {
	comments := page.comments
	if comments == nil {
		comments = make([]map[string]any, 0)
	}
	nextCursor := any(nil)
	if page.nextCursor != "" {
		nextCursor = page.nextCursor
	}
	return map[string]any{
		"nodeId":     page.nodeID,
		"total":      page.total,
		"count":      len(page.comments),
		"hasMore":    page.hasMore,
		"nextCursor": nextCursor,
		"complete":   !page.hasMore,
		"scope":      scope,
		"comments":   comments,
	}
}

func validateFileCommentNextCursor(page fileCommentPage, currentCursor string) error {
	if !page.hasMore {
		return nil
	}
	if page.nextCursor == "" {
		return fileCommentPaginationError("服务端返回 hasMore=true 但 nextCursor 为空，结果可能不完整")
	}
	if !allASCIIDigits(page.nextCursor) {
		return fileCommentPaginationError("服务端返回的 nextCursor 不是非负数字游标，结果可能不完整")
	}
	if _, err := strconv.ParseInt(page.nextCursor, 10, 64); err != nil {
		return fileCommentPaginationError("服务端返回的 nextCursor 超出 64 位整数范围，结果可能不完整")
	}
	if page.nextCursor == currentCursor {
		return fileCommentPaginationError("服务端分页游标未前进，结果可能不完整")
	}
	return nil
}

func fileCommentPaginationError(message string) error {
	return &CLIError{
		Code:       CodeContentTruncated,
		Message:    message,
		Suggestion: "请稍后重试，或去掉 --all 后使用服务端返回的 --cursor 分页读取",
		Operation:  fileCommentServer + "/" + listFileCommentsTool,
	}
}

func invalidFileCommentResponse(tool, reason string, cause error) error {
	return &CLIError{
		Code:       CodeMCPToolError,
		Message:    fmt.Sprintf("%s 返回结构异常：%s", tool, reason),
		Suggestion: "请确认 doc-comment MCP 与当前 DWS 契约一致",
		Operation:  fileCommentServer + "/" + tool,
		Cause:      cause,
	}
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
