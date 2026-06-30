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

// agent_code_detect.go resolves the agent_code — which agent HOST is driving
// dws (claudecode / qoder / cursor / vscode / openclaw / hermes / ...). It
// fills the x-dingtalk-dws-agent-code header for per-channel statistics.
//
// SEPARATE axis from DWS_CHANNEL / x-dws-channel (a distribution channel code);
// the two are never conflated here.
//
// Design contract — ACCURACY OVER COVERAGE, but maximize accurate coverage:
//   - Prefer generalizable, host-declared signals so one rule covers a whole
//     family (VSCODE_BRAND covers every VS Code fork, present and future).
//   - Every per-host signature below is OBSERVED on a real host (live process
//     env via `ps eww`, or the app bundle Info.plist), not guessed.
//   - Anything unidentified stays empty — never guess or synthesize a PAT key.
//   - Deliberately NOT used: TERM_PROGRAM (reports the terminal, e.g. iTerm,
//     not the agent host) and fuzzy parent-process name matching.
package auth

import (
	"os"
	"strings"
)

// AgentCodeCustom is the literal code a host may explicitly declare for a
// custom integration. It is not used as an implicit fallback.
const AgentCodeCustom = "custom"

// hostSignature is a verified env fingerprint for a known agent host. EnvKeys
// match when any listed key is present and non-empty.
type hostSignature struct {
	Code    string
	EnvKeys []string
}

// knownSignatures: CLI / daemon agents that inject a distinctive env var, which
// the dws subprocess they spawn inherits. All verified on a real machine
// (2026-06-16) via live process env / launch env — not guessed.
var knownSignatures = []hostSignature{
	// Claude Code — verified: CLAUDECODE=1, CLAUDE_CODE_ENTRYPOINT=cli.
	{Code: "claudecode", EnvKeys: []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"}},
	// OpenClaw — verified on the running daemon: OPENCLAW_BUNDLE_ROOT.
	{Code: "openclaw", EnvKeys: []string{"OPENCLAW_BUNDLE_ROOT", "OPENCLAW_RUNTIME_ROLE"}},
	// Hermes — verified on the running gateway: HERMES_HOME.
	{Code: "hermes", EnvKeys: []string{"HERMES_HOME"}},
	// OpenAI Codex — CODEX_SANDBOX is auto-set by Codex for the subprocesses it
	// spawns (e.g. CODEX_SANDBOX=seatbelt on macOS), and Codex filters this
	// CODEX_-prefixed name out of user .env to prevent spoofing — so its
	// presence reliably means "running under Codex".
	// Source: developers.openai.com/codex/concepts/sandboxing
	{Code: "codex", EnvKeys: []string{"CODEX_SANDBOX"}},
}

// NOTE on coverage limits (honest, not a TODO to silently ignore):
// Most terminal agents (gemini-cli/antigravity, aider, opencode, qwen-code,
// crush, goose, kimi, amazon-q, continue, ...) expose NO reliable
// self-identifying env marker — only user-set API-key/config vars, which we
// must not key off (a user setting GEMINI_API_KEY is not "running under
// gemini"). They therefore resolve to empty unless they declare themselves.
//
// The authoritative, fully-general path to 100% coverage is the T0 declaration
// contract: a host sets DINGTALK_DWS_AGENTCODE=<code> when it launches dws.
// That is accurate for ANY agent (present or future) on ANY OS, and is what an
// integrating host should wire up. Auto-detection (signatures / VSCODE_BRAND /
// bundle id) is a best-effort supplement for hosts that have not declared.

// bundleIDToCode maps macOS app bundle identifiers to agent codes. The bundle
// id is exposed via __CFBundleIdentifier and inherited by child processes the
// IDE spawns (including dws), so it identifies the host even from an integrated
// terminal. Verified from each app's Info.plist (2026-06-16). Only known agent
// bundles map; everything else (iTerm, Terminal, ...) falls through to empty.
//
// macOS-only signal: __CFBundleIdentifier does not exist on Linux/Windows, so
// this map is simply a no-op there (os.Getenv returns "").
var bundleIDToCode = map[string]string{
	"com.qoder.ide":                 "qoder",
	"com.todesktop.230313mzl4w4u92": "cursor", // Cursor's ToDesktop bundle id
	"com.microsoft.VSCode":          "vscode",
	"com.workbuddy.workbuddy":       "workbuddy",
}

// DetectAgentCode resolves the agent_code via a confidence ladder and returns
// the normalized code plus the signal that decided it:
//
//	T0  explicit host declaration   (DINGTALK_DWS_AGENTCODE — dedicated field)
//	T1  verified per-agent env signature (CLI/daemon agents)
//	T2  VSCODE_BRAND value           (every VS Code fork declares its brand)
//	T3  macOS app bundle id          (known agent bundles only)
//	T4  unresolved -> empty          (never guess)
func DetectAgentCode() (code string, signal string) {
	// T0: host explicitly declares its agent_code — highest confidence.
	if v, name := AgentCodeFromEnv(); v != "" {
		return v, "env:" + name
	}

	// T1: verified per-agent env signature (most specific — wins over the IDE
	// it may be running inside).
	for _, sig := range knownSignatures {
		for _, k := range sig.EnvKeys {
			if strings.TrimSpace(os.Getenv(k)) != "" {
				return sig.Code, "sig:" + k
			}
		}
	}

	// T2: VS Code fork family. The brand value IS the host's self-declaration,
	// so this single rule covers Qoder/Cursor/VS Code/Windsurf/Trae/Kiro/... —
	// including forks that don't exist yet.
	if b := strings.TrimSpace(os.Getenv("VSCODE_BRAND")); b != "" {
		return normalizeAgentCode(b), "env:VSCODE_BRAND"
	}

	// T3: macOS app bundle id (known agent bundles only).
	if id := strings.TrimSpace(os.Getenv("__CFBundleIdentifier")); id != "" {
		if c, ok := bundleIDToCode[id]; ok {
			return c, "bundle:" + id
		}
	}

	// T4: unknown host — leave agent_code empty, no guessing.
	return "", ""
}

// normalizeAgentCode maps host-declared names/brands to canonical agent_code
// values. Unrecognized but non-empty input is lowercased, space-stripped and
// kept as-is — still a host declaration, so still accurate (this is what gives
// automatic coverage of new VS Code forks via VSCODE_BRAND).
func normalizeAgentCode(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	switch s {
	case "":
		return ""
	case "claude", "claude-code", "claude_code", "claudecode":
		return "claudecode"
	case "qoder":
		return "qoder"
	case "qoderwork":
		return "QoderWork"
	case "workbuddy", "work-buddy":
		return "workbuddy"
	case "visualstudiocode", "code", "code-oss", "vscode":
		return "vscode"
	case "cursor":
		return "cursor"
	case "windsurf":
		return "windsurf"
	case "trae", "traecn":
		return "trae"
	default:
		return s
	}
}
