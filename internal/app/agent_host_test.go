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

package app

import (
	"errors"
	"io"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestParseAgentHost(t *testing.T) {
	valid := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unset", raw: "", want: ""},
		{name: "ASCII whitespace only", raw: " \t ", want: ""},
		{name: "cloud", raw: "cloud", want: "cloud"},
		{name: "desktop", raw: "desktop", want: "desktop"},
		{name: "legacy combined label remains valid", raw: "qwenwork_cloud", want: "qwenwork_cloud"},
		{name: "trim", raw: " \tcloud\t ", want: "cloud"},
		{name: "generic", raw: "host-2_alpha", want: "host-2_alpha"},
		{name: "leading digit", raw: "2nd_host", want: "2nd_host"},
		{name: "maximum length", raw: strings.Repeat("a", maxAgentHostBytes), want: strings.Repeat("a", maxAgentHostBytes)},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentHost(tc.raw)
			if err != nil {
				t.Fatalf("parseAgentHost() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("parseAgentHost() = %q, want %q", got, tc.want)
			}
		})
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{name: "carriage return", raw: "qwenwork_cloud\r"},
		{name: "line feed", raw: "\nqwenwork_cloud"},
		{name: "uppercase", raw: "Qwenwork_cloud"},
		{name: "internal space", raw: "qwenwork cloud"},
		{name: "internal tab", raw: "qwenwork\tcloud"},
		{name: "unicode", raw: "千问办公"},
		{name: "leading dash", raw: "-qwenwork"},
		{name: "leading underscore", raw: "_qwenwork"},
		{name: "control character", raw: "qwenwork\x00cloud"},
		{name: "vertical tab", raw: "\vcloud"},
		{name: "form feed", raw: "cloud\f"},
		{name: "next line", raw: "cloud\u0085"},
		{name: "non-breaking space", raw: "\u00a0cloud"},
		{name: "ideographic space", raw: "cloud\u3000"},
		{name: "too long", raw: strings.Repeat("a", maxAgentHostBytes+1)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentHost(tc.raw)
			if err == nil {
				t.Fatalf("parseAgentHost(%q) = %q, want error", tc.raw, got)
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) {
				t.Fatalf("parseAgentHost() error type = %T, want *errors.Error", err)
			}
			if appErr.Category != apperrors.CategoryValidation {
				t.Fatalf("category = %q, want validation", appErr.Category)
			}
			if appErr.Reason != "invalid_agent_host" {
				t.Fatalf("reason = %q, want invalid_agent_host", appErr.Reason)
			}
			if tc.raw != "" && strings.Contains(err.Error(), tc.raw) {
				t.Fatalf("error must not echo invalid value %q: %v", tc.raw, err)
			}
		})
	}
}

func TestResolveIdentityHeadersAddsAgentHostBeforeEditionMerge(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSAgentHost, " qwenwork_desktop ")
	t.Setenv(agentproduct.EnvName, "")
	t.Setenv(envDWSChannel, "channel-test")
	t.Setenv(envDingtalkAgent, "agent-test")
	t.Setenv(authpkg.AgentCodeEnv, "agent-code-test")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	mergeSawAgentHost := ""
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			mergeSawAgentHost = headers[headerDWSAgentHost]
			headers["claw-type"] = "test-claw"
			return headers
		},
	})

	headers := resolveIdentityHeaders()
	if got := mergeSawAgentHost; got != "qwenwork_desktop" {
		t.Fatalf("MergeHeaders saw agent host %q, want qwenwork_desktop", got)
	}
	if got := headers[headerDWSAgentHost]; got != "qwenwork_desktop" {
		t.Fatalf("%s = %q, want qwenwork_desktop", headerDWSAgentHost, got)
	}
	if got := headers["x-dingtalk-source"]; got != "github" {
		t.Fatalf("x-dingtalk-source = %q, want github", got)
	}
	if got := headers["x-dingtalk-dws-agent-code"]; got != "agent-code-test" {
		t.Fatalf("agentCode header = %q, want agent-code-test", got)
	}
	if got := headers["x-dws-channel"]; got != "channel-test" {
		t.Fatalf("channel header = %q, want channel-test", got)
	}
	if got := headers["claw-type"]; got != "test-claw" {
		t.Fatalf("claw-type = %q, want test-claw", got)
	}
}

func TestResolveIdentityHeadersOmitsAbsentOrInvalidAgentHost(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSChannel, "channel-test")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			return headers
		},
	})

	for _, raw := range []string{"", " \t ", "DO_NOT_ECHO"} {
		t.Setenv(envDWSAgentHost, raw)
		headers := resolveIdentityHeaders()
		if _, ok := headers[headerDWSAgentHost]; ok {
			t.Fatalf("%s must be omitted for %q: %#v", headerDWSAgentHost, raw, headers)
		}
		if got := headers["x-dingtalk-source"]; got != "github" {
			t.Fatalf("x-dingtalk-source = %q, want github", got)
		}
		if got := headers["x-dws-channel"]; got != "channel-test" {
			t.Fatalf("channel header = %q, want channel-test", got)
		}
	}
}

func TestCrossPlatformCoverageRootRejectsInvalidAgentHostBeforeEditionHook(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	const invalidValue = "DO_NOT_ECHO"
	t.Setenv(envDWSAgentHost, invalidValue)

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	hookCalled := false
	edition.Override(&edition.Hooks{
		AfterPersistentPreRun: func(_ *cobra.Command, _ []string) error {
			hookCalled = true
			return nil
		},
	})

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version"})
	err := root.Execute()
	if err == nil {
		t.Fatal("root command accepted invalid DWS_AGENT_HOST")
	}
	if hookCalled {
		t.Fatal("edition AfterPersistentPreRun ran before DWS_AGENT_HOST validation")
	}

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("root error type = %T, want *errors.Error", err)
	}
	if appErr.Category != apperrors.CategoryValidation || appErr.Reason != "invalid_agent_host" {
		t.Fatalf("root error = category %q reason %q", appErr.Category, appErr.Reason)
	}
	if strings.Contains(err.Error(), invalidValue) {
		t.Fatalf("root error must not echo invalid value: %v", err)
	}
}
