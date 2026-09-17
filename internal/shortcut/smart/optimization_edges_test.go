package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"testing"
)

func TestCrossPlatformCoverageOptimizationReadFailureLedgerEdges(t *testing.T) {
	msg := `{"openMessageId":"m","openConversationId":"cid","openConvThreadId":"thread","createTime":"2026-01-02 00:00:00","content":"proof"}`
	for _, tc := range []struct {
		name      string
		args      []string
		responses map[string][]string
		fail      map[string]int
		wantErr   bool
	}{
		{"created-time", []string{"+chat-messages", "--chat-id", "cid", "--order", "asc", "--no-reactions"}, map[string][]string{"chat/get_conversation_info": {`{"result":{"openConversationId":"cid","createAt":"2026-01-01T00:00:00+08:00"}}`}, "chat/list_conversation_message_v2": {`{"result":{"messages":[],"hasMore":false}}`}}, nil, false},
		{"creation-unknown", []string{"+chat-messages", "--chat-id", "cid", "--order", "asc"}, nil, nil, true},
		{"creation-failed", []string{"+chat-messages", "--chat-id", "cid", "--order", "asc"}, nil, map[string]int{"chat/get_conversation_info": 1}, true},
		{"direct-asc-needs-start", []string{"+chat-messages", "--open-dingtalk-id", testCurrentDOpenID, "--order", "asc"}, nil, nil, true},
		{"bad-resume", []string{"+chat-messages", "--chat-id", "cid", "--page-token", "bad"}, nil, nil, true},
		{"history-shape", []string{"+chat-messages", "--chat-id", "cid"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"hasMore":false}}`}}, nil, true},
		{"history-shape-all", []string{"+chat-messages", "--chat-id", "cid", "--page-all"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"hasMore":false}}`}}, nil, true},
		{"history-no-cursor", []string{"+chat-messages", "--chat-id", "cid", "--no-reactions"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"messages":[],"hasMore":true}}`}}, nil, true},
		{"history-thread", []string{"+chat-messages", "--chat-id", "cid", "--with-threads", "--no-reactions"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}, "chat/list_topic_replies": {`{"result":{"messages":[],"hasMore":true,"nextCursor":1000}}`}}, nil, false},
		{"history-thread-error", []string{"+chat-messages", "--chat-id", "cid", "--with-threads", "--no-reactions"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}}, map[string]int{"chat/list_topic_replies": 1}, true},
		{"history-reaction-error", []string{"+chat-messages", "--chat-id", "cid"}, map[string][]string{"chat/list_conversation_message_v2": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}}, map[string]int{"im/list_message_emotion_replies": 1}, true},
		{"bot-shape", []string{"+chat-members-list", "--conversation-id", "cid", "--member-types", "bot"}, map[string][]string{"bot/list_group_bots": {`{"result":{}}`}}, nil, true},
		{"user-shape", []string{"+chat-members-list", "--conversation-id", "cid", "--member-types", "user"}, map[string][]string{"chat/get_group_members": {`{"result":{}}`}}, nil, true},
		{"user-single-start", []string{"+chat-members-list", "--conversation-id", "cid", "--member-types", "user", "--cursor=", "--single-page"}, map[string][]string{"chat/get_group_members": {`{"result":{"list":[],"hasMore":false}}`}}, nil, false},
		{"members-delay-invalid", []string{"+chat-members-list", "--conversation-id", "cid", "--page-delay", "60001"}, nil, nil, true},
		{"thread-delay-invalid", []string{"+thread-replies", "--group", "cid", "--thread", "thread", "--page-delay", "60001"}, nil, nil, true},
		{"thread-token-invalid", []string{"+thread-replies", "--group", "cid", "--thread", "thread", "--page-token", "bad"}, nil, nil, true},
		{"thread-time-preserves-format", []string{"+thread-replies", "--group", "cid", "--thread", "thread", "--time", "1780000000000"}, nil, nil, true},
		{"thread-time-token-conflict", []string{"+thread-replies", "--group", "cid", "--thread", "thread", "--time", "2026-09-01", "--page-token", "1780000000000"}, nil, nil, true},
		{"thread-numeric-time", []string{"+thread-replies", "--group", "cid", "--thread", "thread", "--page-token", "1780000000000", "--no-reactions"}, map[string][]string{"chat/list_topic_replies": {`{"result":{"messages":[],"hasMore":false}}`}}, nil, false},
		{"thread-reaction-error", []string{"+thread-replies", "--group", "cid", "--thread", "thread"}, map[string][]string{"chat/list_topic_replies": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}}, map[string]int{"im/list_message_emotion_replies": 1}, true},
		{"search-shape", []string{"+search-msg", "--query", "proof"}, map[string][]string{"im/search_messages": {`{"result":{"hasMore":false}}`}}, nil, true},
		{"search-file-unverified", []string{"+search-msg", "--query", "proof", "--message-type", "file", "--no-enrich"}, map[string][]string{"im/search_messages": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}}, nil, true},
		{"search-thread", []string{"+search-msg", "--query", "proof", "--with-threads", "--no-reactions"}, map[string][]string{"im/search_messages": {`{"result":{"messages":[` + msg + `],"hasMore":false}}`}, "im/list_messages_by_ids": {`{"result":{"messages":[` + msg + `]}}`}, "chat/list_topic_replies": {`{"result":{"messages":[],"hasMore":true,"nextCursor":1000}}`}}, nil, false},
		{"all-time-conflict", []string{"+search-msg", "--all-time", "--days", "1"}, nil, nil, true},
		{"unknown-conversation-type", []string{"+search-msg", "--query", "proof", "--chat-type", "unknown"}, nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &smartCoverageCaller{responses: tc.responses, failAt: tc.fail}
			helpers.InitDeps(f)
			root := newPlatformCoverageRoot()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs(append([]string{"chat"}, tc.args...))
			err := root.Execute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v output=%s calls=%#v", err, out.String(), f.arguments)
			}
		})
	}
}

func TestCrossPlatformCoverageOptimizationAutoPageCancellation(t *testing.T) {
	for _, command := range []string{"+chat-members-list", "+thread-replies"} {
		t.Run(command, func(t *testing.T) {
			f := &smartCoverageCaller{responses: map[string][]string{"chat/get_group_members": {`{"result":{"list":[{"openDingtalkId":"D1"}],"hasMore":true,"nextCursor":1}}`}, "chat/list_topic_replies": {`{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid","createTime":"2026-01-01 00:00:00"}],"hasMore":true,"nextCursor":1780000000000}}`}}}
			helpers.InitDeps(f)
			root := newPlatformCoverageRoot()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			root.SetContext(ctx)
			args := []string{"chat", command, "--page-delay", "1"}
			if command == "+thread-replies" {
				args = append(args, "--group", "cid", "--thread", "thread", "--page-all", "--no-reactions")
			} else {
				args = append(args, "--conversation-id", "cid", "--member-types", "user")
			}
			root.SetArgs(args)
			var output bytes.Buffer
			root.SetOut(&output)
			err := root.Execute()
			if command == "+thread-replies" && err == nil {
				t.Fatal("cancelled Thread succeeded")
			}
			if command == "+chat-members-list" {
				var p map[string]any
				if json.Unmarshal(output.Bytes(), &p) != nil || p["complete"] != false || p["failedCount"] != float64(1) {
					t.Fatalf("cancellation ledger lost: %s err=%v", output.String(), err)
				}
			}
		})
	}
}

type optimizationFailWriter struct{}

func (optimizationFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("output fixture failure")
}
func TestCrossPlatformCoverageOptimizationSearchOutputFailure(t *testing.T) {
	f := &smartCoverageCaller{responses: map[string][]string{"im/search_messages": {`{"result":{"messages":[],"hasMore":false}}`}}}
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	root.SetOut(optimizationFailWriter{})
	root.SetArgs([]string{"chat", "+search-msg", "--query", "proof"})
	if root.Execute() == nil {
		t.Fatal("output failure lost")
	}
}
