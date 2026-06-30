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

package auth

import (
	"strings"
	"testing"
)

// agentCodeSignalEnvs is every env DetectAgentCode consults. Tests clear them
// all so each case starts clean (the suite itself runs under a real host).
var agentCodeSignalEnvs = []string{
	AgentCodeEnv,
	"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT",
	"OPENCLAW_BUNDLE_ROOT", "OPENCLAW_RUNTIME_ROLE",
	"HERMES_HOME", "CODEX_SANDBOX",
	"VSCODE_BRAND", "__CFBundleIdentifier",
	"TERM_PROGRAM", "DWS_CHANNEL",
}

func clearAgentCodeEnv(t *testing.T) {
	t.Helper()
	for _, k := range agentCodeSignalEnvs {
		t.Setenv(k, "")
	}
}

func TestDetectAgentCode_HostDeclaration_T0(t *testing.T) {
	clearAgentCodeEnv(t)
	t.Setenv(AgentCodeEnv, "QoderWork")
	code, sig := DetectAgentCode()
	if code != "QoderWork" {
		t.Fatalf("want verbatim QoderWork, got %q", code)
	}
	if !strings.HasPrefix(sig, "env:"+AgentCodeEnv) {
		t.Fatalf("want env signal, got %q", sig)
	}
}

func TestDetectAgentCode_VerifiedSignatures_T1(t *testing.T) {
	cases := []struct {
		env, val, want string
	}{
		{"CLAUDECODE", "1", "claudecode"},
		{"CLAUDE_CODE_ENTRYPOINT", "cli", "claudecode"},
		{"OPENCLAW_BUNDLE_ROOT", "/Users/x/.openclaw-bundle", "openclaw"},
		{"HERMES_HOME", "/Users/x/.hermes", "hermes"},
		{"CODEX_SANDBOX", "seatbelt", "codex"},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			clearAgentCodeEnv(t)
			t.Setenv(c.env, c.val)
			code, sig := DetectAgentCode()
			if code != c.want {
				t.Fatalf("%s=%s: want %q, got %q", c.env, c.val, c.want, code)
			}
			if !strings.HasPrefix(sig, "sig:") {
				t.Fatalf("want sig:* signal, got %q", sig)
			}
		})
	}
}

func TestDetectAgentCode_VSCodeBrand_T2(t *testing.T) {
	cases := map[string]string{
		"Qoder":              "qoder",
		"Cursor":             "cursor",
		"Visual Studio Code": "vscode",
		"Windsurf":           "windsurf",
		"Trae":               "trae",
		"SomeNewFork":        "somenewfork", // generic coverage of future forks
	}
	for brand, want := range cases {
		t.Run(brand, func(t *testing.T) {
			clearAgentCodeEnv(t)
			t.Setenv("VSCODE_BRAND", brand)
			code, sig := DetectAgentCode()
			if code != want {
				t.Fatalf("VSCODE_BRAND=%q: want %q, got %q", brand, want, code)
			}
			if sig != "env:VSCODE_BRAND" {
				t.Fatalf("want env:VSCODE_BRAND signal, got %q", sig)
			}
		})
	}
}

func TestDetectAgentCode_BundleID_T3(t *testing.T) {
	cases := map[string]string{
		"com.qoder.ide":                 "qoder",
		"com.todesktop.230313mzl4w4u92": "cursor",
		"com.microsoft.VSCode":          "vscode",
		"com.workbuddy.workbuddy":       "workbuddy",
	}
	for id, want := range cases {
		t.Run(id, func(t *testing.T) {
			clearAgentCodeEnv(t)
			t.Setenv("__CFBundleIdentifier", id)
			code, sig := DetectAgentCode()
			if code != want {
				t.Fatalf("bundle %q: want %q, got %q", id, want, code)
			}
			if !strings.HasPrefix(sig, "bundle:") {
				t.Fatalf("want bundle:* signal, got %q", sig)
			}
		})
	}
}

// An unknown bundle id (e.g. a plain terminal) must NOT be labeled.
func TestDetectAgentCode_UnknownBundleIsEmpty(t *testing.T) {
	clearAgentCodeEnv(t)
	t.Setenv("__CFBundleIdentifier", "com.googlecode.iterm2")
	code, _ := DetectAgentCode()
	if code != "" {
		t.Fatalf("unknown bundle must be empty, got %q", code)
	}
}

func TestDetectAgentCode_FallbackEmpty(t *testing.T) {
	clearAgentCodeEnv(t)
	code, sig := DetectAgentCode()
	if code != "" {
		t.Fatalf("want empty code, got %q", code)
	}
	if sig != "" {
		t.Fatalf("want empty signal, got %q", sig)
	}
}

// TERM_PROGRAM and DWS_CHANNEL must never decide agent_code.
func TestDetectAgentCode_IgnoresNoise(t *testing.T) {
	clearAgentCodeEnv(t)
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("DWS_CHANNEL", "Qoderwork")
	code, _ := DetectAgentCode()
	if code != "" {
		t.Fatalf("noise must not decide agent_code; want empty, got %q", code)
	}
}

// Precedence: explicit declaration (T0) > env signature (T1) > VSCODE_BRAND
// (T2). A CLI agent running inside an IDE reports the CLI agent.
func TestDetectAgentCode_Precedence(t *testing.T) {
	clearAgentCodeEnv(t)
	t.Setenv("CLAUDECODE", "1")       // T1
	t.Setenv("VSCODE_BRAND", "Qoder") // T2
	if code, _ := DetectAgentCode(); code != "claudecode" {
		t.Fatalf("T1 must beat T2, got %q", code)
	}
	t.Setenv(AgentCodeEnv, "workbuddy") // T0
	if code, _ := DetectAgentCode(); code != "workbuddy" {
		t.Fatalf("T0 must beat all, got %q", code)
	}
}

func TestNormalizeAgentCode(t *testing.T) {
	cases := map[string]string{
		"claude":             "claudecode",
		"Claude-Code":        "claudecode",
		"CLAUDECODE":         "claudecode",
		"Qoderwork":          "QoderWork",
		"WorkBuddy":          "workbuddy",
		"Visual Studio Code": "vscode",
		"Cursor":             "cursor",
		"":                   "",
		"some-new-ide":       "some-new-ide",
	}
	for in, want := range cases {
		if got := normalizeAgentCode(in); got != want {
			t.Errorf("normalizeAgentCode(%q) = %q, want %q", in, got, want)
		}
	}
}
