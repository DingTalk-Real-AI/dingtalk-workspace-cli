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
	"strconv"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

		data, err := rt.CallMCPReadData("todo", "get_user_todos_in_current_org", params)
		if err != nil {
			return nil, err
		}
		cards, hasMore, err := shortcutTodoCardsStrict(data)
		if err != nil {
			return nil, err
		}
		all = append(all, cards...)
		if !hasMore {
			return all, nil
		}
	}
	return nil, shortcutTodoResponseError("pagination_limit_reached", fmt.Sprintf("连续 %d 页仍返回 hasMore=true，拒绝把不完整列表当作完整结果", todoMaxPages))
}

func shortcutTodoCardsStrict(data map[string]any) ([]map[string]any, bool, error) {
	if len(data) == 0 {
		return nil, false, shortcutTodoResponseError("empty_tool_response", "服务返回空响应")
	}
	if raw, ok := data["success"]; ok {
		success, valid := raw.(bool)
		if !valid || !success {
			return nil, false, shortcutTodoResponseError("remote_failure", "响应 success 不是明确的 true")
		}
	}
	container := data
	for depth := 0; depth < 5; depth++ {
		if raw, ok := container["todoCards"]; ok {
			items, valid := raw.([]any)
			if !valid {
				return nil, false, shortcutTodoResponseError("malformed_collection", "todoCards 不是数组")
			}
			rawMore, exists := container["hasMore"]
			hasMore, valid := rawMore.(bool)
			if !exists || !valid {
				return nil, false, shortcutTodoResponseError("malformed_has_more", "响应缺少布尔 hasMore")
			}
			cards := make([]map[string]any, 0, len(items))
			for i, item := range items {
				card, valid := item.(map[string]any)
				if !valid {
					return nil, false, shortcutTodoResponseError("malformed_item", fmt.Sprintf("todoCards 第 %d 项不是对象", i))
				}
				if shortcutTodoTaskID(card) == "" {
					return nil, false, shortcutTodoResponseError("missing_stable_id", fmt.Sprintf("todoCards 第 %d 项缺少稳定 taskId", i))
				}
				cards = append(cards, card)
			}
			return cards, hasMore, nil
		}
		raw, ok := container["result"]
		if !ok {
			raw, ok = container["data"]
		}
		if !ok {
			return nil, false, shortcutTodoResponseError("missing_collection", "响应缺少 todoCards 容器")
		}
		child, valid := raw.(map[string]any)
		if !valid {
			return nil, false, shortcutTodoResponseError("malformed_wrapper", "result/data 包装层不是对象")
		}
		container = child
	}
	return nil, false, shortcutTodoResponseError("missing_collection", "响应包装过深或缺少 todoCards")
}

func shortcutTodoResponseError(reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation("todo/get_user_todos_in_current_org"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}
