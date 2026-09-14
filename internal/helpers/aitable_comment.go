// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const (
	aitableCommentMaxNodes     = 100
	aitableCommentMaxTextUnits = 10000
	aitableCommentMaxMentions  = 20
	aitableCommentMaxImages    = 9
	aitableCommentMaxImageSize = 20000
	aitableCommentMaxPageSize  = 100
)

var aitableCommentItemResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"AI 表格记录评论详情；稳定标识和正文保持服务端返回类型",
  "properties":{
    "topicId":{"type":"string","description":"评论话题 ID"},
    "commentKey":{"type":"string","description":"评论稳定标识"},
    "replyCommentKey":{"type":["string","null"],"description":"被回复的评论标识；根评论为空"},
    "content":{"type":["string","null"],"description":"便于阅读的评论文本摘要，不能用于还原富文本"},
    "richContent":{"type":["array","null"],"description":"文本、人员和图片的有序安全投影","items":{"type":"object","additionalProperties":true}},
    "createTime":{"type":["integer","null"],"description":"创建时间，毫秒时间戳"},
    "updateTime":{"type":["integer","null"],"description":"最后修改时间，毫秒时间戳"},
    "creatorUserId":{"type":["string","null"],"description":"可解析的评论作者外部 userId"},
    "creatorCorpId":{"type":["string","null"],"description":"评论作者 userId 所属企业"}
  },
  "additionalProperties":true
}`)

var aitableCommentListResultSchema = json.RawMessage(`{
  "type":"object",
  "description":"指定 AI 表格记录的一页评论；空 comments 不代表分页完成",
  "properties":{
    "comments":{"type":"array","description":"当前页中属于目标记录的评论及回复","items":{"type":"object","additionalProperties":true}},
    "hasMore":{"type":"boolean","description":"底层文档评论是否仍有下一页"},
    "nextToken":{"type":["string","null"],"description":"不透明续页令牌；末页为空"}
  },
  "required":["comments","hasMore"],
  "additionalProperties":true
}`)

func newAitableCommentCommand() *cobra.Command {
	commentCmd := newGroupCommand(&cobra.Command{
		Use:   "comment",
		Short: "记录评论管理",
		Long:  "管理 AI 表格记录评论：分页查询、创建、回复、更新和删除。评论绑定 baseId、tableId 与 recordId，不是在线电子表格单元格批注。",
		RunE:  groupRunE,
	})

	listCmd := NewLeafCommand(LeafSpec{
		Use:   "list",
		Short: "分页查询记录评论",
		Long: `分页查询指定记录的评论和回复。

comments 为空不代表已经结束；只有 hasMore=false 才表示遍历完成。
hasMore=true 时必须保持 baseId、tableId、recordId 不变，并把 nextToken 原样传给下一次 --cursor。`,
		Example: "  dws aitable comment list --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --limit 50 --format json",
		Tool:    "list_comments",
		Flags: []LeafFlag{
			aitableCommentBaseIDFlag(),
			aitableCommentTableIDFlag(),
			aitableCommentRecordIDFlag(),
			{Name: "limit", Kind: LeafInt, Default: "50", Bind: "pageSize", Usage: "每次检查的底层评论数量，范围 1-100，默认 50"},
			{Name: "cursor", Bind: "nextToken", Trim: true, OmitEmpty: true, Usage: "上一次相同 Base、数据表和记录查询返回的 nextToken"},
		},
		Constraints: []LeafConstraint{{Kind: "custom", Flags: []string{"limit"}, Description: "--limit 必须在 1-100 之间"}},
		Safety:      aitableSafetyRead(),
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "list_comments", CanonicalPath: "aitable.list_comments", CLIPath: "aitable comment list", PrimaryCLIPath: "aitable comment list"},
			Description: "分页查询指定 AI 表格记录的评论和回复；空页仍按 hasMore/nextToken 判断是否续页。",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: aitableCommentListResultSchema,
			},
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
			Interface:  aitableMCPInterface("list_comments"),
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查询一条 AI 表格记录的评论和回复。",
				UseWhen:      []string{"用户要查看某条 AI 表格记录的评论、回复或继续读取评论下一页时"},
				AvoidWhen:    []string{"在线电子表格单元格批注使用 sheet comment list；comments 为空但 hasMore=true 时不要停止"},
				Examples:     []string{"dws aitable comment list --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --format json"},
			},
			Parameters: aitableCommentListParameters(),
		},
		Validate: validateAitableCommentList,
		Call:     callAitableCommentTool,
	})

	createCmd := NewLeafCommand(LeafSpec{
		Use:   "create",
		Short: "在记录上创建评论",
		Long: `在指定 AI 表格记录上创建新评论话题。

纯文本使用 --content；文本、@人员和图片混排使用 --rich-content JSON 数组。两者必须且只能提供一个。
创建是非幂等写操作；超时或连接中断后先用 comment list 对账，禁止直接重试。`,
		Example: `  dws aitable comment create --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --content "请确认" --format json
  dws aitable comment create --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --rich-content '[{"type":"text","text":"请确认 "},{"type":"mention","userId":"<USER_ID>","corpId":"<CORP_ID>"}]' --format json`,
		Tool:        "create_comment",
		Flags:       aitableCommentWriteFlags(""),
		Constraints: aitableCommentContentConstraint(),
		Safety:      aitableCommentCreateSafety(),
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "create_comment", CanonicalPath: "aitable.create_comment", CLIPath: "aitable comment create", PrimaryCLIPath: "aitable comment create"},
			Description: "在指定 AI 表格记录上创建评论话题，支持纯文本或有序富文本节点。",
			Result:      aitableCommentItemResultSpec(),
			Interface:   aitableMCPInterface("create_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "在一条 AI 表格记录上创建评论，可包含文本、@人员和已上传图片。",
				UseWhen:      []string{"用户明确要给某条 AI 表格记录添加评论或留言时"},
				AvoidWhen:    []string{"回复已有评论用 comment reply；在线电子表格单元格批注用 sheet comment create；不要把评论写入记录字段"},
				Examples:     []string{"dws aitable comment create --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --content \"请确认\" --format json"},
			},
			Parameters: aitableCommentWriteParameters(""),
		},
		Call: callAitableCommentTool,
	})

	replyCmd := NewLeafCommand(LeafSpec{
		Use:   "reply",
		Short: "回复记录评论",
		Long: `回复同一记录、同一话题中的已有评论。

--topic-id 和 --comment-key 必须来自该记录的 comment create/list 真实返回；CLI 将 --comment-key 映射为 replyCommentKey。
回复是非幂等写操作；未知结果先查询对账，不自动重试。`,
		Example:     "  dws aitable comment reply --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --content \"已确认\" --format json",
		Tool:        "reply_comment",
		Flags:       aitableCommentWriteFlags("replyCommentKey"),
		Constraints: aitableCommentContentConstraint(),
		Safety:      aitableCommentCreateSafety(),
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "reply_comment", CanonicalPath: "aitable.reply_comment", CLIPath: "aitable comment reply", PrimaryCLIPath: "aitable comment reply"},
			Description: "回复指定 AI 表格记录评论，保留原话题和回复关系。",
			Result:      aitableCommentItemResultSpec(),
			Interface:   aitableMCPInterface("reply_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "使用真实 topicId 和 commentKey 回复 AI 表格记录评论。",
				UseWhen:      []string{"用户要回复已有记录评论，且已取得同一记录的 topicId/commentKey 时"},
				AvoidWhen:    []string{"新建话题用 comment create；未取得真实 topicId/commentKey 时先 comment list"},
				Examples:     []string{"dws aitable comment reply --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --content \"已确认\" --format json"},
			},
			Parameters: aitableCommentWriteParameters("replyCommentKey"),
		},
		Call: callAitableCommentTool,
	})

	updateCmd := NewLeafCommand(LeafSpec{
		Use:   "update",
		Short: "完整替换记录评论正文",
		Long: `完整替换当前用户创建的指定评论正文。

使用 --content 会替换为纯文本并移除原有 @和图片；需要保留或调整富文本时传完整 --rich-content。
服务端没有 CAS，并发修改存在最后写入覆盖窗口。`,
		Example:     "  dws aitable comment update --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --content \"已修正\" --format json",
		Tool:        "update_comment",
		Flags:       aitableCommentWriteFlags("commentKey"),
		Constraints: aitableCommentContentConstraint(),
		Safety:      aitableSafetyWrite(),
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "update_comment", CanonicalPath: "aitable.update_comment", CLIPath: "aitable comment update", PrimaryCLIPath: "aitable comment update"},
			Description: "完整替换当前用户创建的指定 AI 表格记录评论正文。",
			Result:      aitableCommentItemResultSpec(),
			Interface:   aitableMCPInterface("update_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "完整替换本人创建的 AI 表格记录评论正文。",
				UseWhen:      []string{"用户要修改本人已有记录评论，且已取得同一记录的 topicId/commentKey 时"},
				AvoidWhen:    []string{"回复评论用 comment reply；--content 会移除旧 mention 和图片，需要保留时传完整 --rich-content"},
				Examples:     []string{"dws aitable comment update --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --content \"已修正\" --format json"},
			},
			Parameters: aitableCommentWriteParameters("commentKey"),
		},
		Call: callAitableCommentTool,
	})

	deleteCmd := NewLeafCommand(LeafSpec{
		Use:     "delete",
		Short:   "删除记录评论",
		Long:    "删除当前用户创建的指定 AI 表格记录评论。该操作不可恢复；关联回复如何处理由评论服务决定。",
		Example: "  dws aitable comment delete --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --format json",
		Tool:    "delete_comment",
		Flags: []LeafFlag{
			aitableCommentBaseIDFlag(),
			aitableCommentTableIDFlag(),
			aitableCommentRecordIDFlag(),
			aitableCommentTopicIDFlag(),
			aitableCommentKeyFlag("commentKey"),
		},
		Safety: aitableSafetyDestructive(),
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aitable", Name: "delete_comment", CanonicalPath: "aitable.delete_comment", CLIPath: "aitable comment delete", PrimaryCLIPath: "aitable comment delete"},
			Description: "永久删除当前用户创建的指定 AI 表格记录评论。",
			Result:      aitableCommentItemResultSpec(),
			Interface:   aitableMCPInterface("delete_comment"),
			Selection: contract.SelectionSpec{
				AgentSummary: "永久删除本人创建的指定 AI 表格记录评论。",
				UseWhen:      []string{"用户明确要求删除记录评论，且已核对 base/table/record/topic/comment 标识时"},
				AvoidWhen:    []string{"修改正文用 comment update；回复用 comment reply；目标或删除影响未确认时不要执行"},
				Examples:     []string{"dws aitable comment delete --base-id <BASE_ID> --table-id <TABLE_ID> --record-id <RECORD_ID> --topic-id <TOPIC_ID> --comment-key <COMMENT_KEY> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "base-id", Property: "baseId", Required: boolPtr(true)},
				{Name: "table-id", Property: "tableId", Required: boolPtr(true)},
				{Name: "record-id", Property: "recordId", Required: boolPtr(true)},
				{Name: "topic-id", Property: "topicId", Required: boolPtr(true)},
				{Name: "comment-key", Property: "commentKey", Required: boolPtr(true)},
			},
		},
		Call: callAitableCommentTool,
	})

	commentCmd.AddCommand(listCmd, createCmd, replyCmd, updateCmd, deleteCmd)
	return commentCmd
}

func aitableCommentBaseIDFlag() LeafFlag {
	return LeafFlag{Name: "base-id", Aliases: []string{"base"}, Bind: "baseId", Trim: true, Required: true, Usage: "Base ID，可通过 base list/search 获取 (必填)"}
}

func aitableCommentTableIDFlag() LeafFlag {
	return LeafFlag{Name: "table-id", Bind: "tableId", Trim: true, Required: true, Usage: "数据表 ID，可通过 base get 或 table list 获取 (必填)"}
}

func aitableCommentRecordIDFlag() LeafFlag {
	return LeafFlag{Name: "record-id", Bind: "recordId", Trim: true, Required: true, Usage: "目标记录 ID，可通过 record query/create 获取 (必填)"}
}

func aitableCommentTopicIDFlag() LeafFlag {
	return LeafFlag{Name: "topic-id", Bind: "topicId", Trim: true, Required: true, Usage: "同一记录 comment create/list 返回的 topicId (必填)"}
}

func aitableCommentKeyFlag(bind string) LeafFlag {
	flag := LeafFlag{Name: "comment-key", Bind: bind, Trim: true, Required: true, Usage: "同一记录 comment create/list 返回的 commentKey (必填)"}
	if bind == "replyCommentKey" {
		// replyCommentKey 是 MCP 的接口字段名；CLI 主参数仍与 list 返回的
		// commentKey 对齐，显式别名只在 reply 上开放，避免污染 update/delete。
		flag.Aliases = []string{"reply-comment-key"}
	}
	return flag
}

func aitableCommentWriteFlags(commentKeyProperty string) []LeafFlag {
	flags := []LeafFlag{aitableCommentBaseIDFlag(), aitableCommentTableIDFlag(), aitableCommentRecordIDFlag()}
	if commentKeyProperty != "" {
		flags = append(flags, aitableCommentTopicIDFlag(), aitableCommentKeyFlag(commentKeyProperty))
	}
	flags = append(flags,
		LeafFlag{Name: "content", Bind: "richContent", OmitEmpty: true, Transform: aitableCommentTextContent, Usage: "纯文本评论正文；与 --rich-content 二选一"},
		LeafFlag{Name: "rich-content", Bind: "richContent", Trim: true, OmitEmpty: true, Transform: parseAitableCommentRichContent, Usage: "text/mention/image 有序节点 JSON 数组；与 --content 二选一", SchemaDescription: "有序富文本节点 JSON 数组；text、mention、image 可混排"},
	)
	return flags
}

func aitableCommentContentConstraint() []LeafConstraint {
	return []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"content", "rich-content"}, Description: "--content 与 --rich-content 必须且只能提供一个"}}
}

func aitableCommentCreateSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"}
}

func aitableCommentItemResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: aitableCommentItemResultSchema}
}

func aitableCommentWriteParameters(commentKeyProperty string) []contract.ParamDecl {
	parameters := []contract.ParamDecl{
		{Name: "base-id", Property: "baseId", Required: boolPtr(true)},
		{Name: "table-id", Property: "tableId", Required: boolPtr(true)},
		{Name: "record-id", Property: "recordId", Required: boolPtr(true)},
	}
	if commentKeyProperty != "" {
		parameters = append(parameters,
			contract.ParamDecl{Name: "topic-id", Property: "topicId", Required: boolPtr(true)},
			contract.ParamDecl{Name: "comment-key", Property: commentKeyProperty, Required: boolPtr(true)},
		)
	}
	return append(parameters,
		contract.ParamDecl{Name: "content", Property: "richContent", InterfaceType: "array"},
		contract.ParamDecl{Name: "rich-content", Property: "richContent", InterfaceType: "array"},
	)
}

func aitableCommentListParameters() []contract.ParamDecl {
	return []contract.ParamDecl{
		{Name: "base-id", Property: "baseId", Required: boolPtr(true)},
		{Name: "table-id", Property: "tableId", Required: boolPtr(true)},
		{Name: "record-id", Property: "recordId", Required: boolPtr(true)},
		{Name: "limit", Property: "pageSize", InterfaceType: "number"},
		{Name: "cursor", Property: "nextToken"},
	}
}

func callAitableCommentTool(cmd *cobra.Command, tool string, args map[string]any) error {
	return callAitableToolContext(cmd.Context(), tool, args)
}

func validateAitableCommentList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > aitableCommentMaxPageSize {
		return fmt.Errorf("--limit 必须在 1-%d 之间", aitableCommentMaxPageSize)
	}
	return nil
}

func aitableCommentTextContent(raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("--content 不能为空")
	}
	if len(utf16.Encode([]rune(raw))) > aitableCommentMaxTextUnits {
		return nil, fmt.Errorf("--content 文本长度不能超过 %d 个 UTF-16 字符", aitableCommentMaxTextUnits)
	}
	return []any{map[string]any{"type": "text", "text": raw}}, nil
}

func parseAitableCommentRichContent(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var nodes []map[string]any
	if err := decoder.Decode(&nodes); err != nil {
		return nil, fmt.Errorf("--rich-content 必须是有效的 JSON 对象数组: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("--rich-content 只能包含一个 JSON 数组")
	}
	if len(nodes) == 0 || len(nodes) > aitableCommentMaxNodes {
		return nil, fmt.Errorf("--rich-content 节点数必须在 1-%d 之间", aitableCommentMaxNodes)
	}
	textUnits, mentions, images := 0, 0, 0
	hasContent := false
	for index, node := range nodes {
		typeName, ok := node["type"].(string)
		if !ok || strings.TrimSpace(typeName) == "" {
			return nil, fmt.Errorf("--rich-content[%d].type 必须是非空字符串", index)
		}
		switch typeName {
		case "text":
			if err := rejectAitableCommentNodeFields(index, node, "userId", "corpId", "url", "width", "height"); err != nil {
				return nil, err
			}
			text, ok := node["text"].(string)
			if !ok {
				return nil, fmt.Errorf("--rich-content[%d] text 节点必须包含字符串 text", index)
			}
			textUnits += len(utf16.Encode([]rune(text)))
			hasContent = hasContent || strings.TrimSpace(text) != ""
		case "mention":
			if err := rejectAitableCommentNodeFields(index, node, "text", "url", "width", "height"); err != nil {
				return nil, err
			}
			userID, ok := node["userId"].(string)
			if !ok || strings.TrimSpace(userID) == "" {
				return nil, fmt.Errorf("--rich-content[%d] mention 节点必须包含非空外部 userId", index)
			}
			if corpID, exists := node["corpId"]; exists {
				value, valid := corpID.(string)
				if !valid || strings.TrimSpace(value) == "" {
					return nil, fmt.Errorf("--rich-content[%d].corpId 必须是非空字符串", index)
				}
			}
			mentions++
			hasContent = true
		case "image":
			if err := rejectAitableCommentNodeFields(index, node, "text", "userId", "corpId"); err != nil {
				return nil, err
			}
			url, ok := node["url"].(string)
			if !ok || !validAitableCommentImageURL(url) {
				return nil, fmt.Errorf("--rich-content[%d].url 必须匹配 /core/api/resources/<resourceId>/detail", index)
			}
			for _, field := range []string{"width", "height"} {
				if value, exists := node[field]; exists {
					if err := validateAitableCommentImageSize(index, field, value); err != nil {
						return nil, err
					}
				}
			}
			images++
			hasContent = true
		default:
			return nil, fmt.Errorf("--rich-content[%d].type 只允许 text、mention 或 image", index)
		}
	}
	// MCP 允许空 text 节点参与包含 mention/image 的合法正文，但整段正文
	// 不能只由空白 text 构成。这里保持相同的组合语义，避免 CLI 与服务端漂移。
	if !hasContent {
		return nil, fmt.Errorf("--rich-content 必须包含非空文本、mention 或 image")
	}
	if textUnits > aitableCommentMaxTextUnits {
		return nil, fmt.Errorf("--rich-content 文本总长度不能超过 %d 个 UTF-16 字符", aitableCommentMaxTextUnits)
	}
	if mentions > aitableCommentMaxMentions {
		return nil, fmt.Errorf("--rich-content mention 节点不能超过 %d 个", aitableCommentMaxMentions)
	}
	if images > aitableCommentMaxImages {
		return nil, fmt.Errorf("--rich-content image 节点不能超过 %d 个", aitableCommentMaxImages)
	}
	return nodes, nil
}

func rejectAitableCommentNodeFields(index int, node map[string]any, fields ...string) error {
	for _, field := range fields {
		if _, exists := node[field]; exists {
			return fmt.Errorf("--rich-content[%d] %s 节点不能包含 %s", index, node["type"], field)
		}
	}
	return nil
}

func validAitableCommentImageURL(value string) bool {
	const prefix = "/core/api/resources/"
	const suffix = "/detail"
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return false
	}
	resourceID := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
	if resourceID == "" || len(resourceID) > 128 {
		return false
	}
	for _, char := range resourceID {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validateAitableCommentImageSize(index int, field string, value any) error {
	number, ok := value.(json.Number)
	if !ok {
		return fmt.Errorf("--rich-content[%d].%s 必须是 1-%d 的整数", index, field, aitableCommentMaxImageSize)
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 1 || parsed > aitableCommentMaxImageSize {
		return fmt.Errorf("--rich-content[%d].%s 必须是 1-%d 的整数", index, field, aitableCommentMaxImageSize)
	}
	return nil
}
