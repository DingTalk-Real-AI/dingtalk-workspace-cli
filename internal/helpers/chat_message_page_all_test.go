package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatMessagePageAllCaller struct {
	steps []scriptedToolStep
	dry   bool
	calls []pagedCommandCall
}

func (c *chatMessagePageAllCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, pagedCommandCall{server: serverID, tool: toolName, args: copied})
	if len(c.steps) == 0 {
		return textToolResult(`{"result":{"messages":[],"hasMore":false}}`), nil
	}
	index := len(c.calls) - 1
	if index >= len(c.steps) {
		index = len(c.steps) - 1
	}
	step := c.steps[index]
	if step.err != nil {
		return nil, step.err
	}
	return textToolResult(step.text), nil
}

func (*chatMessagePageAllCaller) Format() string { return "json" }
func (c *chatMessagePageAllCaller) DryRun() bool { return c.dry }
func (*chatMessagePageAllCaller) Fields() string { return "" }
func (*chatMessagePageAllCaller) JQ() string     { return "" }

// chatMessagePageAllPoolCaller models a cooperative lower layer: it honours the
// requested per-page limit and serves messages from an ordered pool, emitting a
// fresh nextCursor per page position.
type chatMessagePageAllPoolCaller struct {
	pool  []string
	pos   int
	calls []pagedCommandCall
}

func (c *chatMessagePageAllPoolCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, pagedCommandCall{server: serverID, tool: toolName, args: copied})
	limit, _ := args["limit"].(int)
	if limit <= 0 {
		limit = len(c.pool)
	}
	end := c.pos + limit
	if end > len(c.pool) {
		end = len(c.pool)
	}
	rows := make([]string, 0, end-c.pos)
	for i := c.pos; i < end; i++ {
		rows = append(rows, fmt.Sprintf(`{"openMessageId":%q}`, c.pool[i]))
	}
	c.pos = end
	response := `{"result":{"messages":[` + joinCommas(rows) + `],"hasMore":` + strconv.FormatBool(end < len(c.pool))
	if end < len(c.pool) {
		response += fmt.Sprintf(`,"nextCursor":%d`, 1787000000000+end)
	}
	return textToolResult(response + `}}`), nil
}

func (*chatMessagePageAllPoolCaller) Format() string { return "json" }
func (*chatMessagePageAllPoolCaller) DryRun() bool   { return false }
func (*chatMessagePageAllPoolCaller) Fields() string { return "" }
func (*chatMessagePageAllPoolCaller) JQ() string     { return "" }

func executeChatMessagePageAllCommand(t *testing.T, caller edition.ToolCaller, args ...string) (map[string]any, error) {
	t.Helper()
	return executeChatMessagePageAllCommandRaw(t, caller, context.Background(), &bytes.Buffer{}, args...)
}

func executeChatMessagePageAllCommandRaw(t *testing.T, caller edition.ToolCaller, ctx context.Context, out io.Writer, args ...string) (map[string]any, error) {
	t.Helper()
	oldDeps := deps
	t.Cleanup(func() { deps = oldDeps })
	InitDeps(caller)
	deps.Out.w = out
	deps.Out.errW = io.Discard

	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(out)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	buffer, _ := out.(*bytes.Buffer)
	if buffer == nil || buffer.Len() == 0 {
		return nil, err
	}
	var parsed map[string]any
	if unmarshalErr := json.Unmarshal(buffer.Bytes(), &parsed); unmarshalErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", buffer.String(), unmarshalErr)
	}
	return parsed, err
}

func pageAllMessages(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	rows, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", payload["messages"])
	}
	messages := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		message, ok := row.(map[string]any)
		if !ok {
			t.Fatalf("message row = %#v, want object", row)
		}
		messages = append(messages, message)
	}
	return messages
}

func pageAllMessageIDs(t *testing.T, payload map[string]any) []string {
	t.Helper()
	ids := make([]string, 0)
	for _, message := range pageAllMessages(t, payload) {
		ids = append(ids, fmt.Sprint(message["messageId"]))
	}
	return ids
}

func TestCrossPlatformCoverageChatMessageListPageAllSinglePageUnchanged(t *testing.T) {
	response := `{"result":{"messages":[{"openMessageId":"m1","content":"hello"}],"hasMore":false}}`
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no paging flags",
			args: []string{"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older"},
		},
		{
			name: "paging flags without page-all stay single page",
			args: []string{"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-limit", "20", "--max-items", "5", "--page-delay", "0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: response}}}
			got, err := executeChatMessagePageAllCommand(t, caller, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %d, want exactly one single-page call", len(caller.calls))
			}
			call := caller.calls[0]
			if call.server != "chat" || call.tool != "list_conversation_message_v2" {
				t.Fatalf("call = %s/%s, want chat/list_conversation_message_v2", call.server, call.tool)
			}
			if call.args["openconversation_id"] != "cidAAAAAAAAAA1" || call.args["time"] != "2025-03-01 00:00:00" || call.args["forward"] != false {
				t.Fatalf("args = %#v", call.args)
			}
			if _, exists := call.args["page-all"]; exists {
				t.Fatalf("single-page request leaked paging args: %#v", call.args)
			}
			for _, key := range []string{"stopReason", "pagesFetched", "truncatedByPageLimit", "truncatedByResultLimit", "paging", "complete"} {
				if _, exists := got[key]; exists {
					t.Fatalf("single-page output gained aggregate field %q: %#v", key, got[key])
				}
			}
			ids := pageAllMessageIDs(t, got)
			if len(ids) != 1 || ids[0] != "m1" {
				t.Fatalf("messages = %#v, want [m1]", ids)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllAggregatesAndDedups(t *testing.T) {
	steps := []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"},{"openMessageId":"m2"}],"hasMore":true,"nextCursor":1787000000123}}`},
		{text: `{"result":{"messages":[{"openMessageId":"m2"},{"openMessageId":"m3"}],"hasMore":false}}`},
	}
	caller := &chatMessagePageAllCaller{steps: steps}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want two pages", len(caller.calls))
	}
	first := caller.calls[0]
	if first.server != "chat" || first.tool != "list_conversation_message_v2" {
		t.Fatalf("first call = %s/%s", first.server, first.tool)
	}
	if first.args["openconversation_id"] != "cidAAAAAAAAAA1" || first.args["time"] != "2025-03-01 00:00:00" || first.args["forward"] != false {
		t.Fatalf("first args = %#v", first.args)
	}
	wantBoundary := time.UnixMilli(1787000000123).UTC().Format(time.RFC3339Nano)
	if caller.calls[1].args["time"] != wantBoundary {
		t.Fatalf("second call time = %#v, want %q", caller.calls[1].args["time"], wantBoundary)
	}
	if caller.calls[1].args["openconversation_id"] != "cidAAAAAAAAAA1" || caller.calls[1].args["forward"] != false {
		t.Fatalf("second args = %#v", caller.calls[1].args)
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 3 || ids[0] != "m1" || ids[1] != "m2" || ids[2] != "m3" {
		t.Fatalf("messages = %#v, want [m1 m2 m3] with m2 deduped", ids)
	}
	if got["count"].(float64) != 3 || got["pagesFetched"].(float64) != 2 {
		t.Fatalf("count/pagesFetched = %#v/%#v, want 3/2", got["count"], got["pagesFetched"])
	}
	if got["complete"] != true || got["hasMore"] != false || got["stopReason"] != "source_complete" {
		t.Fatalf("complete/hasMore/stopReason = %#v/%#v/%#v", got["complete"], got["hasMore"], got["stopReason"])
	}
	if got["paginationKnown"] != true {
		t.Fatalf("paginationKnown = %#v, want true on a reliable happy path", got["paginationKnown"])
	}
	if got["truncatedByPageLimit"] != false || got["truncated"] != false || got["failedCount"].(float64) != 0 {
		t.Fatalf("truncation fields = %#v/%#v/%#v", got["truncatedByPageLimit"], got["truncated"], got["failedCount"])
	}
	if _, exists := got["nextPage"]; exists {
		t.Fatalf("nextPage = %#v, want absent when source complete", got["nextPage"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllPageLimitStops(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-limit", "1", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want one page under --page-limit 1", len(caller.calls))
	}
	if got["stopReason"] != "page_limit" || got["truncatedByPageLimit"] != true {
		t.Fatalf("stopReason/truncatedByPageLimit = %#v/%#v", got["stopReason"], got["truncatedByPageLimit"])
	}
	if got["paginationKnown"] != true {
		t.Fatalf("paginationKnown = %#v, want true: page-limit stop is not a pagination failure", got["paginationKnown"])
	}
	if got["complete"] != false || got["hasMore"] != true {
		t.Fatalf("complete/hasMore = %#v/%#v", got["complete"], got["hasMore"])
	}
	nextPage, ok := got["nextPage"].(map[string]any)
	if !ok || nextPage["time"] != time.UnixMilli(1787000000123).UTC().Format(time.RFC3339Nano) {
		t.Fatalf("nextPage = %#v, want boundary time", got["nextPage"])
	}
	if nextPage["direction"] != "older" {
		t.Fatalf("nextPage.direction = %#v, want older", nextPage["direction"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllMaxItemsTruncates(t *testing.T) {
	t.Run("overshoot within one page", func(t *testing.T) {
		// The stub ignores the clamped limit and returns 50 rows in one page,
		// simulating a lower layer that violates the per-page limit contract.
		rows := make([]string, 0, 50)
		for i := 1; i <= 50; i++ {
			rows = append(rows, fmt.Sprintf(`{"openMessageId":"m%d"}`, i))
		}
		response := `{"result":{"messages":[` + joinCommas(rows) + `],"hasMore":false}}`
		caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: response}}}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--max-items", "30", "--page-delay", "0")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %d, want one page", len(caller.calls))
		}
		if caller.calls[0].args["limit"] != 30 {
			t.Fatalf("first call limit = %#v, want clamped to remaining budget 30", caller.calls[0].args["limit"])
		}
		ids := pageAllMessageIDs(t, got)
		if len(ids) != 30 || ids[29] != "m30" {
			t.Fatalf("messages = %d rows, last = %v, want 30 rows ending m30", len(ids), lastOr(ids))
		}
		if got["truncated"] != true || got["truncatedByResultLimit"] != true {
			t.Fatalf("truncated/truncatedByResultLimit = %#v/%#v", got["truncated"], got["truncatedByResultLimit"])
		}
		if got["stopReason"] != "result_limit" {
			t.Fatalf("stopReason = %#v, want result_limit", got["stopReason"])
		}
		if got["hasMore"] != false || got["complete"] != false {
			t.Fatalf("hasMore/complete = %#v/%#v, want false/false: safety net keeps the lower layer's hasMore", got["hasMore"], got["complete"])
		}
		if _, exists := got["nextPage"]; exists {
			t.Fatalf("nextPage = %#v, want absent: a boundary over a truncated tail would skip dropped messages", got["nextPage"])
		}
	})
	t.Run("accumulated across pages stops at limit", func(t *testing.T) {
		page := func(id string) string {
			return fmt.Sprintf(`{"result":{"messages":[{"openMessageId":%q}],"hasMore":true,"nextCursor":%d}}`, id, len(id)+1700000000000)
		}
		caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: page("m1")}, {text: page("m2")}}}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--max-items", "1", "--page-delay", "0")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %d, want sweep to stop after reaching --max-items", len(caller.calls))
		}
		ids := pageAllMessageIDs(t, got)
		if len(ids) != 1 || ids[0] != "m1" {
			t.Fatalf("messages = %#v, want [m1]", ids)
		}
		if got["truncatedByResultLimit"] != true || got["stopReason"] != "result_limit" {
			t.Fatalf("truncatedByResultLimit/stopReason = %#v/%#v", got["truncatedByResultLimit"], got["stopReason"])
		}
		if got["hasMore"] != true || got["complete"] != false {
			t.Fatalf("hasMore/complete = %#v/%#v", got["hasMore"], got["complete"])
		}
	})
}

func TestCrossPlatformCoverageChatMessageListPageAllDirectionNewer(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000100}}`},
		{text: `{"result":{"messages":[{"openMessageId":"m2"}],"hasMore":true,"nextCursor":1787000000200}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "newer", "--page-all", "--page-limit", "2", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want two pages", len(caller.calls))
	}
	if caller.calls[0].args["forward"] != true || caller.calls[1].args["forward"] != true {
		t.Fatalf("forward args = %#v / %#v, want true", caller.calls[0].args["forward"], caller.calls[1].args["forward"])
	}
	if got["stopReason"] != "page_limit" || got["truncatedByPageLimit"] != true {
		t.Fatalf("stopReason/truncatedByPageLimit = %#v/%#v", got["stopReason"], got["truncatedByPageLimit"])
	}
	if got["paginationKnown"] != true {
		t.Fatalf("paginationKnown = %#v, want true: page-limit stop is not a pagination failure", got["paginationKnown"])
	}
	nextPage, ok := got["nextPage"].(map[string]any)
	if !ok {
		t.Fatalf("nextPage = %#v, want present", got["nextPage"])
	}
	if nextPage["direction"] != "newer" {
		t.Fatalf("nextPage.direction = %#v, want newer", nextPage["direction"])
	}
	if nextPage["time"] != time.UnixMilli(1787000000200).UTC().Format(time.RFC3339Nano) {
		t.Fatalf("nextPage.time = %#v, want boundary of the last page cursor", nextPage["time"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllIndividualChatPath(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`},
		{text: `{"result":{"messages":[{"openMessageId":"m2"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--user", "u1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want two pages", len(caller.calls))
	}
	for _, call := range caller.calls {
		if call.server != "chat" || call.tool != "list_individual_chat_message" {
			t.Fatalf("call = %s/%s, want chat/list_individual_chat_message", call.server, call.tool)
		}
		if call.args["userId"] != "u1" {
			t.Fatalf("args = %#v, want userId=u1", call.args)
		}
		if _, exists := call.args["openconversation_id"]; exists {
			t.Fatalf("individual-chat args leaked group id: %#v", call.args)
		}
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("messages = %#v, want [m1 m2]", ids)
	}
	if got["pagesFetched"].(float64) != 2 || got["stopReason"] != "source_complete" || got["complete"] != true {
		t.Fatalf("pagesFetched/stopReason/complete = %#v/%#v/%#v", got["pagesFetched"], got["stopReason"], got["complete"])
	}
	if got["paginationKnown"] != true {
		t.Fatalf("paginationKnown = %#v, want true on a reliable happy path", got["paginationKnown"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllCursorStallStops(t *testing.T) {
	page := `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: page}, {text: page}}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err == nil {
		t.Fatal("expected non-zero exit for cursor stall, got nil")
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want exactly two pages before stall detection (not page-limit iterations)", len(caller.calls))
	}
	if got["stopReason"] != "pagination_error" {
		t.Fatalf("stopReason = %#v, want pagination_error", got["stopReason"])
	}
	if got["paginationKnown"] != false {
		t.Fatalf("paginationKnown = %#v, want false on cursor stall", got["paginationKnown"])
	}
	if got["failedCount"].(float64) != 1 {
		t.Fatalf("failedCount = %#v, want 1", got["failedCount"])
	}
	failures, ok := got["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %#v, want one diagnostic", got["failures"])
	}
	if got["partial"] != true {
		t.Fatalf("partial = %#v, want true", got["partial"])
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("messages = %#v, want deduped [m1] from the repeated page", ids)
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllMidSweepFailurePartial(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`},
		{err: fmt.Errorf("gateway timeout")},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err == nil {
		t.Fatal("expected non-zero exit for mid-sweep failure, got nil")
	}
	if got["stopReason"] != "read_failure" || got["partial"] != true || got["failedCount"].(float64) != 1 {
		t.Fatalf("stopReason/partial/failedCount = %#v/%#v/%#v", got["stopReason"], got["partial"], got["failedCount"])
	}
	if got["pagesFetched"].(float64) != 1 {
		t.Fatalf("pagesFetched = %#v, want 1", got["pagesFetched"])
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("messages = %#v, want page-one rows preserved", ids)
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllDryRun(t *testing.T) {
	caller := &chatMessagePageAllCaller{dry: true}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-limit", "10", "--max-items", "200", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %d, want zero tool calls in dry-run", len(caller.calls))
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", got["dry_run"])
	}
	request, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v, want object", got["request"])
	}
	if request["server"] != "chat" || request["name"] != "list_conversation_message_v2" {
		t.Fatalf("request = %#v", request)
	}
	args, ok := request["args"].(map[string]any)
	if !ok || args["openconversation_id"] != "cidAAAAAAAAAA1" || args["time"] != "2025-03-01 00:00:00" || args["forward"] != false {
		t.Fatalf("request args = %#v", request["args"])
	}
	paging, ok := got["paging"].(map[string]any)
	if !ok || paging["pageAll"] != true || paging["pageLimit"].(float64) != 10 || paging["maxItems"].(float64) != 200 {
		t.Fatalf("paging = %#v, want page-all sweep plan", got["paging"])
	}
}

func pageAllPoolIDs(count int) []string {
	pool := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		pool = append(pool, fmt.Sprintf("m%d", i))
	}
	return pool
}

func TestCrossPlatformCoverageChatMessageListPageAllMaxItemsClampsPerPageLimit(t *testing.T) {
	caller := &chatMessagePageAllPoolCaller{pool: pageAllPoolIDs(50)}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--max-items", "30", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want one clamped page that exactly fills the budget", len(caller.calls))
	}
	if caller.calls[0].args["limit"] != 30 {
		t.Fatalf("first call limit = %#v, want clamped to remaining budget 30", caller.calls[0].args["limit"])
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 30 || ids[0] != "m1" || ids[29] != "m30" {
		t.Fatalf("messages = %d rows [%v..%v], want 30 rows m1..m30", len(ids), firstOr(ids), lastOr(ids))
	}
	if got["hasMore"] != true || got["complete"] != false || got["stopReason"] != "result_limit" {
		t.Fatalf("hasMore/complete/stopReason = %#v/%#v/%#v", got["hasMore"], got["complete"], got["stopReason"])
	}
	if got["truncatedByResultLimit"] != true || got["paginationKnown"] != true {
		t.Fatalf("truncatedByResultLimit/paginationKnown = %#v/%#v", got["truncatedByResultLimit"], got["paginationKnown"])
	}
	nextPage, ok := got["nextPage"].(map[string]any)
	if !ok {
		t.Fatalf("nextPage = %#v, want the exact page-tail boundary when the budget page was fully kept", got["nextPage"])
	}
	wantBoundary := time.UnixMilli(1787000000030).UTC().Format(time.RFC3339Nano)
	if nextPage["time"] != wantBoundary {
		t.Fatalf("nextPage.time = %#v, want %q derived from the page nextCursor", nextPage["time"], wantBoundary)
	}
	if nextPage["direction"] != "older" {
		t.Fatalf("nextPage.direction = %#v, want older", nextPage["direction"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllResumesAtBoundaryWithoutGaps(t *testing.T) {
	first := &chatMessagePageAllPoolCaller{pool: pageAllPoolIDs(50)}
	got, err := executeChatMessagePageAllCommand(t, first,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--max-items", "30", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	firstIDs := pageAllMessageIDs(t, got)
	if len(firstIDs) != 30 || firstIDs[29] != "m30" {
		t.Fatalf("first segment = %d rows ending %v, want 30 rows ending m30", len(firstIDs), lastOr(firstIDs))
	}
	nextPage, ok := got["nextPage"].(map[string]any)
	if !ok {
		t.Fatalf("nextPage = %#v, want boundary to resume from", got["nextPage"])
	}
	boundary, _ := nextPage["time"].(string)
	if boundary == "" {
		t.Fatalf("nextPage.time = %#v, want non-empty boundary", nextPage["time"])
	}

	rows := make([]string, 0, 20)
	for i := 31; i <= 50; i++ {
		rows = append(rows, fmt.Sprintf(`{"openMessageId":"m%d"}`, i))
	}
	resume := `{"result":{"messages":[` + joinCommas(rows) + `],"hasMore":false}}`
	second := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: resume}}}
	got2, err := executeChatMessagePageAllCommand(t, second,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", boundary, "--direction", "older", "--page-all", "--max-items", "30", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.calls) != 1 {
		t.Fatalf("calls = %d, want one resume page", len(second.calls))
	}
	if second.calls[0].args["time"] != boundary {
		t.Fatalf("resume call time = %#v, want the published nextPage boundary %q", second.calls[0].args["time"], boundary)
	}
	secondIDs := pageAllMessageIDs(t, got2)
	if len(secondIDs) != 20 || secondIDs[0] != "m31" || lastOr(secondIDs) != "m50" {
		t.Fatalf("resume segment = %d rows [%v..%v], want 20 rows m31..m50 with no gap after m30", len(secondIDs), firstOr(secondIDs), lastOr(secondIDs))
	}
	if got2["complete"] != true || got2["stopReason"] != "source_complete" || got2["hasMore"] != false {
		t.Fatalf("complete/stopReason/hasMore = %#v/%#v/%#v", got2["complete"], got2["stopReason"], got2["hasMore"])
	}
	if len(firstIDs)+len(secondIDs) != 50 {
		t.Fatalf("segments cover %d rows, want all 50 without skips", len(firstIDs)+len(secondIDs))
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllUserLimitShrinksWithBudget(t *testing.T) {
	caller := &chatMessagePageAllPoolCaller{pool: pageAllPoolIDs(50)}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--limit", "20", "--max-items", "30", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want two shrinking pages", len(caller.calls))
	}
	if caller.calls[0].args["limit"] != 20 {
		t.Fatalf("first call limit = %#v, want user page size 20 while budget is 30", caller.calls[0].args["limit"])
	}
	if caller.calls[1].args["limit"] != 10 {
		t.Fatalf("second call limit = %#v, want clamped to remaining budget 10", caller.calls[1].args["limit"])
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 30 || ids[0] != "m1" || ids[29] != "m30" {
		t.Fatalf("messages = %d rows [%v..%v], want 30 rows m1..m30", len(ids), firstOr(ids), lastOr(ids))
	}
	if got["stopReason"] != "result_limit" || got["truncatedByResultLimit"] != true || got["hasMore"] != true {
		t.Fatalf("stopReason/truncatedByResultLimit/hasMore = %#v/%#v/%#v", got["stopReason"], got["truncatedByResultLimit"], got["hasMore"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllUnreliablePaginationReportsUnknown(t *testing.T) {
	assertUnreliable := func(t *testing.T, got map[string]any, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected non-zero exit for unreliable pagination, got nil")
		}
		if got["paginationKnown"] != false {
			t.Fatalf("paginationKnown = %#v, want false when pagination metadata is unreliable", got["paginationKnown"])
		}
		if got["stopReason"] != "pagination_error" {
			t.Fatalf("stopReason = %#v, want pagination_error", got["stopReason"])
		}
		if got["complete"] != false {
			t.Fatalf("complete = %#v, want false alongside paginationKnown=false", got["complete"])
		}
		if got["failedCount"].(float64) < 1 {
			t.Fatalf("failedCount = %#v, want at least one diagnostic", got["failedCount"])
		}
		failures, ok := got["failures"].([]any)
		if !ok || len(failures) == 0 {
			t.Fatalf("failures = %#v, want non-empty", got["failures"])
		}
	}
	t.Run("missing hasMore", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
			{text: `{"result":{"messages":[{"openMessageId":"m1"}]}}`},
		}}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
		assertUnreliable(t, got, err)
	})
	t.Run("invalid nextCursor", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
			{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":"abc"}}`},
		}}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
		assertUnreliable(t, got, err)
	})
	t.Run("cursor stall", func(t *testing.T) {
		page := `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`
		caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{{text: page}, {text: page}}}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
		assertUnreliable(t, got, err)
		if len(caller.calls) != 2 {
			t.Fatalf("calls = %d, want stall detected on the second identical cursor", len(caller.calls))
		}
	})
}

func TestCrossPlatformCoverageChatMessageListPageAllArgumentValidation(t *testing.T) {
	base := []string{"message", "list", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0"}
	t.Run("mutually exclusive identity flags", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{}
		got, err := executeChatMessagePageAllCommand(t, caller, append(append([]string{}, base...),
			"--conversation-id", "cidAAAAAAAAAA1", "--user", "u1")...)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err = %v, want mutually exclusive identity error", err)
		}
		if got != nil {
			t.Fatalf("payload = %#v, want no output for argument validation failure", got)
		}
	})
	t.Run("identity flag required", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{}
		got, err := executeChatMessagePageAllCommand(t, caller, base...)
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("err = %v, want required identity error", err)
		}
		if got != nil {
			t.Fatalf("payload = %#v, want no output for argument validation failure", got)
		}
	})
	t.Run("invalid open-dingtalk-id format", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{}
		got, err := executeChatMessagePageAllCommand(t, caller, append(append([]string{}, base...),
			"--open-dingtalk-id", "not-a-current-d-id")...)
		if err == nil || !strings.Contains(err.Error(), "openDingTalkId") {
			t.Fatalf("err = %v, want explicit openDingTalkId format error", err)
		}
		if got != nil {
			t.Fatalf("payload = %#v, want no output for argument validation failure", got)
		}
	})
	t.Run("direction must be newer or older", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{}
		_, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "sideways", "--page-all", "--page-delay", "0")
		if err == nil || !strings.Contains(err.Error(), "newer or older") {
			t.Fatalf("err = %v, want direction validation error", err)
		}
	})
	t.Run("page-limit out of range", func(t *testing.T) {
		caller := &chatMessagePageAllCaller{}
		got, err := executeChatMessagePageAllCommand(t, caller,
			"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-limit", "0", "--page-delay", "0")
		if err == nil || !strings.Contains(err.Error(), "--page-limit must be between 1 and 500") {
			t.Fatalf("err = %v, want page-limit range error", err)
		}
		if got != nil {
			t.Fatalf("payload = %#v, want no output for paging option validation failure", got)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("calls = %d, want no tool calls before paging options validate", len(caller.calls))
		}
	})
}

func TestCrossPlatformCoverageChatMessageListPageAllOpenDingTalkIDPath(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--open-dingtalk-id", "DAAAAAAAAAAAiE", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want one page", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "chat" || call.tool != "list_individual_chat_message" {
		t.Fatalf("call = %s/%s, want chat/list_individual_chat_message", call.server, call.tool)
	}
	if call.args["openDingTalkId"] != "DAAAAAAAAAAAiE" {
		t.Fatalf("args = %#v, want openDingTalkId passthrough", call.args)
	}
	if _, exists := call.args["userId"]; exists {
		t.Fatalf("args = %#v, open-dingtalk-id path must not set userId", call.args)
	}
	if got["stopReason"] != "source_complete" || got["paginationKnown"] != true {
		t.Fatalf("stopReason/paginationKnown = %#v/%#v", got["stopReason"], got["paginationKnown"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllUserOpenDingTalkIDRewrite(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--user", "DAAAAAAAAAAAiE", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want one page", len(caller.calls))
	}
	call := caller.calls[0]
	if call.tool != "list_individual_chat_message" {
		t.Fatalf("tool = %s, want list_individual_chat_message after rewrite", call.tool)
	}
	if call.args["openDingTalkId"] != "DAAAAAAAAAAAiE" {
		t.Fatalf("args = %#v, want --user value rewritten to openDingTalkId", call.args)
	}
	if _, exists := call.args["userId"]; exists {
		t.Fatalf("args = %#v, rewritten user must not stay on userId", call.args)
	}
	if got["stopReason"] != "source_complete" {
		t.Fatalf("stopReason = %#v", got["stopReason"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllDefaultTime(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want one page", len(caller.calls))
	}
	if call := caller.calls[0]; call.args["time"] == "" {
		t.Fatalf("args = %#v, want default Shanghai now when --time is omitted", call.args)
	}
	if got["stopReason"] != "source_complete" {
		t.Fatalf("stopReason = %#v", got["stopReason"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllFirstPageErrorFailsFast(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{err: fmt.Errorf("gateway unavailable")},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err == nil || !strings.Contains(err.Error(), "gateway unavailable") {
		t.Fatalf("err = %v, want first-page failure surfaced directly", err)
	}
	if got != nil {
		t.Fatalf("payload = %#v, want no aggregate output when no page succeeded", got)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want single failed call", len(caller.calls))
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllNonObjectResponseFails(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `[1,2,3]`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err == nil {
		t.Fatal("expected non-zero exit for non-object response, got nil")
	}
	if got["stopReason"] != "read_failure" {
		t.Fatalf("stopReason = %#v, want read_failure", got["stopReason"])
	}
	if got["failedCount"].(float64) != 1 {
		t.Fatalf("failedCount = %#v, want 1", got["failedCount"])
	}
	failures, ok := got["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %#v, want one diagnostic", got["failures"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllDelayBetweenPages(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`},
		{text: `{"result":{"messages":[{"openMessageId":"m2"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommand(t, caller,
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want two pages with the inter-page delay honoured", len(caller.calls))
	}
	ids := pageAllMessageIDs(t, got)
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("messages = %#v, want [m1 m2]", ids)
	}
	if got["stopReason"] != "source_complete" || got["failedCount"].(float64) != 0 {
		t.Fatalf("stopReason/failedCount = %#v/%#v", got["stopReason"], got["failedCount"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllDelayCancelledContextFails(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1787000000123}}`},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := executeChatMessagePageAllCommandRaw(t, caller, ctx, &bytes.Buffer{},
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "5000")
	if err == nil {
		t.Fatal("expected non-zero exit when the context is cancelled during the inter-page delay, got nil")
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want the sweep to stop before a second page", len(caller.calls))
	}
	if got == nil {
		t.Fatal("payload = nil, want partial aggregate output before the delay failure")
	}
	if got["stopReason"] != "read_failure" {
		t.Fatalf("stopReason = %#v, want read_failure", got["stopReason"])
	}
	failures, ok := got["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %#v, want the delay-stage diagnostic", got["failures"])
	}
}

func TestCrossPlatformCoverageChatMessageListPageAllWriteFailurePropagates(t *testing.T) {
	caller := &chatMessagePageAllCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`},
	}}
	got, err := executeChatMessagePageAllCommandRaw(t, caller, context.Background(), failingWriter{},
		"message", "list", "--conversation-id", "cidAAAAAAAAAA1", "--time", "2025-03-01 00:00:00", "--direction", "older", "--page-all", "--page-delay", "0")
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("err = %v, want the payload write failure to propagate", err)
	}
	if got != nil {
		t.Fatalf("payload = %#v, want no parsed output on write failure", got)
	}
}

func firstOr(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func joinCommas(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ","
		}
		out += part
	}
	return out
}

func lastOr(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}
