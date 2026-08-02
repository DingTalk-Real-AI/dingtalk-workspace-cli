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
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestConversationListTopProjectNormalizesAndFiltersType(t *testing.T) {
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

func TestConversationListTopRejectsInvalidType(t *testing.T) {
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
