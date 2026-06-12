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

package compat

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ── deduplicateForwardBoundary unit tests ─────────────────────

func TestDeduplicateForwardBoundary_RemovesBoundaryMessages(t *testing.T) {
	input := map[string]any{
		"hasMore": true,
		"messages": []any{
			map[string]any{"openMessageId": "msg3", "createTime": "2026-05-22 10:00:00"},
			map[string]any{"openMessageId": "msg2", "createTime": "2026-05-22 09:26:59"},
			map[string]any{"openMessageId": "msg1", "createTime": "2026-05-22 09:26:59"},
		},
	}
	data, _ := json.Marshal(input)

	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	msgs := parsed["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after dedup, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["openMessageId"] != "msg3" {
		t.Fatalf("expected msg3 to remain, got %v", msgs[0])
	}
}

func TestDeduplicateForwardBoundary_NestedUnderResult(t *testing.T) {
	input := map[string]any{
		"result": map[string]any{
			"hasMore": true,
			"messages": []any{
				map[string]any{"openMessageId": "msg2", "createTime": "2026-05-22 09:26:59"},
				map[string]any{"openMessageId": "msg1", "createTime": "2026-05-22 09:00:00"},
			},
		},
	}
	data, _ := json.Marshal(input)

	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")

	var parsed map[string]any
	json.Unmarshal(result, &parsed)
	msgs := parsed["result"].(map[string]any)["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after dedup, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["openMessageId"] != "msg1" {
		t.Fatalf("expected msg1 to remain, got %v", msgs[0])
	}
}

func TestDeduplicateForwardBoundary_NoOverlap(t *testing.T) {
	input := map[string]any{
		"hasMore": false,
		"messages": []any{
			map[string]any{"openMessageId": "msg3", "createTime": "2026-05-22 10:00:00"},
			map[string]any{"openMessageId": "msg2", "createTime": "2026-05-22 09:30:00"},
		},
	}
	data, _ := json.Marshal(input)

	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")

	// No messages match anchor → returned unchanged
	if string(result) != string(data) {
		t.Fatal("expected data unchanged when no boundary overlap")
	}
}

func TestDeduplicateForwardBoundary_EmptyMessages(t *testing.T) {
	input := map[string]any{
		"hasMore":  false,
		"messages": []any{},
	}
	data, _ := json.Marshal(input)

	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")
	if string(result) != string(data) {
		t.Fatal("expected data unchanged for empty messages")
	}
}

func TestDeduplicateForwardBoundary_EmptyAnchor(t *testing.T) {
	data := []byte(`{"messages":[{"openMessageId":"msg1","createTime":"2026-05-22 09:26:59"}]}`)
	result := deduplicateForwardBoundary(data, "")
	if string(result) != string(data) {
		t.Fatal("expected data unchanged when anchor is empty")
	}
}

func TestDeduplicateForwardBoundary_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")
	if string(result) != string(data) {
		t.Fatal("expected data unchanged for invalid JSON")
	}
}

func TestDeduplicateForwardBoundary_NilData(t *testing.T) {
	result := deduplicateForwardBoundary(nil, "2026-05-22 09:26:59")
	if result != nil {
		t.Fatal("expected nil for nil data")
	}
}

func TestDeduplicateForwardBoundary_NoMessagesKey(t *testing.T) {
	data := []byte(`{"hasMore":true,"other":123}`)
	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")
	if string(result) != string(data) {
		t.Fatal("expected data unchanged when no messages key")
	}
}

func TestDeduplicateForwardBoundary_NumericCreateTime(t *testing.T) {
	// 2026-05-22 09:26:59 local → some unix millis value
	input := map[string]any{
		"messages": []any{
			map[string]any{"openMessageId": "msg2", "createTime": float64(1747876019000)},
			map[string]any{"openMessageId": "msg1", "createTime": float64(1747876000000)},
		},
	}
	data, _ := json.Marshal(input)

	// The numeric comparison path requires matching exact millis.
	// Parse the anchor to millis first to know the expected value.
	anchorMs, err := parseTimeToMillis("2026-05-22 09:26:59")
	if err != nil {
		t.Skipf("cannot parse anchor time on this platform: %v", err)
	}

	// Rebuild input with the correct millis
	input["messages"] = []any{
		map[string]any{"openMessageId": "msg2", "createTime": float64(anchorMs)},
		map[string]any{"openMessageId": "msg1", "createTime": float64(anchorMs - 19000)},
	}
	data, _ = json.Marshal(input)

	result := deduplicateForwardBoundary(data, "2026-05-22 09:26:59")

	var parsed map[string]any
	json.Unmarshal(result, &parsed)
	msgs := parsed["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after numeric dedup, got %d", len(msgs))
	}
}

// ── messageTimeMatchesAnchor unit tests ───────────────────────

func TestMessageTimeMatchesAnchor_StringMatch(t *testing.T) {
	msg := map[string]any{"createTime": "2026-05-22 09:26:59"}
	if !messageTimeMatchesAnchor(msg, "2026-05-22 09:26:59") {
		t.Fatal("expected string match")
	}
}

func TestMessageTimeMatchesAnchor_StringMismatch(t *testing.T) {
	msg := map[string]any{"createTime": "2026-05-22 09:30:00"}
	if messageTimeMatchesAnchor(msg, "2026-05-22 09:26:59") {
		t.Fatal("expected no match")
	}
}

func TestMessageTimeMatchesAnchor_NoCreateTime(t *testing.T) {
	msg := map[string]any{"openMessageId": "msg1"}
	if messageTimeMatchesAnchor(msg, "2026-05-22 09:26:59") {
		t.Fatal("expected no match when createTime absent")
	}
}

func TestMessageTimeMatchesAnchor_WhitespaceTolerant(t *testing.T) {
	msg := map[string]any{"createTime": " 2026-05-22 09:26:59 "}
	if !messageTimeMatchesAnchor(msg, "2026-05-22 09:26:59") {
		t.Fatal("expected match with whitespace trimming")
	}
}

// ── installChatHook composition tests ─────────────────────────

func newChatMessageListStub() *cobra.Command {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().Bool("forward", false, "")
	cmd.Flags().String("time", "", "")
	return cmd
}

func TestInstallChatHook_NoOpForOtherProduct(t *testing.T) {
	cmd := newChatMessageListStub()
	originalCalled := false
	cmd.RunE = func(*cobra.Command, []string) error {
		originalCalled = true
		return nil
	}
	installChatHook(cmd, "todo", "list_conversation_message_v2")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !originalCalled {
		t.Fatal("original RunE should still run when hook skips")
	}
}

func TestInstallChatHook_NoOpForNonTargetTool(t *testing.T) {
	cmd := newChatMessageListStub()
	installChatHook(cmd, "chat", "send_personal_message")

	if cmd.RunE != nil {
		// RunE was set by the stub; hook should not have wrapped it
		// because the tool is not in chatMessageListTools.
		t.Fatal("hook should not modify RunE for non-target tools")
	}
}

func TestInstallChatHook_TargetToolForwardFalse(t *testing.T) {
	cmd := newChatMessageListStub()
	_ = cmd.Flags().Set("forward", "false")

	runCalled := false
	cmd.RunE = func(c *cobra.Command, args []string) error {
		runCalled = true
		var buf bytes.Buffer
		c.SetOut(&buf)
		// Simulate writing JSON output
		json.NewEncoder(&buf).Encode(map[string]any{"messages": []any{}})
		buf.WriteTo(c.OutOrStdout())
		return nil
	}

	installChatHook(cmd, "chat", "list_conversation_message_v2")

	var out bytes.Buffer
	cmd.SetOut(&out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !runCalled {
		t.Fatal("original RunE should be called")
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no warning for forward=false, got: %s", stderr.String())
	}
}

func TestInstallChatHook_TargetToolForwardTrue(t *testing.T) {
	cmd := newChatMessageListStub()
	_ = cmd.Flags().Set("forward", "true")
	_ = cmd.Flags().Set("time", "2026-05-22 09:26:59")

	cmd.RunE = func(c *cobra.Command, args []string) error {
		payload := map[string]any{
			"hasMore": true,
			"messages": []any{
				map[string]any{"openMessageId": "msg2", "createTime": "2026-05-22 09:26:59"},
				map[string]any{"openMessageId": "msg1", "createTime": "2026-05-22 09:00:00"},
			},
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(payload)
	}

	installChatHook(cmd, "chat", "list_conversation_message_v2")

	var out bytes.Buffer
	cmd.SetOut(&out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Check stderr warning
	if !strings.Contains(stderr.String(), "forward=true") {
		t.Fatalf("expected forward=true warning in stderr, got: %s", stderr.String())
	}

	// Check output is deduplicated
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	msgs := parsed["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after dedup, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["openMessageId"] != "msg1" {
		t.Fatalf("expected msg1 to remain, got %v", msgs[0])
	}
}

func TestInstallChatHook_ChainsExistingRunE(t *testing.T) {
	cmd := newChatMessageListStub()
	_ = cmd.Flags().Set("forward", "false")
	originalCalled := false
	cmd.RunE = func(*cobra.Command, []string) error {
		originalCalled = true
		return nil
	}
	installChatHook(cmd, "chat", "list_conversation_message_v2")

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !originalCalled {
		t.Fatal("original RunE was dropped")
	}
}

func TestInstallChatHook_NilCmdSafe(t *testing.T) {
	installChatHook(nil, "chat", "list_conversation_message_v2")
}

func TestInstallChatHook_ListDirectConversationTool(t *testing.T) {
	cmd := newChatMessageListStub()
	_ = cmd.Flags().Set("forward", "true")
	_ = cmd.Flags().Set("time", "2026-05-22 09:26:59")

	cmd.RunE = func(c *cobra.Command, args []string) error {
		payload := map[string]any{
			"messages": []any{
				map[string]any{"openMessageId": "msg1", "createTime": "2026-05-22 09:26:59"},
			},
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(payload)
	}

	installChatHook(cmd, "chat", "list_direct_conversation_message_v2")

	var out bytes.Buffer
	cmd.SetOut(&out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(out.Bytes(), &parsed)
	msgs := parsed["messages"].([]any)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after dedup, got %d", len(msgs))
	}
}

func TestInstallChatHook_RunEPreservedOnError(t *testing.T) {
	cmd := newChatMessageListStub()
	_ = cmd.Flags().Set("forward", "true")
	_ = cmd.Flags().Set("time", "2026-05-22 09:26:59")

	cmd.RunE = func(c *cobra.Command, args []string) error {
		json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"error": "boom"})
		return apperrors.NewValidation("test error")
	}

	installChatHook(cmd, "chat", "list_conversation_message_v2")

	var out bytes.Buffer
	cmd.SetOut(&out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "test error") {
		t.Fatalf("expected original error to bubble, got %v", err)
	}
	// Output should still be written (passthrough on error)
	if out.Len() == 0 {
		t.Fatal("expected output to be written even on error")
	}
	// No warning on error path
	if stderr.Len() > 0 {
		t.Fatalf("expected no warning on error, got: %s", stderr.String())
	}
}
