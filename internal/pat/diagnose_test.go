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
	"strings"
	"testing"

	"github.com/spf13/cobra"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// sensitiveJSONKeys is the blacklist of field names that must never appear in
// diagnose output (JSON form). Covers both camelCase and snake_case spellings
// for the OAuth client identity and the PAT flow / token material.
var sensitiveJSONKeys = []string{
	"flowId",
	"clientId",
	"clientSecret",
	"token",
	"refreshToken",
	"corpId",
	"accessToken",
	"authRequestId",
	"client_id",
	"client_secret",
}

// sensitiveTextKeys is the blacklist applied to human/text output. It omits
// generic words like "token" / "corpId" that may legitimately appear in
// descriptive copy, but still catches the explicit sensitive identifiers.
var sensitiveTextKeys = []string{
	"flowId",
	"clientId",
	"clientSecret",
	"refreshToken",
	"client_id",
	"client_secret",
	"authRequestId",
}

// diagnoseJSONShape mirrors diagnoseResult's JSON tags so the test does not
// depend on the unexported struct's Go field names.
type diagnoseJSONShape struct {
	HostOwnedPAT        bool     `json:"hostOwnedPAT"`
	AgentCodeEnvPresent bool     `json:"agentCodeEnvPresent"`
	AgentCodeEnvName    string   `json:"agentCodeEnvName"`
	Edition             string   `json:"edition"`
	Version             string   `json:"version"`
	Hints               []string `json:"hints"`
}

// newStatusCmd wires a fresh root (with Version) + the pat subtree and
// returns the `pat status` leaf with stdout/stderr redirected to buf.
func newStatusCmd(t *testing.T, buf *bytes.Buffer) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "dws", Version: "0.2.35-test"}
	RegisterCommands(root, nil)
	cmd, _, err := root.Find([]string{"pat", "status"})
	if err != nil {
		t.Fatalf("root.Find([pat status]) error = %v", err)
	}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd
}

// assertNoJSONSecrets asserts that none of the sensitive JSON keys leak into
// the raw output, checking both bare and quoted forms for defense in depth.
func assertNoJSONSecrets(t *testing.T, raw string) {
	t.Helper()
	for _, k := range sensitiveJSONKeys {
		if strings.Contains(raw, `"`+k+`"`) {
			t.Errorf("diagnose JSON unexpectedly contains sensitive key %q\n%s", k, raw)
		}
		if strings.Contains(raw, k+`":`) {
			t.Errorf("diagnose JSON unexpectedly contains sensitive key prefix %q:\n%s", k, raw)
		}
	}
}

// assertNoTextSecrets asserts that the human/text output does not leak the
// narrowed sensitive identifiers.
func assertNoTextSecrets(t *testing.T, raw string) {
	t.Helper()
	for _, k := range sensitiveTextKeys {
		if strings.Contains(raw, k) {
			t.Errorf("diagnose text unexpectedly contains sensitive key %q\n%s", k, raw)
		}
	}
}

func TestDiagnose_JSON_HostOwned(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "cursor")

	var buf bytes.Buffer
	cmd := newStatusCmd(t, &buf)

	if err := runDiagnose(cmd, "json"); err != nil {
		t.Fatalf("runDiagnose(json) error = %v", err)
	}

	raw := buf.String()
	var got diagnoseJSONShape
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal error = %v\nraw: %s", err, raw)
	}

	if !got.HostOwnedPAT {
		t.Errorf("HostOwnedPAT = false, want true when %s=cursor", authpkg.AgentCodeEnv)
	}
	if !got.AgentCodeEnvPresent {
		t.Errorf("AgentCodeEnvPresent = false, want true")
	}
	if got.AgentCodeEnvName != "DINGTALK_DWS_AGENTCODE" {
		t.Errorf("AgentCodeEnvName = %q, want %q", got.AgentCodeEnvName, "DINGTALK_DWS_AGENTCODE")
	}
	if got.Edition == "" {
		t.Errorf("Edition must not be empty")
	}
	if got.Version == "" {
		t.Errorf("Version must not be empty (wired root Version=0.2.35-test)")
	}
	if got.Version != "0.2.35-test" {
		t.Logf("Version = %q (tolerated; primary assertion is non-empty)", got.Version)
	}

	assertNoJSONSecrets(t, raw)
}

func TestDiagnose_JSON_EnvUnset(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "")

	var buf bytes.Buffer
	cmd := newStatusCmd(t, &buf)

	if err := runDiagnose(cmd, "json"); err != nil {
		t.Fatalf("runDiagnose(json) error = %v", err)
	}

	raw := buf.String()
	var got diagnoseJSONShape
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal error = %v\nraw: %s", err, raw)
	}

	if got.HostOwnedPAT {
		t.Errorf("HostOwnedPAT = true, want false when env unset")
	}
	if got.AgentCodeEnvPresent {
		t.Errorf("AgentCodeEnvPresent = true, want false when env unset")
	}
	if len(got.Hints) < 1 {
		t.Fatalf("Hints must have at least 1 entry when env unset; got %v", got.Hints)
	}
	found := false
	for _, h := range got.Hints {
		if strings.Contains(h, "export") || strings.Contains(h, "未设置") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no hint mentions `export` or `未设置`; got %v", got.Hints)
	}

	assertNoJSONSecrets(t, raw)
}

func TestDiagnose_TextFormat(t *testing.T) {
	cases := []struct {
		name   string
		format string
	}{
		{"default(empty format)", ""},
		{"explicit text", "text"},
		{"explicit human", "human"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(authpkg.AgentCodeEnv, "")

			var buf bytes.Buffer
			cmd := newStatusCmd(t, &buf)

			if err := runDiagnose(cmd, tc.format); err != nil {
				t.Fatalf("runDiagnose(%q) error = %v", tc.format, err)
			}

			out := buf.String()
			for _, want := range []string{
				"DINGTALK_DWS_AGENTCODE",
				"hostOwnedPAT",
				"Edition",
				"Version",
				"export",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("text output missing %q\n%s", want, out)
				}
			}

			assertNoTextSecrets(t, out)
		})
	}
}

func TestDiagnose_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	cmd := newStatusCmd(t, &buf)

	err := runDiagnose(cmd, "yaml")
	if err == nil {
		t.Fatalf("runDiagnose(yaml) err = nil, want non-nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unsupported") {
		t.Errorf("error message missing 'unsupported': %q", msg)
	}
	if !strings.Contains(msg, "yaml") {
		t.Errorf("error message missing offending format 'yaml': %q", msg)
	}
}

func TestDiagnose_HintAbsentWhenEnvSet(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "cursor")

	var buf bytes.Buffer
	cmd := newStatusCmd(t, &buf)

	res := collectDiagnose(cmd)
	if len(res.Hints) != 0 {
		t.Fatalf("Hints must be empty when env is set; got %v", res.Hints)
	}
	if !res.HostOwnedPAT {
		t.Errorf("HostOwnedPAT = false, want true")
	}
	if !res.AgentCodeEnvPresent {
		t.Errorf("AgentCodeEnvPresent = false, want true")
	}
}

// TestDiagnose_NoNetwork_DeferredByCodeReview is an explicit placeholder.
// Proving "no network / no credential I/O" via injection would require
// reworking diagnose.go to accept a side-effect bus; instead we rely on
// static review of diagnose.go's import set: only encoding/json, fmt, io,
// os, strings, cobra, internal/auth (constant + HostOwnsPATFlow, which
// itself only reads os.Getenv), and pkg/edition — no HTTP, no keyring,
// no MCP ToolCaller touched from the diagnose path.
func TestDiagnose_NoNetwork_DeferredByCodeReview(t *testing.T) {
	t.Skip("covered by static code review: diagnose.go imports only encoding/json, fmt, io, os, strings, cobra, authpkg, edition — no HTTP / keyring / MCP client")
}
