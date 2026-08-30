package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	// remove 是 destructive 入口：Safety confirmation=user_required，测试里
	// 注入根级 --yes 后直接确认执行（参数装配本身由下方断言验证）。
	root := newDriveCommand()
	root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	if err := executePR868Command(t, root, "permission", "remove", "--node", "n1",
		"--members", `[{"type":"CONVERSATION","id":"cid1"}]`, "--yes"); err != nil {
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

// ──────────────────────────────────────────────────────
// collectMembers / validateMembersExclusivity 单元级用例
// 同步自内部 MR 28965577 补齐的覆盖：四类型混传、remove 语义、
// 超过 30 个、corpId 按类型校验、--user 别名。
// ──────────────────────────────────────────────────────

// newMembersCmd 构造带 --members/--users/--user/--role 的最小命令，
// 与生产命令的 flag 注册保持一致。
func newMembersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("members", "", "")
	cmd.Flags().String("users", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("role", "", "")
	return cmd
}

func TestCollectMembersMixedTypes(t *testing.T) {
	cmd := newMembersCmd()
	_ = cmd.Flags().Set("members", `[{"type":"USER","id":"u1","roleId":"MANAGER","corpId":"c1"},{"type":"DEPT","id":"d1","roleId":"editor","corpId":"c1"},{"type":"CONVERSATION","id":"cid1","roleId":"READER"},{"type":"TAG","id":"t1","roleId":"DOWNLOADER","corpId":"c1"}]`)
	got, err := collectMembers(cmd, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 members, got %d", len(got))
	}
	if got[1]["roleId"] != "EDITOR" {
		t.Errorf("member[1] roleId should be normalized to EDITOR, got %v", got[1]["roleId"])
	}
	if got[0]["corpId"] != "c1" {
		t.Errorf("corpId should be preserved, got %v", got[0]["corpId"])
	}
}

func TestCollectMembersRemoveSemantics(t *testing.T) {
	t.Run("remove only needs type and id", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"USER","id":"u1","corpId":"c1"},{"type":"DEPT","id":"d1","corpId":"c1"}]`)
		got, err := collectMembers(cmd, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 members, got %d", len(got))
		}
	})
	t.Run("add still requires roleId", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"USER","id":"u1","corpId":"c1"}]`)
		if _, err := collectMembers(cmd, false); err == nil {
			t.Fatal("expected error for missing roleId, got nil")
		}
	})
}

func TestCollectMembersOver30(t *testing.T) {
	cmd := newMembersCmd()
	members := "["
	for i := 0; i < 31; i++ {
		if i > 0 {
			members += ","
		}
		members += fmt.Sprintf(`{"type":"USER","id":"u%d","roleId":"READER","corpId":"c"}`, i)
	}
	members += "]"
	_ = cmd.Flags().Set("members", members)
	if _, err := collectMembers(cmd, false); err == nil {
		t.Fatal("expected error for >30 members, got nil")
	}
}

func TestCollectMembersCorpIDByType(t *testing.T) {
	t.Run("DEPT requires corpId", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"DEPT","id":"d1","roleId":"EDITOR"}]`)
		if _, err := collectMembers(cmd, false); err == nil {
			t.Fatal("expected error for DEPT missing corpId, got nil")
		}
	})
	t.Run("TAG requires corpId", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"TAG","id":"t1","roleId":"DOWNLOADER"}]`)
		if _, err := collectMembers(cmd, false); err == nil {
			t.Fatal("expected error for TAG missing corpId, got nil")
		}
	})
	t.Run("CONVERSATION works without corpId", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"CONVERSATION","id":"cid1","roleId":"READER"}]`)
		got, err := collectMembers(cmd, false)
		if err != nil {
			t.Fatalf("unexpected error for CONVERSATION without corpId: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 member, got %d", len(got))
		}
	})
	t.Run("TAG with corpId passes", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("members", `[{"type":"TAG","id":"t1","roleId":"DOWNLOADER","corpId":"c1"}]`)
		got, err := collectMembers(cmd, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 member, got %d", len(got))
		}
	})
}

func TestValidateMembersExclusivityUserAlias(t *testing.T) {
	t.Run("--user alias counts as users", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("user", "u1")
		if err := validateMembersExclusivity(cmd); err != nil {
			t.Errorf("unexpected error for --user alias: %v", err)
		}
	})
	t.Run("--user alias conflicts with --members", func(t *testing.T) {
		cmd := newMembersCmd()
		_ = cmd.Flags().Set("user", "u1")
		_ = cmd.Flags().Set("members", `[{"type":"USER","id":"u1","roleId":"READER","corpId":"c1"}]`)
		if err := validateMembersExclusivity(cmd); err == nil {
			t.Error("expected error when both --user and --members set, got nil")
		}
	})
}

// ──────────────────────────────────────────────────────
// isNoPermissionError — 新接口 forbidden.accessDenied 错误码
// ──────────────────────────────────────────────────────

func TestIsNoPermissionErrorAccessDenied(t *testing.T) {
	t.Run("server code forbidden.accessDenied", func(t *testing.T) {
		for _, key := range []string{"code", "errorCode", "server_error_code"} {
			body := map[string]any{key: "forbidden.accessDenied"}
			if !isNoPermissionError(body) {
				t.Errorf("isNoPermissionError(%s=forbidden.accessDenied) = false, want true", key)
			}
		}
	})
	t.Run("access-denied message text", func(t *testing.T) {
		for _, msg := range []string{
			"需要您具备 EDITOR 及以上角色",
			"需要您具备 MANAGER 及以上角色才能执行此操作",
			"forbidden.accessDenied",
			"Forbidden.AccessDenied",
		} {
			body := map[string]any{"errorMsg": msg}
			if !isNoPermissionError(body) {
				t.Errorf("isNoPermissionError(msg=%q) = false, want true", msg)
			}
		}
	})
	t.Run("normal body is not permission error", func(t *testing.T) {
		body := map[string]any{"success": true, "errorMsg": ""}
		if isNoPermissionError(body) {
			t.Error("isNoPermissionError(success body) = true, want false")
		}
	})
}

// ──────────────────────────────────────────────────────
// --notify bool flag 解析（NoArgs 防御 `--notify false` 空格形式）
// ──────────────────────────────────────────────────────

// newNotifyCmd 构造与生产 add/update 命令一致的最小命令：
// --notify bool flag + Args: cobra.NoArgs。
func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.Flags().Bool("notify", false, "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

func TestNotifyFlagParsing(t *testing.T) {
	t.Run("equals form sets false", func(t *testing.T) {
		cmd := newNotifyCmd()
		cmd.SetArgs([]string{"--notify=false"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := cmd.Flags().GetBool("notify"); got {
			t.Error("--notify=false: got true, want false")
		}
		if !cmd.Flags().Changed("notify") {
			t.Error("--notify=false should mark flag as changed")
		}
	})
	t.Run("bare flag defaults true via NoOptDefVal", func(t *testing.T) {
		cmd := newNotifyCmd()
		cmd.SetArgs([]string{"--notify"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := cmd.Flags().GetBool("notify"); !got {
			t.Error("--notify (no value): got false, want true (NoOptDefVal)")
		}
		if !cmd.Flags().Changed("notify") {
			t.Error("--notify should mark flag as changed")
		}
	})
	t.Run("space form rejected by NoArgs", func(t *testing.T) {
		cmd := newNotifyCmd()
		// "--notify false" 被 pflag 解析为 notify=true（NoOptDefVal）+ 位置参数 "false"。
		// Args: cobra.NoArgs 拒绝位置参数，防止用户误以为传了 false。
		cmd.SetArgs([]string{"--notify", "false"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected error for '--notify false' (space form) with NoArgs, got nil")
		}
		if got, _ := cmd.Flags().GetBool("notify"); !got {
			t.Error("pflag should set notify=true via NoOptDefVal for bare --notify, got false")
		}
	})
	t.Run("omitted stays unchanged", func(t *testing.T) {
		cmd := newNotifyCmd()
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd.Flags().Changed("notify") {
			t.Error("notify should not be marked changed when omitted")
		}
		if got, _ := cmd.Flags().GetBool("notify"); got {
			t.Error("omitted notify: got true, want false (default)")
		}
	})
}

// ──────────────────────────────────────────────────────
// pageSize 1..50 运行时边界（P2 意见：服务端 pageSize 上限 50）
// ──────────────────────────────────────────────────────

func newPageSizeCmd(changed string, value int) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("limit", 30, "")
	cmd.Flags().Int("max-results", 0, "")
	_ = cmd.Flags().Set(changed, fmt.Sprintf("%d", value))
	return cmd
}

func TestPermissionPageSizeFromFlagsBoundaries(t *testing.T) {
	t.Run("rejects out-of-range values", func(t *testing.T) {
		for _, flag := range []string{"limit", "max-results"} {
			for _, size := range []int{-1, 0, 51, 100} {
				if _, _, err := permissionPageSizeFromFlags(newPageSizeCmd(flag, size)); err == nil {
					t.Errorf("--%s %d: expected error, got nil", flag, size)
				}
			}
		}
	})
	t.Run("accepts in-range values", func(t *testing.T) {
		for _, size := range []int{1, 30, 50} {
			got, ok, err := permissionPageSizeFromFlags(newPageSizeCmd("limit", size))
			if err != nil {
				t.Errorf("--limit %d: unexpected error %v", size, err)
				continue
			}
			if !ok || got != size {
				t.Errorf("--limit %d: got (%d, %v)", size, got, ok)
			}
		}
	})
	t.Run("omitted returns ok=false", func(t *testing.T) {
		// newPageSizeCmd 强制 Set(changed)，先验证 Set 后 Changed 生效，
		// 再用未设置任何 flag 的命令验证默认路径。
		if got, ok, err := permissionPageSizeFromFlags(newPageSizeCmd("limit", 30)); err != nil || !ok || got != 30 {
			t.Fatalf("setup sanity failed: got (%d, %v, %v), want (30, true, nil)", got, ok, err)
		}
		fresh := &cobra.Command{Use: "test"}
		fresh.Flags().Int("limit", 30, "")
		fresh.Flags().Int("max-results", 0, "")
		if got, ok, err := permissionPageSizeFromFlags(fresh); err != nil || ok || got != 0 {
			t.Errorf("omitted flags: got (%d, %v, %v), want (0, false, nil)", got, ok, err)
		}
	})
}

func TestPermissionListPageSizeRuntimeGuard(t *testing.T) {
	type listCase struct {
		name   string
		root   func() *cobra.Command
		path   []string
		nodeID []string
	}
	cases := []listCase{
		{"drive", newDriveCommand, []string{"permission", "list"}, []string{"--node", "n1"}},
		{"doc", newDocCommand, []string{"permission", "list"}, []string{"--node", "n1"}},
		{"wiki", newWikiCommand, []string{"member", "list"}, []string{"--workspace", "ws1"}},
	}
	for _, lc := range cases {
		t.Run(lc.name+" rejects --limit 51", func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			args := append(append([]string{}, lc.path...), lc.nodeID...)
			args = append(args, "--limit", "51")
			err := executePR868Command(t, lc.root(), args...)
			if err == nil || !strings.Contains(err.Error(), "1..50") {
				t.Fatalf("expected 1..50 validation error, got %v", err)
			}
			if caller.calls != 0 {
				t.Fatalf("list should not call MCP with invalid pageSize, called %d times", caller.calls)
			}
		})
		t.Run(lc.name+" accepts --limit 50", func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
			installScriptedCaller(t, caller)
			args := append(append([]string{}, lc.path...), lc.nodeID...)
			args = append(args, "--limit", "50")
			if err := executePR868Command(t, lc.root(), args...); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if caller.args["pageSize"] != 50 {
				t.Fatalf("pageSize=%#v, want 50", caller.args["pageSize"])
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// add 的 --notify 声明默认值必须与实际透传行为一致
//
// 三个 add 命令都用 `if Flags().Changed("notify")` 守卫来决定是否把 notify
// 写入 toolArgs，所以省略该 flag 时字段根本不会发给服务端，实测服务端按
// 不通知处理。声明默认值因此必须是 false —— 若有人把它改回 true，help 会
// 承诺一个从未透传的默认值（曾经的真实缺陷）。
// ──────────────────────────────────────────────────────

func TestPermissionAddNotifyDefaultMatchesWireBehavior(t *testing.T) {
	type addCase struct {
		name   string
		root   func() *cobra.Command
		path   []string
		target []string
	}
	cases := []addCase{
		{"drive", newDriveCommand, []string{"permission", "add"}, []string{"--node", "n1"}},
		{"doc", newDocCommand, []string{"permission", "add"}, []string{"--node", "n1"}},
		{"wiki", newWikiCommand, []string{"member", "add"}, []string{"--workspace", "ws1"}},
	}
	const members = `[{"type":"USER","id":"u1","roleId":"READER","corpId":"c1"}]`

	runAdd := func(t *testing.T, lc addCase, extra ...string) *scriptedToolCaller {
		t.Helper()
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		args := append(append([]string{}, lc.path...), lc.target...)
		args = append(args, "--members", members)
		args = append(args, extra...)
		if err := executePR868Command(t, lc.root(), args...); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return caller
	}

	for _, lc := range cases {
		t.Run(lc.name+" omitted sends no notify field", func(t *testing.T) {
			caller := runAdd(t, lc)
			if v, present := caller.args["notify"]; present {
				t.Fatalf("notify must be absent from toolArgs when --notify is omitted, got %#v", v)
			}
		})
		t.Run(lc.name+" declared default is false", func(t *testing.T) {
			// 声明默认值必须与上面的透传行为一致，否则 help 在说谎。
			root := lc.root()
			leaf, _, err := root.Find(lc.path)
			if err != nil {
				t.Fatalf("find %v: %v", lc.path, err)
			}
			flag := leaf.Flags().Lookup("notify")
			if flag == nil {
				t.Fatal("notify flag not declared")
			}
			if flag.DefValue != "false" {
				t.Errorf("notify DefValue=%q, want \"false\": 省略时不透传该字段，声明 true 会承诺一个未生效的默认值", flag.DefValue)
			}
		})
		t.Run(lc.name+" bare --notify opts in", func(t *testing.T) {
			caller := runAdd(t, lc, "--notify")
			if notify, ok := caller.args["notify"].(bool); !ok || !notify {
				t.Fatalf("notify=%#v, want true for bare --notify", caller.args["notify"])
			}
		})
		t.Run(lc.name+" --notify=false is sent explicitly", func(t *testing.T) {
			caller := runAdd(t, lc, "--notify=false")
			notify, ok := caller.args["notify"].(bool)
			if !ok || notify {
				t.Fatalf("notify=%#v, want explicit false", caller.args["notify"])
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// 服务端对已确认“空响应=写成功”契约的 permission/member update/remove
// 工具可能返回字面 "null"（操作成功但无返回数据）。CLI 仅对这几个
// 工具将其渲染为空对象 {}，避免 Agent/下游把 null 当作对象解析时失败；
// 其它工具的合法 null 保持原样输出，公共渲染器的机器输出契约不变。
// ──────────────────────────────────────────────────────

func TestCrossPlatformCoverageNullToolResponseRendersEmptyObject(t *testing.T) {
	// nullOnSuccessTools 集合内：null 适配为 {}
	for _, format := range []string{"json", "raw"} {
		t.Run("adapted_"+format, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: "null"}}, format: format}
			installScriptedCaller(t, caller)
			out := &bytes.Buffer{}
			deps.Out.w = out
			if err := callMCPToolInternalOptsContext(context.Background(), "drive", "update_permission", nil, false); err != nil {
				t.Fatalf("null tool response should render as {}: %v", err)
			}
			if got := strings.TrimSpace(out.String()); got != "{}" {
				t.Errorf("format %s renders %q, want {}", format, got)
			}
		})
	}
	// 集合外的工具：null 原样输出，不再被公共渲染路径改写为 {}
	for _, format := range []string{"json", "raw"} {
		t.Run("passthrough_"+format, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: "null"}}, format: format}
			installScriptedCaller(t, caller)
			out := &bytes.Buffer{}
			deps.Out.w = out
			if err := callMCPToolInternalOptsContext(context.Background(), "drive", "update_node_permission", nil, false); err != nil {
				t.Fatalf("null passthrough should not error: %v", err)
			}
			if got := strings.TrimSpace(out.String()); got != "null" {
				t.Errorf("format %s renders %q, want null unchanged", format, got)
			}
		})
	}
}

// ──────────────────────────────────────────────────────
// update / remove 的 --members 装配与 --users 解析失败分支
//
// add 的 members+notify 透传已有
// TestPermissionAddNotifyDefaultMatchesWireBehavior 覆盖；这里补齐
// update（members + notify）与 doc/wiki remove（members）的同构分支，
// 以及各产品 add/update/remove 中 collectUserIDs 的错误路径：
// --users 传纯空白时 flagOrFallback 视为已提供（非空字符串），
// 但 parseCommentMentionIds 过滤空白后为空，collectUserIDs 必须报错
// 而不是向服务端发送空 userIds。
// ──────────────────────────────────────────────────────

func TestCrossPlatformCoveragePermissionUpdateWithMembers(t *testing.T) {
	type updateCase struct {
		name   string
		root   func() *cobra.Command
		path   []string
		target []string
		tool   string
	}
	cases := []updateCase{
		{"drive", newDriveCommand, []string{"permission", "update"}, []string{"--node", "n1"}, "update_permission"},
		{"doc", newDocCommand, []string{"permission", "update"}, []string{"--node", "n1"}, "update_permission"},
		{"wiki", newWikiCommand, []string{"member", "update"}, []string{"--workspace", "ws1"}, "update_member"},
	}
	const members = `[{"type":"USER","id":"u1","roleId":"READER","corpId":"c1"}]`
	for _, uc := range cases {
		t.Run(uc.name+" passes members and notify", func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
			installScriptedCaller(t, caller)
			args := append(append([]string{}, uc.path...), uc.target...)
			args = append(args, "--members", members, "--notify")
			if err := executePR868Command(t, uc.root(), args...); err != nil {
				t.Fatalf("update --members: %v", err)
			}
			if caller.tool != uc.tool {
				t.Fatalf("tool=%q, want %q", caller.tool, uc.tool)
			}
			got, ok := caller.args["members"].([]map[string]any)
			if !ok || len(got) != 1 || got[0]["id"] != "u1" || got[0]["roleId"] != "READER" {
				t.Fatalf("members=%#v", caller.args["members"])
			}
			if notify, ok := caller.args["notify"].(bool); !ok || !notify {
				t.Fatalf("notify=%#v, want true for bare --notify", caller.args["notify"])
			}
		})
	}
}

func TestCrossPlatformCoveragePermissionRemoveWithMembersProducts(t *testing.T) {
	// drive remove 已由 TestPermissionRemoveWithMembers 覆盖；这里补 doc/wiki。
	type removeCase struct {
		name   string
		root   func() *cobra.Command
		path   []string
		target []string
		tool   string
	}
	cases := []removeCase{
		{"doc", newDocCommand, []string{"permission", "remove"}, []string{"--node", "n1"}, "remove_permission"},
		{"wiki", newWikiCommand, []string{"member", "remove"}, []string{"--workspace", "ws1"}, "remove_member"},
	}
	for _, rc := range cases {
		t.Run(rc.name+" passes members", func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
			installScriptedCaller(t, caller)
			// remove 需用户确认（confirmation=user_required）：注入根级 --yes。
			root := rc.root()
			root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
			args := append(append([]string{}, rc.path...), rc.target...)
			args = append(args, "--members", `[{"type":"CONVERSATION","id":"cid1"}]`, "--yes")
			if err := executePR868Command(t, root, args...); err != nil {
				t.Fatalf("remove --members: %v", err)
			}
			if caller.tool != rc.tool {
				t.Fatalf("tool=%q, want %q", caller.tool, rc.tool)
			}
			got, ok := caller.args["members"].([]map[string]any)
			if !ok || len(got) != 1 || got[0]["id"] != "cid1" {
				t.Fatalf("members=%#v", caller.args["members"])
			}
		})
	}
}

func TestCrossPlatformCoveragePermissionUsersBlankParseError(t *testing.T) {
	type blankCase struct {
		name     string
		root     func() *cobra.Command
		path     []string
		target   []string
		needRole bool
	}
	cases := []blankCase{
		{"drive add", newDriveCommand, []string{"permission", "add"}, []string{"--node", "n1"}, true},
		{"drive update", newDriveCommand, []string{"permission", "update"}, []string{"--node", "n1"}, true},
		{"drive remove", newDriveCommand, []string{"permission", "remove"}, []string{"--node", "n1"}, false},
		{"doc add", newDocCommand, []string{"permission", "add"}, []string{"--node", "n1"}, true},
		{"doc update", newDocCommand, []string{"permission", "update"}, []string{"--node", "n1"}, true},
		{"doc remove", newDocCommand, []string{"permission", "remove"}, []string{"--node", "n1"}, false},
		{"wiki add", newWikiCommand, []string{"member", "add"}, []string{"--workspace", "ws1"}, true},
		{"wiki update", newWikiCommand, []string{"member", "update"}, []string{"--workspace", "ws1"}, true},
		{"wiki remove", newWikiCommand, []string{"member", "remove"}, []string{"--workspace", "ws1"}, false},
	}
	for _, bc := range cases {
		t.Run(bc.name+" rejects blank --users", func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			args := append(append([]string{}, bc.path...), bc.target...)
			args = append(args, "--users", " ")
			if bc.needRole {
				// add/update 的旧格式分支在 collectUserIDs 之前校验必填 --role。
				args = append(args, "--role", "READER")
			}
			err := executePR868Command(t, bc.root(), args...)
			if err == nil || !strings.Contains(err.Error(), "--users is required") {
				t.Fatalf("expected --users is required error, got %v", err)
			}
			if caller.calls != 0 {
				t.Fatalf("MCP must not be called when --users resolves empty, called %d times", caller.calls)
			}
		})
	}
}
