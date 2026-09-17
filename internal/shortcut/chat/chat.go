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

// Package chat provides declarative shortcuts for the DingTalk chat service
// (群聊 / 会话 / 消息 / 机器人). Tool names and parameter keys are copied verbatim
// from internal/helpers/chat.go, the single source of truth for the real MCP
// tools. Shortcuts route to the correct MCP server via each Shortcut.Product:
//   - "chat" (default): tools invoked via the plain helper callMCPTool path
//   - "im":   tools invoked via callMCPToolOnServer("im", ...)
//   - "bot":  tools invoked via callMCPToolOnServer("bot", ...)
package chat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// isOpenID classifies mixed userId/openDingTalkId inputs using the canonical
// current-D wire format shared with helpers and smart shortcuts.
func isOpenID(v string) bool {
	return targetresolver.LooksLikeCurrentDOpenDingTalkID(v)
}

// splitIDs partitions a mixed list of userId / openDingTalkId values, mirroring
// helpers.splitChatIDValues.
func splitIDs(vals []string) (userIDs, openIDs []string) {
	for _, raw := range vals {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if isOpenID(v) {
			openIDs = append(openIDs, v)
		} else {
			userIDs = append(userIDs, v)
		}
	}
	return userIDs, openIDs
}

func validateExplicitOpenIDs(flagName string, values []string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := targetresolver.ValidateExplicitOpenDingTalkID(flagName, value); err != nil {
			return err
		}
	}
	return nil
}

// toInt64Slice converts string values to []int64, mirroring helpers.parseCSVInt64.
func toInt64Slice(vals []string) ([]int64, error) {
	out := make([]int64, 0, len(vals))
	for _, raw := range vals {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", v)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one ID is required")
	}
	return out, nil
}
