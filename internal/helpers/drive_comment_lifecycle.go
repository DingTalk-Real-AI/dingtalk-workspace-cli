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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const (
	driveCommentGlobalTopic = "global"
	driveCommentMaxPageSize = 50
)

var driveCommentListResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"Drive 本地文件的全局评论列表",
  "properties":{
    "commentList":{
      "type":"array",
      "description":"当前页的文件级全局评论",
      "items":{
        "type":"object",
        "description":"一条全局评论",
        "properties":{
          "commentKey":{"type":"string","description":"评论生命周期唯一标识"},
          "isGlobal":{"type":"boolean","description":"固定为 true"},
          "topicId":{"type":"string","description":"固定为 global"},
          "isSolved":{"type":"boolean","description":"是否已解决"},
          "content":{"type":["string","null"],"description":"评论纯文本内容"},
          "creatorId":{"type":["string","null"],"description":"创建者用户 ID"},
          "createTime":{"type":["integer","null"],"description":"创建时间，毫秒时间戳"},
          "updateTime":{"type":["integer","null"],"description":"更新时间，毫秒时间戳"}
        },
        "required":["commentKey","isGlobal","topicId","isSolved"],
        "additionalProperties":true
      }
    },
    "hasMore":{"type":"boolean","description":"是否还有下一页"},
    "nextToken":{"type":["string","null"],"description":"下一页 opaque 游标"}
  },
  "required":["commentList","hasMore"],
  "additionalProperties":true
}`)

var driveCommentMutationResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"Drive 本地文件评论写操作结果",
  "properties":{
    "commentKey":{"type":"string","description":"创建或操作的评论唯一标识"},
    "message":{"type":["string","null"],"description":"操作结果消息"}
  },
  "required":["commentKey"],
  "additionalProperties":true
}`)

func driveCommentNodeFlag() LeafFlag {
	return LeafFlag{
		Name: "node", Usage: "Drive 本地文件 ID (dentryUuid) 或文件 URL", Required: true,
		Aliases: []string{"url", "id", "node-id", "doc-id", "file-id"}, Bind: "nodeId", Trim: true,
	}
}

// newDriveCommentLifecycleCommands exposes the shared Doc/Sheet comment
// lifecycle without replacing the published legacy list/create contracts.
func newDriveCommentLifecycleCommands() []*cobra.Command {
	listCmd := NewLeafCommand(LeafSpec{
		Use:   "list-v2",
		Short: "查询本地文件全局评论列表",
		Long: `查询 Drive 本地文件的新体系全局评论。每页最多 50 条；--cursor 必须
原样使用上一页返回的 nextToken，不要把 opaque 游标当作数字处理。`,
		Example: `  dws drive comment list-v2 --node <dentryUuid> --format json
  dws drive comment list-v2 --node <dentryUuid> --resolve-status unresolved --limit 20 --format json`,
		Server: commentServer,
		Tool:   "list_comments",
		Flags: []LeafFlag{
			driveCommentNodeFlag(),
			{Name: "limit", Usage: "每页评论数，范围 1-50", Kind: LeafInt, Default: "50", Aliases: []string{"page-size"}, Bind: "pageSize"},
			{Name: "cursor", Usage: "分页游标，取自上页 nextToken", Bind: "nextToken", OmitEmpty: true, Trim: true},
			{Name: "resolve-status", Usage: "解决状态: resolved / unresolved", Bind: "resolveStatus", OmitEmpty: true, Trim: true, Enum: []string{"resolved", "unresolved"}},
		},
		Constraints: []LeafConstraint{
			{Kind: "custom", Flags: []string{"limit"}, Description: "--limit/--page-size 必须在 1-50 之间"},
			{Kind: "custom", Flags: []string{"cursor"}, Description: "--cursor 使用服务端返回的游标"},
		},
		ConstParams: map[string]any{"commentType": driveCommentGlobalTopic},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("drive", "list_comments", "list-v2"),
			Description: "查询 Drive 本地文件的新体系全局评论列表",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: driveCommentListResultSchema,
			},
			Interface: commentInterface("list_comments"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询 Drive 本地文件的全局评论，返回 commentKey 和解决状态",
				UseWhen: []string{
					"用户要查看 PDF、DOCX、XLSX、图片等 Drive 本地文件的新体系评论时",
					"需要取得 commentKey 以继续回复、修改、解决、恢复或删除时",
				},
				AvoidWhen: []string{
					"在线文档使用 dws doc comment list；在线表格单元格评论使用 dws sheet comment list",
					"只需要评论数量统计时使用 dws drive stats",
				},
				Examples: []string{
					"dws drive comment list-v2 --node <dentryUuid> --format json",
					"dws drive comment list-v2 --node <dentryUuid> --resolve-status unresolved --limit 20 --format json",
				},
			},
		},
		Validate: validateDriveCommentList,
	})

	createCmd := NewLeafCommand(LeafSpec{
		Use:     "create-v2",
		Short:   "创建本地文件全局评论",
		Long:    "在 Drive 本地文件上创建一条新体系全局纯文本评论；不支持 mention 或局部锚点。",
		Example: `  dws drive comment create-v2 --node <dentryUuid> --content "请补充最终结论" --format json`,
		Server:  commentServer,
		Tool:    "create_comment",
		Flags: []LeafFlag{
			driveCommentNodeFlag(),
			{Name: "content", Usage: "评论纯文本内容", Required: true, Bind: "content"},
		},
		Constraints: []LeafConstraint{
			{Kind: "custom", Flags: []string{"content"}, Description: "--content 去除首尾空白后必须非空"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("drive", "create_comment", "create-v2"),
			Description: "在 Drive 本地文件上创建全局纯文本评论",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: driveCommentMutationResultSchema,
			},
			Interface: commentInterface("create_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "在 Drive 本地文件上创建一条全局评论",
				UseWhen:      []string{"用户明确要求在本地文件上留下文件级评论时"},
				AvoidWhen: []string{
					"在线文档全文评论使用 dws doc comment create",
					"Drive 本地文件不支持 @用户、@群、单元格或局部锚点",
				},
				Examples: []string{"dws drive comment create-v2 --node <dentryUuid> --content \"请补充最终结论\" --format json"},
			},
		},
		Validate: validateDriveCommentContent,
	})

	replyCmd := NewLeafCommand(LeafSpec{
		Use:     "reply",
		Short:   "回复本地文件评论",
		Long:    "回复一条 Drive 本地文件全局评论。commentKey 从 create-v2/list-v2/list-replies 返回结果取得；表情回应请用 react-reply。",
		Example: `  dws drive comment reply --node <dentryUuid> --comment-key <COMMENT_KEY> --content "已确认" --format json`,
		Server:  commentServer,
		Tool:    "reply_comment",
		Flags: []LeafFlag{
			driveCommentNodeFlag(),
			{Name: "comment-key", Usage: "被回复评论的 commentKey", Required: true, Bind: "replyCommentKey", Trim: true},
			{Name: "content", Usage: "回复纯文本内容", Required: true, Bind: "content"},
		},
		ConstParams: map[string]any{"emoji": false},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("drive", "reply_comment", "reply"),
			Description: "回复 Drive 本地文件全局评论",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: driveCommentMutationResultSchema,
			},
			Interface: commentInterface("reply_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "用纯文本回复已有 Drive 文件评论",
				UseWhen:      []string{"用户要回复一条已有 Drive 文件评论，且已取得 commentKey 时"},
				AvoidWhen:    []string{"新建根评论使用 create-v2；表情回应使用 react-reply；Drive 不支持 mention"},
				Examples:     []string{"dws drive comment reply --node <dentryUuid> --comment-key <COMMENT_KEY> --content \"已确认\" --format json"},
			},
		},
		Validate: validateDriveCommentContent,
	})

	updateCmd := NewLeafCommand(LeafSpec{
		Use:     "update",
		Short:   "更新本地文件评论",
		Long:    "更新 Drive 本地文件中一条全局评论或回复的纯文本内容。",
		Example: `  dws drive comment update --node <dentryUuid> --comment-key <COMMENT_KEY> --content "已按最新结论修正" --format json`,
		Server:  commentServer,
		Tool:    "update_comment",
		Flags: []LeafFlag{
			driveCommentNodeFlag(),
			{Name: "comment-key", Usage: "待更新评论的 commentKey", Required: true, Bind: "commentKey", Trim: true},
			{Name: "content", Usage: "更新后的纯文本内容", Required: true, Bind: "content"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("drive", "update_comment", "update"),
			Description: "更新 Drive 本地文件全局评论内容",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: driveCommentMutationResultSchema,
			},
			Interface: commentInterface("update_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "修改已有 Drive 文件评论或回复的文字内容",
				UseWhen:      []string{"用户明确要求修改一条已有 Drive 文件评论，且已取得 commentKey 时"},
				AvoidWhen:    []string{"回复评论使用 reply；改变解决状态使用 resolve/restore；Drive 不支持 mention"},
				Examples:     []string{"dws drive comment update --node <dentryUuid> --comment-key <COMMENT_KEY> --content \"已修正\" --format json"},
			},
		},
		Validate: validateDriveCommentContent,
	})

	deleteCmd := NewLeafCommand(LeafSpec{
		Use:     "delete",
		Short:   "永久删除本地文件评论",
		Long:    "永久删除 Drive 本地文件中的一条评论或回复。操作不可恢复，执行前必须获得用户确认。",
		Example: `  dws drive comment delete --node <dentryUuid> --comment-key <COMMENT_KEY> --format json`,
		Server:  commentServer,
		Tool:    "delete_comment",
		Flags: []LeafFlag{
			driveCommentNodeFlag(),
			{Name: "comment-key", Usage: "待删除评论的 commentKey", Required: true, Bind: "commentKey", Trim: true},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("drive", "delete_comment", "delete"),
			Description: "永久删除 Drive 本地文件评论或回复",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: driveCommentMutationResultSchema,
			},
			Interface: commentInterface("delete_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "永久删除一条 Drive 文件评论或回复",
				UseWhen:      []string{"用户明确要求永久删除指定 Drive 文件评论，且已确认目标 commentKey 时"},
				AvoidWhen:    []string{"只需改内容使用 update；标记已解决使用 resolve；未确认目标时不要删除"},
				Examples:     []string{"dws drive comment delete --node <dentryUuid> --comment-key <COMMENT_KEY> --format json"},
			},
		},
	})

	commands := []*cobra.Command{listCmd, createCmd, replyCmd, updateCmd, deleteCmd}
	return append(commands, newCommentBaseCommands("drive")...)
}

func validateDriveCommentList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if !cmd.Flags().Changed("limit") && cmd.Flags().Changed("page-size") {
		limit, _ = cmd.Flags().GetInt("page-size")
	}
	if limit < 1 || limit > driveCommentMaxPageSize {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--limit/--page-size 必须在 1-%d 之间", driveCommentMaxPageSize),
		}
	}
	return nil
}

func validateDriveCommentContent(cmd *cobra.Command, _ []string) error {
	content, _ := cmd.Flags().GetString("content")
	if strings.TrimSpace(content) == "" {
		return &CLIError{Code: CodeInvalidParam, Message: "--content 去除首尾空白后不能为空"}
	}
	return nil
}
