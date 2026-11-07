// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeNoRemoteAccess(t *testing.T) *digitalEmployeeProtocolCaller {
	t.Helper()
	caller := &digitalEmployeeProtocolCaller{}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) {
		t.Error("入站不应枚举主管账号")
		return nil, errors.New("unexpected profile lookup")
	})
	t.Cleanup(func() {
		if len(caller.calls) != 0 || len(caller.tokenCalls) != 0 {
			t.Errorf("入站不应查询远端: %v %v", caller.calls, caller.tokenCalls)
		}
	})
	return caller
}

func TestCrossPlatformCoverageEmployeeBindingPublishesPlatformPolicyWithoutSupervisor(t *testing.T) {
	for _, scenario := range []string{"managed", "legacy", "supervisor-only", "old-supervisor-retained"} {
		t.Run(scenario, func(t *testing.T) {
			dir, b := lifecycleFixture(t)
			switch scenario {
			case "legacy":
				b.RuntimeBindingID = ""
			case "supervisor-only":
				b.RuntimeBindingID = ""
				b.SupervisorProfile = "corp:old-manager"
			case "old-supervisor-retained":
				b.SupervisorProfile = "corp:old-manager"
			}
			if err := saveDigitalEmployeeBinding(dir, b); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(digitalEmployeeBindingPath(dir, b.DWSProfile))
			if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"), digitalEmployeeAdapterConfig{Binding: b, SelfOpenDingTalkID: "employee-self", SelfIdentityProfile: b.DWSProfile}); err != nil {
				t.Fatal(err)
			}
			employeeNoRemoteAccess(t)
			previous := auth.RuntimeProfile()
			auth.SetRuntimeProfile(b.DWSProfile)
			t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
			cmd := newEmployeeBindingCommand()
			cmd.SetArgs([]string{"--channel", "dsh", "--stdin"})
			cmd.SetIn(strings.NewReader(fmt.Sprintf(`{"agentUuid":%q,"bindingRevision":7}`, b.AgentUUID)))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := corecmd.ExecuteForTest(cmd); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err, out.String())
			}
			want := "platform"
			if b.RuntimeBindingID == "" {
				want = "local_allowlist"
			}
			if result.Data["accessPolicy"] != want || result.Data["allowed"] != nil || result.Data["selfOpenDingTalkId"] != "employee-self" {
				t.Fatalf("result=%v", result.Data)
			}
			after, _ := os.ReadFile(digitalEmployeeBindingPath(dir, b.DWSProfile))
			if !bytes.Equal(before, after) {
				t.Fatal("只读 binding 改写了历史字段")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeBindingRejectsInvalidAuthority(t *testing.T) {
	for _, scenario := range []string{"revision", "unbound", "unbinding", "corrupt-runtime-id", "pending-operation", "wrong-channel", "sender-input"} {
		t.Run(scenario, func(t *testing.T) {
			dir, b := lifecycleFixture(t)
			employeeNoRemoteAccess(t)
			revision, channel := 7, "dsh"
			switch scenario {
			case "revision":
				revision--
			case "unbound":
				b.BindingState = "unbound"
			case "unbinding":
				b.BindingState = "unbinding"
			case "corrupt-runtime-id":
				b.RuntimeBindingID = "bad\nid"
			case "pending-operation":
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Action: "bind", Phase: "pending"}); err != nil {
					t.Fatal(err)
				}
			case "wrong-channel":
				channel = "codex"
			}
			if err := writeEmployeeJSON(digitalEmployeeBindingPath(dir, b.DWSProfile), b); err != nil {
				t.Fatal(err)
			}
			previous := auth.RuntimeProfile()
			auth.SetRuntimeProfile(b.DWSProfile)
			t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
			cmd := newEmployeeBindingCommand()
			cmd.SetArgs([]string{"--channel", channel, "--stdin"})
			input := fmt.Sprintf(`{"agentUuid":%q,"bindingRevision":%d}`, b.AgentUUID, revision)
			if scenario == "sender-input" {
				input = strings.TrimSuffix(input, "}") + `,"senderOpenDingTalkId":"sender"}`
			}
			cmd.SetIn(strings.NewReader(input))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := corecmd.ExecuteForTest(cmd); err == nil {
				t.Fatalf("invalid binding accepted: %s", out.String())
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeePlatformInboundUsesNoRemoteQueries(t *testing.T) {
	for _, scenario := range []struct {
		name, kind                                       string
		managed, supervisor, allowUser, allowGroup, want bool
	}{
		{name: "managed direct", kind: "user_im_message_receive_o2o_all", managed: true, want: true},
		{name: "managed mention", kind: "user_im_message_receive_at", managed: true, want: true},
		{name: "managed ordinary group", kind: "user_im_message_receive_group_all", managed: true},
		{name: "managed self", kind: "user_im_message_receive_o2o_all", managed: true},
		{name: "managed invalid", kind: "user_im_message_receive_o2o_all", managed: true},
		{name: "legacy denied", kind: "user_im_message_receive_o2o_all"},
		{name: "supervisor is not authority", kind: "user_im_message_receive_o2o_all", supervisor: true},
		{name: "legacy allowed", kind: "user_im_message_receive_o2o_all", allowUser: true, want: true},
		{name: "legacy group denied", kind: "user_im_message_receive_at", allowUser: true},
		{name: "legacy group allowed", kind: "user_im_message_receive_at", allowUser: true, allowGroup: true, want: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			employeeNoRemoteAccess(t)
			r, e := employeeLedgerFixture(t)
			r.cfg.Options.AllowedUsers = nil
			r.cfg.SelfOpenDingTalkID = "self"
			if scenario.managed {
				r.cfg.Binding.RuntimeBindingID = "server-binding"
			}
			if scenario.supervisor {
				r.cfg.Binding.SupervisorProfile = "corp:manager"
			}
			if scenario.allowUser {
				r.cfg.Options.AllowedUsers = []string{e.SenderID}
			}
			if scenario.allowGroup {
				r.cfg.Options.AllowedGroups = []string{e.ConversationID}
			}
			e.Type = scenario.kind
			if scenario.name == "managed self" {
				e.SenderID = "self"
			}
			if scenario.name == "managed invalid" {
				e.MessageID = ""
			}
			q := make(chan employeeEvent, 2)
			r.queues[e.ConversationID] = q
			if err := r.enqueue(e); err != nil {
				t.Fatal(err)
			}
			if (len(q) == 1) != scenario.want {
				t.Fatalf("queue=%d want accepted=%t", len(q), scenario.want)
			}
			if scenario.want {
				if err := r.enqueue(e); err != nil {
					t.Fatal(err)
				}
				if len(q) != 1 {
					t.Fatal("重复事件再次入队")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeOldManagedBindingStartsWithoutSupervisor(t *testing.T) {
	t.Run("lease", func(t *testing.T) {
		dir, b := lifecycleFixture(t)
		employeeNoRemoteAccess(t)
		previous := auth.RuntimeProfile()
		auth.SetRuntimeProfile(b.DWSProfile)
		t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
		before, _ := os.ReadFile(digitalEmployeeBindingPath(dir, b.DWSProfile))
		cmd := newDeapConnectCommand()
		cmd.SetContext(context.Background())
		for key, value := range map[string]string{"agent-uuid": b.AgentUUID, "binding-revision": "7", "runtime-instance-id": "00000000-0000-4000-8000-000000000001"} {
			if err := cmd.Flags().Set(key, value); err != nil {
				t.Fatal(err)
			}
		}
		cmd.SetIn(strings.NewReader("released\n"))
		var out bytes.Buffer
		cmd.SetErr(&out)
		if err := runEmployeeLease(cmd); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "[employee] leased") {
			t.Fatalf("lease未就绪: %s", out.String())
		}
		after, _ := os.ReadFile(digitalEmployeeBindingPath(dir, b.DWSProfile))
		if !bytes.Equal(before, after) {
			t.Fatal("启动修改旧绑定")
		}
		if _, err := os.Stat(employeeLeaseGuardPath(b.DWSProfile)); !os.IsNotExist(err) {
			t.Fatalf("release未确认: %v", err)
		}
	})
	t.Run("native", func(t *testing.T) {
		cfg := employeeBoundaryConfig(t)
		cfg.Binding.RuntimeBindingID = "server-binding"
		if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding); err != nil {
			t.Fatal(err)
		}
		employeeNoRemoteAccess(t)
		called := false
		testseam.Swap(t, &digitalEmployeeNewForwarder, func(_ context.Context, actual digitalEmployeeAdapterConfig) (forwarder, error) {
			called = true
			if actual.Binding.SupervisorProfile != "" || employeeBindingAccessPolicy(actual.Binding) != "platform" {
				t.Fatal("unexpected binding")
			}
			return nil, errors.New("controlled stop after authorization")
		})
		err := runEmployeeWorker(context.Background(), cfg, 0)
		if !called || err == nil || err.Error() != "agent_unavailable" {
			t.Fatalf("启动仍依赖主管查询: called=%v err=%v", called, err)
		}
	})
}

func TestCrossPlatformCoverageEmployeeBindingLegacySelfIdentity(t *testing.T) {
	for _, scenario := range []string{"missing-adapter", "stale-adapter", "unverified-adapter", "wrong-token", "lookup-fails", "binding-changed"} {
		t.Run(scenario, func(t *testing.T) {
			dir, b := lifecycleFixture(t)
			if scenario == "unverified-adapter" {
				if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"), digitalEmployeeAdapterConfig{Binding: b, SelfOpenDingTalkID: "supervisor-scope-self"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "stale-adapter" {
				stale := b
				stale.BindingRevision--
				if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"), digitalEmployeeAdapterConfig{Binding: stale, SelfOpenDingTalkID: "stale-self"}); err != nil {
					t.Fatal(err)
				}
			}
			tokenCalls := 0
			testseam.Swap(t, &employeeSelfIdentityToken, func(_ context.Context, profile string) (*auth.TokenData, error) {
				tokenCalls++
				if profile != b.DWSProfile {
					t.Fatal("使用了主管身份")
				}
				user := "employee"
				if scenario == "wrong-token" {
					user = "other"
				}
				if scenario == "binding-changed" {
					changed := b
					changed.BindingRevision++
					if err := writeEmployeeJSON(digitalEmployeeBindingPath(dir, b.DWSProfile), changed); err != nil {
						t.Fatal(err)
					}
				}
				return &auth.TokenData{CorpID: "corp", UserID: user, AccessToken: "employee-token"}, nil
			})
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"contact/search_contact_by_key_word": {`{"result":[{"userId":"employee","openDingTalkId":"employee-self"}]}`}}}
			if scenario == "lookup-fails" {
				caller.responses = nil
			}
			InitDepsForTest(t, caller)
			previous := auth.RuntimeProfile()
			auth.SetRuntimeProfile(b.DWSProfile)
			t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
			cmd := newEmployeeBindingCommand()
			cmd.SetArgs([]string{"--channel", "dsh", "--stdin"})
			cmd.SetIn(strings.NewReader(fmt.Sprintf(`{"agentUuid":%q,"bindingRevision":7}`, b.AgentUUID)))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := corecmd.ExecuteForTest(cmd)
			wantErr := scenario == "wrong-token" || scenario == "lookup-fails" || scenario == "binding-changed"
			if (err != nil) != wantErr {
				t.Fatalf("err=%v output=%s", err, out.String())
			}
			if tokenCalls != 1 || len(caller.calls) != 0 {
				t.Fatalf("tokenCalls=%d calls=%v", tokenCalls, caller.calls)
			}
			if scenario == "wrong-token" {
				if len(caller.tokenCalls) != 0 {
					t.Fatal("身份错配仍查了通讯录")
				}
			} else {
				if len(caller.tokenCalls) != 1 || caller.tokens[0] != "employee-token" || caller.tokenCalls[0].toolName != "search_contact_by_key_word" || caller.tokenCalls[0].args["keyword"] != "employee" {
					t.Fatalf("非员工自身单次查询: %v", caller.tokenCalls)
				}
			}
			if !wantErr && !strings.Contains(out.String(), `"selfOpenDingTalkId": "employee-self"`) {
				t.Fatal(out.String())
			}
			current, e := loadDigitalEmployeeBinding(dir, b.DWSProfile)
			if e != nil || current.SupervisorProfile != "" {
				t.Fatalf("启动改写主管字段: %+v %v", current, e)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeSelfIdentityValidation(t *testing.T) {
	for _, id := range []string{"", " self", "self ", "self\n", "self\tloop", "self\x00loop", strings.Repeat("x", 513)} {
		if validEmployeeSelfIdentity(id) {
			t.Fatalf("无效自身 ID 被接受: %q", id)
		}
	}
	if !validEmployeeSelfIdentity("employee-self") {
		t.Fatal("正常自身 ID 被拒绝")
	}
}
