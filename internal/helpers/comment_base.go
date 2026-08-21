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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/commentreaction"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const commentServer = "doc-comment"

var commentBatchResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"按请求顺序返回的评论详情",
  "properties":{
    "commentList":{
      "type":"array",
      "description":"与请求 comments 顺序一致的评论详情列表",
      "items":{
        "type":"object",
        "description":"单条评论查询结果；不存在时 found=false",
        "properties":{
          "topicId":{"type":"string","description":"评论主题 ID"},
          "commentKey":{"type":"string","description":"评论唯一标识"},
          "found":{"type":"boolean","description":"该 topicId/commentKey 是否存在"},
          "content":{"type":["string","null"],"description":"纯文本评论内容"},
          "quote":{"type":["string","null"],"description":"划词评论引用内容"},
          "creatorId":{"type":["string","null"],"description":"创建者用户 ID"},
          "createTime":{"type":["integer","null"],"description":"创建时间，毫秒时间戳"},
          "updateTime":{"type":["integer","null"],"description":"更新时间，毫秒时间戳"},
          "isSolved":{"type":["boolean","null"],"description":"是否已解决"},
          "isEmoji":{"type":["boolean","null"],"description":"是否为表情回复"},
          "replyCommentKey":{"type":["string","null"],"description":"被回复评论的标识"}
        },
        "required":["topicId","commentKey","found"],
        "additionalProperties":true
      }
    }
  },
  "required":["commentList"],
  "additionalProperties":true
}`)

var commentStatusResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"评论解决状态更新结果",
  "properties":{
    "commentKey":{"type":"string","description":"评论唯一标识"},
    "resolved":{"type":"boolean","description":"更新后的解决状态"},
    "message":{"type":"string","description":"操作结果消息"}
  },
  "required":["commentKey","resolved"],
  "additionalProperties":true
}`)

var commentReactionResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"表情回复创建结果",
  "properties":{
    "commentKey":{"type":"string","description":"新建表情回复的评论标识"},
    "message":{"type":"string","description":"操作结果消息"}
  },
  "required":["commentKey"],
  "additionalProperties":true
}`)

// newCommentBaseCommands returns the base comment commands shared by Doc and Sheet.
// Both resource domains call the same comment MCP tools; only CLI identity and
// guidance differ.
func newCommentBaseCommands(surface string) []*cobra.Command {
	batchCmd := newCommentBatchQueryCommand(surface)
	resolveCmd := newCommentStatusCommand(surface, true)
	restoreCmd := newCommentStatusCommand(surface, false)
	reactCmd := newCommentReactionCommand(surface)
	commands := []*cobra.Command{batchCmd, resolveCmd, restoreCmd, reactCmd}
	for _, cmd := range commands {
		addCommentNodeAliases(cmd)
	}
	return commands
}

func newCommentBatchQueryCommand(surface string) *cobra.Command {
	resourceName := commentResourceName(surface)
	cmd := &cobra.Command{
		Use:     "batch-query",
		Aliases: []string{"batch-query-comments", "batch"},
		Short:   "按 topicId + commentKey 批量查询评论详情",
		Long: `批量查询同一文档或表格中的评论详情，单次最多 100 条。

--comment-ref 可重复传入，格式为 topicId:commentKey。topicId 和 commentKey
均可从 comment list 的返回结果中获得。结果严格保持输入顺序；不存在的评论
会返回 found=false 的占位项。`,
		Example: fmt.Sprintf("  dws %s comment batch-query --node <NODE_ID> --comment-ref global:<COMMENT_KEY> --comment-ref <TOPIC_ID>:<COMMENT_KEY> --format json", surface),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			rawRefs, _ := cmd.Flags().GetStringSlice("comment-ref")
			refs, err := parseCommentRefs(rawRefs)
			if err != nil {
				return err
			}
			return callMCPToolOnServer(commentServer, "batch_query_comments", map[string]any{
				"nodeId":   nodeID,
				"comments": refs,
			})
		},
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity(surface, "batch_query_comments", "batch-query"),
			Description: "按 topicId + commentKey 批量查询评论详情，保持输入顺序并显式标记缺失项",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: commentBatchResultSchema,
			},
			Interface: commentInterface("batch_query_comments"),
			Selection: contract.SelectionSpec{
				AgentSummary: "已知多组 topicId/commentKey 时，一次获取完整评论详情",
				UseWhen:      []string{fmt.Sprintf("需要回查%s comment list 结果中的多条评论详情，或批量核对评论是否仍存在时", resourceName)},
				AvoidWhen:    []string{fmt.Sprintf("尚未取得%s评论的 topicId/commentKey 时先使用 comment list", resourceName)},
				Examples:     []string{fmt.Sprintf("dws %s comment batch-query --node <NODE_ID> --comment-ref <TOPIC_ID>:<COMMENT_KEY> --format json", surface)},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "comment-ref", Property: "comments", InterfaceType: "array"},
			},
		},
	})
	cmd.Flags().String("node", "", "文档或表格 ID / URL (必填)")
	cmd.Flags().StringSlice("comment-ref", nil, "评论引用 topicId:commentKey，可重复；最多 100 条 (必填)")
	return cmd
}

func newCommentStatusCommand(surface string, resolve bool) *cobra.Command {
	resourceName := commentResourceName(surface)
	use := "restore"
	rpc := "restore_comment"
	short := "将已解决评论恢复为未解决"
	resolved := false
	if resolve {
		use = "resolve"
		rpc = "resolve_comment"
		short = "将评论标记为已解决"
		resolved = true
	}
	cmd := &cobra.Command{
		Use:     use,
		Aliases: []string{use + "-comment"},
		Short:   short,
		Example: fmt.Sprintf("  dws %s comment %s --node <NODE_ID> --comment-key <COMMENT_KEY> --format json", surface, use),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "comment-key"); err != nil {
				return err
			}
			return callMCPToolOnServer(commentServer, rpc, map[string]any{
				"nodeId":     nodeID,
				"commentKey": mustGetFlag(cmd, "comment-key"),
			})
		},
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity(surface, rpc, use),
			Description: short,
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: commentStatusResultSchema,
			},
			Interface: commentInterface(rpc),
			Selection: contract.SelectionSpec{
				AgentSummary: short,
				UseWhen:      []string{fmt.Sprintf("用户明确要求把%s中的某条评论标记为%s时", resourceName, map[bool]string{true: "已解决", false: "未解决"}[resolved])},
				AvoidWhen:    []string{fmt.Sprintf("永久删除%s评论使用 comment delete；修改文字使用 comment update", resourceName)},
				Examples:     []string{fmt.Sprintf("dws %s comment %s --node <NODE_ID> --comment-key <COMMENT_KEY> --format json", surface, use)},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "comment-key", Property: "commentKey"},
			},
		},
	})
	cmd.Flags().String("node", "", "文档或表格 ID / URL (必填)")
	cmd.Flags().String("comment-key", "", "目标评论的 commentKey (必填)")
	return cmd
}

func newCommentReactionCommand(surface string) *cobra.Command {
	resourceName := commentResourceName(surface)
	cmd := &cobra.Command{
		Use:   "react-reply",
		Short: "创建一条表情回复",
		Long: `创建表情回复的便捷命令。底层复用 reply_comment，并固定传入 emoji=true。

与 comment reply --emoji 一致，--reaction 必须填写钉钉表情名称，不要直接传
Unicode Emoji。例如用户要求 😄 时传“憨笑”，要求 👏 时传“鼓掌”。

当前轻量版仅支持创建，不包含删除或聚合能力。`,
		Example: fmt.Sprintf("  dws %s comment react-reply --node <NODE_ID> --comment-key <COMMENT_KEY> --reaction \"憨笑\" --format json", surface),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "comment-key", "reaction"); err != nil {
				return err
			}
			if err := commentreaction.Validate(mustGetFlag(cmd, "reaction")); err != nil {
				return err
			}
			return callMCPToolOnServer(commentServer, "reply_comment", map[string]any{
				"nodeId":          nodeID,
				"replyCommentKey": mustGetFlag(cmd, "comment-key"),
				"content":         mustGetFlag(cmd, "reaction"),
				"emoji":           true,
			})
		},
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity(surface, "react_reply", "react-reply"),
			Description: "为指定评论创建表情回复（轻量版）",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: commentReactionResultSchema,
			},
			Interface: commentInterface("reply_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "使用钉钉表情名称为指定评论创建表情回复",
				UseWhen:      []string{fmt.Sprintf("用户要用表情回应%s中的某条评论时；先将 Unicode Emoji 转为钉钉表情名称，例如 😄 转为憨笑、👏 转为鼓掌", resourceName)},
				AvoidWhen:    []string{fmt.Sprintf("%s评论的普通文字回复使用 comment reply；当前轻量版不支持删除 reaction", resourceName)},
				Examples:     []string{fmt.Sprintf("dws %s comment react-reply --node <NODE_ID> --comment-key <COMMENT_KEY> --reaction \"憨笑\" --format json", surface)},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "comment-key", Property: "replyCommentKey"},
				{Name: "reaction", Property: "content"},
			},
		},
	})
	cmd.Flags().String("node", "", "文档或表格 ID / URL (必填)")
	cmd.Flags().String("comment-key", "", "被回应评论的 commentKey (必填)")
	cmd.Flags().String("reaction", "", "钉钉表情名称，不是 Unicode Emoji；例如 😄=憨笑、👏=鼓掌 (必填)")
	return cmd
}

func parseCommentRefs(rawRefs []string) ([]map[string]any, error) {
	if len(rawRefs) == 0 {
		return nil, fmt.Errorf("missing required flag(s): --comment-ref")
	}
	if len(rawRefs) > 100 {
		return nil, fmt.Errorf("--comment-ref 最多可传 100 条，当前为 %d 条", len(rawRefs))
	}
	refs := make([]map[string]any, 0, len(rawRefs))
	for index, raw := range rawRefs {
		parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --comment-ref[%d] %q: expected topicId:commentKey", index, raw)
		}
		refs = append(refs, map[string]any{
			"topicId":    strings.TrimSpace(parts[0]),
			"commentKey": strings.TrimSpace(parts[1]),
		})
	}
	return refs, nil
}

func addCommentNodeAliases(cmd *cobra.Command) {
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("id", "", "")
	cmd.Flags().String("node-id", "", "")
	cmd.Flags().String("doc-id", "", "")
	cmd.Flags().String("file-id", "", "")
	for _, name := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		_ = cmd.Flags().MarkHidden(name)
	}
}

func commentIdentity(surface, name, cliLeaf string) contract.ToolIdentitySpec {
	return contract.ToolIdentitySpec{
		ProductID:      surface,
		Name:           name,
		CanonicalPath:  surface + "." + name,
		CLIPath:        surface + " comment " + cliLeaf,
		PrimaryCLIPath: surface + " comment " + cliLeaf,
	}
}

func commentResourceName(surface string) string {
	if surface == "sheet" {
		return "表格"
	}
	return "文档"
}

func commentInterface(rpc string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "mcp",
		Availability: "available",
		Ref:          &contract.InterfaceRefSpec{ProductID: commentServer, RPCName: rpc},
	}
}
