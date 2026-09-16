// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func conversationDiscoveryResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"会话发现结果、完整性账本以及可继续执行的消息读取动作",
			"properties":{
				"count":{"type":"integer","minimum":0,"description":"当前结果中的去重会话数量"},
				"conversations":{"type":"array","description":"发现的会话；字段名可直接用于 --jq/--fields","items":{"type":"object","description":"会话摘要","properties":{"openConversationId":{"description":"后续消息读取使用的稳定会话 ID"},"conversationName":{"type":"string","description":"会话名称；仅下层返回时存在"},"conversationType":{"description":"会话类型；仅下层返回时存在"}},"required":["openConversationId"],"additionalProperties":true}},
				"pagesFetched":{"type":"integer","minimum":0,"description":"已成功读取的分页数量"},
				"complete":{"type":"boolean","description":"是否有证据证明请求范围已完整读取"},
				"hasMore":{"type":"boolean","description":"服务端是否仍有后续会话"},
				"nextCursor":{"type":"integer","description":"可安全续页时使用的整数游标"},
				"paginationKnown":{"type":"boolean","description":"分页状态是否可验证"},
				"stopReason":{"type":"string","description":"读取停止原因"},
				"truncated":{"type":"boolean","description":"结果是否因本地边界被截断"},
				"truncatedByPageLimit":{"type":"boolean","description":"是否因达到本地页数上限而截断"},
				"truncatedByResultLimit":{"type":"boolean","description":"是否因达到本地结果条数上限而截断"},
				"failedCount":{"type":"integer","minimum":0,"description":"分页或投影失败项数量"},
				"failures":{"type":"array","description":"未丢失的逐项失败账本","items":{"type":"object","description":"失败项","properties":{"page":{"type":"integer","description":"失败分页序号"},"cursor":{"description":"失败分页游标"},"stage":{"type":"string","description":"失败阶段"},"error":{"type":"string","description":"原始失败信息"}},"additionalProperties":true}},
				"partial":{"type":"boolean","description":"是否同时包含已取得数据和失败项"},
				"discoveryOnly":{"type":"boolean","description":"恒为 true；该命令只发现会话，正文、总结或统计仍需读取消息"},
				"nextActions":{"type":"array","description":"基于当前稳定 ID 和分页状态生成的后续 CLI 动作","items":{"type":"object","description":"后续动作","properties":{"cliPath":{"type":"string","description":"可执行 CLI leaf 路径"},"arguments":{"type":"object","description":"已知参数补丁","additionalProperties":true},"requiredArguments":{"type":"array","description":"执行前仍需绑定的参数名","items":{"type":"string","description":"参数名"}},"argumentSource":{"type":"string","description":"参数从当前结果取得的路径"},"ready":{"type":"boolean","description":"动作是否已具备全部参数可直接执行"},"when":{"type":"string","description":"执行该动作的条件"}},"required":["cliPath","arguments","ready"],"additionalProperties":false}}
			},
			"required":["count","conversations","complete","hasMore","stopReason","failures","discoveryOnly","nextActions"],
			"additionalProperties":true
		}`),
	}
}

func messageResourceURLResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"短时有效的消息资源下载凭据；实际下载应优先使用 +messages-resource-download",
			"properties":{
				"resourceUrl":{"type":"string","description":"短时有效且可能包含签名参数的敏感下载地址"},
				"downloadUrl":{"type":"string","description":"resourceUrl 的兼容字段，同样按敏感临时凭据处理"},
				"headers":{"type":"object","description":"下载所需的敏感请求头；仅下层返回时存在","additionalProperties":true},
				"expiresAt":{"description":"下载凭据失效时间；仅下层返回时存在"},
				"result":{"type":"object","description":"兼容的下层嵌套结果","additionalProperties":true}
			},
			"additionalProperties":true
		}`),
		SensitivePaths: []string{"resourceUrl", "downloadUrl", "headers", "result.resourceUrl", "result.downloadUrl", "result.headers"},
	}
}

func conversationDiscoveryNextActions(
	conversations []map[string]any,
	hasMore bool,
	nextCursor int64,
	unsafeContinuation bool,
	limit int,
	excludeMuted bool,
) []map[string]any {
	actions := make([]map[string]any, 0, 2)
	if len(conversations) > 0 {
		actions = append(actions, map[string]any{
			"cliPath":           "chat +chat-messages",
			"arguments":         map[string]any{"direction": "older"},
			"requiredArguments": []string{"group"},
			"argumentSource":    "conversations[].openConversationId",
			"ready":             false,
			"when":              "按任务范围读取每个会话的消息正文；会话列表本身不能完成总结或统计",
		})
	}
	if hasMore && nextCursor > 0 && !unsafeContinuation {
		arguments := map[string]any{"cursor": nextCursor, "limit": limit}
		if excludeMuted {
			arguments["exclude-muted"] = true
		}
		actions = append(actions, map[string]any{
			"cliPath":   "chat +conversation-list",
			"arguments": arguments,
			"ready":     true,
			"when":      "继续读取下一页会话",
		})
	}
	return actions
}
