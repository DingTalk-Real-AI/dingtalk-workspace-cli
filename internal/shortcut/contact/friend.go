// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const (
	friendOperationList          = "contact/get_friend_list"
	friendOperationRequestList   = "contact/get_friend_request_list"
	friendOperationRequestSend   = "contact/send_friend_request"
	friendOperationRequestAccept = "contact/accept_friend_request"
	friendOperationRequestReject = "contact/remove_friend_request"
	friendOperationRemove        = "contact/remove_friend"
)

var friendReadSafety = contract.SafetySpec{
	Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
}

var friendWriteSafety = contract.SafetySpec{
	Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent",
}

var friendRemoveSafety = contract.SafetySpec{
	Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent",
}

// ListFriends 查询当前用户的好友列表。
var ListFriends = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-list",
	Product:     "contact",
	Description: "查询当前用户的好友列表",
	Intent:      "当你想查看当前登录用户已经添加的好友名单、核对某位联系人是否已经是好友，或需要好友的钉钉号/备注信息时使用；支持分页翻页。",
	Risk:        shortcut.RiskRead,
	Safety:      friendReadSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_list_friends",
			CanonicalPath:  "contact.shortcut_list_friends",
			CLIPath:        "contact +friend-list",
			PrimaryCLIPath: "contact +friend-list",
		},
		Description: "查询当前用户的好友列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询当前用户的好友列表",
			UseWhen:      []string{"当你想查看当前登录用户已经添加的好友名单、核对某位联系人是否已经是好友，或需要好友的钉钉号/备注信息时使用；支持分页翻页。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-list", "dws contact +friend-list --cursor 100 --size 50"},
		},
		Parameters: []contract.ParamDecl{{Name: "cursor"}, {Name: "size"}},
	},
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标；首页不传，翻页时传上一页返回的 cursor", Default: "0"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "每页数量，默认 20", Default: "20"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor"}, Description: "--cursor 必须大于或等于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "--size 必须大于 0 且不超过 100"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("cursor") < 0 {
			return responsecheck.Error(friendOperationList, "invalid_parameter", "--cursor 必须大于或等于 0")
		}
		if rt.Int("size") <= 0 || rt.Int("size") > 100 {
			return responsecheck.Error(friendOperationList, "invalid_parameter", "--size 必须大于 0 且不超过 100")
		}
		return nil
	},
	Tips: []string{
		`dws contact +friend-list`,
		`dws contact +friend-list --cursor 100 --size 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_friend_list", map[string]any{
			"cursor": rt.Int("cursor"),
			"size":   rt.Int("size"),
		})
		if err != nil {
			return err
		}
		friends, cursor, hasMore, err := strictFriendList(data, friendOperationList)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(friends), "cursor": cursor, "hasMore": hasMore, "friends": friends})
	},
}

// ListFriendRequests 查询当前用户收到的好友申请列表。
var ListFriendRequests = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-request-list",
	Product:     "contact",
	Description: "查询当前用户收到的好友申请列表",
	Intent:      "当你想查看当前登录用户收到的好友申请、了解待处理申请数量或核对某位联系人是否已发起申请时使用；支持分页翻页，查询后未读申请会被标记为已读。",
	Risk:        shortcut.RiskRead,
	Safety:      friendReadSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_list_friend_requests",
			CanonicalPath:  "contact.shortcut_list_friend_requests",
			CLIPath:        "contact +friend-request-list",
			PrimaryCLIPath: "contact +friend-request-list",
		},
		Description: "查询当前用户收到的好友申请列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询当前用户收到的好友申请列表",
			UseWhen:      []string{"当你想查看当前登录用户收到的好友申请、了解待处理申请数量或核对某位联系人是否已发起申请时使用；支持分页翻页，查询后未读申请会被标记为已读。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-request-list", "dws contact +friend-request-list --size 50"},
		},
		Parameters: []contract.ParamDecl{{Name: "cursor"}, {Name: "size"}},
	},
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标；首页不传，翻页时传上一页返回的 cursor", Default: "0"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "每页数量，默认 20", Default: "20"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor"}, Description: "--cursor 必须大于或等于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "--size 必须大于 0 且不超过 100"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("cursor") < 0 {
			return responsecheck.Error(friendOperationRequestList, "invalid_parameter", "--cursor 必须大于或等于 0")
		}
		if rt.Int("size") <= 0 || rt.Int("size") > 100 {
			return responsecheck.Error(friendOperationRequestList, "invalid_parameter", "--size 必须大于 0 且不超过 100")
		}
		return nil
	},
	Tips: []string{
		`dws contact +friend-request-list`,
		`dws contact +friend-request-list --size 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		args := map[string]any{}
		if rt.Int("cursor") > 0 {
			args["cursor"] = rt.Int("cursor")
		}
		args["size"] = rt.Int("size")
		data, err := rt.CallMCPData("contact", "get_friend_request_list", args)
		if err != nil {
			return err
		}
		requests, cursor, hasMore, pendingCount, err := strictFriendRequestList(data, friendOperationRequestList)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"count":        len(requests),
			"pendingCount": pendingCount,
			"cursor":       cursor,
			"hasMore":      hasMore,
			"requests":     requests,
		})
	},
}

// SendFriendRequest 向指定用户发起好友申请。
var SendFriendRequest = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-request-send",
	Product:     "contact",
	Description: "向指定用户发起好友申请",
	Intent:      "当你想添加某人为好友、需要向对方发送好友申请时使用；传入对方钉钉号（--to）和可选的验证留言（--remark）。",
	Risk:        shortcut.RiskWrite,
	Safety:      friendWriteSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_send_friend_request",
			CanonicalPath:  "contact.shortcut_send_friend_request",
			CLIPath:        "contact +friend-request-send",
			PrimaryCLIPath: "contact +friend-request-send",
		},
		Description: "向指定用户发起好友申请",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "向指定用户发起好友申请",
			UseWhen:      []string{"当你想添加某人为好友、需要向对方发送好友申请时使用；传入对方钉钉号（--to）和可选的验证留言（--remark）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-request-send --to alice", "dws contact +friend-request-send --to alice --remark \"我是研发部的 Bob\""},
		},
		Parameters: []contract.ParamDecl{{Name: "to"}, {Name: "remark"}},
	},
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "对方钉钉号（dingtalkId）；--to 不能为空白", Required: true},
		{Name: "remark", Type: shortcut.FlagString, Desc: "好友申请验证留言（可选）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"to"}, Description: "--to 必须是非空字符串"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateContactNonBlank(rt, friendOperationRequestSend, "to")
	},
	Tips: []string{
		`dws contact +friend-request-send --to alice`,
		`dws contact +friend-request-send --to alice --remark "我是研发部的 Bob"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		args := map[string]any{"targetDingtalkId": strings.TrimSpace(rt.Str("to"))}
		if rt.Changed("remark") {
			args["remark"] = rt.Str("remark")
		}
		return rt.CallMCP("send_friend_request", args)
	},
}

// AcceptFriendRequest 同意指定用户发来的好友申请。
var AcceptFriendRequest = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-request-accept",
	Product:     "contact",
	Description: "同意指定用户发来的好友申请",
	Intent:      "当你想同意某位联系人发来好友申请、与对方建立好友关系时使用；传入对方钉钉号（--from），可选项为该好友设置备注名（--alias）。",
	Risk:        shortcut.RiskWrite,
	Safety:      friendWriteSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_accept_friend_request",
			CanonicalPath:  "contact.shortcut_accept_friend_request",
			CLIPath:        "contact +friend-request-accept",
			PrimaryCLIPath: "contact +friend-request-accept",
		},
		Description: "同意指定用户发来的好友申请",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "同意指定用户发来的好友申请",
			UseWhen:      []string{"当你想同意某位联系人发来好友申请、与对方建立好友关系时使用；传入对方钉钉号（--from），可选项为该好友设置备注名（--alias）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-request-accept --from alice", "dws contact +friend-request-accept --from alice --alias \"Alice Li\""},
		},
		Parameters: []contract.ParamDecl{{Name: "from"}, {Name: "alias"}},
	},
	Flags: []shortcut.Flag{
		{Name: "from", Type: shortcut.FlagString, Desc: "对方钉钉号（dingtalkId）；--from 不能为空白", Required: true},
		{Name: "alias", Type: shortcut.FlagString, Desc: "同意后为该好友设置的备注名（可选）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"from"}, Description: "--from 必须是非空字符串"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateContactNonBlank(rt, friendOperationRequestAccept, "from")
	},
	Tips: []string{
		`dws contact +friend-request-accept --from alice`,
		`dws contact +friend-request-accept --from alice --alias "Alice Li"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		args := map[string]any{"targetDingtalkId": strings.TrimSpace(rt.Str("from"))}
		if rt.Changed("alias") {
			args["alias"] = rt.Str("alias")
		}
		return rt.CallMCP("accept_friend_request", args)
	},
}

// RejectFriendRequest 删除（忽略）指定用户发来的好友申请。
var RejectFriendRequest = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-request-reject",
	Product:     "contact",
	Description: "删除（忽略）指定用户发来的好友申请",
	Intent:      "当你想忽略某位联系人发来的好友申请、让该申请不再出现在好友请求列表中时使用；传入对方钉钉号（--from）。",
	Risk:        shortcut.RiskWrite,
	Safety:      friendWriteSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_reject_friend_request",
			CanonicalPath:  "contact.shortcut_reject_friend_request",
			CLIPath:        "contact +friend-request-reject",
			PrimaryCLIPath: "contact +friend-request-reject",
		},
		Description: "删除（忽略）指定用户发来的好友申请",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "删除（忽略）指定用户发来的好友申请",
			UseWhen:      []string{"当你想忽略某位联系人发来的好友申请、让该申请不再出现在好友请求列表中时使用；传入对方钉钉号（--from）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-request-reject --from alice"},
		},
		Parameters: []contract.ParamDecl{{Name: "from"}},
	},
	Flags: []shortcut.Flag{
		{Name: "from", Type: shortcut.FlagString, Desc: "对方钉钉号（dingtalkId）；--from 不能为空白", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"from"}, Description: "--from 必须是非空字符串"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateContactNonBlank(rt, friendOperationRequestReject, "from")
	},
	Tips: []string{
		`dws contact +friend-request-reject --from alice`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_friend_request", map[string]any{
			"targetDingtalkId": strings.TrimSpace(rt.Str("from")),
		})
	},
}

// RemoveFriend 删除指定好友。
var RemoveFriend = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+friend-remove",
	Product:     "contact",
	Description: "删除指定好友",
	Intent:      "当你想解除与某位联系人的好友关系时使用；传入对方钉钉号（--friend）。删除后双方不再是好友，如需恢复需重新发起好友申请，请谨慎操作。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      friendRemoveSafety,
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_remove_friend",
			CanonicalPath:  "contact.shortcut_remove_friend",
			CLIPath:        "contact +friend-remove",
			PrimaryCLIPath: "contact +friend-remove",
		},
		Description: "删除指定好友",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       contactCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "删除指定好友",
			UseWhen:      []string{"当你想解除与某位联系人的好友关系时使用；传入对方钉钉号（--friend）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +friend-remove --friend alice"},
		},
		Parameters: []contract.ParamDecl{{Name: "friend"}},
	},
	Flags: []shortcut.Flag{
		{Name: "friend", Type: shortcut.FlagString, Desc: "要删除的好友钉钉号（dingtalkId）；--friend 不能为空白", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"friend"}, Description: "--friend 必须是非空字符串"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateContactNonBlank(rt, friendOperationRemove, "friend")
	},
	Tips: []string{
		`dws contact +friend-remove --friend alice`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_friend", map[string]any{
			"targetDingtalkId": strings.TrimSpace(rt.Str("friend")),
		})
	},
}

func init() {
	finalizeContactShortcut(&ListFriends, friendCollectionResult("friends", "好友列表项"), true)
	finalizeContactShortcut(&ListFriendRequests, friendRequestCollectionResult(), true)
	finalizeContactShortcut(&SendFriendRequest, contactObjectResult(SendFriendRequest.Description), true)
	finalizeContactShortcut(&AcceptFriendRequest, contactObjectResult(AcceptFriendRequest.Description), true)
	finalizeContactShortcut(&RejectFriendRequest, contactObjectResult(RejectFriendRequest.Description), true)
	finalizeContactShortcut(&RemoveFriend, contactObjectResult(RemoveFriend.Description), true)
	SendFriendRequest.OutputRollout = output.RolloutUnifiedActive
	AcceptFriendRequest.OutputRollout = output.RolloutUnifiedActive
	RejectFriendRequest.OutputRollout = output.RolloutUnifiedActive
	RemoveFriend.OutputRollout = output.RolloutUnifiedActive
	shortcut.Register(
		ListFriends,
		ListFriendRequests,
		SendFriendRequest,
		AcceptFriendRequest,
		RejectFriendRequest,
		RemoveFriend,
	)
}

func friendCollectionResult(collection, description string) *contract.ResultSpec {
	itemSchema := `{"type":"object","description":"好友列表项","properties":{"dingtalkId":{"type":"string","minLength":1,"description":"好友钉钉号"},"alias":{"type":"string","description":"好友昵称"},"remark":{"type":"string","description":"好友备注"},"status":{"type":"number","description":"好友状态"},"gmtCreate":{"type":"number","description":"加好友时间（毫秒时间戳）"}},"required":["dingtalkId"],"additionalProperties":false}`
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中通过严格校验的项目数量"},"cursor":{"type":"number","description":"分页游标"},"hasMore":{"type":"boolean","description":"是否还有更多数据"},%q:{"type":"array","description":%q,"items":%s}},"required":["count","cursor","hasMore",%q],"additionalProperties":false}`,
			description, collection, description, itemSchema, collection,
		)),
		SensitivePaths: []string{"friends.dingtalkId", "friends.alias", "friends.remark"},
	}
}

func friendRequestCollectionResult() *contract.ResultSpec {
	itemSchema := `{"type":"object","description":"好友申请列表项","properties":{"dingtalkId":{"type":"string","minLength":1,"description":"申请人钉钉号"},"status":{"type":"number","description":"申请状态"},"remark":{"type":"string","description":"申请留言"},"modifyAt":{"type":"number","description":"申请时间（毫秒时间戳）"},"isRead":{"type":"boolean","description":"是否已读"}},"required":["dingtalkId"],"additionalProperties":false}`
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"好友申请列表","properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中通过严格校验的项目数量"},"pendingCount":{"type":"number","description":"待处理申请数量"},"cursor":{"type":"number","description":"分页游标"},"hasMore":{"type":"boolean","description":"是否还有更多数据"},"requests":{"type":"array","description":"好友申请列表","items":%s}},"required":["count","pendingCount","cursor","hasMore","requests"],"additionalProperties":false}`,
			itemSchema,
		)),
		SensitivePaths: []string{"requests.dingtalkId", "requests.remark"},
	}
}

func strictFriendList(data map[string]any, operation string) ([]map[string]any, int64, bool, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, 0, false, err
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok || result == nil {
		return nil, 0, false, responsecheck.Error(operation, "malformed_result", "响应 result 应为对象")
	}
	cursor, _ := contactInt64(result["cursor"])
	hasMore := false
	if v, ok := result["hasMore"].(bool); ok {
		hasMore = v
	}
	raw, present := result["friendList"]
	if !present {
		return []map[string]any{}, cursor, hasMore, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, 0, false, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result.friendList 应为数组，实际为 %T", raw))
	}
	out := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			return nil, 0, false, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("响应 result.friendList[%d] 应为对象，实际为 %T", index, rawItem))
		}
		dingtalkId := contactString(item, "dingtalkId")
		if dingtalkId == "" {
			return nil, 0, false, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("响应 result.friendList[%d] 缺少 dingtalkId", index))
		}
		if seen[dingtalkId] {
			return nil, 0, false, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("响应包含重复 dingtalkId（索引 %d）", index))
		}
		seen[dingtalkId] = true
		row := map[string]any{"dingtalkId": dingtalkId}
		if v, _, valid := contactOptionalString(item, "alias"); valid && v != "" {
			row["alias"] = v
		}
		if v, _, valid := contactOptionalString(item, "remark"); valid && v != "" {
			row["remark"] = v
		}
		if v, ok := contactInt64(item["status"]); ok {
			row["status"] = v
		}
		if v, ok := contactInt64(item["gmtCreate"]); ok {
			row["gmtCreate"] = v
		}
		out = append(out, row)
	}
	return out, cursor, hasMore, nil
}

func strictFriendRequestList(data map[string]any, operation string) ([]map[string]any, int64, bool, int64, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, 0, false, 0, err
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok || result == nil {
		return nil, 0, false, 0, responsecheck.Error(operation, "malformed_result", "响应 result 应为对象")
	}
	cursor, _ := contactInt64(result["cursor"])
	hasMore := false
	if v, ok := result["hasMore"].(bool); ok {
		hasMore = v
	}
	pendingCount, _ := contactInt64(result["pendingCount"])
	raw, present := result["friendList"]
	if !present {
		return []map[string]any{}, cursor, hasMore, pendingCount, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, 0, false, 0, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result.friendList 应为数组，实际为 %T", raw))
	}
	out := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			return nil, 0, false, 0, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("响应 result.friendList[%d] 应为对象，实际为 %T", index, rawItem))
		}
		dingtalkId := contactString(item, "dingtalkId")
		if dingtalkId == "" {
			return nil, 0, false, 0, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("响应 result.friendList[%d] 缺少 dingtalkId", index))
		}
		if seen[dingtalkId] {
			return nil, 0, false, 0, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("响应包含重复 dingtalkId（索引 %d）", index))
		}
		seen[dingtalkId] = true
		row := map[string]any{"dingtalkId": dingtalkId}
		if v, ok := contactInt64(item["status"]); ok {
			row["status"] = v
		}
		if v, _, valid := contactOptionalString(item, "remark"); valid && v != "" {
			row["remark"] = v
		}
		if v, ok := contactInt64(item["modifyAt"]); ok {
			row["modifyAt"] = v
		}
		if v, ok := item["isRead"].(bool); ok {
			row["isRead"] = v
		}
		out = append(out, row)
	}
	return out, cursor, hasMore, pendingCount, nil
}

// parseFriendCursor normalizes an optional integer cursor. A zero or absent
// value is returned as the empty string so the caller can omit the field.
func parseFriendCursor(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("cursor must be non-negative")
	}
	return parsed, nil
}
