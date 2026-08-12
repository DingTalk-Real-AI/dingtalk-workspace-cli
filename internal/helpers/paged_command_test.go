package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type pagedCommandCall struct {
	server string
	tool   string
	args   map[string]any
	ctx    context.Context
}

type pagedCommandCaller struct {
	steps  []scriptedToolStep
	calls  []pagedCommandCall
	format string
	dry    bool
}

func (c *pagedCommandCaller) CallTool(ctx context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, pagedCommandCall{server: serverID, tool: toolName, args: copied, ctx: ctx})
	if len(c.steps) == 0 {
		return textToolResult(`{"result":{"messages":[],"hasMore":false}}`), nil
	}
	step := c.steps[len(c.calls)-1]
	if step.err != nil {
		return nil, step.err
	}
	return textToolResult(step.text), nil
}

func (c *pagedCommandCaller) Format() string { return c.format }
func (c *pagedCommandCaller) DryRun() bool   { return c.dry }
func (*pagedCommandCaller) Fields() string   { return "" }
func (*pagedCommandCaller) JQ() string       { return "" }

func runPagedCommandTest(t *testing.T, caller *pagedCommandCaller, cfg PagedMCPCommandConfig, args ...string) (map[string]any, string, error) {
	t.Helper()
	return runPagedCommandTestWithSleep(t, caller, cfg, func(time.Duration) {}, args...)
}

func runPagedCommandTestWithSleep(t *testing.T, caller *pagedCommandCaller, cfg PagedMCPCommandConfig, sleep func(time.Duration), args ...string) (map[string]any, string, error) {
	t.Helper()
	out, stderr, err := executePagedCommandTest(t, caller, cfg, sleep, &bytes.Buffer{}, args...)
	if strings.TrimSpace(out) == "" {
		return nil, stderr, err
	}
	var parsed map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", out, unmarshalErr)
	}
	return parsed, stderr, err
}

func executePagedCommandTest(t *testing.T, caller *pagedCommandCaller, cfg PagedMCPCommandConfig, sleep func(time.Duration), stdout io.Writer, args ...string) (string, string, error) {
	t.Helper()
	return executePagedCommandTestWithContext(t, context.Background(), caller, cfg, sleep, stdout, args...)
}

func executePagedCommandTestWithContext(t *testing.T, ctx context.Context, caller *pagedCommandCaller, cfg PagedMCPCommandConfig, sleep func(time.Duration), stdout io.Writer, args ...string) (string, string, error) {
	t.Helper()
	oldDeps := deps
	oldSleep := helperSleep
	oldAfter := helperAfter
	t.Cleanup(func() {
		deps = oldDeps
		helperSleep = oldSleep
		helperAfter = oldAfter
	})
	InitDeps(caller)
	out := stdout
	errOut := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = errOut
	if sleep != nil {
		helperSleep = sleep
		helperAfter = func(d time.Duration) <-chan time.Time {
			sleep(d)
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		}
	}

	cmd := &cobra.Command{
		Use:          "paged",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunPagedMCPCommand(cmd, cfg)
		},
	}
	cmd.SetContext(ctx)
	cmd.Flags().String("cursor", "0", "")
	AddPagedMCPFlags(cmd)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if buf, ok := out.(*bytes.Buffer); ok {
		return buf.String(), errOut.String(), err
	}
	return "", errOut.String(), err
}

func pagedCommandMessagesConfig(fallback func(map[string]any) error) PagedMCPCommandConfig {
	if fallback == nil {
		fallback = func(map[string]any) error { return nil }
	}
	return PagedMCPCommandConfig{
		ServerID:    "chat",
		ToolName:    "search_messages_by_time_range",
		ItemPath:    "result.messages",
		CursorPath:  "result.nextCursor",
		HasMorePath: "result.hasMore",
		CursorArg:   "cursor",
		CursorKind:  PagedCursorString,
		BuildArgs: func(cmd *cobra.Command) (map[string]any, error) {
			cursor, _ := cmd.Flags().GetString("cursor")
			return map[string]any{"cursor": cursor, "limit": 2}, nil
		},
		Fallback: fallback,
	}
}

func pagedCommandConversationMessagesConfig(fallback func(map[string]any) error) PagedMCPCommandConfig {
	cfg := pagedCommandMessagesConfig(fallback)
	cfg.ItemPath = "result.conversationMessagesList"
	cfg.AggregationMode = PagedAggregationConversationMessages
	return cfg
}

func TestPagedMCPCommandDefaultUsesFallbackOnly(t *testing.T) {
	caller := &pagedCommandCaller{}
	fallbackCalls := 0
	cfg := pagedCommandMessagesConfig(func(args map[string]any) error {
		fallbackCalls++
		if args["cursor"] != "0" {
			t.Fatalf("fallback args = %#v", args)
		}
		return nil
	})
	_, _, err := runPagedCommandTest(t, caller, cfg, "--page-limit", "2", "--max-items", "1", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if fallbackCalls != 1 || len(caller.calls) != 0 {
		t.Fatalf("fallback=%d remote=%d, want fallback only", fallbackCalls, len(caller.calls))
	}
}

func TestPagedMCPCommandRejectsInvalidConfigWhenPageAll(t *testing.T) {
	caller := &pagedCommandCaller{}
	cfg := pagedCommandMessagesConfig(nil)
	cfg.ServerID = " "

	got, _, err := runPagedCommandTest(t, caller, cfg, "--page-all")
	if err == nil || !strings.Contains(err.Error(), "server is required") {
		t.Fatalf("result=%#v err=%v, want config error", got, err)
	}
	if got != nil {
		t.Fatalf("result=%#v, want no stdout", got)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls=%#v, want no remote call", caller.calls)
	}
}

func TestPagedMCPCommandDryRunPrintsRequestAndSkipsRemote(t *testing.T) {
	caller := &pagedCommandCaller{dry: true}

	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-limit", "3", "--max-items", "7", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls=%#v, want no remote call", caller.calls)
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run=%#v, want true", got["dry_run"])
	}
	request := got["request"].(map[string]any)
	if request["server"] != "chat" || request["name"] != "search_messages_by_time_range" {
		t.Fatalf("request=%#v", request)
	}
	args := request["args"].(map[string]any)
	if args["cursor"] != "0" || args["limit"].(float64) != 2 {
		t.Fatalf("args=%#v", args)
	}
	paging := got["paging"].(map[string]any)
	if paging["pageAll"] != true || paging["pageLimit"].(float64) != 3 || paging["maxItems"].(float64) != 7 || paging["pageDelay"].(float64) != 0 {
		t.Fatalf("paging=%#v", paging)
	}
}

func TestPagedMCPCommandValidateConfigRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*PagedMCPCommandConfig)
		want string
	}{
		{
			name: "server",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.ServerID = ""
			},
			want: "server is required",
		},
		{
			name: "tool",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.ToolName = ""
			},
			want: "tool is required",
		},
		{
			name: "item path",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.ItemPath = ""
			},
			want: "item path is required",
		},
		{
			name: "cursor path",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.CursorPath = ""
			},
			want: "cursor path is required",
		},
		{
			name: "hasMore path",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.HasMorePath = ""
			},
			want: "hasMore path is required",
		},
		{
			name: "cursor arg",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.CursorArg = ""
			},
			want: "cursor arg is required",
		},
		{
			name: "build args callback",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.BuildArgs = nil
			},
			want: "callbacks are required",
		},
		{
			name: "fallback callback",
			edit: func(cfg *PagedMCPCommandConfig) {
				cfg.Fallback = nil
			},
			want: "callbacks are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pagedCommandMessagesConfig(nil)
			tt.edit(&cfg)
			err := validatePagedConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestPagedMCPCommandStringCursorAggregatesAndPageLimit(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
		{text: `{"result":{"messages":[{"id":"m2"}],"hasMore":true,"nextCursor":"c3"}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-limit", "2", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	items := got["result"].(map[string]any)["messages"].([]any)
	result := got["result"].(map[string]any)
	paging := got["paging"].(map[string]any)
	if len(items) != 2 || paging["truncated"] != true || paging["pages"].(float64) != 2 {
		t.Fatalf("result = %#v", got)
	}
	if result["hasMore"] != true || result["nextCursor"] != "c3" {
		t.Fatalf("result=%#v, want final page-limit cursor state", result)
	}
	if caller.calls[0].args["cursor"] != "0" || caller.calls[1].args["cursor"] != "c2" {
		t.Fatalf("call args = %#v", caller.calls)
	}
}

func TestPagedMCPCommandStringCursorAggregatesAndSyncsCompletionFields(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
		{text: `{"result":{"messages":[{"id":"m2"}],"hasMore":false,"nextCursor":""}}`},
	}}

	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	result := got["result"].(map[string]any)
	items := result["messages"].([]any)
	if len(items) != 2 || result["hasMore"] != false || result["nextCursor"] != "" {
		t.Fatalf("result=%#v, want complete aggregate with final cursor state", result)
	}
	paging := got["paging"].(map[string]any)
	if paging["truncated"] != false || paging["hasMore"] != false || paging["lastCursor"] != "" {
		t.Fatalf("paging=%#v, want complete pagination metadata", paging)
	}
}

func TestPagedMCPCommandConversationMessagesMergeSameConversation(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"群1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","title":"ignored","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	if len(conversations) != 1 {
		t.Fatalf("conversations=%#v, want one merged conversation", conversations)
	}
	conversation := conversations[0].(map[string]any)
	messages := conversation["messages"].([]any)
	if conversation["title"] != "群1" || len(messages) != 2 {
		t.Fatalf("conversation=%#v, want preserved title and two messages", conversation)
	}
	paging := got["paging"].(map[string]any)
	if paging["total"].(float64) != 2 {
		t.Fatalf("paging=%#v, want total message count 2", paging)
	}
}

func TestPagedMCPCommandConversationMessagesPreserveFirstConversationOrder(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid2","messages":[{"id":"m2"}]},{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid3","messages":[{"id":"m3"}]}],"hasMore":false,"nextCursor":""}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	gotIDs := []string{
		conversations[0].(map[string]any)["openConversationId"].(string),
		conversations[1].(map[string]any)["openConversationId"].(string),
		conversations[2].(map[string]any)["openConversationId"].(string),
	}
	if strings.Join(gotIDs, ",") != "cid2,cid1,cid3" {
		t.Fatalf("conversation order=%v", gotIDs)
	}
}

func TestPagedMCPCommandConversationMessagesMaxItemsTruncatesMessages(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"},{"id":"m2"}]},{"openConversationId":"cid2","messages":[{"id":"m3"},{"id":"m4"}]}],"hasMore":true,"nextCursor":"c2"}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--max-items", "3", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	if len(conversations) != 2 {
		t.Fatalf("conversations=%#v, want two conversations", conversations)
	}
	secondMessages := conversations[1].(map[string]any)["messages"].([]any)
	paging := got["paging"].(map[string]any)
	if len(secondMessages) != 1 || paging["total"].(float64) != 3 || paging["truncated"] != true {
		t.Fatalf("result=%#v", got)
	}
}

func TestPagedMCPCommandConversationMessagesMissingListOnFinalPageIsEmpty(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"hasMore":false,"nextCursor":""}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	if len(conversations) != 0 {
		t.Fatalf("conversations=%#v, want empty", conversations)
	}
}

func TestPagedMCPCommandConversationMessagesLaterFailureOutputsPartial(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
		{err: errors.New("page failed")},
	}}
	got, stderr, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err == nil || !strings.Contains(stderr, "pagination stopped") {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	paging := got["paging"].(map[string]any)
	if len(conversations) != 1 || paging["partial"] != true || paging["itemsFetched"].(float64) != 1 {
		t.Fatalf("result=%#v", got)
	}
}

func TestPagedMCPCommandConversationMessagesAddErrorsOutputPartial(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "conversation item not object",
			response: `{"result":{"conversationMessagesList":["bad"],"hasMore":false,"nextCursor":""}}`,
			want:     "conversation item must be object",
		},
		{
			name:     "conversation missing openConversationId",
			response: `{"result":{"conversationMessagesList":[{"messages":[]}],"hasMore":false,"nextCursor":""}}`,
			want:     "missing openConversationId",
		},
		{
			name:     "conversation messages not array",
			response: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":"bad"}],"hasMore":false,"nextCursor":""}}`,
			want:     "conversation messages must be array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &pagedCommandCaller{steps: []scriptedToolStep{{text: tt.response}}}
			got, stderr, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--page-delay", "0")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("result=%#v err=%v, want %q", got, err, tt.want)
			}
			if !strings.Contains(stderr, "pagination stopped at page 1") {
				t.Fatalf("stderr=%q", stderr)
			}
			paging := got["paging"].(map[string]any)
			if paging["partial"] != true || paging["itemsFetched"].(float64) != 0 {
				t.Fatalf("paging=%#v", paging)
			}
		})
	}
}

func TestPagedMCPCommandConversationMessagesMaxItemsTruncatesAtConversationBoundary(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"},{"id":"m2"}]},{"openConversationId":"cid2","messages":[{"id":"m3"},{"id":"m4"}]}],"hasMore":true,"nextCursor":"c2"}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--max-items", "2", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	paging := got["paging"].(map[string]any)
	if len(conversations) != 1 || paging["total"].(float64) != 2 || paging["truncated"] != true {
		t.Fatalf("result=%#v", got)
	}
}

func TestPagedMCPCommandConversationMessagesAcceptsMissingMessages(t *testing.T) {
	messages, err := conversationMessages(map[string]any{"openConversationId": "cid1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages=%#v, want empty", messages)
	}
}

func TestPagedMCPCommandConversationMessagesRejectsCorruptExistingMessages(t *testing.T) {
	collection := newPagedCollection(PagedMCPCommandConfig{AggregationMode: PagedAggregationConversationMessages})
	collection.items = []any{map[string]any{"openConversationId": "cid1", "messages": "bad"}}
	collection.conversationIndex["cid1"] = 0

	err := collection.Add([]any{map[string]any{"openConversationId": "cid1", "messages": []any{map[string]any{"id": "m2"}}}})
	if err == nil || !strings.Contains(err.Error(), "conversation messages must be array") {
		t.Fatalf("err=%v, want corrupt existing messages error", err)
	}
}

func TestPagedMCPCommandResponseShapeErrors(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "items not array",
			response: `{"result":{"messages":"bad","hasMore":false}}`,
			want:     "result.messages must be array",
		},
		{
			name:     "missing hasMore",
			response: `{"result":{"messages":[]}}`,
			want:     "missing result.hasMore",
		},
		{
			name:     "hasMore not bool",
			response: `{"result":{"messages":[],"hasMore":"yes"}}`,
			want:     "result.hasMore must be boolean",
		},
		{
			name:     "missing next cursor",
			response: `{"result":{"messages":[{"id":"m1"}],"hasMore":true}}`,
			want:     "missing result.nextCursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &pagedCommandCaller{steps: []scriptedToolStep{{text: tt.response}}}
			got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-delay", "0")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("result=%#v err=%v, want %q", got, err, tt.want)
			}
			if got != nil {
				t.Fatalf("result=%#v, want no partial stdout", got)
			}
		})
	}
}

func TestPagedMCPCommandPageDelayControlsSleep(t *testing.T) {
	tests := []struct {
		name       string
		delay      string
		wantSleeps []time.Duration
	}{
		{
			name:       "non zero delay sleeps between pages",
			delay:      "200",
			wantSleeps: []time.Duration{200 * time.Millisecond},
		},
		{
			name:       "zero delay skips sleep",
			delay:      "0",
			wantSleeps: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &pagedCommandCaller{steps: []scriptedToolStep{
				{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
				{text: `{"result":{"messages":[{"id":"m2"}],"hasMore":false,"nextCursor":""}}`},
			}}
			var sleeps []time.Duration
			got, _, err := runPagedCommandTestWithSleep(t, caller, pagedCommandMessagesConfig(nil), func(d time.Duration) {
				sleeps = append(sleeps, d)
			}, "--page-all", "--page-delay", tt.delay)
			if err != nil {
				t.Fatal(err)
			}
			items := got["result"].(map[string]any)["messages"].([]any)
			if len(items) != 2 || len(caller.calls) != 2 {
				t.Fatalf("items=%#v calls=%#v", items, caller.calls)
			}
			if len(sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleeps=%v, want %v", sleeps, tt.wantSleeps)
			}
			for i := range tt.wantSleeps {
				if sleeps[i] != tt.wantSleeps[i] {
					t.Fatalf("sleeps=%v, want %v", sleeps, tt.wantSleeps)
				}
			}
		})
	}
}

func TestPagedMCPCommandMaxItemsTruncatesPrecisely(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"},{"id":"m2"}],"hasMore":true,"nextCursor":"c2"}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--max-items", "1", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	items := got["result"].(map[string]any)["messages"].([]any)
	paging := got["paging"].(map[string]any)
	if len(items) != 1 || paging["total"].(float64) != 1 || paging["truncated"] != true {
		t.Fatalf("result = %#v", got)
	}
}

func TestPagedMCPCommandMaxItemsStopsWhenPageExactlyReachesLimit(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"},{"id":"m2"}],"hasMore":true,"nextCursor":"c2"}}`},
		{err: errors.New("second page should not run")},
	}}

	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--max-items", "2", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	items := got["result"].(map[string]any)["messages"].([]any)
	result := got["result"].(map[string]any)
	paging := got["paging"].(map[string]any)
	if len(caller.calls) != 1 || len(items) != 2 {
		t.Fatalf("calls=%#v items=%#v, want one full page", caller.calls, items)
	}
	if paging["truncated"] != true || paging["hasMore"] != true || paging["lastCursor"] != "c2" {
		t.Fatalf("paging=%#v, want safe page-boundary cursor", paging)
	}
	if result["hasMore"] != true || result["nextCursor"] != "c2" {
		t.Fatalf("result=%#v, want safe page-boundary cursor fields", result)
	}
	if _, ok := paging["truncatedWithinPage"]; ok {
		t.Fatalf("paging=%#v, want no within-page truncation marker", paging)
	}
}

func TestPagedMCPCommandMaxItemsWithinPageKeepsCurrentCursor(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"},{"id":"m2"}],"hasMore":true,"nextCursor":"c2"}}`},
	}}

	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--max-items", "1", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	items := got["result"].(map[string]any)["messages"].([]any)
	result := got["result"].(map[string]any)
	paging := got["paging"].(map[string]any)
	if len(items) != 1 || paging["lastCursor"] != "0" {
		t.Fatalf("result=%#v, want current-page cursor after within-page truncation", got)
	}
	if result["hasMore"] != true || result["nextCursor"] != "0" {
		t.Fatalf("result=%#v, want unreliable current-page cursor fields", result)
	}
	if paging["truncatedWithinPage"] != true || paging["resumeCursorReliable"] != false {
		t.Fatalf("paging=%#v, want unreliable resume marker", paging)
	}
}

func TestPagedCollectionTruncateReturnsFalseWhenLimitDoesNotTrim(t *testing.T) {
	collection := newPagedCollection(PagedMCPCommandConfig{})
	if err := collection.Add([]any{map[string]any{"id": "m1"}}); err != nil {
		t.Fatal(err)
	}

	if collection.Truncate(0) {
		t.Fatal("Truncate(0) should not trim")
	}
	if collection.Truncate(1) {
		t.Fatal("Truncate(total) should not trim")
	}
	if collection.Total() != 1 || len(collection.Values()) != 1 {
		t.Fatalf("collection=%#v, want unchanged single item", collection.Values())
	}
}

func TestPagedMCPCommandConversationMessagesMaxItemsWithinPageKeepsCurrentCursor(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"},{"id":"m2"}]}],"hasMore":true,"nextCursor":"c2"}}`},
	}}

	got, _, err := runPagedCommandTest(t, caller, pagedCommandConversationMessagesConfig(nil), "--page-all", "--max-items", "1", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	conversations := got["result"].(map[string]any)["conversationMessagesList"].([]any)
	messages := conversations[0].(map[string]any)["messages"].([]any)
	paging := got["paging"].(map[string]any)
	if len(messages) != 1 || paging["lastCursor"] != "0" {
		t.Fatalf("result=%#v, want truncated conversation with current-page cursor", got)
	}
	if paging["truncatedWithinPage"] != true || paging["resumeCursorReliable"] != false {
		t.Fatalf("paging=%#v, want unreliable resume marker", paging)
	}
}

func TestPagedMCPCommandPassesCommandContextToCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":false,"nextCursor":""}}`},
	}}

	out, _, err := executePagedCommandTestWithContext(t, ctx, caller, pagedCommandMessagesConfig(nil), func(time.Duration) {}, &bytes.Buffer{}, "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) == "" || len(caller.calls) != 1 {
		t.Fatalf("stdout=%q calls=%#v, want one successful call", out, caller.calls)
	}
	if caller.calls[0].ctx != ctx || caller.calls[0].ctx.Err() != context.Canceled {
		t.Fatalf("call ctx=%#v err=%v, want canceled command context", caller.calls[0].ctx, caller.calls[0].ctx.Err())
	}
}

func TestPagedMCPCommandPageDelayStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
		{text: `{"result":{"messages":[{"id":"m2"}],"hasMore":false,"nextCursor":""}}`},
	}}
	var out bytes.Buffer

	stdout, stderr, err := executePagedCommandTestWithContext(t, ctx, caller, pagedCommandMessagesConfig(nil), nil, &out, "--page-all", "--page-delay", "10")
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("stdout is empty, want partial pagination JSON")
	}
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context canceled", err)
	}
	if !strings.Contains(stderr, "pagination stopped at page 2") || len(caller.calls) != 1 {
		t.Fatalf("stderr=%q calls=%#v, want cancellation before second call", stderr, caller.calls)
	}
	var got map[string]any
	if unmarshalErr := json.Unmarshal([]byte(stdout), &got); unmarshalErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", stdout, unmarshalErr)
	}
	paging := got["paging"].(map[string]any)
	if paging["partial"] != true || paging["failedPage"].(float64) != 2 || paging["itemsFetched"].(float64) != 1 {
		t.Fatalf("paging=%#v, want partial cancellation metadata", paging)
	}
}

func TestPagedMCPCommandPropagatesAggregatedOutputErrors(t *testing.T) {
	tests := []struct {
		name       string
		steps      []scriptedToolStep
		args       []string
		wantStderr string
	}{
		{
			name: "normal end",
			steps: []scriptedToolStep{
				{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":false,"nextCursor":""}}`},
			},
			args: []string{"--page-all", "--page-delay", "0"},
		},
		{
			name: "max items truncation",
			steps: []scriptedToolStep{
				{text: `{"result":{"messages":[{"id":"m1"},{"id":"m2"}],"hasMore":true,"nextCursor":"c2"}}`},
			},
			args: []string{"--page-all", "--max-items", "1", "--page-delay", "0"},
		},
		{
			name: "page limit truncation",
			steps: []scriptedToolStep{
				{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
			},
			args: []string{"--page-all", "--page-limit", "1", "--page-delay", "0"},
		},
		{
			name: "partial result after later failure",
			steps: []scriptedToolStep{
				{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
				{err: errors.New("page failed")},
			},
			args:       []string{"--page-all", "--page-delay", "0"},
			wantStderr: "pagination stopped at page 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TC-output-error: aggregate stdout write failures must fail the command.
			caller := &pagedCommandCaller{steps: tt.steps}
			_, stderr, err := executePagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), func(time.Duration) {}, failingWriter{}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), "write failed") {
				t.Fatalf("err=%v, want propagated write failure", err)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr, tt.wantStderr) {
				t.Fatalf("stderr=%q, want %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestPagedMCPCommandInt64CursorAndItemsPath(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"items":[{"id":"f1"}],"hasMore":true,"nextCursor":20}}`},
		{text: `{"result":{"items":[{"id":"f2"}],"hasMore":false,"nextCursor":0}}`},
	}}
	cfg := PagedMCPCommandConfig{
		ServerID:    "im",
		ToolName:    "list_message_favorites",
		ItemPath:    "result.items",
		CursorPath:  "result.nextCursor",
		HasMorePath: "result.hasMore",
		CursorArg:   "cursor",
		CursorKind:  PagedCursorInt64,
		BuildArgs: func(cmd *cobra.Command) (map[string]any, error) {
			return map[string]any{"cursor": int64(0), "size": "20"}, nil
		},
		Fallback: func(map[string]any) error { return nil },
	}
	got, _, err := runPagedCommandTest(t, caller, cfg, "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	items := got["result"].(map[string]any)["items"].([]any)
	if len(items) != 2 || caller.calls[1].args["cursor"] != int64(20) {
		t.Fatalf("items=%#v calls=%#v", items, caller.calls)
	}
}

func TestPagedMCPCommandInt64CursorRejectsNonNumericNextCursor(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"items":[{"id":"f1"}],"hasMore":true,"nextCursor":"not-a-number"}}`},
	}}
	cfg := PagedMCPCommandConfig{
		ServerID:    "im",
		ToolName:    "list_message_favorites",
		ItemPath:    "result.items",
		CursorPath:  "result.nextCursor",
		HasMorePath: "result.hasMore",
		CursorArg:   "cursor",
		CursorKind:  PagedCursorInt64,
		BuildArgs: func(cmd *cobra.Command) (map[string]any, error) {
			return map[string]any{"cursor": int64(0), "size": "20"}, nil
		},
		Fallback: func(map[string]any) error { return nil },
	}
	got, stderr, err := runPagedCommandTest(t, caller, cfg, "--page-all", "--page-delay", "0")
	if err == nil || !strings.Contains(err.Error(), "base-10 int64 string") {
		t.Fatalf("err=%v, want invalid int64 cursor error", err)
	}
	if !strings.Contains(stderr, "pagination stopped at page 2") {
		t.Fatalf("stderr=%q", stderr)
	}
	paging := got["paging"].(map[string]any)
	if paging["partial"] != true || paging["failedCursor"] != "not-a-number" || paging["pagesFetched"].(float64) != 1 {
		t.Fatalf("paging = %#v", paging)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls=%#v, want no second call with cursor 0", caller.calls)
	}
}

func TestPagedMCPCommandFirstPageFailureReturnsNoPartial(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all")
	if err == nil || got != nil {
		t.Fatalf("result=%#v err=%v, want first-page error without stdout", got, err)
	}
}

func TestPagedMCPCommandLaterFailureOutputsPartial(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"c2"}}`},
		{err: errors.New("page failed")},
	}}
	got, stderr, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err == nil || !strings.Contains(stderr, "pagination stopped") {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
	paging := got["paging"].(map[string]any)
	if paging["partial"] != true || paging["failedPage"].(float64) != 2 || paging["itemsFetched"].(float64) != 1 {
		t.Fatalf("paging = %#v", paging)
	}
}

func TestPagedMCPCommandCursorCycleOutputsPartial(t *testing.T) {
	caller := &pagedCommandCaller{steps: []scriptedToolStep{
		{text: `{"result":{"messages":[{"id":"m1"}],"hasMore":true,"nextCursor":"0"}}`},
	}}
	got, _, err := runPagedCommandTest(t, caller, pagedCommandMessagesConfig(nil), "--page-all", "--page-delay", "0")
	if err == nil {
		t.Fatal("cursor cycle should return error")
	}
	paging := got["paging"].(map[string]any)
	if paging["partial"] != true || paging["pagesFetched"].(float64) != 1 {
		t.Fatalf("paging = %#v", paging)
	}
}

func TestPagedMCPCommandSetJSONPathRejectsNonObjectIntermediate(t *testing.T) {
	root := map[string]any{"result": "not-object"}

	if setJSONPath(root, "result.messages", []any{}) {
		t.Fatal("setJSONPath should reject a non-object intermediate")
	}
	if root["result"] != "not-object" {
		t.Fatalf("root=%#v, want original intermediate preserved", root)
	}
}

func TestPagedMCPCommandCursorValueKeyCoversBoundaryKinds(t *testing.T) {
	if got := cursorValueKey(7, PagedCursorInt64); got != "7" {
		t.Fatalf("int cursor key=%q, want 7", got)
	}
	if got := cursorValueKey(nil, PagedCursorString); got != "" {
		t.Fatalf("nil string cursor key=%q, want empty", got)
	}
}

func TestPagedMCPCommandNormalizeCursorArgCoversBoundaryKinds(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		kind    PagedCursorKind
		want    any
		wantErr string
	}{
		{
			name:  "nil string cursor",
			value: nil,
			kind:  PagedCursorString,
			want:  "",
		},
		{
			name:  "int64 cursor",
			value: int64(9),
			kind:  PagedCursorInt64,
			want:  int64(9),
		},
		{
			name:  "int cursor",
			value: 10,
			kind:  PagedCursorInt64,
			want:  int64(10),
		},
		{
			name:  "numeric string cursor",
			value: " 11 ",
			kind:  PagedCursorInt64,
			want:  int64(11),
		},
		{
			name:    "fractional float cursor",
			value:   1.5,
			kind:    PagedCursorInt64,
			wantErr: "must be an integer",
		},
		{
			name:    "unsupported cursor type",
			value:   []string{"bad"},
			kind:    PagedCursorInt64,
			wantErr: "int64-compatible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCursorArg(tt.value, tt.kind)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("value=%#v err=%v, want %q", tt.value, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("value=%#v got=%#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}
