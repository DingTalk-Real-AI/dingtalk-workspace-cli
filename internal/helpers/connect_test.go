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

import "testing"

// clearChannelEnv clears every env var that participates in channel detection
// (empty == treated as unset), so the test host's own QODER_CLI etc. cannot
// leak into a case. t.Setenv restores them when the case ends.
func clearChannelEnv(t *testing.T) {
	for _, k := range []string{
		"DWS_AGENT_CHANNEL", "DINGTALK_AGENT", "OPENCLAW", "OPENCLAW_GATEWAY",
		"HERMES_AGENT", "HERMES", "QODER_CLI", "QODERCLI_INTEGRATION_MODE", "DWS_CONNECT_CMD",
		"WORKBUDDY_CONFIG_DIR", "WORKBUDDY_APP_NAME", "CLAUDECODE",
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
	for _, ch := range []string{"openclaw", "qoder", "qoderwork", "hermes", "workbuddy", "claudecode"} {
		if _, ok := connectChannels[ch]; !ok {
			t.Errorf("channel %q should be in connectChannels", ch)
		}
	}
	if _, ok := connectChannels["weird"]; ok {
		t.Error("unknown channel should not be in connectChannels")
	}
}

func TestBuildConnectPlanMethod(t *testing.T) {
	want := map[string]string{
		"openclaw":   "openclaw-connector",
		"qoder":      "stream-bridge",
		"qoderwork":  "stream-bridge",
		"hermes":     "official-channel",
		"workbuddy":  "stream-bridge",
		"claudecode": "stream-bridge",
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
		for _, ch := range []string{"qoder", "qoderwork", "claudecode", "workbuddy"} {
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

func TestForwarderForChannel(t *testing.T) {
	clearChannelEnv(t)
	// exec-type channels (fresh one-shot CLI): execForwarder.
	for _, ch := range []string{"qoder", "claudecode"} {
		fwd, err := forwarderForChannel(ch)
		if err != nil {
			t.Fatalf("forwarderForChannel(%q) err = %v", ch, err)
		}
		if _, ok := fwd.(*execForwarder); !ok {
			t.Errorf("channel %q should yield *execForwarder, got %T", ch, fwd)
		}
	}
	// session-bridge channels (reach the current live session via bridge):
	// httpForwarder, overridable via per-channel env vars.
	t.Setenv("WB_GATEWAY", "http://localhost:9999")
	t.Setenv("WB_MODEL", "wb-model")
	t.Setenv("QW_GATEWAY", "http://localhost:8888")
	t.Setenv("QW_MODEL", "qw-model")
	for _, tc := range []struct{ ch, url, model string }{
		{"workbuddy", "http://localhost:9999/v1/chat/completions", "wb-model"},
		{"qoderwork", "http://localhost:8888/v1/chat/completions", "qw-model"},
	} {
		fwd, err := forwarderForChannel(tc.ch)
		if err != nil {
			t.Fatalf("forwarderForChannel(%q) err = %v", tc.ch, err)
		}
		hf, ok := fwd.(*httpForwarder)
		if !ok {
			t.Fatalf("%s should yield *httpForwarder, got %T", tc.ch, fwd)
		}
		if hf.url != tc.url {
			t.Errorf("%s gateway not applied: url = %q, want %q", tc.ch, hf.url, tc.url)
		}
		if hf.model != tc.model {
			t.Errorf("%s model not applied: model = %q, want %q", tc.ch, hf.model, tc.model)
		}
	}
	// DWS_AGENT_CMD overrides exec argv.
	t.Setenv("DWS_AGENT_CMD", "custom-cli --foo")
	cf, _ := forwarderForChannel("qoder")
	ef := cf.(*execForwarder)
	if !equalStringSlice(ef.argv, []string{"custom-cli", "--foo"}) {
		t.Errorf("DWS_AGENT_CMD not applied: argv = %v", ef.argv)
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
