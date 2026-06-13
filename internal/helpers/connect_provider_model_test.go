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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeClaudeSettings drops a $CLAUDE_CONFIG_DIR/settings.json whose env block
// carries a custom provider base URL, mirroring how cc-switch and similar tools
// store third-party Anthropic-compatible credentials (issue #10 / #14).
func writeClaudeSettings(t *testing.T, baseURL string) string {
	t.Helper()
	dir := t.TempDir()
	body := `{"env":{"ANTHROPIC_BASE_URL":"` + baseURL + `","ANTHROPIC_AUTH_TOKEN":"sk-test"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	return dir
}

// TestForwarderProviderBaseURLDropsHaiku is the issue #14 root-cause fix: when a
// custom provider base URL is injected and the user did not pick a model, the
// built-in --model claude-haiku-... pin must be dropped (the provider may not
// map it, returning a 422). An explicit --agent-model still wins, and an
// official-login user (no base URL) keeps the pin.
func TestForwarderProviderBaseURLDropsHaiku(t *testing.T) {
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	t.Setenv("DWS_AGENT_CMD", "")
	t.Setenv("DWS_AGENT_MODEL", "")
	// Make sure the connector's own env does not carry a base URL for the
	// no-provider subtest.
	t.Setenv("ANTHROPIC_BASE_URL", "")
	os.Unsetenv("ANTHROPIC_BASE_URL")

	stub := t.TempDir()
	if err := writeExecStub(stub, "claude"); err != nil {
		t.Fatalf("stub claude: %v", err)
	}
	t.Setenv("PATH", stub)

	argvOf := func(fwd forwarder) string {
		ef, ok := fwd.(*execForwarder)
		if !ok {
			t.Fatalf("not an execForwarder: %T", fwd)
		}
		return strings.Join(ef.argv, " ")
	}
	streamArgvOf := func(fwd forwarder) string {
		ef := fwd.(*execForwarder)
		return strings.Join(ef.streamArgv, " ")
	}

	// 1) Provider base URL injected via Claude settings, no --agent-model:
	//    haiku pin dropped from both argv and streamArgv.
	cfg := writeClaudeSettings(t, "https://my-proxy.example.com")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	fwd, err := forwarderForChannel("claudecode", "", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("forwarder (provider): %v", err)
	}
	if a := argvOf(fwd); strings.Contains(a, "--model") || strings.Contains(a, "haiku") {
		t.Fatalf("provider+no-model: argv should drop haiku pin, got: %s", a)
	}
	if a := streamArgvOf(fwd); strings.Contains(a, "--model") || strings.Contains(a, "haiku") {
		t.Fatalf("provider+no-model: streamArgv should drop haiku pin, got: %s", a)
	}

	// 2) Provider base URL injected but user picked a model: user's model wins.
	fwd2, err := forwarderForChannel("claudecode", "", connectAgentOptions{Model: "my-custom-model"})
	if err != nil {
		t.Fatalf("forwarder (provider+model): %v", err)
	}
	if a := argvOf(fwd2); !strings.Contains(a, "--model my-custom-model") {
		t.Fatalf("provider+model: argv should carry user model, got: %s", a)
	}
	if a := argvOf(fwd2); strings.Contains(a, "haiku") {
		t.Fatalf("provider+model: haiku pin should be replaced, got: %s", a)
	}

	// 3) No provider (official login): built-in haiku pin kept.
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()) // empty dir -> no settings.json
	fwd3, err := forwarderForChannel("claudecode", "", connectAgentOptions{})
	if err != nil {
		t.Fatalf("forwarder (official): %v", err)
	}
	if a := argvOf(fwd3); !strings.Contains(a, "--model claude-haiku-4-5-20251001") {
		t.Fatalf("official login: built-in haiku pin must be kept, got: %s", a)
	}
}

// TestForwardReturnsHintOnAPIError covers the issue #14 anti-echo guard: an
// agent that prints "API Error: 422 ..." to stdout (and exits 0) must NOT have
// that raw error forwarded as the answer; forward returns an actionable hint.
func TestForwardReturnsHintOnAPIError(t *testing.T) {
	stub := t.TempDir()
	bin := filepath.Join(stub, "fakeagent")
	// Print a provider 422 to stdout and exit 0, exactly like claude does when
	// the upstream provider rejects the model.
	script := "#!/bin/sh\necho 'API Error: 422 未找到匹配的自定义供应商配置，请检查模型名称'\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agent: %v", err)
	}

	f := &execForwarder{name: "claudecode", argv: []string{bin}, timeout: 10_000_000_000}
	got, err := f.forward(context.Background(), "conv-1", "hi")
	if err != nil {
		t.Fatalf("forward returned err: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(got), "API Error:") {
		t.Fatalf("raw backend error was forwarded as the answer: %q", got)
	}
	if !strings.Contains(got, "AI 后端调用失败") || !strings.Contains(got, "--agent-model") {
		t.Fatalf("expected actionable hint, got: %q", got)
	}

	// A normal reply is untouched.
	okBin := filepath.Join(stub, "okagent")
	if err := os.WriteFile(okBin, []byte("#!/bin/sh\necho '你好，这是正常回复'\n"), 0o755); err != nil {
		t.Fatalf("write ok agent: %v", err)
	}
	f2 := &execForwarder{name: "claudecode", argv: []string{okBin}, timeout: 10_000_000_000}
	got2, err := f2.forward(context.Background(), "conv-2", "hi")
	if err != nil {
		t.Fatalf("forward (normal) err: %v", err)
	}
	if strings.TrimSpace(got2) != "你好，这是正常回复" {
		t.Fatalf("normal reply altered: %q", got2)
	}
}

// TestAgentBackendErrorHelpers locks the heuristic and message shape.
func TestAgentBackendErrorHelpers(t *testing.T) {
	if !agentReplyIsError("API Error: 422 boom") {
		t.Fatal("API Error prefix should be detected")
	}
	if agentReplyIsError("这是正常回复，里面提到 API Error 不算") {
		t.Fatal("mid-text mention must not be flagged")
	}
	msg := agentBackendErrorReply("API Error: 422 line one\nline two ignored")
	if strings.Contains(msg, "line two") {
		t.Fatalf("only first line should be kept: %q", msg)
	}
	if !strings.Contains(msg, "--agent-model") {
		t.Fatalf("hint must mention --agent-model: %q", msg)
	}
}

// TestStripModelArg covers removing a flag+value and the absent-flag no-op.
func TestStripModelArg(t *testing.T) {
	got := stripModelArg([]string{"claude", "-p", "--model", "haiku", "--strict-mcp-config"}, "--model")
	want := "claude -p --strict-mcp-config"
	if strings.Join(got, " ") != want {
		t.Fatalf("strip: got %v, want %s", got, want)
	}
	noop := stripModelArg([]string{"claude", "-p"}, "--model")
	if strings.Join(noop, " ") != "claude -p" {
		t.Fatalf("no-op: got %v", noop)
	}
}
