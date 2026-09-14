// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// chatMessageRangeAllResult documents the stable projected fields returned by
// chat message list-all. The command intentionally reuses the existing
// search_messages_by_time_range runtime; this declaration makes that one-call
// cross-conversation route discoverable without requiring Help or guessing raw
// MCP field aliases.
func chatMessageRangeAllResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"跨全部可见会话的时间范围消息结果；默认单页，--page-all 时附带 paging 完整性账本",
			"properties":{
				"contractVersion":{"type":"string","description":"消息投影契约版本"},
				"count":{"type":"integer","minimum":0,"description":"当前结果中的消息总数"},
				"conversationCount":{"type":"integer","minimum":0,"description":"当前结果中的会话数量"},
				"pagesFetched":{"type":"integer","minimum":0,"description":"已读取页数"},
				"paginationKnown":{"type":"boolean","description":"下层分页状态是否可验证"},
				"complete":{"type":"boolean","description":"是否有证据证明请求时间范围已完整读取"},
				"hasMore":{"type":"boolean","description":"停止时服务端是否仍有后续消息"},
				"stopReason":{"type":"string","description":"读取停止原因"},
				"truncated":{"type":"boolean","description":"是否因本地页数或条数边界截断"},
				"failedCount":{"type":"integer","minimum":0,"description":"分页或投影失败项数量"},
				"failures":{"type":"array","description":"逐项失败账本","items":{"type":"object","description":"失败项","properties":{"page":{"description":"失败页序号"},"cursor":{"description":"失败游标"},"stage":{"type":"string","description":"失败阶段"},"error":{"description":"失败信息"}},"additionalProperties":true}},
				"partial":{"type":"boolean","description":"是否同时包含已取得数据和失败项"},
				"queryRange":{"type":"object","description":"实际执行的时间范围","properties":{"start":{"description":"实际开始时间"},"end":{"description":"实际结束时间"}},"required":["start","end"],"additionalProperties":false},
				"nextActions":{"type":"array","description":"安全续读动作；必须复用原 start/end/limit","items":{"type":"object","description":"后续动作","properties":{"cliPath":{"type":"string","description":"可执行 CLI leaf 路径"},"arguments":{"type":"object","description":"续读参数补丁","additionalProperties":true},"reuseArguments":{"type":"boolean","description":"是否必须复用原调用参数"},"ready":{"type":"boolean","description":"动作是否已包含全部参数"},"when":{"type":"string","description":"执行条件"}},"required":["cliPath","arguments","ready"],"additionalProperties":false}},
				"result":{"type":"object","description":"下层时间范围查询结果","properties":{
					"conversationMessagesList":{"type":"array","description":"按会话分组的消息","items":{"type":"object","description":"一个会话及其消息","properties":{
						"openConversationId":{"description":"稳定会话 ID"},
						"title":{"type":"string","description":"会话名称；仅下层返回时存在"},
						"singleChat":{"type":"boolean","description":"是否为单聊；仅下层返回时存在"},
						"messages":{"type":"array","description":"当前会话内的消息记录","items":{"type":"object","description":"消息记录；稳定字段可直接用于 --jq/--fields","properties":{
							"messageId":{"description":"稳定消息 ID；对应下层 openMessageId"},
							"openMessageId":{"description":"兼容保留的下层消息 ID"},
							"conversationId":{"description":"消息所属会话 ID"},
							"openConversationId":{"description":"兼容保留的下层会话 ID"},
							"conversationTitle":{"type":"string","description":"会话名称"},
							"sender":{"description":"发送者展示值"},
							"senderId":{"description":"发送者稳定身份 ID"},
							"senderType":{"description":"发送者类型"},
							"messageType":{"description":"消息类型"},
							"text":{"type":["string","null"],"description":"适合直接阅读的正文或资源摘要；消息无可读正文时为 null"},
							"content":{"description":"兼容保留的下层消息内容"},
							"createTime":{"description":"消息创建时间"},
							"updateTime":{"description":"消息更新时间；仅确有更新证据时存在"},
							"resourceRefs":{"type":"array","description":"可继续下载的资源引用","items":{"type":"object","description":"资源引用","additionalProperties":true}}
						},"additionalProperties":true}}
					},"required":["openConversationId","messages"],"additionalProperties":true}},
					"hasMore":{"type":"boolean","description":"服务端是否还有后续消息"},
					"nextCursor":{"description":"hasMore=true 时用于续页的游标"}
				},"required":["conversationMessagesList","hasMore"],"additionalProperties":true},
				"paging":{"type":"object","description":"--page-all 的完整性账本","properties":{
					"truncated":{"type":"boolean","description":"是否因页数或条数边界截断"},
					"hasMore":{"type":"boolean","description":"停止时服务端是否仍有后续页"},
					"lastCursor":{"description":"安全续页游标；页内截断时不可依赖"},
					"pages":{"type":"integer","minimum":0,"description":"已读取页数"},
					"total":{"type":"integer","minimum":0,"description":"已保留消息总数"},
					"partial":{"type":"boolean","description":"是否在取得部分数据后失败"},
					"failedPage":{"type":"integer","description":"失败页序号"},
					"failedCursor":{"description":"失败页游标"},
					"pagesFetched":{"type":"integer","minimum":0,"description":"失败前已成功读取的页数"},
					"itemsFetched":{"type":"integer","minimum":0,"description":"失败前已保留的消息数量"},
					"error":{"type":"string","description":"分页失败信息"},
					"truncatedWithinPage":{"type":"boolean","description":"是否在下层单页内部达到条数上限并丢弃页内后缀"},
					"resumeCursorReliable":{"type":"boolean","description":"lastCursor 是否可安全续读"}
				},"additionalProperties":true}
			},
			"required":["result","count","conversationCount","complete","hasMore","stopReason","failures","queryRange","nextActions"],
			"additionalProperties":true
		}`),
		SensitivePaths: []string{
			"result.conversationMessagesList.messages.senderId",
			"result.conversationMessagesList.messages.text",
			"result.conversationMessagesList.messages.content",
			"result.conversationMessagesList.messages.resourceRefs",
		},
	}
}
