// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chat

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageConversationListTopProjectNormalizesAndFiltersType(t *testing.T) {
	data := map[string]any{
		"result": map[string]any{
			"items": []any{
				map[string]any{
					"openConversationId": "cid-direct",
					"title":              "张三",
					"singleChat":         true,
				},
				map[string]any{
					"openConversationId": "cid-group",
					"title":              "项目群",
					"singleChat":         false,
				},
				map[string]any{
					"openConversationId": "cid-legacy",
					"title":              "旧版单聊",
					"conversationType":   "P2P",
				},
				map[string]any{
					"openConversationId": "cid-unknown",
					"title":              "未知类型",
				},
			},
		},
	}

	all := conversationListTopProject(data)
	if len(all) != 4 {
		t.Fatalf("all conversations = %#v", all)
	}
	if got := all[0]["conversationType"]; got != "direct" {
		t.Errorf("singleChat=true type = %#v, want direct", got)
	}
	if got := all[1]["conversationType"]; got != "group" {
		t.Errorf("singleChat=false type = %#v, want group", got)
	}
	if got := all[2]["conversationType"]; got != "direct" {
		t.Errorf("legacy P2P type = %#v, want direct", got)
	}
	if _, ok := all[3]["conversationType"]; ok {
		t.Errorf("unknown type was fabricated: %#v", all[3])
	}

	if got, want := conversationListTopFilter(all, "group"), []map[string]any{all[1]}; !reflect.DeepEqual(got, want) {
		t.Errorf("group filter = %#v, want %#v", got, want)
	}
	if got, want := conversationListTopFilter(all, "direct"), []map[string]any{all[0], all[2]}; !reflect.DeepEqual(got, want) {
		t.Errorf("direct filter = %#v, want %#v", got, want)
	}
	if got := conversationListTopFilter(all, "all"); !reflect.DeepEqual(got, all) {
		t.Errorf("all filter changed rows: %#v", got)
	}
}

func TestCrossPlatformCoverageConversationListTopRejectsInvalidType(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+conversation-list-top", "--type", "bot"})
	if err := root.Execute(); err == nil {
		t.Fatal("invalid --type unexpectedly succeeded")
	}
	if fake.tool != "" {
		t.Fatalf("invalid --type reached lower tool %s/%s", fake.product, fake.tool)
	}
}

func TestCrossPlatformCoverageConversationListProjectUnwrapsGatewayTuple(t *testing.T) {
	data := map[string]any{
		"result": []any{
			[]any{map[string]any{"openConversationId": "cid-1", "title": "项目群"}},
			float64(2),
			true,
		},
	}
	if got := conversationListProject(data); len(got) != 1 || got[0]["openConversationId"] != "cid-1" {
		t.Fatalf("conversation tuple projection = %#v", got)
	}
	if got := conversationListTopProject(data); len(got) != 1 || got[0]["openConversationId"] != "cid-1" {
		t.Fatalf("top tuple projection = %#v", got)
	}
}

func TestCrossPlatformCoverageConversationListPageAllFollowsTypedCursor(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/list_all_conversations": {
			`{"result":{"conversationList":[{"openConversationId":"cid-1","title":"一"}],"hasMore":true,"nextCursor":2}}`,
			`{"result":{"conversationList":[{"openConversationId":"cid-2","title":"二"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+conversation-list", "--page-all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1].args["cursor"] != int64(2) {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageConversationListSinglePagePreservesTypedCursor(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_all_conversations": `{"result":{"conversationList":[{"openConversationId":"cid-1","title":"一"}],"hasMore":true,"nextCursor":2}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+conversation-list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one page", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["hasMore"] != true || payload["nextCursor"] != float64(2) {
		t.Fatalf("pagination payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageConversationListMaxItemsPublishesStableTruncation(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_all_conversations": `{"result":{"conversationList":[{"openConversationId":"cid-1"},{"openConversationId":"cid-2"}],"hasMore":true,"nextCursor":2}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+conversation-list", "--page-all", "--max-items", "1", "--page-delay", "0"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["truncated"] != true ||
		payload["truncatedByResultLimit"] != true || payload["stopReason"] != "result_limit" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageConversationListDeduplicatesStableIDs(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_all_conversations": `{"result":{"conversationList":[{"openConversationId":"cid-1","title":"一"},{"openConversationId":"cid-1","title":"重复"}],"hasMore":false}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+conversation-list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) {
		t.Fatalf("deduplicated payload = %#v", payload)
	}
}
