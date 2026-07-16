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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type connectCoverageRunner struct {
	response map[string]any
	err      error
}

func (r connectCoverageRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	return executor.Result{Invocation: inv, Response: r.response}, r.err
}

func connectCoverageCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := newDevAppRobotConnectCommand(&captureRunner{})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(context.Background())
	return cmd, buf
}

func writeKnowledgeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("# handbook\ncovered knowledge"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageBuildConnectPlanQoderAndOpencodeCoverage(t *testing.T) {
	for _, ch := range []string{"qoder", "qoderwork", "opencode", "gemini"} {
		if got := buildConnectPlan(ch, "id", "robot")["method"]; got == "unknown" {
			t.Fatalf("buildConnectPlan(%q) = unknown", ch)
		}
	}
}

func TestCrossPlatformCoverageLaunchConnectorExternalAndGuidanceCoverage(t *testing.T) {
	cmd, out := connectCoverageCommand(t)
	binDir := t.TempDir()
	writeShellExecutable(t, binDir, "connector-ok", "test \"$CID:$SEC:$DWS_AGENT_CHANNEL\" = \"id:secret:hermes\"\n")
	writeShellExecutable(t, binDir, "connector-fail", "exit 7\n")
	t.Setenv("DWS_CONNECT_CMD", filepath.Join(binDir, "connector-ok"))
	if err := launchConnector(cmd, &captureRunner{}, "hermes", "id", "secret", connectAgentOptions{}); err != nil {
		t.Fatalf("external connector: %v\n%s", err, out.String())
	}
	t.Setenv("DWS_CONNECT_CMD", filepath.Join(binDir, "connector-fail"))
	if err := launchConnector(cmd, &captureRunner{}, "hermes", "id", "secret", connectAgentOptions{}); err == nil {
		t.Fatal("failing external connector returned nil")
	}

	t.Setenv("DWS_CONNECT_CMD", "")
	if err := launchConnector(cmd, &captureRunner{}, "hermes", "id", "secret", connectAgentOptions{}); err != nil {
		t.Fatalf("official guidance: %v", err)
	}
	if err := launchConnector(cmd, &captureRunner{}, "unknown", "id", "secret", connectAgentOptions{}); err == nil {
		t.Fatal("unknown connector returned nil")
	}
}

func TestCrossPlatformCoverageLaunchConnectorValidationAndKnowledgeCoverage(t *testing.T) {
	t.Setenv("DWS_CONNECT_CMD", "")
	t.Setenv("DWS_AGENT_CMD", "sh -c printf\\ ok")
	cmd, _ := connectCoverageCommand(t)

	badRole := filepath.Join(t.TempDir(), "missing.yaml")
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{RoleConfigPath: badRole}); err == nil {
		t.Fatal("missing role returned nil")
	}
	roleDir := t.TempDir()
	rolePath := filepath.Join(roleDir, "role.yaml")
	if err := os.WriteFile(rolePath, []byte("name: role\nclient_id: other\nowner_user_id: owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{RoleConfigPath: rolePath}); err == nil {
		t.Fatal("mismatched role returned nil")
	}

	t.Setenv("DWS_AGENT_CMD", "")
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{}); err == nil {
		t.Fatal("missing custom agent command returned nil")
	}
	t.Setenv("DWS_AGENT_CMD", "sh -c printf\\ ok")
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{KnowledgeDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing knowledge directory returned nil")
	}

	knowledge := t.TempDir()
	writeKnowledgeFile(t, knowledge, "guide.md")
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{KnowledgeSource: filepath.Join(knowledge, "missing")}); err == nil {
		t.Fatal("missing knowledge source returned nil")
	}
}

func TestCrossPlatformCoverageLaunchConnectorFullStreamAssemblyCoverage(t *testing.T) {
	t.Setenv("DWS_CONNECT_CMD", "")
	t.Setenv("DWS_AGENT_CMD", "sh -c printf\\ ok")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	knowledgeA := t.TempDir()
	knowledgeB := t.TempDir()
	writeKnowledgeFile(t, knowledgeA, "a.md")
	writeKnowledgeFile(t, knowledgeB, "b.txt")
	rolePath := filepath.Join(t.TempDir(), "role.yaml")
	role := fmt.Sprintf("name: helper\nclient_id: id\nowner_user_id: owner\nconfirm_policy: auto\npersona: precise\nknowledge_sources:\n  - %s\n  - %s\nallowed_scopes:\n  - todo\n", knowledgeA, knowledgeB)
	if err := os.WriteFile(rolePath, []byte(role), 0o600); err != nil {
		t.Fatal(err)
	}

	origRun := devAppRunStreamConnector
	origLoad := devAppLoadConnectKnowledge
	t.Cleanup(func() {
		devAppRunStreamConnector = origRun
		devAppLoadConnectKnowledge = origLoad
	})
	var calls int
	devAppLoadConnectKnowledge = func(cmd *cobra.Command, runner executor.Runner, clientID, raw string) (*knowledgeBase, error) {
		calls++
		switch raw {
		case "nil-source":
			return nil, nil
		case "error-source":
			return nil, errors.New("source failed")
		default:
			return loadConnectKnowledgeSource(cmd, runner, clientID, raw)
		}
	}
	var gotExtras *connectExtras
	var gotCard *aiCardClient
	devAppRunStreamConnector = func(_ context.Context, channel, clientID, clientSecret string, fwd forwarder, card *aiCardClient, extras *connectExtras) error {
		if channel != "custom" || clientID != "id" || clientSecret != "secret" || fwd == nil {
			return fmt.Errorf("unexpected stream arguments")
		}
		gotExtras, gotCard = extras, card
		return nil
	}

	cmd, out := connectCoverageCommand(t)
	opts := connectAgentOptions{
		RoleConfigPath: rolePath, ReplyCard: true, CardTemplate: "card-template",
		KnowledgeDir: knowledgeA, KnowledgeSource: knowledgeB,
		KnowledgeSources: []string{"nil-source", knowledgeA, knowledgeB},
		AllowedUsers:     []string{"u1"}, AllowedGroups: []string{"g1"}, UserRateLimit: 3,
		OwnerUserID: "owner", ApprovalCardTemplate: "approval-template",
		AuditSheetNode: "node", AuditSheetTab: "tab", RoleScopes: []string{"todo"}, ConfirmPolicy: "auto",
	}
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", opts); err != nil {
		t.Fatalf("launchConnector: %v\n%s", err, out.String())
	}
	if calls < 5 || gotExtras == nil || gotExtras.kb == nil || gotExtras.gate == nil || gotExtras.approval == nil || gotCard == nil {
		t.Fatalf("assembled connector incomplete: calls=%d extras=%+v card=%v", calls, gotExtras, gotCard)
	}
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{
		KnowledgeSources: []string{knowledgeA},
	}); err != nil {
		t.Fatalf("first role knowledge source: %v", err)
	}

	// No card template selects the text-approval path and also covers the
	// card-enabled/plain-template reply style. A cancelled context stops retry.
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{
		ReplyCard: true, OwnerUserID: "owner", WorkDir: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	cancel()

	if err := launchConnector(cmd, &captureRunner{}, "custom", "id", "secret", connectAgentOptions{
		KnowledgeSources: []string{"error-source"},
	}); err == nil {
		t.Fatal("role knowledge error returned nil")
	}
}

func TestCrossPlatformCoverageConnectAgentOptionRemainingCoverage(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_REPLY_CARD", "false")
	t.Setenv("DWS_AGENT_MODEL", "model")
	t.Setenv("DWS_AGENT_WORKDIR", "/work")
	t.Setenv("DWS_CARD_TEMPLATE", "public")
	t.Setenv("DWS_KNOWLEDGE_DIR", "/knowledge")
	t.Setenv("DWS_KNOWLEDGE_SOURCE", "wiki:space")
	t.Setenv("DWS_ALLOWED_USERS", "u1,u2")
	t.Setenv("DWS_ALLOWED_GROUPS", "g1")
	t.Setenv("DWS_USER_RATE_LIMIT", "7")
	t.Setenv("DWS_OWNER_USER_ID", "owner")
	t.Setenv("DWS_APPROVAL_CARD_TEMPLATE", "approval")
	t.Setenv("DWS_ROLE_CONFIG", "role.yaml")
	t.Setenv("DWS_AUDIT_SHEET", "node")
	t.Setenv("DWS_AUDIT_SHEET_TAB", "tab")
	cmd := newDevAppRobotConnectCommand(nil)
	if err := cmd.Flags().Set("audit-sheet-tab", ""); err != nil {
		t.Fatal(err)
	}
	opts, err := connectAgentOptionsFromCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "model" || opts.WorkDir != "/work" || opts.ReplyCard || opts.CardTemplate != defaultAICardTemplateID || opts.UserRateLimit != 7 || opts.AuditSheetTab != "tab" {
		t.Fatalf("env options = %+v", opts)
	}

	t.Setenv("DWS_AUDIT_SHEET_TAB", "")
	opts, err = connectAgentOptionsFromCommand(cmd)
	if err != nil || opts.AuditSheetTab != "Sheet1" {
		t.Fatalf("default audit tab = %q, err=%v", opts.AuditSheetTab, err)
	}
	t.Setenv("DWS_USER_RATE_LIMIT", "invalid")
	if _, err := connectAgentOptionsFromCommand(cmd); err != nil {
		t.Fatalf("invalid rate should retain default: %v", err)
	}

	cases := []struct {
		flag, value, env, envValue string
		want                       bool
		wantErr                    bool
	}{
		{env: "DWS_AGENT_PERMISSION_MODE", envValue: "ask", want: false},
		{env: "DWS_AGENT_PERMISSION_MODE", envValue: "bad", wantErr: true},
		{flag: "agent-approval-mode", value: "ask", want: false},
		{env: "DWS_AGENT_APPROVAL_MODE", envValue: "ask", want: false},
		{env: "DWS_AGENT_APPROVAL_MODE", envValue: "bad", wantErr: true},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			clearChannelEnv(t)
			c := newDevAppRobotConnectCommand(nil)
			if tc.env != "" {
				t.Setenv(tc.env, tc.envValue)
			}
			if tc.flag != "" {
				if err := c.Flags().Set(tc.flag, tc.value); err != nil {
					t.Fatal(err)
				}
			}
			got, err := resolveAgentYoloMode(c)
			if (err != nil) != tc.wantErr || (!tc.wantErr && got != tc.want) {
				t.Fatalf("got (%v,%v), want (%v, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageConnectAgentOptionsPayloadRemainingCoverage(t *testing.T) {
	cases := []struct {
		channel string
		memory  bool
	}{
		{"codex", false}, {"opencode", true}, {"opencode", false},
		{"claudecode", true}, {"claudecode", false}, {"gemini", true},
	}
	for _, tc := range cases {
		payload := connectAgentOptionsPayload(tc.channel, connectAgentOptions{
			Memory: tc.memory, ReplyCard: true, CardTemplate: "template", Yolo: true,
		})
		if payload["replyStyle"] != "ai-card" || payload["yolo"] != "enabled" {
			t.Fatalf("payload(%s,%v) = %#v", tc.channel, tc.memory, payload)
		}
	}
}

func TestCrossPlatformCoverageDevAppConnectCommandRemainingCoverage(t *testing.T) {
	origInteractive := devAppConnectStdinInteractive
	origOnboarding := devAppRunConnectOnboarding
	origDaemon := devAppStartConnectDaemon
	origStream := devAppRunStreamConnector
	t.Cleanup(func() {
		devAppConnectStdinInteractive = origInteractive
		devAppRunConnectOnboarding = origOnboarding
		devAppStartConnectDaemon = origDaemon
		devAppRunStreamConnector = origStream
	})
	devAppRunStreamConnector = func(context.Context, string, string, string, forwarder, *aiCardClient, *connectExtras) error {
		return nil
	}

	run := func(runner executor.Runner, args ...string) (string, error) {
		root := newDevAppTestRoot(runner)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetIn(strings.NewReader(""))
		root.SetArgs(append([]string{"dev", "connect"}, args...))
		return out.String(), root.Execute()
	}

	clearChannelEnv(t)
	if _, err := run(&captureRunner{}, "--dry-run"); err == nil {
		t.Fatal("undetected channel returned nil")
	}
	if _, err := run(&captureRunner{}, "--channel", "custom", "--agent-cmd", "sh -c printf\\ ok", "--robot-client-id", "id", "--robot-client-secret", "secret"); err != nil {
		t.Fatalf("agent-cmd launch: %v", err)
	}
	if _, err := run(&captureRunner{}, "--channel", "custom", "--robot-client-id", "id", "--robot-client-secret", "secret", "--agent-permission-mode", "bad"); err == nil {
		t.Fatal("invalid agent options returned nil")
	}

	boom := errors.New("credentials boom")
	if _, err := run(connectCoverageRunner{err: boom}, "--channel", "hermes", "--unified-app-id", "app"); !errors.Is(err, boom) {
		t.Fatalf("credential error = %v", err)
	}
	if _, err := run(connectCoverageRunner{response: map[string]any{}}, "--channel", "hermes", "--unified-app-id", "app"); err == nil {
		t.Fatal("empty credentials returned nil")
	}
	response := map[string]any{"result": map[string]any{"clientId": "id", "clientSecret": "secret"}}
	if _, err := run(connectCoverageRunner{response: response}, "--channel", "hermes", "--unified-app-id", "app"); err != nil {
		t.Fatalf("resolved credentials launch: %v", err)
	}

	devAppConnectStdinInteractive = func() bool { return true }
	devAppRunConnectOnboarding = func(executor.Runner, *cobra.Command, io.Reader, io.Writer) (connectCreds, error) {
		return connectCreds{}, boom
	}
	if _, err := run(&captureRunner{}, "--channel", "hermes"); !errors.Is(err, boom) {
		t.Fatalf("onboarding error = %v", err)
	}
	devAppRunConnectOnboarding = func(executor.Runner, *cobra.Command, io.Reader, io.Writer) (connectCreds, error) {
		return connectCreds{clientID: "id", clientSecret: "secret", source: "onboarding"}, nil
	}
	if _, err := run(&captureRunner{}, "--channel", "hermes"); err != nil {
		t.Fatalf("onboarding success: %v", err)
	}

	var daemonCalled bool
	devAppStartConnectDaemon = func(_ *cobra.Command, dirKey, clientID, unifiedAppID, channel, notifyStaffID, profile string, alwaysOn bool) error {
		daemonCalled = dirKey != "" && clientID == "id" && channel == "hermes" && notifyStaffID == "staff" && alwaysOn
		return boom
	}
	if _, err := run(&captureRunner{}, "--channel", "hermes", "--robot-client-id", "id", "--robot-client-secret", "secret", "--daemon", "--alwayson", "--notify-staff-id", "staff"); !errors.Is(err, boom) || !daemonCalled {
		t.Fatalf("daemon result err=%v called=%v", err, daemonCalled)
	}

	clearChannelEnv(t)
	if _, err := run(&captureRunner{}, "--daemon-supervise"); err == nil || !strings.Contains(err.Error(), "DIRKEY") {
		t.Fatalf("supervisor error = %v", err)
	}
}

func TestCrossPlatformCoverageDevAppFetchCredentialsErrorCoverage(t *testing.T) {
	cmd, _ := connectCoverageCommand(t)
	want := errors.New("runner failure")
	if _, _, err := devAppFetchCredentials(connectCoverageRunner{err: want}, cmd, "app"); !errors.Is(err, want) {
		t.Fatalf("devAppFetchCredentials error = %v", err)
	}
}
