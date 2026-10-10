// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeVisibilityAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, kind, scope, status, sender string
		users, depts                      []any
		want                              bool
		failTool                          string
		wantErr                           bool
	}{
		{name: "listed member", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", users: []any{"member"}, want: true},
		{name: "operator has no bypass", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "operator", users: []any{"member"}},
		{name: "removed member", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member"},
		{name: "ALL requires organization membership", kind: "local_agent", scope: "ALL", status: "online", sender: "open-member", want: true},
		{name: "ALL rejects unresolvable outsider", kind: "local_agent", scope: "ALL", status: "online", sender: "outsider", wantErr: true},
		{name: "department member", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", depts: []any{"20"}, want: true},
		{name: "child department member", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", depts: []any{"10"}, want: true},
		{name: "other department", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", depts: []any{"30"}},
		{name: "unpublished", kind: "local_agent", scope: "ALL", status: "offline", sender: "open-member"},
		{name: "other scenario", kind: "open_code", scope: "ALL", status: "online", sender: "open-member"},
		{name: "unknown type", kind: "future", scope: "ALL", status: "online", sender: "open-member", wantErr: true},
		{name: "query failure", kind: "local_agent", scope: "ALL", status: "online", sender: "operator", failTool: deapAgentDetailTool, wantErr: true},
		{name: "identity lookup failure", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", users: []any{"member"}, failTool: "search_contact_by_key_word", wantErr: true},
		{name: "department lookup failure", kind: "local_agent", scope: "PARTIAL", status: "online", sender: "open-member", depts: []any{"10"}, failTool: "get_sub_depts_by_dept_id", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:employee", SupervisorProfile: "corp:manager", OperatorOpenDingTalkID: "operator"}
			testseam.Swap(t, &employeeVisibilityCall, func(ctx context.Context, profile, server, tool string, args map[string]any) (map[string]any, error) {
				if tool == tc.failTool {
					return nil, fmt.Errorf("unavailable")
				}
				switch tool {
				case deapAgentDetailTool:
					if profile != b.SupervisorProfile || args["snapshot"] != "published" || args["agentUuid"] != b.AgentUUID {
						t.Fatal("wrong published authority")
					}
					return map[string]any{"data": map[string]any{"agentUuid": "agent", "type": tc.kind, "snapshot": "published", "status": tc.status, "visibility": tc.scope, "staffIds": tc.users, "deptIds": tc.depts, "profile": map[string]any{"corpId": "corp", "userId": "employee", "openDingTalkId": "self"}}}, nil
				case "search_contact_by_key_word":
					if profile != b.DWSProfile {
						t.Fatal("open ID resolved with manager identity")
					}
					return map[string]any{"result": []any{map[string]any{"userId": "member", "openDingTalkId": "open-member"}}}, nil
				case "get_user_info_by_user_ids":
					return map[string]any{"result": []any{map[string]any{"orgEmployeeModel": map[string]any{"orgUserId": "member", "depts": []any{map[string]any{"deptId": "20"}}}}}}, nil
				case "get_sub_depts_by_dept_id":
					children := []any{}
					if args["deptId"] == int64(10) {
						children = append(children, map[string]any{"deptId": "20"})
					}
					return map[string]any{"result": children}, nil
				}
				t.Fatalf("unexpected tool %s", tool)
				return nil, nil
			})
			policy, allowed, err := employeeVisibilityAccess(context.Background(), b, tc.sender, "same name")
			if (err != nil) != tc.wantErr || allowed != tc.want {
				t.Fatalf("policy=%s allowed=%v err=%v", policy, allowed, err)
			}
			if tc.kind == "open_code" && policy != "local_allowlist" {
				t.Fatal("other scenario lost local allowlist")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeVisibilityFailClosedAndRevocation(t *testing.T) {
	b := digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:employee", SupervisorProfile: "corp:manager", RuntimeBindingID: "binding"}
	data := map[string]any{"agentUuid": "agent", "type": "local_agent", "snapshot": "published", "status": "online", "visibility": "PARTIAL", "staffIds": []any{"member"}, "profile": map[string]any{"corpId": "corp", "userId": "employee"}}
	testseam.Swap(t, &employeeVisibilityCall, func(_ context.Context, _, _, tool string, _ map[string]any) (map[string]any, error) {
		if tool == deapAgentDetailTool {
			return map[string]any{"data": data}, nil
		}
		return map[string]any{"result": []any{map[string]any{"userId": "member", "openDingTalkId": "open-member"}}}, nil
	})
	check := func(want, wantErr bool) {
		t.Helper()
		_, allowed, err := employeeVisibilityAccess(context.Background(), b, "open-member", "")
		if allowed != want || (err != nil) != wantErr {
			t.Fatalf("allowed=%v err=%v", allowed, err)
		}
	}
	check(true, false)
	data["staffIds"] = []any{}
	check(false, false)
	data["snapshot"] = "draft"
	check(false, true)
	data["snapshot"] = "published"
	data["agentUuid"] = "other"
	check(false, true)
	b.SupervisorProfile = ""
	check(false, true)
	b.RuntimeBindingID = ""
	policy, _, err := employeeVisibilityAccess(context.Background(), b, "sender", "")
	if err != nil || policy != "local_allowlist" {
		t.Fatal("legacy local scenario changed")
	}
}

func TestCrossPlatformCoverageEmployeeVisibilityBeforeDispatch(t *testing.T) {
	for _, mode := range []string{"allow", "deny", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.Binding = digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:employee", SupervisorProfile: "corp:manager"}
			r.cfg.Options.AllowedUsers = nil
			q := make(chan employeeEvent, 2)
			r.queues[e.ConversationID] = q
			testseam.Swap(t, &employeeVisibilityCall, func(_ context.Context, _, _, tool string, _ map[string]any) (map[string]any, error) {
				if mode == "unavailable" {
					return nil, fmt.Errorf("offline")
				}
				if tool == deapAgentDetailTool {
					users := []any{}
					if mode == "allow" {
						users = []any{"member"}
					}
					return map[string]any{"data": map[string]any{"agentUuid": "agent", "type": "local_agent", "snapshot": "published", "status": "online", "visibility": "PARTIAL", "staffIds": users, "profile": map[string]any{"corpId": "corp", "userId": "employee"}}}, nil
				}
				return map[string]any{"result": []any{map[string]any{"userId": "member", "openDingTalkId": e.SenderID}}}, nil
			})
			err := r.enqueue(e)
			if (err != nil) != (mode == "unavailable") {
				t.Fatalf("enqueue=%v", err)
			}
			_, statErr := os.Stat(r.recordPath(e))
			if (statErr == nil) != (mode == "allow") {
				t.Fatalf("unexpected ledger state: %v", statErr)
			}
			if (len(q) > 0) != (mode == "allow") {
				t.Fatal("authorization was not applied before dispatch")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeVisibilityMigration(t *testing.T) {
	for _, operator := range []string{"operator-open", "other"} {
		t.Run(operator, func(t *testing.T) {
			setupConnectSupervisorSeams(t)
			InitDepsForTest(t, newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`))
			testseam.Swap(t, &employeeVisibilityToken, func(context.Context, string) (*auth.TokenData, error) {
				return &auth.TokenData{AccessToken: "fixture-only"}, nil
			})
			testseam.Swap(t, &employeeVisibilityCall, func(_ context.Context, profile, _, _ string, _ map[string]any) (map[string]any, error) {
				if profile != "supervisor-corp:supervisor-user" {
					t.Fatal("wrong supervisor")
				}
				return map[string]any{"data": map[string]any{"agentUuid": "agent", "profile": map[string]any{"corpId": "corp", "userId": "employee"}}}, nil
			})
			b := digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:employee", RuntimeBindingID: "binding", OperatorOpenDingTalkID: operator}
			err := migrateEmployeeVisibilitySupervisor(context.Background(), &b)
			if operator == "operator-open" {
				if err != nil || b.SupervisorProfile != "supervisor-corp:supervisor-user" {
					t.Fatalf("migration=%v", err)
				}
			} else if err == nil || b.SupervisorProfile != "" {
				t.Fatal("another manager silently took over")
			}
		})
	}
}
