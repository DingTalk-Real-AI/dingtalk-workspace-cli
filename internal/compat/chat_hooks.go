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

// chat_hooks.go — CLI-side post-processing for the `chat` product's message
// list commands. The upstream API returns messages where createTime >= T when
// forward=true (inclusive boundary), causing infinite pagination loops when
// the caller uses the last message's createTime as the next page's --time.
// This hook detects forward=true, deduplicates boundary messages whose
// createTime matches the --time anchor, and emits a stderr warning recommending
// forward=false pagination. See https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/430.

package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// chatMessageListTools lists every chat toolName whose response contains a
// "messages" array subject to the forward=true boundary overlap.
var chatMessageListTools = map[string]bool{
	"list_conversation_message_v2":        true,
	"list_direct_conversation_message_v2": true,
}

// installChatHook wires chat-specific RunE post-processing onto leaf commands
// emitted by BuildDynamicCommands. It is a no-op for non-chat products and
// for chat tools that do not need boundary dedup.
//
// The hook wraps the existing RunE (installed by NewDirectCommand) to capture
// JSON output, remove duplicate boundary messages when forward=true, and emit
// a pagination hint to stderr.
func installChatHook(cmd *cobra.Command, canonicalProduct, toolName string) {
	if cmd == nil {
		return
	}
	if strings.TrimSpace(canonicalProduct) != "chat" {
		return
	}
	if !chatMessageListTools[toolName] {
		return
	}
	wrapRunEForForwardDedup(cmd)
}

// wrapRunEForForwardDedup wraps the command's RunE to intercept JSON output
// when --forward=true, deduplicate boundary messages, and emit a warning.
func wrapRunEForForwardDedup(cmd *cobra.Command) {
	original := cmd.RunE
	if original == nil {
		return
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		forward, _ := c.Flags().GetBool("forward")
		if !forward {
			return original(c, args)
		}

		timeAnchor, _ := c.Flags().GetString("time")

		origOut := c.OutOrStdout()
		var buf bytes.Buffer
		c.SetOut(&buf)

		runErr := original(c, args)
		c.SetOut(origOut)

		if runErr != nil {
			buf.WriteTo(origOut)
			return runErr
		}

		fmt.Fprint(c.ErrOrStderr(),
			"⚠ forward=true 翻页时 API 返回 createTime >= T 的消息（含边界），"+
				"使用最后一条 createTime 作为下页 --time 会无限循环。"+
				"建议改用 --forward=false 从新往老翻页。\n")

		output := deduplicateForwardBoundary(buf.Bytes(), timeAnchor)
		_, writeErr := origOut.Write(output)
		return writeErr
	}
}

// deduplicateForwardBoundary parses JSON output and removes messages whose
// createTime matches the anchor time, preventing infinite pagination loops
// when forward=true. Returns the original data unchanged when no duplicates
// are found or when parsing fails.
func deduplicateForwardBoundary(data []byte, anchorTime string) []byte {
	if anchorTime == "" || len(data) == 0 {
		return data
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}

	messages := findMessagesArray(payload)
	if messages == nil {
		return data
	}

	filtered := make([]any, 0, len(messages))
	removed := 0
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			filtered = append(filtered, msg)
			continue
		}
		if messageTimeMatchesAnchor(m, anchorTime) {
			removed++
			continue
		}
		filtered = append(filtered, msg)
	}

	if removed == 0 {
		return data
	}

	setMessagesArray(payload, filtered)
	result, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return result
}

// findMessagesArray locates the "messages" array in the payload, handling
// both top-level and nested-under-"result" response shapes.
func findMessagesArray(payload map[string]any) []any {
	if msgs, ok := payload["messages"].([]any); ok {
		return msgs
	}
	if inner, ok := payload["result"].(map[string]any); ok {
		if msgs, ok := inner["messages"].([]any); ok {
			return msgs
		}
	}
	return nil
}

// setMessagesArray writes the filtered messages back to the same nesting level
// where findMessagesArray found them.
func setMessagesArray(payload map[string]any, msgs []any) {
	if _, ok := payload["messages"]; ok {
		payload["messages"] = msgs
		return
	}
	if inner, ok := payload["result"].(map[string]any); ok {
		if _, ok := inner["messages"]; ok {
			inner["messages"] = msgs
		}
	}
}

// messageTimeMatchesAnchor checks whether a message's createTime field
// matches the pagination anchor time. Handles both string and numeric
// (millisecond timestamp) createTime values.
func messageTimeMatchesAnchor(msg map[string]any, anchor string) bool {
	ct, ok := msg["createTime"]
	if !ok {
		return false
	}
	switch v := ct.(type) {
	case string:
		return strings.TrimSpace(v) == strings.TrimSpace(anchor)
	case float64:
		anchorMs, err := parseTimeToMillis(anchor)
		if err != nil {
			return false
		}
		return int64(v) == anchorMs
	}
	return false
}

// parseTimeToMillis converts a "yyyy-MM-dd HH:mm:ss" string to unix
// milliseconds in the local timezone, matching the --time flag format.
func parseTimeToMillis(s string) (int64, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), time.Local); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("cannot parse time: %q", s)
}
