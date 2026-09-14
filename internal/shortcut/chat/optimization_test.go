package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

func TestCrossPlatformCoverageOptimizationCreateDefaultsAndGate(t *testing.T) {
	fake := &larkAlignmentCaller{}
	_, err := runChatParity(t, fake, "+chat-create")
	if err == nil || len(fake.calls) != 0 {
		t.Fatalf("confirmation must precede calls: %v %#v", err, fake.calls)
	}
	_, err = runChatParity(t, fake, "+chat-create", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	call := fake.calls[len(fake.calls)-1]
	if call.tool != "create_group_conversation" {
		t.Fatalf("default payload %#v", call)
	}
	if _, supplied := call.args["groupName"]; supplied {
		t.Fatal("default creation must omit groupName")
	}
	members, ok := call.args["groupMembers"].([]string)
	if !ok || len(members) != 1 {
		t.Fatalf("default members %#v", call.args)
	}
}

func TestCrossPlatformCoverageOptimizationExactContextStopsWrongTarget(t *testing.T) {
	for _, cmd := range []string{"+flag-create", "+flag-cancel", "+message-read-users"} {
		f := &larkAlignmentCaller{}
		_, err := runChatParity(t, f, cmd, "--message-id", "msg", "--conversation-id", "wrong", "--yes")
		if err == nil || len(f.calls) != 1 || f.calls[0].tool != "list_messages_by_ids" {
			t.Fatalf("%s: %v %#v", cmd, err, f.calls)
		}
	}
}

func TestCrossPlatformCoverageOptimizationResourceAliasesAndTypes(t *testing.T) {
	data := `{"result":{"messages":[{"openMessageId":"msg","openConversationId":"cid","resources":[{"resourceId":"media","resourceIdType":"mediaId","resourceType":"image"}]}]}}`
	f := &larkAlignmentCaller{dryRun: true, responses: map[string]string{"im/list_messages_by_ids": data}}
	out, err := runChatParity(t, f, "+messages-resources-download", "--file-key", "media", "--message-id", "msg", "--type", "image", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err = json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	if p["resourceId"] != "media" || p["resourceType"] != "mediaId" || p["openConversationId"] != "cid" {
		t.Fatalf("bad alias/route %s", out)
	}
	if len(f.calls) != 1 || f.calls[0].tool != "list_messages_by_ids" {
		t.Fatal(f.calls)
	}
	f = &larkAlignmentCaller{dryRun: true, responses: map[string]string{"im/list_messages_by_ids": data}}
	_, err = runChatParity(t, f, "+messages-resources-download", "--resource-id", "other", "--message-id", "msg", "--type", "image", "--dry-run")
	if err == nil || len(f.calls) != 1 {
		t.Fatalf("wrong resource accepted %v", err)
	}
	refs := chatmsg.Resources(map[string]any{"resources": []any{map[string]any{"resourceId": "media", "resourceIdType": "mediaId", "resourceType": "image"}}})
	if len(refs) != 1 || refs[0]["type"] != "mediaId" || refs[0]["contentType"] != "image" {
		t.Fatal(refs)
	}
}

func TestCrossPlatformCoverageOptimizationUnknownIsNotAbsent(t *testing.T) {
	f := &larkAlignmentCaller{category: `{"result":{"conversations":[{"openConversationId":"cid-a"}]}}`}
	out, err := runChatParity(t, f, "+feed-group-query-item", "--feed-group-id", "1", "--feed-id", "cid-a,cid-missing,cid-a", "--no-detail")
	if err == nil {
		t.Fatal("unknown must not exit success")
	}
	var p map[string]any
	if e := json.Unmarshal([]byte(out), &p); e != nil {
		t.Fatal(e)
	}
	if p["complete"] != false || p["notFoundCount"] != float64(0) || p["foundCount"] != float64(1) || p["unresolvedCount"] != float64(1) || p["requestedCount"] != float64(2) {
		t.Fatal(p)
	}
	for _, data := range []map[string]any{{}, {"items": nil}, {"items": []any{1}}, {"items": map[string]any{}}} {
		if _, err := StrictChatCollection(data, "items"); err == nil {
			t.Fatalf("invalid collection accepted %#v", data)
		}
	}
	if rows, err := StrictChatCollection(map[string]any{"items": []any{}}, "items"); err != nil || len(rows) != 0 {
		t.Fatalf("explicit empty: %v %#v", err, rows)
	}
}

func TestCrossPlatformCoverageOptimizationNativeTextRoute(t *testing.T) {
	f := &larkAlignmentCaller{}
	_, err := runChatParity(t, f, "+messages-send", "--chat-id", "cid", "--text", "**literal**", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	c := f.calls[len(f.calls)-1]
	var body map[string]string
	if c.args["msgType"] != "text" {
		t.Fatal(c.args)
	}
	if err = json.Unmarshal([]byte(fmt.Sprint(c.args["content"])), &body); err != nil || body["content"] != "**literal**" || body["text"] != "" {
		t.Fatalf("wrong native payload %#v %v", body, err)
	}
}

type optimizationRoundTrip func(*http.Request) (*http.Response, error)

func (f optimizationRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCrossPlatformCoverageOptimizationRangeRecoveryAndVersionGuard(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			calls, resolves := 0, 0
			client := &http.Client{Transport: optimizationRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				start := 0
				if strings.HasPrefix(req.Header.Get("Range"), "bytes=4-") {
					start = 4
				}
				if start == 4 && calls == 2 {
					return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				version := `"v1"`
				if mismatch && start == 4 {
					version = `"v2"`
				}
				body := "abcd"
				if start == 4 {
					body = "efgh"
				}
				return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": []string{fmt.Sprintf("bytes %d-%d/8", start, start+3)}, "Etag": []string{version}}, ContentLength: 4, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			dir := t.TempDir()
			dest := filepath.Join(dir, "result")
			n, err := downloadWithRecovery(context.Background(), client, "https://files.example.com/object", nil, dest, false, 4, 1, 0, func() (string, map[string]string, error) {
				resolves++
				return "https://files.example.com/object-new", nil, nil
			})
			if resolves != 1 || calls != 3 {
				t.Fatalf("bounded retries: %d %d", calls, resolves)
			}
			if mismatch {
				if err == nil {
					t.Fatal("mixed version accepted")
				}
				if _, e := os.Stat(dest); !os.IsNotExist(e) {
					t.Fatal("partial published")
				}
			} else {
				data, e := os.ReadFile(dest)
				if err != nil || e != nil || n != 8 || string(data) != "abcdefgh" {
					t.Fatalf("download %d %v %v", n, err, e)
				}
			}
			files, _ := os.ReadDir(dir)
			for _, file := range files {
				if strings.Contains(file.Name(), "range-") {
					t.Fatal("temporary residue")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOptimizationValidationMakesNoCalls(t *testing.T) {
	for _, args := range [][]string{
		{"+chat-search", "--query", "x", "--member-ids", "invalid"},
		{"+chat-search", "--query", "x", "--chat-modes", "invalid"},
		{"+chat-search", "--query", "x", "--page-delay", "60001"},
		{"+messages-resources-download", "--file-key", "f", "--part-size", "1"},
		{"+messages-resources-download", "--file-key", "f", "--part-size", "4096", "--retries", "4"},
		{"+messages-resources-download", "--file-key", "f", "--retry-delay", "1"},
		{"+messages-reply", "--message-id", "m", "--content", "x", "--create-thread", "--yes"},
		{"+messages-reply", "--message-id", "m", "--content", "x", "--create-thread", "--reply-in-thread", "--thread-id", "t", "--yes"},
	} {
		f := &larkAlignmentCaller{}
		_, err := runChatParity(t, f, args...)
		if err == nil || len(f.calls) != 0 {
			t.Fatalf("%v err=%v calls=%#v", args, err, f.calls)
		}
	}
}
func TestCrossPlatformCoverageOptimizationSortAndProjection(t *testing.T) {
	rows := []map[string]any{{"openConversationId": "b", "createAt": 2, "memberCount": 1, "lastMsgCreateAt": 3}, {"openConversationId": "a", "createAt": 1, "memberCount": 3, "lastMsgCreateAt": 4}}
	for _, kind := range []string{"create_time", "active_time", "member_count"} {
		if err := sortVerifiedGroups(rows, kind, true); err != nil || rows[0]["openConversationId"] != "a" {
			t.Fatalf("%s %#v %v", kind, rows, err)
		}
	}
	for _, bad := range []map[string]any{{"openConversationId": "x"}, {"createAt": 1}, {"openConversationId": "x", "createAt": "NaN"}} {
		if sortVerifiedGroups([]map[string]any{bad}, "create_time", true) == nil {
			t.Fatal(bad)
		}
	}
}

func TestCrossPlatformCoverageOptimizationRangeFullResponseRestartsAtomically(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: optimizationRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		h := http.Header{}
		body := "new-full"
		status := 200
		if calls == 1 {
			status = 206
			body = "old-"
			h.Set("ETag", `"old"`)
			h.Set("Content-Range", "bytes 0-3/8")
		}
		if calls == 3 && r.Header.Get("Range") != "" {
			t.Fatal("whole restart retained range")
		}
		return &http.Response{StatusCode: status, Header: h, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	dest := filepath.Join(t.TempDir(), "out")
	n, err := downloadWithRecovery(context.Background(), client, "https://files.example.com/object", nil, dest, false, 4, 0, 0, nil)
	data, _ := os.ReadFile(dest)
	if err != nil || n != 8 || string(data) != "new-full" || calls != 3 {
		t.Fatalf("mixed/restarted data: %q %d %v calls=%d", data, n, err, calls)
	}
}

func TestCrossPlatformCoverageOptimizationThreadBudgetIncludesEmptyThreads(t *testing.T) {
	f := &larkAlignmentCaller{responses: map[string]string{"chat/list_topic_replies": `{"result":{"messages":[],"hasMore":false}}`}}
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	cmd, _, err := root.Find([]string{"chat", "+messages-mget"})
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("no-reactions", "true")
	rt := shortcut.RuntimeContextForTest(cmd, MessagesMget)
	rows := []map[string]any{}
	for i := 0; i < 51; i++ {
		rows = append(rows, map[string]any{"openMessageId": fmt.Sprintf("m%d", i), "openConversationId": "cid", "openConvThreadId": fmt.Sprintf("t%d", i)})
	}
	ledger, _, _ := EnrichMessageDetails(rt, rows)
	if len(f.calls) != 50 || ledger["threadRequests"] != 50 || ledger["complete"] != false {
		t.Fatalf("empty threads escaped budget: %#v calls=%d", ledger, len(f.calls))
	}
}

func TestCrossPlatformCoverageCreateNameDelegatesOnlyMissingName(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		supplied bool
		want     string
	}{
		{name: "omitted"},
		{name: "empty", args: []string{"--name", ""}},
		{name: "whitespace", args: []string{"--name", "  \t"}},
		{name: "explicit", args: []string{"--name", "Project team"}, supplied: true, want: "Project team"},
		{name: "framework trims explicit spacing", args: []string{"--name", " Project team "}, supplied: true, want: "Project team"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			args := append([]string{"+chat-create", "--yes"}, tc.args...)
			if _, err := runChatParity(t, fake, args...); err != nil {
				t.Fatal(err)
			}
			call := fake.calls[len(fake.calls)-1]
			got, present := call.args["groupName"]
			if call.tool != "create_group_conversation" || present != tc.supplied || (present && got != tc.want) {
				t.Fatalf("groupName=%#v present=%v; want %q present=%v", got, present, tc.want, tc.supplied)
			}
		})
	}
}

func TestCrossPlatformCoverageExactFeedQueryPreservesIntegerCategoryID(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{name: "primary", args: []string{"--category-id", "42", "--conversation-ids", "cid-a"}, want: 42},
		{name: "aliases", args: []string{"--feed-group-id", "4242", "--feed-id", "cid-a"}, want: 4242},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{category: `{"result":{"conversations":[{"openConversationId":"cid-a"}],"hasMore":false}}`}
			args := append([]string{"+feed-group-query-item", "--no-detail"}, tc.args...)
			out, err := runChatParity(t, fake, args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 || fake.calls[0].product != "im" || fake.calls[0].tool != "list_conversations_by_category" {
				t.Fatalf("unexpected calls: %#v", fake.calls)
			}
			value, ok := fake.calls[0].args["categoryId"].(int)
			if !ok || value != tc.want {
				t.Fatalf("categoryId = %#v (%T), want integer %d", fake.calls[0].args["categoryId"], fake.calls[0].args["categoryId"], tc.want)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["foundCount"] != float64(1) || payload["complete"] != true {
				t.Fatalf("positive query not verified: %s", out)
			}
		})
	}
}
