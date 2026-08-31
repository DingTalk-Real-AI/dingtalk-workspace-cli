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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const (
	markdownCommentMaxPageSize = 50
)

var markdownCommentListResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"Markdown 文件的评论列表",
  "properties":{
    "commentList":{
      "type":"array",
      "description":"当前页的 Markdown 评论",
      "items":{
        "type":"object",
        "description":"一条 Markdown 评论",
        "properties":{
          "commentKey":{"type":"string","description":"评论生命周期唯一标识"},
          "isGlobal":{"type":"boolean","description":"是否为全文评论；false 表示划词评论"},
          "topicId":{"type":"string","description":"评论主题 ID"},
          "quote":{"type":["string","null"],"description":"划词评论引用的原文"},
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

// newMarkdownCommentCmd is intentionally independent from Drive comments.
// Both products call the same neutral doc-comment MCP contract, while command
// identity, help, selection and exposed lifecycle remain Markdown-owned.
func newMarkdownCommentCmd() *cobra.Command {
	commentCmd := newGroupCommand(&cobra.Command{
		Use:   "comment",
		Short: "Markdown 评论",
		Long: `读取原生 Markdown 文件的新体系评论，支持全文和划词评论。

当前阶段仅开放列表读取；评论写操作暂不在 markdown 产品下暴露。评论 ID 使用
commentKey，topicId 从列表结果中取得。`,
		RunE: groupRunE,
	})

	listCmd := NewLeafCommand(LeafSpec{
		Use:   "list",
		Short: "查询 Markdown 评论列表",
		Long: `查询原生 Markdown 文件的新体系评论，行为与文字文档评论列表一致：
支持全部、全文（global）和划词（inline）评论。每页最多 50 条；--cursor 必须原样
使用上一页返回的 nextToken，不要把 opaque 游标当作数字处理。`,
		Example: `  dws markdown comment list --node <nodeId> --format json
  dws markdown comment list --node <nodeId> --type inline --resolve-status unresolved --limit 20 --format json`,
		Server: commentServer,
		Tool:   "list_comments",
		Flags: []LeafFlag{
			{
				Name: "node", Usage: "Markdown 文件 ID (nodeId/dentryUuid) 或文件 URL", Required: true,
				Aliases: []string{"url", "id", "node-id", "doc-id", "file-id"}, Bind: "nodeId", Trim: true,
			},
			{Name: "limit", Usage: "每页评论数，范围 1-50", Kind: LeafInt, Default: "50", Bind: "pageSize"},
			{Name: "cursor", Usage: "分页游标，取自上页 nextToken", Bind: "nextToken", OmitEmpty: true, Trim: true},
			{Name: "type", Usage: "评论类型: global / inline；不传返回全部", Bind: "commentType", OmitEmpty: true, Trim: true, Enum: []string{"global", "inline"}},
			{Name: "resolve-status", Usage: "解决状态: resolved / unresolved", Bind: "resolveStatus", OmitEmpty: true, Trim: true, Enum: []string{"resolved", "unresolved"}},
		},
		Constraints: []LeafConstraint{
			{Kind: "custom", Flags: []string{"limit"}, Description: "--limit 必须在 1-50 之间"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity("markdown", "list_comments", "list"),
			Description: "查询原生 Markdown 文件的新体系评论列表",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: markdownCommentListResultSchema,
			},
			Interface: commentInterface("list_comments"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询原生 Markdown 文件的全文和划词评论，返回 commentKey、topicId 和解决状态",
				UseWhen: []string{
					"用户要查看原生 .md 文件的新体系全文或划词评论时",
					"需要取得 Markdown 评论 commentKey 供后续处理时",
				},
				AvoidWhen: []string{
					"在线富文本文档评论使用 dws doc comment list",
					"普通非 Markdown 文件评论使用 dws drive comment list-v2",
				},
				Examples: []string{
					"dws markdown comment list --node <nodeId> --format json",
					"dws markdown comment list --node <nodeId> --type inline --resolve-status unresolved --limit 20 --format json",
				},
			},
		},
		Validate: validateMarkdownCommentList,
	})

	commentCmd.AddCommand(listCmd)
	return commentCmd
}

func validateMarkdownCommentList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > markdownCommentMaxPageSize {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--limit 必须在 1-%d 之间", markdownCommentMaxPageSize),
		}
	}
	return nil
}
