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

package pat

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBrowserPolicy_DefaultRoundTrip(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	saved, err := SetBrowserPolicy(configDir, "", false)
	if err != nil {
		t.Fatalf("SetBrowserPolicy(default) error = %v", err)
	}
	if saved.Scope != "default" {
		t.Fatalf("saved.Scope = %q, want default", saved.Scope)
	}
	if saved.OpenBrowser {
		t.Fatal("saved.OpenBrowser = true, want false")
	}

	loaded, err := ResolveBrowserPolicy(configDir, "")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(default) error = %v", err)
	}
	if loaded.Scope != "default" {
		t.Fatalf("loaded.Scope = %q, want default", loaded.Scope)
	}
	if loaded.OpenBrowser {
		t.Fatal("loaded.OpenBrowser = true, want false")
	}
}

func TestBrowserPolicy_AgentOverridesDefault(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	if _, err := SetBrowserPolicy(configDir, "", true); err != nil {
		t.Fatalf("SetBrowserPolicy(default) error = %v", err)
	}
	if _, err := SetBrowserPolicy(configDir, "agt-sales", false); err != nil {
		t.Fatalf("SetBrowserPolicy(agent) error = %v", err)
	}

	agentLoaded, err := ResolveBrowserPolicy(configDir, "agt-sales")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(agent) error = %v", err)
	}
	if agentLoaded.Scope != "agent" {
		t.Fatalf("agentLoaded.Scope = %q, want agent", agentLoaded.Scope)
	}
	if agentLoaded.AgentCode != "agt-sales" {
		t.Fatalf("agentLoaded.AgentCode = %q, want agt-sales", agentLoaded.AgentCode)
	}
	if agentLoaded.OpenBrowser {
		t.Fatal("agentLoaded.OpenBrowser = true, want false")
	}

	defaultLoaded, err := ResolveBrowserPolicy(configDir, "agt-other")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(default fallback) error = %v", err)
	}
	if defaultLoaded.Scope != "default" {
		t.Fatalf("defaultLoaded.Scope = %q, want default", defaultLoaded.Scope)
	}
	if !defaultLoaded.OpenBrowser {
		t.Fatal("defaultLoaded.OpenBrowser = false, want true")
	}
}

func TestResolveBrowserPolicy_EnvFallback(t *testing.T) {
	t.Setenv(agentCodeEnv, "agt-env")
	configDir := t.TempDir()
	if _, err := SetBrowserPolicy(configDir, "agt-env", false); err != nil {
		t.Fatalf("SetBrowserPolicy(agent) error = %v", err)
	}

	loaded, err := ResolveBrowserPolicy(configDir, "")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(env fallback) error = %v", err)
	}
	if loaded.Scope != "agent" {
		t.Fatalf("loaded.Scope = %q, want agent", loaded.Scope)
	}
	if loaded.AgentCode != "agt-env" {
		t.Fatalf("loaded.AgentCode = %q, want agt-env", loaded.AgentCode)
	}
	if loaded.OpenBrowser {
		t.Fatal("loaded.OpenBrowser = true, want false")
	}
}

func TestResolveBrowserPolicy_FallsBackToOpenSourceDefault(t *testing.T) {
	t.Parallel()

	loaded, err := ResolveBrowserPolicy(t.TempDir(), "")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(fallback) error = %v", err)
	}
	if loaded.Scope != "builtin_default" {
		t.Fatalf("loaded.Scope = %q, want builtin_default", loaded.Scope)
	}
	if !loaded.OpenBrowser {
		t.Fatal("loaded.OpenBrowser = false, want true")
	}
}

func TestBrowserPolicyCommand_WritesAgentPolicy(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)

	cmd := newBrowserPolicyCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--enabled=false", "--agentCode", "agt-command"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("browser-policy Execute() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(command output) error = %v\nraw=%s", err, stdout.String())
	}
	if got, _ := payload["scope"].(string); got != "agent" {
		t.Fatalf("scope = %q, want agent", got)
	}
	if got, _ := payload["agentCode"].(string); got != "agt-command" {
		t.Fatalf("agentCode = %q, want agt-command", got)
	}
	if got, _ := payload["openBrowser"].(bool); got {
		t.Fatal("openBrowser = true, want false")
	}

	loaded, err := ResolveBrowserPolicy(configDir, "agt-command")
	if err != nil {
		t.Fatalf("ResolveBrowserPolicy(agent) error = %v", err)
	}
	if loaded.OpenBrowser {
		t.Fatal("loaded.OpenBrowser = true, want false")
	}
}
