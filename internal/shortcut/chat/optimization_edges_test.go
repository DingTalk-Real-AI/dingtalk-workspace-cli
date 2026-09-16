package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

const optimizationOpenID = "DAAAAAAAAAAAiE"

func TestCrossPlatformCoverageOptimizationChatReadBoundaryCases(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		responses map[string]string
		err       bool
	}{
		{"top-shape", []string{"+feed-shortcut-list"}, map[string]string{"chat/list_top_conversations": `{"result":{}}`}, true},
		{"top-detail-fails", []string{"+feed-shortcut-list", "--page-token", "1"}, map[string]string{"chat/list_top_conversations": `{"result":{"conversations":[{"openConversationId":"cid"}],"hasMore":false}}`, "chat/get_conversation_info": `{"result":{"openConversationId":"other"}}`}, true},
		{"categories-shape", []string{"+feed-group-list"}, map[string]string{"im/list_user_define_conv_categories": `{"result":{}}`}, true},
		{"categories-project", []string{"+feed-group-list"}, map[string]string{"im/list_user_define_conv_categories": `{"result":{"categories":[{"categoryId":1,"createAt":2,"title":"fixture"}]}}`}, false},
		{"category-items-shape", []string{"+feed-group-list-item", "--feed-group-id", "1"}, map[string]string{"im/list_conversations_by_category": `{"result":{}}`}, true},
		{"category-items-type", []string{"+feed-group-list-item", "--feed-group-id", "1"}, map[string]string{"im/list_conversations_by_category": `{"result":{"conversations":[{"openConversationId":"cid","singleChat":true,"createAt":1}],"hasMore":false}}`}, false},
		{"group-shape", []string{"+chat-search", "--query", "x"}, map[string]string{"im/search_groups": `{"result":{}}`}, true},
		{"group-missing-id", []string{"+chat-search", "--query", "x"}, map[string]string{"im/search_groups": `{"result":{"groups":[{"name":"x"}],"hasMore":false}}`}, true},
		{"group-mode-unknown", []string{"+chat-search", "--query", "x", "--chat-modes", "topic"}, map[string]string{"im/search_groups": `{"result":{"groups":[{"openConversationId":"cid"}],"hasMore":false}}`}, true},
		{"group-sort-unknown", []string{"+chat-search", "--query", "x", "--sort", "create_time"}, map[string]string{"im/search_groups": `{"result":{"groups":[{"openConversationId":"cid"}],"hasMore":false}}`}, true},
		{"list-sort-unknown", []string{"+chat-list", "--sort", "create_time"}, map[string]string{"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"cid","singleChat":false}],"hasMore":false}}`}, true},
		{"list-delay-invalid", []string{"+chat-list", "--page-delay", "60001"}, nil, true},
		{"favorite-no-exhaustion", []string{"+flag-list", "--no-enrich"}, map[string]string{"im/list_message_favorites": `{"result":{"items":[]}}`}, true},
		{"favorite-enrichment-missing", []string{"+flag-list"}, map[string]string{"im/list_message_favorites": `{"result":{"items":[{"openMessageId":"missing","openConversationId":"cid"}],"hasMore":false}}`, "im/list_messages_by_ids": `{"result":{"messages":[]}}`}, true},
		{"favorite-thread-incomplete", []string{"+flag-list", "--with-threads", "--no-reactions"}, map[string]string{"im/list_message_favorites": `{"result":{"items":[{"openMessageId":"m","openConversationId":"cid"}],"hasMore":false}}`, "im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid","openConvThreadId":"t"}]}}`, "chat/list_topic_replies": `{"result":{"messages":[],"hasMore":true,"nextCursor":1000}}`}, false},
		{"native-text-uuid", []string{"+messages-send", "--chat-id", "cid", "--text", "proof", "--uuid", "fixture", "--yes"}, nil, false},
		{"create-too-many-bots", []string{"+chat-create", "--bots", "a,b,c,d,e,f,g,h,i,j,k", "--yes"}, nil, true},
		{"retry-delay-invalid", []string{"+messages-resource-download", "--resource-id", "id", "--part-size", "4096", "--retry-delay", "10001"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: tc.responses}
			out, err := runChatParity(t, f, tc.args...)
			if (err != nil) != tc.err {
				t.Fatalf("err=%v out=%s calls=%#v", err, out, f.calls)
			}
		})
	}
	f := &larkAlignmentCaller{dryRun: true}
	out, err := runChatParity(t, f, "+chat-create", "--bots", "bot", "--dry-run")
	if err != nil || !strings.Contains(out, "createArguments") {
		t.Fatalf("bot preview=%s %v", out, err)
	}
}

func TestCrossPlatformCoverageOptimizationGroupFilterIdentityAndRoles(t *testing.T) {
	self := "D-resolved"
	for _, tc := range []struct {
		name  string
		flags map[string]string
		rows  []map[string]any
		reply string
		fail  string
		want  int
		err   bool
	}{
		{"member", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[{"openDingtalkId":"` + optimizationOpenID + `"}]}}`, "", 1, false},
		{"member-absent", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[]}}`, "", 0, false},
		{"wrong-member", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[{"openDingtalkId":"other"}]}}`, "", 0, true},
		{"missing-group", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{}}, `{"result":{"members":[]}}`, "", 0, true},
		{"manager", map[string]string{"is-manager": "true"}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[{"openDingtalkId":"` + self + `","roleType":1}]}}`, "", 1, false},
		{"ordinary", map[string]string{"is-manager": "true"}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[{"openDingtalkId":"` + self + `","roleType":0}]}}`, "", 0, false},
		{"role-missing", map[string]string{"is-manager": "true"}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{"members":[{"openDingtalkId":"` + self + `"}]}}`, "", 0, true},
		{"profile-failed", map[string]string{"is-manager": "true"}, []map[string]any{{"openConversationId": "cid"}}, "", "contact/get_current_user_profile", 0, true},
		{"members-failed", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{"openConversationId": "cid"}}, "", "im/list_group_member_by_ids", 0, true},
		{"members-shape", map[string]string{"member-ids": optimizationOpenID}, []map[string]any{{"openConversationId": "cid"}}, `{"result":{}}`, "", 0, true},
		{"topic", map[string]string{"chat-modes": "topic"}, []map[string]any{{"openConversationId": "cid", "channel": true}, {"openConversationId": "other", "channel": false}}, "", "", 1, false},
		{"bad-mode", map[string]string{"chat-modes": "unknown"}, nil, "", "", 0, true},
		{"bad-id", map[string]string{"member-ids": "bad"}, nil, "", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{failProductTool: tc.fail, responses: map[string]string{"im/list_group_member_by_ids": tc.reply}}
			helpers.InitDeps(f)
			root := newPlatformCoverageRoot()
			cmd, _, _ := root.Find([]string{"chat", "+chat-search"})
			for k, v := range tc.flags {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := filterVerifiedGroups(shortcut.RuntimeContextForTest(cmd, ChatSearch), tc.rows)
			if (err != nil) != tc.err || len(rows) != tc.want {
				t.Fatalf("rows=%#v err=%v", rows, err)
			}
		})
	}
}

func TestCrossPlatformCoverageOptimizationChatDelayCancellation(t *testing.T) {
	for _, name := range []string{"+chat-list", "+chat-search"} {
		t.Run(name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"cid"}],"hasMore":true,"nextCursor":1}}`, "im/search_groups": `{"result":{"groups":[{"openConversationId":"cid"}],"hasMore":true,"nextCursor":"next"}}`}}
			helpers.InitDeps(f)
			root := newPlatformCoverageRoot()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			root.SetContext(ctx)
			var out bytes.Buffer
			root.SetOut(&out)
			args := []string{"chat", name, "--page-all", "--page-delay", "1"}
			if name == "+chat-search" {
				args = append(args, "--query", "x")
			}
			root.SetArgs(args)
			if root.Execute() == nil {
				t.Fatal("cancelled paging success")
			}
			var p map[string]any
			if json.Unmarshal(out.Bytes(), &p) != nil || p["complete"] != false || p["failedCount"] != float64(1) {
				t.Fatalf("cancellation discarded data: %s", out.String())
			}
		})
	}
}

func TestCrossPlatformCoverageOptimizationValidationHelpers(t *testing.T) {
	f := &larkAlignmentCaller{}
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	cmd, _, _ := root.Find([]string{"chat", "+flag-list"})
	_ = cmd.Flags().Set("with-threads", "true")
	_ = cmd.Flags().Set("no-enrich", "true")
	if validateFlagList(shortcut.RuntimeContextForTest(cmd, FlagList)) == nil {
		t.Fatal("conflicting enrich controls")
	}
	for _, flag := range []string{"a2ui-messages", "image-url", "contact-id", "share-chat-id"} {
		root := newPlatformCoverageRoot()
		cmd, _, _ := root.Find([]string{"chat", "+messages-send"})
		_ = cmd.Flags().Set("msg-type", "text")
		_ = cmd.Flags().Set(flag, "fixture")
		if _, err := messagesSendContentType(shortcut.RuntimeContextForTest(cmd, MessagesSend)); err == nil {
			t.Fatalf("conflicting %s type accepted", flag)
		}
	}
}

func TestCrossPlatformCoverageOptimizationResourceRefreshAndNames(t *testing.T) {
	for _, tc := range []struct {
		name        string
		metadata    string
		failRefresh bool
		ext         string
	}{{"url-name", "", false, ".bin"}, {"metadata-name", `,"fileName":"report.dat"`, false, ".dat"}, {"refresh-failure", "", true, ".bin"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			f := &larkAlignmentCaller{responses: map[string]string{"drive/download_file": `{"result":{"resourceUrl":"https://download.dingtalk.com/file.bin"` + tc.metadata + `}}`}}
			if tc.failRefresh {
				f.failProductToolAt = map[string]int{"drive/download_file": 2}
			}
			calls := 0
			testseam.Swap(t, &resourceSecureClient, func() *http.Client {
				return &http.Client{Transport: optimizationRoundTrip(func(r *http.Request) (*http.Response, error) {
					calls++
					status := 200
					body := "verified"
					if calls == 1 {
						status = 503
						body = ""
					}
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
				})}
			})
			out, err := runChatParity(t, f, "+messages-resource-download", "--resource-id", "file", "--type", "fileId", "--output", "artifact", "--part-size", "4096", "--retries", "1", "--retry-delay", "0")
			if (err != nil) != tc.failRefresh {
				t.Fatalf("%s err=%v calls=%#v", out, err, f.calls)
			}
			data, e := os.ReadFile("artifact" + tc.ext)
			if !tc.failRefresh && (e != nil || string(data) != "verified") {
				t.Fatalf("bad file %q %v", data, e)
			}
			if tc.failRefresh && !os.IsNotExist(e) {
				t.Fatal("partial file published")
			}
		})
	}
}
