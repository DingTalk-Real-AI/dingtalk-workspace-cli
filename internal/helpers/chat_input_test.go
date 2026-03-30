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

package helpers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// inputCaptureRunner records invocations with a call counter.
type inputCaptureRunner struct {
	last   executor.Invocation
	called int
}

func (r *inputCaptureRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.last = inv
	r.called++
	return executor.Result{Invocation: inv}, nil
}

func writeInputTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}

// newInputTestChatRoot builds the full chat command tree for testing.
func newInputTestChatRoot(t *testing.T, runner executor.Runner) *cobra.Command {
	t.Helper()
	h := chatHandler{}
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(h.Command(runner))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	return root
}

// ---------------------------------------------------------------------------
// send-by-bot: --text @file
// ---------------------------------------------------------------------------

func TestSendByBotTextFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "msg.md")
	writeInputTestFile(t, filePath, []byte("# Weekly Report\n\nAll green."))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "周报",
		"--text", "@" + filePath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.called != 1 {
		t.Fatalf("runner called = %d, want 1", runner.called)
	}
	if runner.last.Params["markdown"] != "# Weekly Report\n\nAll green." {
		t.Errorf("params[markdown] = %q, want file content", runner.last.Params["markdown"])
	}
}

func TestSendByBotTitleFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "title.txt")
	writeInputTestFile(t, filePath, []byte("Dynamic Title"))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "@" + filePath,
		"--text", "content here",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["title"] != "Dynamic Title" {
		t.Errorf("params[title] = %q, want %q", runner.last.Params["title"], "Dynamic Title")
	}
}

func TestSendByBotTextAndTitleBothFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	titlePath := filepath.Join(dir, "title.txt")
	textPath := filepath.Join(dir, "body.md")
	writeInputTestFile(t, titlePath, []byte("File Title"))
	writeInputTestFile(t, textPath, []byte("File Body"))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "@" + titlePath,
		"--text", "@" + textPath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["title"] != "File Title" {
		t.Errorf("params[title] = %q, want %q", runner.last.Params["title"], "File Title")
	}
	if runner.last.Params["markdown"] != "File Body" {
		t.Errorf("params[markdown] = %q, want %q", runner.last.Params["markdown"], "File Body")
	}
}

func TestSendByBotTextFromFileMissingReturnsError(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "test",
		"--text", "@/nonexistent/file.md",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail for missing @file")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error should mention --text, got: %v", err)
	}
	if runner.called != 0 {
		t.Error("runner should not be called on @file error")
	}
}

func TestSendByBotTextUTF8Preserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "chinese.md")
	content := "你好世界 🌍\n第二行"
	writeInputTestFile(t, filePath, []byte(content))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "测试",
		"--text", "@" + filePath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["markdown"] != content {
		t.Errorf("params[markdown] = %q, want %q", runner.last.Params["markdown"], content)
	}
}

// ---------------------------------------------------------------------------
// send-by-bot: backward compatibility (plain --text)
// ---------------------------------------------------------------------------

func TestSendByBotPlainTextStillWorks(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "test",
		"--text", "plain message",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["markdown"] != "plain message" {
		t.Errorf("params[markdown] = %q, want %q", runner.last.Params["markdown"], "plain message")
	}
}

func TestSendByBotSingleChatStillWorks(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--users", "u001,u002",
		"--title", "test",
		"--text", "hello",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Tool != "batch_send_robot_msg_to_users" {
		t.Errorf("tool = %q, want batch_send_robot_msg_to_users", runner.last.Tool)
	}
}

// ---------------------------------------------------------------------------
// send-by-bot: validation still works
// ---------------------------------------------------------------------------

func TestSendByBotEmptyTextStillErrors(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--group", "G001",
		"--title", "test",
		// --text not provided
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail when --text is empty")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error should mention --text, got: %v", err)
	}
}

func TestSendByBotMissingGroupAndUsersStillErrors(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-bot",
		"--robot-code", "BOT001",
		"--title", "test",
		"--text", "hello",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail when --group and --users both missing")
	}
}

// ---------------------------------------------------------------------------
// send-by-webhook: --text @file and --title @file
// ---------------------------------------------------------------------------

func TestWebhookTextFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "alert.md")
	writeInputTestFile(t, filePath, []byte("CPU > 90%"))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-webhook",
		"--token", "TOKEN001",
		"--title", "告警",
		"--text", "@" + filePath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["text"] != "CPU > 90%" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "CPU > 90%")
	}
}

func TestWebhookTitleFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "title.txt")
	writeInputTestFile(t, filePath, []byte("Alert Title"))

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-webhook",
		"--token", "TOKEN001",
		"--title", "@" + filePath,
		"--text", "body content",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["title"] != "Alert Title" {
		t.Errorf("params[title] = %q, want %q", runner.last.Params["title"], "Alert Title")
	}
}

func TestWebhookPlainTextStillWorks(t *testing.T) {
	t.Parallel()

	runner := &inputCaptureRunner{}
	cmd := newInputTestChatRoot(t, runner)
	cmd.SetArgs([]string{"chat", "message", "send-by-webhook",
		"--token", "TOKEN001",
		"--title", "test",
		"--text", "plain webhook message",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.last.Params["text"] != "plain webhook message" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "plain webhook message")
	}
}

// ---------------------------------------------------------------------------
// resolveStringFlag unit tests
// ---------------------------------------------------------------------------

func TestResolveStringFlagPlainValue(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("text", "", "")
	cmd.SetArgs([]string{"--text", "hello"})
	_ = cmd.Execute()

	guard := cli.NewStdinGuard()
	val, err := resolveStringFlag(cmd, "text", guard, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("got %q, want %q", val, "hello")
	}
}

func TestResolveStringFlagAtFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.txt")
	writeInputTestFile(t, filePath, []byte("file content"))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("body", "", "")
	cmd.SetArgs([]string{"--body", "@" + filePath})
	_ = cmd.Execute()

	guard := cli.NewStdinGuard()
	val, err := resolveStringFlag(cmd, "body", guard, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "file content" {
		t.Errorf("got %q, want %q", val, "file content")
	}
	if guard.Claimed() {
		t.Error("@file should not claim stdin")
	}
}

func TestResolveStringFlagAtFileMissing(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("text", "", "")
	cmd.SetArgs([]string{"--text", "@/no/such/file"})
	_ = cmd.Execute()

	guard := cli.NewStdinGuard()
	_, err := resolveStringFlag(cmd, "text", guard, false)
	if err == nil {
		t.Fatal("expected error for missing @file")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error should mention flag name, got: %v", err)
	}
}

func TestResolveStringFlagPrimaryContentNoStdinInTerminal(t *testing.T) {
	t.Parallel()

	// In go test context, stdin is a terminal — primary content fallback
	// should NOT read stdin (StdinIsPipe returns false).
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("text", "", "")
	cmd.SetArgs([]string{})
	_ = cmd.Execute()

	guard := cli.NewStdinGuard()
	val, err := resolveStringFlag(cmd, "text", guard, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("got %q, want empty (no stdin pipe in terminal)", val)
	}
	if guard.Claimed() {
		t.Error("guard should not be claimed in terminal context")
	}
}

func TestResolveStringFlagExplicitValueBlocksStdinFallback(t *testing.T) {
	t.Parallel()

	// When --text has an explicit value, primaryContent stdin fallback
	// should NOT activate even if it's the primary flag.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("text", "", "")
	cmd.SetArgs([]string{"--text", "explicit"})
	_ = cmd.Execute()

	guard := cli.NewStdinGuard()
	val, err := resolveStringFlag(cmd, "text", guard, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "explicit" {
		t.Errorf("got %q, want %q", val, "explicit")
	}
	if guard.Claimed() {
		t.Error("explicit value should not claim stdin")
	}
}
