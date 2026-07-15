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
	"os"
	"path/filepath"
	"testing"
)

// clearChannelEnv clears every env var that participates in channel detection
// (empty == treated as unset), so the test host's own QODER_CLI etc. cannot
// leak into a case. t.Setenv restores them when the case ends.
func clearChannelEnv(t *testing.T) {
	for _, k := range []string{
		"DWS_AGENT_CHANNEL", "DINGTALK_AGENT", "OPENCLAW", "OPENCLAW_GATEWAY",
		"HERMES_AGENT", "HERMES", "QODER_CLI", "QODERCLI_INTEGRATION_MODE",
		"DWS_CONNECT_CMD", "DWS_AGENT_CMD",
		"WORKBUDDY_CONFIG_DIR", "WORKBUDDY_APP_NAME", "CLAUDECODE",
		"DWS_AGENT_PERMISSION_MODE", "DWS_AGENT_APPROVAL_MODE",
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_BASE_URL",
		"GOOGLE_GEMINI_API_BASE_URL", "GEMINI_MODEL",
	} {
		t.Setenv(k, "")
	}
}

func TestResolveConnectChannel(t *testing.T) {
	cases := []struct {
		name           string
		flag           string
		env            map[string]string
		wantChannel    string
		wantDetectedBy string
	}{
		{"explicit flag wins", "openclaw", map[string]string{"DWS_AGENT_CHANNEL": "qoder", "QODER_CLI": "1"}, "openclaw", "flag:--channel"},
		{"env overrides signal", "auto", map[string]string{"DWS_AGENT_CHANNEL": "qoderwork", "QODER_CLI": "1"}, "qoderwork", "env:DWS_AGENT_CHANNEL"},
		{"signal openclaw(DINGTALK_AGENT)", "auto", map[string]string{"DINGTALK_AGENT": "DING_DWS_CLAW"}, "openclaw", "signal:DINGTALK_AGENT"},
		{"signal openclaw(OPENCLAW)", "", map[string]string{"OPENCLAW": "1"}, "openclaw", "signal:OPENCLAW"},
		{"signal qoder family", "", map[string]string{"QODER_CLI": "1"}, "qoder", "signal:QODER_CLI"},
		{"signal qoderwork(INTEGRATION_MODE)", "auto", map[string]string{"QODERCLI_INTEGRATION_MODE": "qoder_work"}, "qoderwork", "signal:QODERCLI_INTEGRATION_MODE"},
		{"qoderwork precedes qoder/claudecode", "auto", map[string]string{"QODERCLI_INTEGRATION_MODE": "qoder_work", "QODER_CLI": "1", "CLAUDECODE": "1"}, "qoderwork", "signal:QODERCLI_INTEGRATION_MODE"},
		{"signal hermes", "auto", map[string]string{"HERMES_AGENT": "1"}, "hermes", "signal:HERMES"},
		{"signal workbuddy(WORKBUDDY_CONFIG_DIR)", "auto", map[string]string{"WORKBUDDY_CONFIG_DIR": "/Users/x/.workbuddy"}, "workbuddy", "signal:WORKBUDDY_CONFIG_DIR"},
		{"signal workbuddy(WORKBUDDY_APP_NAME)", "auto", map[string]string{"WORKBUDDY_APP_NAME": "WorkBuddy"}, "workbuddy", "signal:WORKBUDDY_CONFIG_DIR"},
		{"signal claudecode", "auto", map[string]string{"CLAUDECODE": "1"}, "claudecode", "signal:CLAUDECODE"},
		{"qoder fork precedes claudecode", "auto", map[string]string{"QODER_CLI": "1", "CLAUDECODE": "1"}, "qoder", "signal:QODER_CLI"},
		{"custom via DWS_AGENT_CMD", "auto", map[string]string{"DWS_AGENT_CMD": "lobster -p"}, "custom", "env:DWS_AGENT_CMD"},
		{"explicit channel beats DWS_AGENT_CMD", "claudecode", map[string]string{"DWS_AGENT_CMD": "lobster -p"}, "claudecode", "flag:--channel"},
		{"undetected", "auto", nil, "", "undetected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearChannelEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			ch, by := resolveConnectChannel(tc.flag)
			if ch != tc.wantChannel || by != tc.wantDetectedBy {
				t.Fatalf("resolveConnectChannel(%q) = (%q,%q), want (%q,%q)", tc.flag, ch, by, tc.wantChannel, tc.wantDetectedBy)
			}
		})
	}
}

func TestConnectChannelsKnown(t *testing.T) {
	for _, ch := range []string{"openclaw", "qoder", "qoderwork", "hermes", "workbuddy", "claudecode", "codebuddy", "custom"} {
		if _, ok := connectChannels[ch]; !ok {
			t.Errorf("channel %q should be in connectChannels", ch)
		}
	}
	if _, ok := connectChannels["weird"]; ok {
		t.Error("unknown channel should not be in connectChannels")
	}
}

// TestCustomChannel covers the self-built / unsupported-agent escape hatch
// (issue #37): custom is a stream-bridge channel whose argv comes entirely from
// DWS_AGENT_CMD (set by --agent-cmd); without a command it errors clearly.
func TestCustomChannel(t *testing.T) {
	if !isStreamBridgeChannel("custom") {
		t.Fatal("custom should be a stream-bridge channel")
	}
	if got, _ := buildConnectPlan("custom", "cid", "rc")["method"].(string); got != "stream-bridge-custom" {
		t.Errorf("buildConnectPlan(custom).method = %q, want stream-bridge-custom", got)
	}
	t.Run("missing command errors", func(t *testing.T) {
		clearChannelEnv(t)
		if _, _, err := resolveExecAgent("custom"); err == nil {
			t.Error("custom without DWS_AGENT_CMD should error")
		}
	})
	t.Run("command wins", func(t *testing.T) {
		clearChannelEnv(t)
		t.Setenv("DWS_AGENT_CMD", "lobster -p")
		argv, _, err := resolveExecAgent("custom")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !equalStringSlice(argv, []string{"lobster", "-p"}) {
			t.Errorf("argv = %v, want [lobster -p]", argv)
		}
	})
}

func TestBuildConnectPlanMethod(t *testing.T) {
	want := map[string]string{
		"openclaw":   "openclaw-connector",
		"qoder":      "stream-bridge-qoder-stream",
		"qoderwork":  "stream-bridge-qoder-stream",
		"hermes":     "official-channel",
		"workbuddy":  "stream-bridge",
		"claudecode": "stream-bridge",
		"codebuddy":  "stream-bridge",
		"codex":      "stream-bridge-codex-app-server",
		"weird":      "unknown",
	}
	for ch, m := range want {
		got, _ := buildConnectPlan(ch, "cid", "rc")["method"].(string)
		if got != m {
			t.Errorf("buildConnectPlan(%q).method = %q, want %q", ch, got, m)
		}
	}
}

func TestConnectExternalCommand(t *testing.T) {
	t.Run("DWS_CONNECT_CMD override (applies to all channels)", func(t *testing.T) {
		clearChannelEnv(t)
		t.Setenv("DWS_CONNECT_CMD", "my-bridge --flag x")
		want := []string{"my-bridge", "--flag", "x"}
		for _, ch := range []string{"qoder", "workbuddy", "openclaw", "hermes"} {
			if got := connectExternalCommand(ch); !equalStringSlice(got, want) {
				t.Fatalf("channel %q: got %v, want %v", ch, got, want)
			}
		}
	})
	t.Run("stream-bridge channels go Go-native, no external command", func(t *testing.T) {
		clearChannelEnv(t)
		for _, ch := range []string{"qoder", "qoderwork", "claudecode", "codebuddy", "workbuddy"} {
			if got := connectExternalCommand(ch); got != nil {
				t.Errorf("stream-bridge channel %q should return nil (Go-native), got %v", ch, got)
			}
			if !isStreamBridgeChannel(ch) {
				t.Errorf("channel %q should be recognised as stream-bridge", ch)
			}
		}
	})
	t.Run("openclaw uses external gateway", func(t *testing.T) {
		clearChannelEnv(t)
		if got := connectExternalCommand("openclaw"); len(got) == 0 || got[0] != "openclaw" {
			t.Errorf("openclaw default should be openclaw ..., got %v", got)
		}
		if isStreamBridgeChannel("openclaw") {
			t.Error("openclaw should not be a stream-bridge channel")
		}
	})
	t.Run("hermes has no built-in command", func(t *testing.T) {
		clearChannelEnv(t)
		if got := connectExternalCommand("hermes"); got != nil {
			t.Errorf("hermes with no built-in command should return nil, got %v", got)
		}
	})
}

func TestResolveAgentYoloMode(t *testing.T) {
	cases := []struct {
		name    string
		flags   map[string]string
		env     map[string]string
		want    bool
		wantErr bool
	}{
		{"default yolo", nil, nil, true, false},
		{"permission bypass", map[string]string{"agent-permission-mode": "bypass"}, nil, true, false},
		{"permission ask", map[string]string{"agent-permission-mode": "ask", "yolo": "true"}, nil, false, false},
		{"approval yolo", map[string]string{"agent-approval-mode": "yolo"}, nil, true, false},
		{"short yolo", map[string]string{"yolo": "true"}, nil, true, false},
		{"env permission bypass", nil, map[string]string{"DWS_AGENT_PERMISSION_MODE": "bypass"}, true, false},
		{"env approval yolo", nil, map[string]string{"DWS_AGENT_APPROVAL_MODE": "yolo"}, true, false},
		{"invalid permission mode", map[string]string{"agent-permission-mode": "full"}, nil, false, true},
		{"invalid approval mode", map[string]string{"agent-approval-mode": "bypass"}, nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearChannelEnv(t)
			cmd := newDevAppRobotConnectCommand(nil)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			for k, v := range tc.flags {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatalf("set flag %s=%s: %v", k, v, err)
				}
			}
			got, err := resolveAgentYoloMode(cmd)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveAgentYoloMode err = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAgentYoloMode err = %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveAgentYoloMode = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestForwarderForChannel(t *testing.T) {
	clearChannelEnv(t)
	stub := t.TempDir()
	if err := writeExecStub(stub, "codex"); err != nil {
		t.Fatalf("write codex stub: %v", err)
	}
	t.Setenv("PATH", stub)
	// DWS_AGENT_CMD covers ordinary stream-bridge channels so the test does not
	// depend on those binaries being installed. Codex ignores it and stays on
	// app-server.
	t.Setenv("DWS_AGENT_CMD", "fake-cli --flag")
	for ch := range agentSpecs {
		fwd, err := forwarderForChannel(ch, "", connectAgentOptions{})
		if err != nil {
			t.Fatalf("forwarderForChannel(%q) err = %v", ch, err)
		}
		if ch == "codex" {
			if _, ok := fwd.(*codexAppServerForwarder); !ok {
				t.Errorf("codex should ignore DWS_AGENT_CMD and yield app-server, got %T", fwd)
			}
			continue
		}
		ef, ok := fwd.(*execForwarder)
		if !ok {
			t.Errorf("channel %q should yield *execForwarder, got %T", ch, fwd)
			continue
		}
		if !equalStringSlice(ef.argv, []string{"fake-cli", "--flag"}) {
			t.Errorf("channel %q DWS_AGENT_CMD not applied: argv = %v", ch, ef.argv)
		}
	}
	// Non-agent channel has no forwarder.
	if _, err := forwarderForChannel("openclaw", "", connectAgentOptions{}); err == nil {
		t.Error("openclaw should not yield a forwarder")
	}
}

func TestAgentSpecsCoverMainstreamAgents(t *testing.T) {
	// The mainstream agents the bot must support, all present as channels.
	for _, ch := range []string{
		"claudecode", "codex", "gemini", "opencode",
		"qoder", "qoderwork", "codebuddy", "workbuddy",
	} {
		spec, ok := agentSpecs[ch]
		if !ok {
			t.Errorf("agentSpecs missing channel %q", ch)
			continue
		}
		if ch != "gemini" && len(spec.bins) == 0 {
			t.Errorf("channel %q has no bins", ch)
		}
		if spec.hint == "" {
			t.Errorf("channel %q has no install hint", ch)
		}
		if _, ok := connectChannels[ch]; !ok {
			t.Errorf("channel %q not in connectChannels", ch)
		}
		if !isStreamBridgeChannel(ch) {
			t.Errorf("channel %q should be stream-bridge", ch)
		}
	}
}

func TestMsgDedup(t *testing.T) {
	d := newMsgDedup(3)
	if !d.first("a") {
		t.Fatal("first a should be new")
	}
	if d.first("a") {
		t.Fatal("second a should be a duplicate")
	}
	if !d.first("b") || !d.first("c") {
		t.Fatal("b and c should be new")
	}
	// seen now holds {a,b,c} == limit; the next new id triggers a reset.
	if !d.first("x") {
		t.Fatal("x should be new (and trigger reset at limit)")
	}
	// After the reset, a was evicted, so it is treated as new again.
	if !d.first("a") {
		t.Fatal("a should be new again after reset")
	}
}

func TestLocateBinary(t *testing.T) {
	// On PATH: sh exists on every unix test host.
	if _, ok := locateBinary([]string{"sh"}, nil); !ok {
		t.Error("sh should be found on PATH")
	}
	// Miss: nonexistent name + non-matching glob.
	if _, ok := locateBinary([]string{"definitely-not-a-real-binary-xyz"}, []string{"/no/such/path/*/nope"}); ok {
		t.Error("should not find a nonexistent binary")
	}
	// Glob hit: a real file matched by a wildcard dir.
	dir := t.TempDir()
	sub := filepath.Join(dir, "archX")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(sub, "mycli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := locateBinary([]string{"no-such"}, []string{filepath.Join(dir, "*", "mycli")})
	if !ok || got != bin {
		t.Errorf("glob locate = (%q,%v), want (%q,true)", got, ok, bin)
	}
}

func TestResolveExecAgentOverride(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_AGENT_CMD", "my-agent --foo bar")
	argv, env, err := resolveExecAgent("qoder")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !equalStringSlice(argv, []string{"my-agent", "--foo", "bar"}) {
		t.Errorf("override argv = %v", argv)
	}
	if env != nil {
		t.Errorf("override env should be nil, got %v", env)
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
