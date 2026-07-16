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

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// contactUser is the minimal identity a smart shortcut needs after resolving a
// name.
type contactUser struct {
	userID         string
	openDingTalkID string
	name           string
}

// resolveUser turns a human name into a single unique contact, the "name → ID"
// intelligence that distinguishes a smart shortcut from a raw tool call
// (mirrors the shared name→ID resolver). It searches the directory and:
//   - errors with a clear message if nobody matches;
//   - errors listing the candidates if the name is ambiguous (never guesses).
func resolveUser(rt *shortcut.RuntimeContext, name string) (contactUser, error) {
	data, err := rt.CallMCPData("contact", "search_contact_by_key_word", map[string]any{
		"keyword": name,
	})
	if err != nil {
		return contactUser{}, err
	}
	users := extractUsers(data)
	switch {
	case len(users) == 0:
		return contactUser{}, apperrors.NewValidation(
			fmt.Sprintf("通讯录里没找到叫 %q 的人；换个更完整的姓名再试。", name))
	case len(users) > 1:
		return contactUser{}, apperrors.NewValidation(fmt.Sprintf(
			"%q 匹配到 %d 个人：%s。请用更精确的姓名，或直接用对应命令传 userId。",
			name, len(users), strings.Join(userLabels(users), "、")))
	}
	return users[0], nil
}

// extractUsers pulls {userId, openDingTalkId, name} out of a
// search_contact_by_key_word response ({"result": [ {userId, name, ...} ]}).
func extractUsers(data map[string]any) []contactUser {
	raw, ok := data["result"].([]any)
	if !ok {
		return nil
	}
	var out []contactUser
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["userId"].(string)
		if id == "" {
			continue
		}
		nm, _ := m["name"].(string)
		openID, _ := m["openDingTalkId"].(string)
		if openID == "" {
			openID, _ = m["openDingtalkId"].(string)
		}
		out = append(out, contactUser{userID: id, openDingTalkID: openID, name: nm})
	}
	return out
}

func userLabels(users []contactUser) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, fmt.Sprintf("%s(%s)", u.name, u.userID))
	}
	return out
}
