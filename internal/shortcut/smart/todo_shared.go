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
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// todoPageSize is the per-page fetch size for get_user_todos_in_current_org.
// The real backend currently returns an empty page for pageSize=50 while
// pageSize=20 works, so keep this aligned with the public +get-my-tasks default
// instead of pushing the maximum. todoMaxPages caps total pages so a runaway
// list can never loop unbounded (40 * 20 = 800 todos, well past any realistic
// backlog a single user filters/matches against).
const (
	todoPageSize = 20
	todoMaxPages = 40
)

// shortcutListAllTodoCards pages through get_user_todos_in_current_org, merging
// every page's todoCards into one slice so callers match/filter across the FULL
// list instead of silently seeing only the first page. base carries the call's
// fixed params (roleTypes, date window, todoStatus, …); pageNum / pageSize are
// managed here. Paging stops when a page returns fewer than pageSize cards or
// the safety cap is hit.
func shortcutListAllTodoCards(rt *shortcut.RuntimeContext, base map[string]any) ([]map[string]any, error) {
	var all []map[string]any
	for page := 1; page <= todoMaxPages; page++ {
		params := make(map[string]any, len(base)+2)
		for k, v := range base {
			params[k] = v
		}
		params["pageNum"] = strconv.Itoa(page)
		params["pageSize"] = strconv.Itoa(todoPageSize)

		data, err := rt.CallMCPData("todo", "get_user_todos_in_current_org", params)
		if err != nil {
			return nil, err
		}
		cards := shortcutTodoCards(data)
		all = append(all, cards...)
		if len(cards) < todoPageSize {
			break
		}
	}
	return all, nil
}
