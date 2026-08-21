package helpers

import (
	"strings"
	"testing"
)

// 同步自闭源 MR 28965577（知识库/节点权限增删改查改造）：
// 权限/成员 add/update/remove 支持 --members 新格式（USER/DEPT/CONVERSATION/TAG），
// list 支持 nextToken 翻页。以下用例覆盖 CLI → MCP 参数装配契约。

func TestCollectMembersParsesNewFormat(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
	installScriptedCaller(t, caller)
	if err := executePR868Command(t, newDriveCommand(), "permission", "add", "--node", "n1",
		"--members", `[{"type":"USER","id":"u1","roleId":"reader","corpId":"c1"},{"type":"TAG","id":"t1","roleId":"editor","corpId":"c1"}]`,
		"--notify=false"); err != nil {
		t.Fatalf("permission add --members: %v", err)
	}
	if caller.tool != "add_permission" {
		t.Fatalf("tool=%q", caller.tool)
	}
	members, ok := caller.args["members"].([]map[string]any)
	if !ok || len(members) != 2 {
		t.Fatalf("members=%#v", caller.args["members"])
	}
	if members[0]["roleId"] != "READER" || members[1]["roleId"] != "EDITOR" {
		t.Fatalf("roleId normalize failed: %#v", members)
	}
	if notify, ok := caller.args["notify"].(bool); !ok || notify {
		t.Fatalf("notify should be false when --notify=false: %#v", caller.args["notify"])
	}
}

func TestCollectMembersValidation(t *testing.T) {
	cases := []struct {
		name      string
		members   string
		wantError string
	}{
		{"invalid json", "[{", "JSON 解析失败"},
		{"empty array", "[]", "不能为空数组"},
		{"missing type", `[{"id":"u1","roleId":"READER","corpId":"c1"}]`, "缺少必填字段 type"},
		{"missing id", `[{"type":"USER","roleId":"READER","corpId":"c1"}]`, "缺少必填字段 id"},
		{"missing corpId", `[{"type":"USER","id":"u1","roleId":"READER"}]`, "需携带 corpId"},
		{"missing roleId", `[{"type":"USER","id":"u1","corpId":"c1"}]`, "缺少必填字段 roleId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			err := executePR868Command(t, newDriveCommand(), "permission", "add", "--node", "n1", "--members", tc.members)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
		})
	}
}

func TestValidateMembersExclusivity(t *testing.T) {
	t.Run("members and users are mutually exclusive", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newDriveCommand(), "permission", "add", "--node", "n1",
			"--users", "u1", "--members", `[{"type":"USER","id":"u1","roleId":"READER","corpId":"c1"}]`)
		if err == nil || !strings.Contains(err.Error(), "互斥") {
			t.Fatalf("expected mutual exclusion error, got %v", err)
		}
	})
	t.Run("one of members or users is required", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newDriveCommand(), "permission", "add", "--node", "n1")
		if err == nil || !strings.Contains(err.Error(), "之一") {
			t.Fatalf("expected required error, got %v", err)
		}
	})
	t.Run("role is redundant with members", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newDriveCommand(), "permission", "add", "--node", "n1",
			"--role", "READER", "--members", `[{"type":"USER","id":"u1","roleId":"READER","corpId":"c1"}]`)
		if err == nil || !strings.Contains(err.Error(), "不需要 --role") {
			t.Fatalf("expected no-role error, got %v", err)
		}
	})
}

func TestPermissionListPagination(t *testing.T) {
	t.Run("drive permission list passes nextToken and pageSize", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newDriveCommand(), "permission", "list", "--node", "n1",
			"--limit", "50", "--next-token", "50"); err != nil {
			t.Fatalf("permission list: %v", err)
		}
		if caller.tool != "list_permission" {
			t.Fatalf("tool=%q", caller.tool)
		}
		if caller.args["pageSize"] != 50 {
			t.Fatalf("pageSize=%#v", caller.args["pageSize"])
		}
		if caller.args["nextToken"] != "50" {
			t.Fatalf("nextToken=%#v", caller.args["nextToken"])
		}
	})
	t.Run("doc permission list passes nextToken", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newDocCommand(), "permission", "list", "--node", "n1",
			"--next-token", "30"); err != nil {
			t.Fatalf("doc permission list: %v", err)
		}
		if caller.args["nextToken"] != "30" {
			t.Fatalf("nextToken=%#v", caller.args["nextToken"])
		}
	})
	t.Run("wiki member list passes nextToken and pageSize", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newWikiCommand(), "member", "list", "--workspace", "ws1",
			"--limit", "50", "--next-token", "50"); err != nil {
			t.Fatalf("wiki member list: %v", err)
		}
		if caller.tool != "list_member" {
			t.Fatalf("tool=%q", caller.tool)
		}
		if caller.args["pageSize"] != 50 || caller.args["nextToken"] != "50" {
			t.Fatalf("args=%#v", caller.args)
		}
	})
}

func TestPermissionRemoveWithMembers(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
	installScriptedCaller(t, caller)
	if err := executePR868Command(t, newDriveCommand(), "permission", "remove", "--node", "n1",
		"--members", `[{"type":"CONVERSATION","id":"cid1"}]`); err != nil {
		t.Fatalf("permission remove --members: %v", err)
	}
	if caller.tool != "remove_permission" {
		t.Fatalf("tool=%q", caller.tool)
	}
	members, ok := caller.args["members"].([]map[string]any)
	if !ok || len(members) != 1 || members[0]["id"] != "cid1" {
		t.Fatalf("members=%#v", caller.args["members"])
	}
	// remove 语义下 roleId 不应被要求
	if _, hasRole := members[0]["roleId"]; hasRole {
		t.Fatalf("remove members should not require roleId: %#v", members[0])
	}
}

func TestWikiMemberAddWithMembersRejectsOwner(t *testing.T) {
	caller := &scriptedToolCaller{}
	installScriptedCaller(t, caller)
	err := executePR868Command(t, newWikiCommand(), "member", "add", "--workspace", "ws1",
		"--members", `[{"type":"USER","id":"u1","roleId":"OWNER","corpId":"c1"}]`)
	if err == nil || !strings.Contains(err.Error(), "OWNER") {
		t.Fatalf("expected OWNER rejection, got %v", err)
	}
}
