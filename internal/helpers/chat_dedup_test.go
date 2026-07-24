package helpers

import (
	"encoding/json"
	"testing"
)

// messageIDsFrom extracts the openMessageId list from result.messages of a
// chat message-list response, for assertion convenience.
func messageIDsFrom(t *testing.T, raw string) []string {
	t.Helper()
	var doc struct {
		Result struct {
			Messages []struct {
				OpenMessageID string `json:"openMessageId"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid json %q: %v", raw, err)
	}
	ids := make([]string, 0, len(doc.Result.Messages))
	for _, m := range doc.Result.Messages {
		ids = append(ids, m.OpenMessageID)
	}
	return ids
}

// 复现 #430：上游 newer/forward 返回 createTime >= 锚点，调用方把边界消息的
// createTime 作为下一页 --time 传入时，该边界消息会被原样重复返回。去重应丢弃它。
func TestDedupChatMessageBoundary_ForwardNewerDropsAnchorMessage(t *testing.T) {
	raw := `{"result":{"messages":[` +
		`{"openMessageId":"m1","createTime":"2026-05-22 09:30:00"},` +
		`{"openMessageId":"m2","createTime":"2026-05-22 09:26:59"},` +
		`{"openMessageId":"m3","createTime":"2026-05-22 09:26:59"}` + // 与锚点相等的重复边界
		`],"hasMore":true}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")
	if ids := messageIDsFrom(t, got); len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("expected only [m1] to remain, got %v", ids)
	}
}

// older 方向同样存在边界重复（<= 锚点），去重方向无关，只看 createTime 是否等于锚点。
func TestDedupChatMessageBoundary_OlderDirectionDropsAnchorMessage(t *testing.T) {
	raw := `{"result":{"messages":[` +
		`{"openMessageId":"m1","createTime":"2026-05-22 09:26:59"},` + // 边界，等于锚点
		`{"openMessageId":"m2","createTime":"2026-05-22 08:00:00"}` +
		`]}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")
	if ids := messageIDsFrom(t, got); len(ids) != 1 || ids[0] != "m2" {
		t.Fatalf("expected only [m2] to remain, got %v", ids)
	}
}

// 没有命中锚点的消息时，必须原样返回（字节不变），保证非翻页场景零副作用。
func TestDedupChatMessageBoundary_NoMatchReturnsOriginalVerbatim(t *testing.T) {
	raw := `{"result":{"messages":[{"openMessageId":"m1","createTime":"2026-05-22 09:30:00"}]}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")
	if got != raw {
		t.Fatalf("expected verbatim original, got %q", got)
	}
}

func TestDedupChatMessageBoundary_EmptyAnchorIsNoOp(t *testing.T) {
	raw := `{"result":{"messages":[{"openMessageId":"m1","createTime":""}]}}`
	if got := dedupChatMessageBoundary(raw, ""); got != raw {
		t.Fatalf("empty anchor must return original verbatim, got %q", got)
	}
}

func TestDedupChatMessageBoundary_EmptyMessagesIsNoOp(t *testing.T) {
	raw := `{"result":{"messages":[]}}`
	if got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59"); got != raw {
		t.Fatalf("empty messages must return original verbatim, got %q", got)
	}
}

func TestDedupChatMessageBoundary_MissingResultIsNoOp(t *testing.T) {
	raw := `{"success":true,"data":[1,2,3]}`
	if got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59"); got != raw {
		t.Fatalf("missing result must return original verbatim, got %q", got)
	}
}

func TestDedupChatMessageBoundary_InvalidJSONIsNoOp(t *testing.T) {
	raw := `not-json-at-all`
	if got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59"); got != raw {
		t.Fatalf("invalid json must return original verbatim, got %q", got)
	}
}

// 关键安全保证：只去重顶层 result.messages，绝不递归进嵌套的 forwardMessages，
// 即便其 createTime 恰好等于锚点（转发消息体不是翻页边界）。
func TestDedupChatMessageBoundary_DoesNotTouchNestedForwardMessages(t *testing.T) {
	raw := `{"result":{"messages":[` +
		// m1 自身 createTime 等于锚点 → 应被丢弃；其 forwardMessages 随之消失属正常
		`{"openMessageId":"m1","createTime":"2026-05-22 09:26:59","forwardMessages":[` +
		`{"openMessageId":"f1","createTime":"2026-05-22 09:26:59"}]},` +
		// m2 自身不等于锚点 → 保留；其内部 createTime==锚点 的转发消息必须原样保留
		`{"openMessageId":"m2","createTime":"2026-05-22 08:00:00","forwardMessages":[` +
		`{"openMessageId":"f2","createTime":"2026-05-22 09:26:59"}]}` +
		`]}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")

	var doc struct {
		Result struct {
			Messages []struct {
				OpenMessageID   string `json:"openMessageId"`
				ForwardMessages []struct {
					OpenMessageID string `json:"openMessageId"`
					CreateTime    string `json:"createTime"`
				} `json:"forwardMessages"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	if len(doc.Result.Messages) != 1 || doc.Result.Messages[0].OpenMessageID != "m2" {
		t.Fatalf("expected only m2 to remain at top level, got %+v", doc.Result.Messages)
	}
	fwd := doc.Result.Messages[0].ForwardMessages
	if len(fwd) != 1 || fwd[0].OpenMessageID != "f2" || fwd[0].CreateTime != "2026-05-22 09:26:59" {
		t.Fatalf("nested forwardMessage f2 must be preserved unchanged, got %+v", fwd)
	}
}

// 锚点周围的空白不影响精确匹配（调用方可能传入带空格的时间串）。
func TestDedupChatMessageBoundary_TrimsWhitespaceAroundAnchor(t *testing.T) {
	raw := `{"result":{"messages":[{"openMessageId":"m1","createTime":"2026-05-22 09:26:59"}]}}`
	got := dedupChatMessageBoundary(raw, "  2026-05-22 09:26:59  ")
	if ids := messageIDsFrom(t, got); len(ids) != 0 {
		t.Fatalf("expected m1 dropped after trimming anchor, got %v", ids)
	}
}

// 非 string 的 createTime（理论上不会出现，但防御）不应 panic，且该元素被原样保留。
func TestDedupChatMessageBoundary_NonStringCreateTimeKeepsElement(t *testing.T) {
	raw := `{"result":{"messages":[{"openMessageId":"m1","createTime":1779000000000}]}}`
	got := dedupChatMessageBoundary(raw, "1779000000000")
	if ids := messageIDsFrom(t, got); len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("non-string createTime must be left intact, got %v", ids)
	}
}

// result.messages 不是数组（畸形响应）时，原样返回，不 panic。
func TestDedupChatMessageBoundary_MessagesNotArrayIsNoOp(t *testing.T) {
	raw := `{"result":{"messages":"not-an-array"}}`
	if got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59"); got != raw {
		t.Fatalf("non-array messages must return original verbatim, got %q", got)
	}
}

// messages 数组中混入非对象元素（防御）时，该元素原样保留，命中锚点的对象元素仍被去重。
func TestDedupChatMessageBoundary_NonObjectElementKeptAndOthersDeduped(t *testing.T) {
	raw := `{"result":{"messages":["str",{"openMessageId":"m1","createTime":"2026-05-22 09:26:59"}]}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")
	var doc struct {
		Result struct {
			Messages []any `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(doc.Result.Messages) != 1 || doc.Result.Messages[0] != "str" {
		t.Fatalf("expected only the non-object element \"str\" to remain, got %v", doc.Result.Messages)
	}
}

// 去重后其它字段（hasMore、result 外层字段）保持不变。
func TestDedupChatMessageBoundary_PreservesOtherFields(t *testing.T) {
	raw := `{"requestId":"abc","result":{"messages":[{"openMessageId":"m1","createTime":"2026-05-22 09:26:59"}],"hasMore":true}}`
	got := dedupChatMessageBoundary(raw, "2026-05-22 09:26:59")
	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc["requestId"] != "abc" {
		t.Fatalf("requestId not preserved: %v", doc["requestId"])
	}
	result, _ := doc["result"].(map[string]any)
	if result["hasMore"] != true {
		t.Fatalf("hasMore not preserved: %v", result["hasMore"])
	}
	if msgs, _ := result["messages"].([]any); len(msgs) != 0 {
		t.Fatalf("expected empty messages after dedup, got %v", msgs)
	}
}

// 端到端：callMCPToolDedupBoundary 经由 callMCPToolInternalOptsPost 的 post 钩子，
// 在 --format json 输出中真正去掉了边界消息（fake caller 模拟服务端响应）。
func TestCallMCPToolDedupBoundary_AppliesPostProcessInJSONOutput(t *testing.T) {
	caller := &helpersCoreCaller{
		format: "json",
		result: textToolResult(`{"result":{"messages":[` +
			`{"openMessageId":"m1","createTime":"2026-05-22 09:30:00"},` +
			`{"openMessageId":"m2","createTime":"2026-05-22 09:26:59"}],"hasMore":true}}`),
	}
	out, _ := installHelpersCoreDeps(t, caller)

	if err := callMCPToolDedupBoundary("list_conversation_message_v2", map[string]any{"time": "2026-05-22 09:26:59"}, "2026-05-22 09:26:59"); err != nil {
		t.Fatalf("callMCPToolDedupBoundary returned error: %v", err)
	}
	if ids := messageIDsFrom(t, out.String()); len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("expected boundary message m2 removed from output, got %v", ids)
	}
	var doc struct {
		Result struct {
			HasMore bool `json:"hasMore"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	if !doc.Result.HasMore {
		t.Fatalf("hasMore should be preserved in output: %s", out.String())
	}
}

// 端到端：没有命中时，输出与普通 callMCPTool 完全一致（零回归）。
func TestCallMCPToolDedupBoundary_NoOpMatchesPlainCallMCPTool(t *testing.T) {
	resp := `{"result":{"messages":[{"openMessageId":"m1","createTime":"2026-05-22 09:30:00"}],"hasMore":false}}`

	callerA := &helpersCoreCaller{format: "json", result: textToolResult(resp)}
	outA, _ := installHelpersCoreDeps(t, callerA)
	if err := callMCPTool("list_conversation_message_v2", nil); err != nil {
		t.Fatalf("callMCPTool error: %v", err)
	}
	plain := outA.String()

	callerB := &helpersCoreCaller{format: "json", result: textToolResult(resp)}
	outB, _ := installHelpersCoreDeps(t, callerB)
	if err := callMCPToolDedupBoundary("list_conversation_message_v2", nil, "2026-05-22 09:26:59"); err != nil {
		t.Fatalf("callMCPToolDedupBoundary error: %v", err)
	}
	if outB.String() != plain {
		t.Fatalf("no-match dedup output differs from plain callMCPTool:\nplain: %s\ndedup: %s", plain, outB.String())
	}
}
