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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/commentreaction"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	commentServer             = "doc-comment"
	commentListRepliesTool    = "list_replies"
	commentRepliesMaxPageSize = 50
)

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

var commentRepliesResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"指定评论的一页直接子回复",
  "properties":{
    "nodeId":{"type":"string","description":"Doc、Sheet 或 Drive 节点 ID"},
    "scope":{"type":"string","enum":["DIRECT"],"description":"固定为 DIRECT"},
    "topicId":{"type":"string","description":"评论主题 ID"},
    "commentKey":{"type":"string","description":"目标父评论标识"},
    "replies":{
      "type":"array",
      "description":"按服务端稳定顺序返回的直接子回复",
      "items":{
        "type":"object",
        "description":"一条直接子回复",
        "properties":{
          "commentKey":{"type":"string","description":"回复评论的唯一标识"},
          "replyToCommentKey":{"type":"string","description":"该回复直接回应的父评论标识"},
          "content":{"type":["string","null"],"description":"回复的纯文本或表情名称"},
          "creatorId":{"type":["string","null"],"description":"回复创建者用户 ID"},
          "createTime":{"type":["integer","null"],"description":"回复创建时间，毫秒时间戳"},
          "updateTime":{"type":["integer","null"],"description":"回复更新时间，毫秒时间戳"},
          "isEmoji":{"type":"boolean","description":"是否为表情回复"}
        },
        "required":["commentKey","replyToCommentKey","isEmoji"],
        "additionalProperties":false
      }
    },
    "complete":{"type":"boolean","description":"是否已扫描到当前 topic 末尾"},
    "scannedCount":{"type":"integer","description":"本次检查的底层原始行数"},
    "stopReason":{"type":"string","enum":["END","PAGE_SIZE","SCAN_LIMIT","TIME_LIMIT"],"description":"本页停止原因"}
  },
  "required":["nodeId","scope","topicId","commentKey","replies","complete","scannedCount","stopReason"],
  "additionalProperties":false
}`)

type commentRepliesPage struct {
	complete     bool
	nextToken    string
	scannedCount int
	stopReason   string
	replies      []map[string]any
}

// newCommentBaseCommands returns the lifecycle commands shared by Doc, Sheet
// and Drive. Drive fixes every topic to global and does not expose cell/inline
// topic inputs.
func newCommentBaseCommands(surface string) []*cobra.Command {
	batchCmd := newCommentBatchQueryCommand(surface)
	resolveCmd := newCommentStatusCommand(surface, true)
	restoreCmd := newCommentStatusCommand(surface, false)
	reactCmd := newCommentReactionCommand(surface)
	commands := []*cobra.Command{batchCmd, resolveCmd, restoreCmd, reactCmd}
	for _, cmd := range commands {
		addCommentNodeAliases(cmd)
	}
	return append(commands, newCommentListRepliesCommand(surface))
}

func newCommentListRepliesCommand(surface string) *cobra.Command {
	surfaceLabel := commentSurfaceLabel(surface)
	drive := surface == "drive"
	long := `Tool 名与 Lark list_replies 对齐，但返回 scope=DIRECT，只包含目标评论
的直接子回复。topicId 和 commentKey 均从 comment list 的同一条结果中取得。

pageSize 只限制本次最多返回多少条直接子回复，不保证凑满。首次不传
--page-token；complete=false 且确实需要继续时，将 meta.pagination.next_token
原样传给下一次 --page-token。服务端不会自动递归展开全部后代。`
	example := fmt.Sprintf("  dws %s comment list-replies --node <NODE_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --page-size 20 --format json", surface)
	agentSummary := "使用 comment list 返回的 topicId + commentKey 分页读取直接子回复"
	avoidWhen := []string{
		"尚未取得 topicId + commentKey 时先使用 comment list",
		"只需首屏回复时不要自动续页或递归",
	}
	examples := []string{
		fmt.Sprintf("dws %s comment list-replies --node <NODE_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --format json", surface),
		fmt.Sprintf("dws %s comment list-replies --node <NODE_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --page-token <NEXT_TOKEN> --page-size 20 --format json", surface),
	}
	flags := []LeafFlag{{
		Name: "node", Usage: commentNodeUsage(surface), Required: true,
		Aliases: []string{"url", "id", "node-id", "doc-id", "file-id"}, Bind: "nodeId", Trim: true,
	}}
	constParams := map[string]any(nil)
	if drive {
		long = `分页查询 Drive 本地文件评论的直接子回复，返回 scope=DIRECT。
Drive 评论固定使用 global topic，因此无需也不允许传入 topicId。

pageSize 只限制本次最多返回多少条直接子回复，不保证凑满。首次不传
--page-token；complete=false 且确实需要继续时，将 meta.pagination.next_token
原样传给下一次 --page-token。服务端不会自动递归展开全部后代。`
		example = "  dws drive comment list-replies --node <dentryUuid> --comment-key <COMMENT_KEY> --page-size 20 --format json"
		agentSummary = "使用 commentKey 分页读取 Drive 文件评论的直接子回复；topic 固定为 global"
		avoidWhen = []string{
			"尚未取得 commentKey 时先使用 drive comment list-v2",
			"只需首屏回复时不要自动续页或递归",
		}
		examples = []string{
			"dws drive comment list-replies --node <dentryUuid> --comment-key <COMMENT_KEY> --format json",
			"dws drive comment list-replies --node <dentryUuid> --comment-key <COMMENT_KEY> --page-token <NEXT_TOKEN> --page-size 20 --format json",
		}
		constParams = map[string]any{"topicId": driveCommentGlobalTopic}
	} else {
		flags = append(flags, LeafFlag{
			Name: "topic-id", Usage: "comment list 返回的 topicId", Required: true,
			Bind: "topicId", Trim: true,
		})
	}
	flags = append(flags,
		LeafFlag{Name: "comment-key", Usage: "目标父评论的 commentKey", Required: true, Bind: "commentKey", Trim: true},
		LeafFlag{Name: "page-size", Usage: "本次最多返回的直接子回复数，范围 1-50", Kind: LeafInt, Default: "20", Bind: "pageSize"},
		LeafFlag{Name: "page-token", Usage: "扫描断点，取自上页 meta.pagination.next_token", Bind: "pageToken", OmitEmpty: true, Trim: true},
	)
	return NewLeafCommand(LeafSpec{
		Use:           "list-replies",
		Short:         "分页查询评论的直接子回复",
		OutputRollout: output.RolloutUnifiedActive,
		Long:          long,
		Example:       example,
		Tool:          commentListRepliesTool,
		Flags:         flags,
		Constraints: []LeafConstraint{
			{Kind: "custom", Flags: []string{"page-size"}, Description: "--page-size 必须在 1-50 之间"},
		},
		ConstParams: constParams,
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity:    commentIdentity(surface, commentListRepliesTool, "list-replies"),
			Description: "分页查询指定评论的直接子回复；返回 scope=DIRECT",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: commentRepliesResultSchema,
			},
			Pagination: &contract.PaginationSpec{
				Kind:            contract.PaginationKindCursor,
				CursorParameter: "page-token",
			},
			Interface: commentInterface(commentListRepliesTool),
			Selection: contract.SelectionSpec{
				AgentSummary: agentSummary,
				UseWhen: []string{
					fmt.Sprintf("需要查看某条 %s 评论的直接文字回复和表情回复时", surfaceLabel),
					fmt.Sprintf("上一页 %s 评论回复 complete=false，且用户确实需要继续读取时", surfaceLabel),
					fmt.Sprintf("用户明确要求读取 %s 评论的全部后代时，对已返回的每条子回复递归调用，并限制总调用次数", surfaceLabel),
				},
				AvoidWhen: avoidWhen,
				Examples:  examples,
			},
		},
		Validate:   validateCommentListReplies,
		ResultCall: runCommentListReplies,
	})
}

func validateCommentListReplies(cmd *cobra.Command, _ []string) error {
	pageSize, _ := cmd.Flags().GetInt("page-size")
	if pageSize < 1 || pageSize > commentRepliesMaxPageSize {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--page-size 必须在 1-%d 之间", commentRepliesMaxPageSize),
		}
	}
	return nil
}

func runCommentListReplies(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	nodeID, _ := args["nodeId"].(string)
	topicID, _ := args["topicId"].(string)
	commentKey, _ := args["commentKey"].(string)
	if deps != nil && deps.Caller != nil && deps.Caller.DryRun() {
		return output.Success(map[string]any{
			"nodeId": nodeID, "scope": "DIRECT", "topicId": topicID, "commentKey": commentKey,
			"replies": []map[string]any{}, "complete": true, "scannedCount": 0, "stopReason": "END",
		}, output.WithDryRun(), output.WithMeta(&output.Meta{Count: output.NewCount(0)})), nil
	}

	page, err := fetchCommentRepliesPage(cmd.Context(), tool, args)
	if err != nil {
		return nil, err
	}
	// fetchCommentRepliesPage has already enforced the two valid pagination
	// states, so NewPagination cannot reject this pair.
	pagination, _ := output.NewPagination(page.complete, page.nextToken)
	pagination.Pages = 1
	pagination.Items = len(page.replies)
	return output.Success(map[string]any{
		"nodeId": nodeID, "scope": "DIRECT", "topicId": topicID, "commentKey": commentKey,
		"replies": page.replies, "complete": page.complete,
		"scannedCount": page.scannedCount, "stopReason": page.stopReason,
	}, output.WithMeta(&output.Meta{
		Count:      output.NewCount(len(page.replies)),
		Pagination: pagination,
	})), nil
}

func fetchCommentRepliesPage(ctx context.Context, tool string, args map[string]any) (commentRepliesPage, error) {
	raw, err := callMCPToolReturnTextOnServer(ctx, commentServer, tool, args)
	if err != nil {
		return commentRepliesPage{}, err
	}
	payload, err := decodeCommentRepliesPayload(tool, raw)
	if err != nil {
		return commentRepliesPage{}, err
	}
	scope, ok := nonEmptyStringField(payload, "scope")
	if !ok || scope != "DIRECT" {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "scope 必须为 DIRECT", nil)
	}
	complete, ok := payload["complete"].(bool)
	if !ok {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "缺少布尔字段 complete", nil)
	}

	rawReplies, found := payload["replyList"]
	if !found {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "缺少 replyList 数组", nil)
	}
	items, ok := rawReplies.([]any)
	if rawReplies == nil {
		items = []any{}
		ok = true
	}
	if !ok {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "回复列表不是数组", nil)
	}
	replies := make([]map[string]any, 0, len(items))
	for index, item := range items {
		reply, ok := item.(map[string]any)
		if !ok {
			return commentRepliesPage{}, invalidCommentRepliesResponse(tool, fmt.Sprintf("replies[%d] 不是对象", index), nil)
		}
		projected, err := projectCommentReply(tool, index, reply)
		if err != nil {
			return commentRepliesPage{}, err
		}
		replies = append(replies, projected)
	}

	nextToken := ""
	if value, exists := payload["nextPageToken"]; exists && value != nil {
		var tokenOK bool
		nextToken, tokenOK = value.(string)
		if !tokenOK {
			return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "nextPageToken 不是字符串", nil)
		}
		nextToken = strings.TrimSpace(nextToken)
	}
	if complete {
		nextToken = ""
	} else if nextToken == "" {
		return commentRepliesPage{}, commentRepliesPaginationError("服务端返回 complete=false 但没有 nextPageToken")
	}
	if current, _ := args["pageToken"].(string); !complete && strings.TrimSpace(current) == nextToken {
		return commentRepliesPage{}, commentRepliesPaginationError("服务端分页游标未前进")
	}
	scannedCount, ok := nonNegativeJSONInt(payload["scannedCount"])
	if !ok {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "scannedCount 必须为非负整数", nil)
	}
	stopReason, ok := nonEmptyStringField(payload, "stopReason")
	if !ok || !validCommentRepliesStopReason(stopReason) {
		return commentRepliesPage{}, invalidCommentRepliesResponse(tool, "stopReason 非法", nil)
	}
	return commentRepliesPage{complete: complete, nextToken: nextToken,
		scannedCount: scannedCount, stopReason: stopReason, replies: replies}, nil
}

func decodeCommentRepliesPayload(tool, raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, invalidCommentRepliesResponse(tool, "返回为空", nil)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, invalidCommentRepliesResponse(tool, "返回不是有效 JSON", err)
	}
	if result, ok := payload["result"].(map[string]any); ok {
		payload = result
	}
	return payload, nil
}

func validCommentRepliesStopReason(reason string) bool {
	switch reason {
	case "END", "PAGE_SIZE", "SCAN_LIMIT", "TIME_LIMIT":
		return true
	default:
		return false
	}
}

func projectCommentReply(tool string, index int, reply map[string]any) (map[string]any, error) {
	commentKey, ok := nonEmptyStringField(reply, "commentKey")
	if !ok {
		return nil, invalidCommentRepliesResponse(tool, fmt.Sprintf("replies[%d] 缺少 commentKey", index), nil)
	}
	replyToCommentKey, ok := nonEmptyStringField(reply, "replyToCommentKey")
	if !ok {
		return nil, invalidCommentRepliesResponse(tool, fmt.Sprintf("replies[%d] 缺少 replyToCommentKey", index), nil)
	}
	isEmoji, ok := reply["isEmoji"].(bool)
	if !ok {
		return nil, invalidCommentRepliesResponse(tool, fmt.Sprintf("replies[%d] 缺少布尔字段 isEmoji", index), nil)
	}
	projected := map[string]any{
		"commentKey":        commentKey,
		"replyToCommentKey": replyToCommentKey,
		"isEmoji":           isEmoji,
	}
	for _, key := range []string{"content", "creatorId", "createTime", "updateTime"} {
		if value, exists := reply[key]; exists {
			projected[key] = value
		}
	}
	return projected, nil
}

func invalidCommentRepliesResponse(tool, reason string, cause error) error {
	return &CLIError{
		Code:       CodeMCPToolError,
		Message:    fmt.Sprintf("%s 返回结构异常：%s", tool, reason),
		Suggestion: "请确认 doc-comment/list_replies 已按 DIRECT 契约发布",
		Operation:  commentServer + "/" + tool,
		Cause:      cause,
	}
}

func commentRepliesPaginationError(message string) error {
	return &CLIError{
		Code:       CodeContentTruncated,
		Message:    message,
		Suggestion: "请稍后重试；不要猜测或复用异常游标",
		Operation:  commentServer + "/" + commentListRepliesTool,
	}
}

func newCommentBatchQueryCommand(surface string) *cobra.Command {
	surfaceLabel := commentSurfaceLabel(surface)
	drive := surface == "drive"
	short := "按 topicId + commentKey 批量查询评论详情"
	long := `批量查询同一文档或表格中的评论详情，单次最多 100 条。

--comment-ref 可重复传入，格式为 topicId:commentKey。topicId 和 commentKey
均可从 comment list 的返回结果中获得。结果严格保持输入顺序；不存在的评论
会返回 found=false 的占位项。`
	example := fmt.Sprintf("  dws %s comment batch-query --node <NODE_ID> --comment-ref global:<COMMENT_KEY> --comment-ref <TOPIC_ID>:<COMMENT_KEY> --format json", surface)
	agentSummary := "已知多组 topicId/commentKey 时，一次获取完整评论详情"
	avoidWhen := []string{"尚未取得 topicId/commentKey 时先使用 comment list"}
	selectionExample := fmt.Sprintf("dws %s comment batch-query --node <NODE_ID> --comment-ref <TOPIC_ID>:<COMMENT_KEY> --format json", surface)
	parameters := []contract.ParamDecl{
		{Name: "node", Property: "nodeId"},
		{Name: "comment-ref", Property: "comments", InterfaceType: "array"},
	}
	if drive {
		short = "按 commentKey 批量查询文件全局评论详情"
		long = `批量查询同一 Drive 本地文件中的全局评论详情，单次最多 100 条。

--comment-key 可重复传入；CLI 自动为每一项补充 topicId=global。结果严格保持
输入顺序，不存在的评论返回 found=false。不要传入 topicId 或旧 commentId。`
		example = "  dws drive comment batch-query --node <dentryUuid> --comment-key <COMMENT_KEY> --comment-key <COMMENT_KEY> --format json"
		agentSummary = "已知多个 commentKey 时，一次获取 Drive 文件全局评论详情"
		avoidWhen = []string{"尚未取得 commentKey 时先使用 drive comment list-v2"}
		selectionExample = "dws drive comment batch-query --node <dentryUuid> --comment-key <COMMENT_KEY> --format json"
		parameters = []contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "comment-key", Property: "comments", InterfaceType: "array"},
		}
	}
	cmd := &cobra.Command{
		Use:     "batch-query",
		Aliases: []string{"batch-query-comments", "batch"},
		Short:   short,
		Long:    long,
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			var refs []map[string]any
			if drive {
				rawKeys, _ := cmd.Flags().GetStringSlice("comment-key")
				refs, err = parseDriveCommentKeys(rawKeys)
			} else {
				rawRefs, _ := cmd.Flags().GetStringSlice("comment-ref")
				refs, err = parseCommentRefs(rawRefs)
			}
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
				AgentSummary: agentSummary,
				UseWhen:      []string{fmt.Sprintf("需要回查 %s 评论列表中的多条详情，或批量核对评论是否仍存在时", surfaceLabel)},
				AvoidWhen:    avoidWhen,
				Examples:     []string{selectionExample},
			},
			Parameters: parameters,
		},
	})
	cmd.Flags().String("node", "", commentNodeUsage(surface)+" (必填)")
	if drive {
		cmd.Flags().StringSlice("comment-key", nil, "评论 commentKey，可重复；最多 100 条 (必填)")
	} else {
		cmd.Flags().StringSlice("comment-ref", nil, "评论引用 topicId:commentKey，可重复；最多 100 条 (必填)")
	}
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

func parseDriveCommentKeys(rawKeys []string) ([]map[string]any, error) {
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("missing required flag(s): --comment-key")
	}
	if len(rawKeys) > 100 {
		return nil, fmt.Errorf("--comment-key 最多可传 100 条，当前为 %d 条", len(rawKeys))
	}
	refs := make([]map[string]any, 0, len(rawKeys))
	for index, raw := range rawKeys {
		commentKey := strings.TrimSpace(raw)
		if commentKey == "" {
			return nil, fmt.Errorf("invalid --comment-key[%d]: value is empty", index)
		}
		refs = append(refs, map[string]any{
			"topicId": driveCommentGlobalTopic, "commentKey": commentKey,
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
	switch surface {
	case "sheet":
		return "表格"
	case "drive":
		return "Drive 本地文件"
	default:
		return "文档"
	}
}

func commentSurfaceLabel(surface string) string {
	switch surface {
	case "sheet":
		return "Sheet 表格"
	case "drive":
		return "Drive 本地文件"
	default:
		return "Doc 文档"
	}
}

func commentNodeUsage(surface string) string {
	if surface == "drive" {
		return "Drive 本地文件 ID (dentryUuid) 或文件 URL"
	}
	return "文档或表格 ID / URL"
}

func nonEmptyStringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key].(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func commentInterface(rpc string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "mcp",
		Availability: "available",
		Ref:          &contract.InterfaceRefSpec{ProductID: commentServer, RPCName: rpc},
	}
}
