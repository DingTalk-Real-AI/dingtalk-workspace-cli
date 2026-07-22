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

package smart

import "testing"

func TestExtractUsers(t *testing.T) {
	data := map[string]any{
		"result": []any{
			// in-org contact: full identity
			map[string]any{"userId": "024083", "name": "朱鸿", "openDingTalkId": "DHUGO"},
			// external / cross-org contact: no userId, only openDingTalkId +
			// display name under a non-"name" key — must be kept, not dropped.
			map[string]any{"openDingtalkId": "DEXT", "nick": "外部小王"},
			// unusable row (no identity at all) is skipped
			map[string]any{"name": "无ID的人"},
			// non-map entry is skipped
			"garbage",
		},
	}
	users := extractUsers(data)
	if len(users) != 2 {
		t.Fatalf("extractUsers kept %d, want 2: %#v", len(users), users)
	}
	if users[0].userID != "024083" || users[0].openDingTalkID != "DHUGO" || users[0].name != "朱鸿" {
		t.Errorf("in-org user = %#v", users[0])
	}
	// external contact kept via openDingTalkId, name resolved from "nick"
	if users[1].userID != "" || users[1].openDingTalkID != "DEXT" || users[1].name != "外部小王" {
		t.Errorf("external user = %#v", users[1])
	}
}

func TestExtractUsersNoResult(t *testing.T) {
	if got := extractUsers(map[string]any{}); got != nil {
		t.Errorf("no result should be nil, got %#v", got)
	}
}
