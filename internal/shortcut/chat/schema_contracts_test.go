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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageChatCategoryAndGroupRoutingBoundary(t *testing.T) {
	tests := []struct {
		name string
		item shortcut.Shortcut
		want string
	}{
		{name: "create real group", item: ChatCreate, want: "不是创建会话分组/分类"},
		{name: "search real group", item: ChatSearch, want: "不是搜索会话分组/分类"},
		{name: "list categories", item: CategoryList, want: "不是查看群聊/聊天群列表"},
		{name: "create category", item: CategoryCreate, want: "不是创建群聊/聊天群"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.item.Intent, tc.want) {
				t.Fatalf("%s intent = %q, want routing boundary %q", tc.item.Command, tc.item.Intent, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageReviewedChatShortcutContracts(t *testing.T) {
	base := shortcut.Shortcut{
		Service:     "chat",
		Command:     "+chat-create",
		Aliases:     []string{"+chat-create-alias"},
		Description: "创建群聊",
		Intent:      "需要创建群聊时使用",
		Risk:        shortcut.RiskWrite,
		Tips: []string{
			"dws chat +chat-create --title demo",
			"dws chat +chat-create --title demo2",
			"dws chat +chat-create --title ignored",
		},
	}
	got := withReviewedChatShortcutContracts(base)
	if len(got) != 1 || got[0].Contract.Empty() {
		t.Fatalf("reviewed contract = %#v", got)
	}
	declared := got[0]
	if declared.Safety.Effect != "write" || declared.Safety.Confirmation != "user_required" {
		t.Fatalf("write safety = %#v", declared.Safety)
	}
	if declared.Contract.Identity.CanonicalPath != "chat.shortcut_chat_create" ||
		declared.Contract.Identity.PrimaryCLIPath != "chat +chat-create" ||
		len(declared.Contract.Identity.Aliases) != 1 ||
		declared.Contract.Identity.Aliases[0] != "chat +chat-create-alias" {
		t.Fatalf("identity = %#v", declared.Contract.Identity)
	}
	if len(declared.Contract.Selection.Examples) != 2 {
		t.Fatalf("examples = %#v, want reviewed maximum of two", declared.Contract.Selection.Examples)
	}

	high := base
	high.Risk = shortcut.RiskHighWrite
	if safety := withReviewedChatShortcutContracts(high)[0].Safety; safety.Effect != "destructive" || safety.Risk != "high" {
		t.Fatalf("high-write safety = %#v", safety)
	}
	read := base
	read.Risk = shortcut.RiskRead
	read.Intent = ""
	if declared := withReviewedChatShortcutContracts(read)[0]; declared.Safety.Effect != "read" || declared.Contract.Description != read.Description {
		t.Fatalf("read/fallback declaration = %#v", declared)
	}

	unreviewed := base
	unreviewed.Command = "+future-unreviewed"
	if declared := withReviewedChatShortcutContracts(unreviewed)[0]; !declared.Contract.Empty() {
		t.Fatalf("unreviewed future shortcut entered Schema: %#v", declared.Contract)
	}
	explicit := base
	explicit.Contract = corecmd.ContractDecl{Description: "preserve"}
	if declared := withReviewedChatShortcutContracts(explicit)[0]; declared.Contract.Description != "preserve" {
		t.Fatalf("explicit contract overwritten: %#v", declared.Contract)
	}
}
