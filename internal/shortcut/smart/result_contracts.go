// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func chatMessageLedgerResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"` + description + `",
			"properties":{
				"contractVersion":{"type":"string","description":"消息投影契约版本"},
				"count":{"type":"integer","minimum":0,"description":"当前响应中的消息数量"},
				"messages":{"type":"array","description":"稳定投影后的消息记录","items":{
					"type":"object",
					"description":"消息记录；字段名可直接用于 --jq/--fields，不使用 openMessageId、senderName、createAt 等未声明别名",
					"properties":{
						"messageId":{"description":"稳定消息 ID；对应下层 openMessageId，可用于详情、回复或资源读取"},
						"conversationId":{"description":"消息所属会话的稳定 openConversationId"},
						"conversationTitle":{"type":"string","description":"会话名称；仅下层返回时存在"},
						"threadId":{"description":"话题或 Thread ID；仅话题消息存在"},
						"sender":{"description":"发送者展示值；可能为姓名或下层身份对象"},
						"senderId":{"description":"发送者稳定身份 ID；具体 ID 域以 senderType 和调用结果为准"},
						"senderType":{"description":"发送者类型；用于区分用户、机器人或系统主体"},
						"messageType":{"description":"消息类型，如 text、image、file、audio 或 video"},
						"messageAiSendFlag":{"description":"消息 AI 发送标记；仅下层返回时存在"},
						"text":{"type":["string","null"],"description":"适合直接阅读的消息正文或资源摘要；消息无可读正文时为 null"},
						"time":{"description":"搜索结果兼容时间字段；保留下层可用类型和值"},
						"createTime":{"description":"消息创建时间；保留下层可用类型和值"},
						"updateTime":{"description":"消息更新时间；仅确有更新证据时存在"},
						"reactions":{"type":"array","description":"消息 reactions；仅请求并存在时返回","items":{"description":"reaction 记录"}},
						"quotedMessage":{"type":"object","description":"被引用消息的稳定投影","additionalProperties":true},
						"forwarded":{"type":"array","description":"转发消息的稳定投影","items":{"type":"object","description":"转发子消息","additionalProperties":true}},
						"resourceRefs":{"type":"array","description":"可继续下载的媒体或文件引用","items":{"type":"object","description":"资源引用","properties":{"type":{"type":"string","description":"资源类型 mediaId 或 fileId"},"resourceId":{"description":"稳定资源 ID"},"messageId":{"description":"资源所属消息 ID"},"openConversationId":{"description":"资源所属会话 ID"},"name":{"type":"string","description":"资源名称；仅下层返回时存在"}},"additionalProperties":true}}
					},
					"additionalProperties":true
				}},
				"pagesFetched":{"type":"integer","minimum":0,"description":"已成功读取的分页数量"},
				"enrichedCount":{"type":"integer","minimum":0,"description":"通过消息详情接口成功富化的消息数量"},
				"complete":{"type":"boolean","description":"是否有证据证明请求范围已完整读取"},
				"hasMore":{"type":"boolean","description":"服务端是否仍有后续消息"},
				"nextPage":{"type":"object","description":"会话消息可安全续读时的时间和方向参数","properties":{"time":{"description":"下一页时间边界"},"direction":{"type":"string","description":"下一页读取方向 newer 或 older"},"nextCursor":{"description":"下层原始分页游标"}},"additionalProperties":false},
				"nextCursor":{"type":"string","description":"搜索消息可安全续页时的游标"},
				"paginationKnown":{"type":"boolean","description":"分页状态是否可验证"},
				"stopReason":{"type":"string","description":"读取停止原因"},
				"truncated":{"type":"boolean","description":"结果是否因本地边界被截断"},
				"truncatedByPageLimit":{"type":"boolean","description":"是否因达到本地页数上限而截断"},
				"truncatedByResultLimit":{"type":"boolean","description":"是否因达到本地结果条数上限而截断"},
				"failedCount":{"type":"integer","minimum":0,"description":"分页、富化、过滤或资源处理失败项数量"},
				"failures":{"type":"array","description":"未丢失的逐项失败账本","items":{"type":"object","description":"失败项","additionalProperties":true}},
				"warningCount":{"type":"integer","minimum":0,"description":"不阻断交付、但限制结论范围的警告数量"},
				"warnings":{"type":"array","description":"不应升级为任务失败的警告账本，例如 identity_unverified","items":{"type":"object","description":"警告项","additionalProperties":true}},
				"partial":{"type":"boolean","description":"是否同时包含已取得数据和失败项"},
				"queryRange":{"type":"object","description":"实际执行的查询范围","properties":{"start":{"description":"跨会话原子查询的实际开始时间"},"end":{"description":"跨会话原子查询的实际结束时间"},"startTime":{"description":"消息 Shortcut 的实际开始时间"},"endTime":{"description":"消息 Shortcut 的实际结束时间"},"order":{"type":"string","description":"实际排序方向"},"days":{"type":"integer","description":"按默认回溯窗口查询的天数"},"semantics":{"type":"string","description":"时间范围边界语义"}},"additionalProperties":true},
				"timeCoverage":{"type":"object","description":"实际覆盖的时间范围及是否覆盖全部历史","additionalProperties":true},
				"conclusionGuard":{"type":"object","description":"允许从当前完整性证据得出的结论边界","additionalProperties":true},
				"resolvedFilters":{"description":"已解析为稳定 ID 的会话或发送者过滤条件"},
				"senderScope":{"type":"object","description":"搜索发送者过滤的解析与本地范围校验状态","additionalProperties":true},
				"senderFilter":{"type":"object","description":"会话消息读取的可选发送者过滤状态","additionalProperties":true},
				"identityResult":{"type":"object","description":"稳定身份比较与负面结论是否允许的证据","additionalProperties":true},
				"scope":{"type":"object","description":"显式会话范围的服务端校验和本地过滤证据","additionalProperties":true},
				"resourceDownloads":{"type":"object","description":"可选资源下载的成功与逐项失败账本","additionalProperties":true},
				"export":{"type":"object","description":"可选原子 JSON 导出的路径、大小或预演信息","additionalProperties":true},
				"decryptCandidateCount":{"type":"integer","minimum":0,"description":"需要解密处理的候选消息数量"},
				"decryptedCount":{"type":"integer","minimum":0,"description":"成功解密的消息数量"},
				"decryptFailedCount":{"type":"integer","minimum":0,"description":"解密失败的消息数量"},
				"decryptFailures":{"type":"array","description":"逐项解密失败账本","items":{"type":"object","description":"解密失败项","additionalProperties":true}},
				"nextActions":{"type":"array","description":"复用当前参数并应用参数补丁的安全续读动作","items":{"type":"object","description":"后续动作","properties":{"cliPath":{"type":"string","description":"可执行 CLI leaf 路径"},"arguments":{"type":"object","description":"续读参数补丁","additionalProperties":true},"reuseArguments":{"type":"boolean","description":"是否必须复用原调用其余参数"},"requiredArguments":{"type":"array","description":"仍需从原调用复用的参数说明","items":{"type":"string","description":"必需参数说明"}},"ready":{"type":"boolean","description":"动作是否已具备全部参数可直接执行"},"when":{"type":"string","description":"执行该动作的条件"}},"required":["cliPath","arguments","ready"],"additionalProperties":false}}
			},
			"required":["count","messages","complete","hasMore","failures","nextActions"],
			"additionalProperties":true
		}`),
		SensitivePaths: []string{"messages.senderId", "messages.senderOpenDingTalkId", "messages.text", "messages.resourceRefs"},
	}
}

func attachChatMessageContinuation(payload map[string]any, cliPath string) {
	if payload == nil {
		return
	}
	actions := make([]map[string]any, 0, 1)
	hasMore, _ := payload["hasMore"].(bool)
	if hasMore {
		patch := map[string]any{}
		switch cliPath {
		case "chat +chat-messages":
			if nextPage, ok := payload["nextPage"].(map[string]any); ok {
				boundary, boundaryKnown := nextPage["time"]
				queryRange, rangeMode := payload["queryRange"].(map[string]any)
				if rangeMode && boundaryKnown {
					order, _ := queryRange["order"].(string)
					if order == "asc" {
						patch["start"] = boundary
						if end, present := queryRange["endTime"]; present {
							patch["end"] = end
						}
					} else {
						if start, present := queryRange["startTime"]; present {
							patch["start"] = start
						}
						patch["end"] = boundary
						order = "desc"
					}
					patch["order"] = order
				} else {
					if boundaryKnown {
						patch["time"] = boundary
					}
					if value, present := nextPage["direction"]; present {
						patch["direction"] = value
					}
				}
			}
		case "chat +search-msg":
			if cursor, ok := payload["nextCursor"].(string); ok && cursor != "" {
				patch["cursor"] = cursor
			}
		}
		if len(patch) > 0 {
			required := []string{"复用原调用的目标和过滤参数"}
			if cliPath == "chat +chat-messages" {
				required = []string{"复用原调用的会话目标、页大小和分页边界设置"}
			}
			actions = append(actions, map[string]any{
				"cliPath":           cliPath,
				"arguments":         patch,
				"reuseArguments":    true,
				"requiredArguments": required,
				"ready":             false,
				"when":              "沿已验证的分页位置继续读取，不重新发现命令或目标",
			})
		}
	}
	payload["nextActions"] = actions
}
