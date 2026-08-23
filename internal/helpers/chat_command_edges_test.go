package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func runChatCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	installExampleGlobalFlags(root)
	root.PersistentFlags().Bool("debug", false, "")
	root.PersistentFlags().Bool("verbose", false, "")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append(append([]string(nil), args...), "--yes"))
	return root.ExecuteContext(context.Background())
}

func runChatCoverageDirect(t *testing.T, path []string, flags map[string]string) error {
	t.Helper()
	InitDeps(&scriptedToolCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	installExampleGlobalFlags(root)
	root.PersistentFlags().Bool("debug", false, "")
	root.PersistentFlags().Bool("verbose", false, "")
	command, _, err := root.Find(path)
	if err != nil {
		return err
	}
	for name, value := range flags {
		flag := command.Flag(name)
		if flag == nil {
			return fmt.Errorf("no such flag -%s", name)
		}
		if err := flag.Value.Set(value); err != nil {
			return err
		}
		flag.Changed = true
	}
	return command.RunE(command, nil)
}

func TestCrossPlatformCoverageEvaluationRegressionChatSearchSpellingsAndNaturalBotTarget(t *testing.T) {
	if got, err := resolveNativeChatTarget("  cid123456789  "); err != nil || got != "cid123456789" {
		t.Fatalf("stable native chat target = %q, %v", got, err)
	}
	t.Run("group search path accepts query", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		if err := runChatCoverageCommand(t, caller, "group", "search", "--query", "项目群"); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 {
			t.Fatalf("calls = %d", caller.calls)
		}
	})

	t.Run("group search accepts positional", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		if err := runChatCoverageCommand(t, caller, "group", "search", "项目群"); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 {
			t.Fatalf("calls = %d", caller.calls)
		}
	})

	t.Run("native bots resolves group name", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"result":[{"openConversationId":"cid-project","title":"项目群"}],"hasMore":false}`},
			{text: `{"result":{"bots":[]}}`},
		}}
		if err := runChatCoverageCommand(t, caller, "group", "bots", "--group", "项目群"); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 2 {
			t.Fatalf("calls = %d", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageChatStableCompatibilityHintsRemainAvailable(t *testing.T) {
	root := newChatCommand()
	if len(root.Aliases) != 1 || root.Aliases[0] != "im" {
		t.Fatalf("chat aliases = %v, want [im]", root.Aliases)
	}
	for _, tc := range []struct {
		path string
		args []string
		hint string
	}{
		{path: "send", args: []string{"send", "--group", "cid-stable", "--text", "hello"}, hint: "dws chat message send"},
		{path: "history", args: []string{"history", "--group", "cid-stable", "--limit", "20"}, hint: "dws chat message list --conversation-id <GROUP_OPEN_CONVERSATION_ID>"},
	} {
		command, remaining, err := root.Find([]string{tc.path})
		if err != nil {
			t.Fatalf("find chat %s: %v", tc.path, err)
		}
		if len(remaining) != 0 || command.Name() != tc.path {
			t.Fatalf("find chat %s = command %q, remaining %v", tc.path, command.Name(), remaining)
		}
		if !command.Hidden || !command.Runnable() {
			t.Fatalf("chat %s compatibility contract: hidden=%v runnable=%v", tc.path, command.Hidden, command.Runnable())
		}
		root.SetArgs(tc.args)
		err = root.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "ambiguous command") || !strings.Contains(err.Error(), tc.hint) {
			t.Fatalf("chat %s with legacy flags error = %v, want migration hint %q", tc.path, err, tc.hint)
		}
	}
}

func TestCrossPlatformCoverageChatAliasInstallerRemainingEdges(t *testing.T) {
	restoreChatManifestExternalVisibleFlags(nil)

	mismatchedRoot := &cobra.Command{Use: "chat"}
	mismatchedRoot.AddCommand(&cobra.Command{Use: "other"})
	restoreChatManifestExternalVisibleFlags(mismatchedRoot)

	missingPrimary := &cobra.Command{Use: "leaf"}
	installChatFlagAliases(missingPrimary, "conversation-id", []string{"group"}, requireChatConversationID)
	if flag := missingPrimary.Flags().Lookup("group"); flag != nil {
		t.Fatalf("alias registered without canonical flag: %#v", flag)
	}

	skipGroup := &cobra.Command{Use: "leaf", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	skipGroup.Flags().String("conversation-id", "", "")
	skipGroup.Flags().String("group-name", "", "")
	installChatFlagAliases(skipGroup, "conversation-id", []string{"group", "chat"}, requireChatConversationID)
	if flag := skipGroup.Flags().Lookup("group"); flag != nil {
		t.Fatalf("group alias registered beside group-name: %#v", flag)
	}
	if flag := skipGroup.Flags().Lookup("chat"); flag == nil {
		t.Fatal("non-group alias was not registered")
	}

	preRunCalled := false
	withPreRun := &cobra.Command{
		Use: "leaf",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			preRunCalled = true
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	withPreRun.Flags().String("conversation-id", "", "")
	installChatFlagAliases(withPreRun, "conversation-id", []string{"group"}, requireChatConversationID)
	withPreRun.SetArgs([]string{"--group", "cid"})
	if err := withPreRun.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute with alias and previous PreRunE: %v", err)
	}
	if !preRunCalled {
		t.Fatal("previous PreRunE was not called")
	}
}

func TestCrossPlatformCoverageChatMessageForwardRequiresMessageID(t *testing.T) {
	caller := &productExampleCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	command, _, err := root.Find([]string{"message", "forward"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"message-id", "msg-id"} {
		if flag := command.Flags().Lookup(name); flag != nil && flag.Annotations != nil {
			delete(flag.Annotations, cobra.BashCompOneRequiredFlag)
		}
	}
	if err := command.Flags().Set("src-conversation-id", "src"); err != nil {
		t.Fatal(err)
	}
	if err := command.Flags().Set("dest-conversation-id", "dest"); err != nil {
		t.Fatal(err)
	}
	err = command.RunE(command, nil)
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --message-id") {
		t.Fatalf("forward missing message id error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", caller.calls)
	}
}

func TestCrossPlatformCoverageChatGroupUpdateIconAcceptsUploadedMediaIDPrefixes(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	for _, tc := range []struct {
		name    string
		mediaID string
	}{
		{name: "at prefix", mediaID: "@lADPvalidMediaID"},
		{name: "dollar prefix", mediaID: "$iAEvalidMediaID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &productExampleCaller{}
			err := runChatCoverageCommand(t, caller,
				"group", "update-icon", "--group=cid", "--icon-media-id="+tc.mediaID)
			if err != nil {
				t.Fatalf("update group icon with uploaded media ID %q: %v", tc.mediaID, err)
			}
			if caller.calls != 1 {
				t.Fatalf("tool calls = %d, want 1", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupUpdateIconRejectsBlankMediaID(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	caller := &productExampleCaller{}
	err := runChatCoverageCommand(t, caller,
		"group", "update-icon", "--group=cid", "--icon-media-id=   ")
	if err == nil {
		t.Fatal("update group icon with a blank media ID succeeded, want validation error")
	}
	if caller.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", caller.calls)
	}
}

func TestChatGroupRoleSetUserAcceptsSingleRoleIDAndLegacyRoleIDs(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "public single role id",
			args: []string{"group-role", "set-user", "--group=cid", "--user=D1", "--role-id=r1"},
			want: []string{"r1"},
		},
		{
			name: "hidden legacy role ids",
			args: []string{"group-role", "set-user", "--group=cid", "--user=D1", "--role-ids=r1,r2"},
			want: []string{"r1", "r2"},
		},
		{
			name: "hidden legacy empty role ids",
			args: []string{"group-role", "set-user", "--group=cid", "--user=D1", "--role-ids="},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			if err := runChatCoverageCommand(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 {
				t.Fatalf("tool calls = %d, want 1", caller.calls)
			}
			if got := caller.args["openRoleIds"]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("openRoleIds = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestChatGroupRoleSetUserRejectsConflictingRoleFlags(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	caller := &scriptedToolCaller{}
	err := runChatCoverageCommand(t, caller,
		"group-role", "set-user", "--group=cid", "--user=D1", "--role-id=r1", "--role-ids=r2")
	if err == nil {
		t.Fatal("set-user accepted --role-id with --role-ids, want validation error")
	}
	if !strings.Contains(err.Error(), "--role-id 与 --role-ids 不能同时指定") {
		t.Fatalf("error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", caller.calls)
	}
}

func TestChatGroupRoleSetUserRejectsMultiplePublicRoleIDs(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	caller := &scriptedToolCaller{}
	err := runChatCoverageCommand(t, caller,
		"group-role", "set-user", "--group=cid", "--user=D1", "--role-id=r1,r2")
	if err == nil {
		t.Fatal("set-user accepted multiple --role-id values, want validation error")
	}
	if !strings.Contains(err.Error(), "--role-id 只允许指定一个群身份") {
		t.Fatalf("error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", caller.calls)
	}
}

func TestChatGroupRoleSetUserRejectsMissingOrEmptyPublicRoleID(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing public role id",
			args: []string{"group-role", "set-user", "--group=cid", "--user=D1"},
			want: "role-id",
		},
		{
			name: "empty public role id",
			args: []string{"group-role", "set-user", "--group=cid", "--user=D1", "--role-id="},
			want: "--role-id 不能为空",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runChatCoverageCommand(t, caller, tc.args...)
			if err == nil {
				t.Fatal("set-user accepted a missing or empty --role-id, want validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("tool calls = %d, want 0", caller.calls)
			}
		})
	}
}

func TestChatGroupRoleSetUserRoleIDResolverDefensiveBranches(t *testing.T) {
	newCommand := func(t *testing.T) *cobra.Command {
		t.Helper()
		cmd := &cobra.Command{}
		cmd.Flags().String("role-id", "", "")
		cmd.Flags().String("role-ids", "", "")
		return cmd
	}

	t.Run("legacy flag without pre-run promotion", func(t *testing.T) {
		cmd := newCommand(t)
		if err := cmd.Flags().Set("role-ids", "r1,r2"); err != nil {
			t.Fatal(err)
		}
		got, err := resolveChatGroupRoleSetUserRoleIDs(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"r1", "r2"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("role IDs = %#v, want %#v", got, want)
		}
	})

	t.Run("no role flag", func(t *testing.T) {
		_, err := resolveChatGroupRoleSetUserRoleIDs(newCommand(t))
		if err == nil || !strings.Contains(err.Error(), "缺少必填参数 --role-id") {
			t.Fatalf("error = %v, want missing --role-id validation", err)
		}
	})
}

func TestCrossPlatformCoverageChatCommandValidationAndSuccessEdges(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })
	caller := &productExampleCaller{}
	if got := formatChatMessageListAllTime(0); got == "" {
		t.Fatal("formatChatMessageListAllTime returned empty string")
	}

	commands := [][]string{
		{"chmod", "chat.read", "--ttl="},
		{"chmod", "chat.read", "--conversation-id=cid"},
		{"message", "list", "--group=cid", "--time=2026-01-01", "--limit=1"},
		{"message", "list", "--user=D-user", "--time=2026-01-01", "--limit=1"},
		{"message", "list", "--open-dingtalk-id=D-user", "--time=2026-01-01", "--direction=sideways"},
		{"message", "list-direct", "--user=D-user", "--limit=1"},
		{"message", "list-all"},
		{"message", "list-all", "--end=2026-01-02T00:00:00Z"},
		{"message", "list-all", "--end=bad"},
		{"message", "list-all", "--start=not-a-time", "--end=2026-01-02T00:00:00Z"},
		{"message", "list-all", "--start=2026-01-01 00:00:00", "--end=bad"},
		{"message", "list-all", "--start=2026-01-02 00:00:00", "--end=2026-01-01 00:00:00"},
		{"message", "list-all", "--start=2027-01-01 00:00:00"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=bad"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=2026-01-01T00:00:00Z", "--end=bad"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=2026-01-02T00:00:00Z", "--end=2026-01-01T00:00:00Z"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--end=2026-01-02T00:00:00Z"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=2026-01-01T00:00:00Z", "--end=2026-01-02T00:00:00Z"},
		{"message", "list-by-sender", "--sender-open-dingtalk-id=D1", "--start=2026-01-01T00:00:00Z"},
		{"message", "list-mentions", "--start=bad", "--end=2026-01-02T00:00:00Z"},
		{"message", "list-mentions", "--start=2026-01-01T00:00:00Z", "--end=bad"},
		{"message", "list-mentions", "--start=2026-01-02T00:00:00Z", "--end=2026-01-01T00:00:00Z"},
		{"message", "list-mentions", "--start=2026-01-01T00:00:00Z", "--end=2026-01-02T00:00:00Z", "--group=cid"},
		{"message", "search", "--query=q", "--start=bad", "--end=2026-01-02T00:00:00Z"},
		{"message", "search", "--query=q", "--start=2026-01-01T00:00:00Z", "--end=bad"},
		{"message", "search", "--query=q", "--start=2026-01-02T00:00:00Z", "--end=2026-01-01T00:00:00Z"},
		{"message", "search", "--query=q", "--start=2026-01-01T00:00:00Z", "--end=2026-01-02T00:00:00Z", "--group=cid"},
		{"message", "recall", "--conversation-id=cid", "--msg-id=mid"},
		{"category", "delete", "--category-id=1"},
		{"category", "rename", "--category-id=1", "--title=renamed"},
		{"category", "add-conv", "--group=cid", "--category-ids=1,2"},
		{"category", "remove-conv", "--group=cid", "--category-ids=1,2"},
		{"message", "list-by-ids", "--msg-ids=" + strings.Repeat("id,", 51) + "last"},
		{"group", "transfer-owner", "--group=cid", "--new-owner=D-owner"},
		{"group", "transfer-owner", "--group=cid", "--new-owner=DAAAAAAAAAAAiE"},
		{"group", "update-icon", "--group=cid", "--icon-media-id=@valid"},
		{"group", "set-history", "--group=cid", "--option=ALL"},
		{"group", "audit-join-validation", "--conversation-id=cid", "--record-id=1", "--applicant=D1", "--inviter=D2", "--status=AuditApprove", "--description=ok"},
		{"mark-read", "--conversation-id=cid", "--message-id=mid"},
		{"text", "translate", "--query=hello", "--to=zh_CN"},
		{"group-role", "set-user", "--group=cid", "--user=D1", "--role-id=r1"},
		{"group-role", "remove-user", "--group=cid", "--user=D1", "--role-ids=r1"},
		{"group-role", "query-user", "--group=cid", "--user=D1"},
		{"group", "set-admin", "--group=cid", "--users=u1,D1"},
		{"group-mute-member", "--group=cid", "--users=D1", "--mute-time=300000"},
		{"group-mute-member", "--group=cid", "--users=u1", "--off"},
	}
	for _, args := range commands {
		_ = runChatCoverageCommand(t, caller, args...)
	}
	for _, tc := range []struct {
		path  []string
		flags map[string]string
	}{
		{[]string{"message", "search"}, map[string]string{"query": "q"}},
		{[]string{"message", "recall"}, map[string]string{"conversation-id": "cid"}},
		{[]string{"category", "rename"}, map[string]string{"category-id": "1"}},
		{[]string{"category", "add-conv"}, map[string]string{"group": "cid"}},
		{[]string{"category", "remove-conv"}, map[string]string{"group": "cid"}},
		{[]string{"mark-read"}, map[string]string{"conversation-id": "cid"}},
	} {
		_ = runChatCoverageDirect(t, tc.path, tc.flags)
	}
	_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"list":[{"memberRoleType":1,"openDingtalkId":"D-owner"}]}}`}}}, "group", "members", "remove", "--id=cid", "--users=D-owner")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"userId":"u1","openDingTalkId":"D1"}]}`}, {text: `{}`}}}, "group-mute-member", "--group=cid", "--users=u1", "--off")
}

func TestCrossPlatformCoverageChatCreateAndMessageSendEdges(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat", "--debug"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	profile := `{"result":{"userId":"owner"}}`
	for _, response := range []scriptedToolStep{
		{text: `{"result":{"openCid":"cid-new","cid":"legacy"}}`},
		{text: `not-json`},
		{err: errors.New("create")},
	} {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: profile}, response}}
		_ = runChatCoverageCommand(t, caller, "group", "create", "--name=group", "--users=owner,u2", "--thread")
	}

	file := filepath.Join(t.TempDir(), "message.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args  []string
		steps []scriptedToolStep
		dry   bool
	}{
		{args: []string{"message", "send", "--group=cid", "--text=hello", "--at-all", "--at-open-dingtalk-ids=D1,D2", "--uuid=u"}},
		{args: []string{"message", "send", "--user=D1", "--text=hello", "--uuid=u", "--debug"}},
		{args: []string{"message", "send", "--open-dingtalk-id=D1", "--text=hello", "--uuid=u"}},
		{args: []string{"message", "send", "--user=u1", "--text=hello", "--uuid=u", "--debug"}, steps: []scriptedToolStep{{err: errors.New("contact")}, {err: errors.New("search")}, {text: `{}`}}},
		{args: []string{"message", "send", "--user=u1", "--text=hello", "--verbose"}, steps: []scriptedToolStep{{text: `{"result":[{"userId":"u1","openDingTalkId":"D1"}]}`}, {text: `{}`}}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=image"}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=image", "--media-id=@media", "--uuid=u"}},
		{args: []string{"message", "send", "--open-dingtalk-id=D1", "--msg-type=image", "--media-id=@media"}},
		{args: []string{"message", "send", "--user=u1", "--msg-type=image", "--media-id=@media", "--uuid=u"}, steps: []scriptedToolStep{{err: errors.New("contact")}, {err: errors.New("search")}, {text: `{}`}}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--dentry-id=1"}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file"}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--dentry-id=1", "--space-id=2", "--file-name=f.txt", "--file-size=7"}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + file, "--dentry-id=1", "--space-id=2"}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + filepath.Join(t.TempDir(), "missing")}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + file}, dry: true},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + file}, steps: []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {text: `{"dentryId":1,"spaceId":2}`}, {text: `{}`}}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + file}, steps: []scriptedToolStep{{err: errors.New("init")}}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=file", "--file-path=" + file}, steps: []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {text: `{}`}}},
		{args: []string{"message", "send", "--group=cid", "--msg-type=unknown"}},
	}
	oldPut := httpPutFile
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	t.Cleanup(func() { httpPutFile = oldPut })
	for _, tc := range tests {
		caller := &scriptedToolCaller{steps: tc.steps, dry: tc.dry}
		_ = runChatCoverageCommand(t, caller, tc.args...)
	}
}

func TestCrossPlatformCoverageChatNativeSendCardMentions(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	t.Run("group forwards mention arguments", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		err := runChatCoverageCommand(t, caller,
			"message", "send-card",
			"--conversation-id=cid",
			"--at-open-dingtalk-ids=D1,D2,D1",
			"--at-all",
		)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"openConversationId": "cid",
			"atOpenDingTalkIds":  []string{"D1", "D2"},
			"atAll":              true,
		}
		if caller.calls != 1 || caller.server != "im" || caller.tool != "create_and_send_card" || !reflect.DeepEqual(caller.args, want) {
			t.Fatalf("call = count:%d server:%q tool:%q args:%#v, want %#v", caller.calls, caller.server, caller.tool, caller.args, want)
		}
	})

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "member mention rejects direct message", args: []string{"--open-dingtalk-id=D1", "--at-open-dingtalk-ids=D2"}},
		{name: "at all rejects direct message", args: []string{"--open-dingtalk-id=D1", "--at-all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runChatCoverageCommand(t, caller, append([]string{"message", "send-card"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), "only supported with --conversation-id") {
				t.Fatalf("error = %v, want group-only mention validation", err)
			}
			if caller.calls != 0 {
				t.Fatalf("invalid direct-message mentions made %d tool calls", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatSendCardHiddenAliasesMapToCanonicalPayload(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	for _, tc := range []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "group alias",
			args: []string{"--group=cid"},
			want: map[string]any{"openConversationId": "cid"},
		},
		{
			name: "receiver alias",
			args: []string{"--receiver=DAAAAAAAAAAAiE"},
			want: map[string]any{"receiverOpenDingTalkId": "DAAAAAAAAAAAiE"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runChatCoverageCommand(t, caller, append([]string{"message", "send-card"}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.server != "im" || caller.tool != "create_and_send_card" || !reflect.DeepEqual(caller.args, tc.want) {
				t.Fatalf("call = count:%d server:%q tool:%q args:%#v, want %#v", caller.calls, caller.server, caller.tool, caller.args, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupAuditJoinValidationUsesCanonicalAndAliasPayload(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	for _, tc := range []struct {
		name string
		flag string
	}{
		{name: "canonical conversation-id", flag: "--conversation-id=cid"},
		{name: "hidden group alias", flag: "--group=cid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runChatCoverageCommand(t, caller,
				"group", "audit-join-validation",
				tc.flag,
				"--record-id=123",
				"--applicant=D-applicant",
				"--inviter=D-inviter",
				"--status=AuditDelete",
				"--description=deny",
			)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{
				"openConversationId": "cid",
				"applyRecordId":      int64(123),
				"applicantUid":       "D-applicant",
				"inviterUid":         "D-inviter",
				"status":             "AuditDelete",
				"auditDescription":   "deny",
			}
			if caller.calls != 1 || caller.server != "im" || caller.tool != "audit_join_group" || !reflect.DeepEqual(caller.args, want) {
				t.Fatalf("call = count:%d server:%q tool:%q args:%#v, want %#v", caller.calls, caller.server, caller.tool, caller.args, want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatIMIDMigrationRequiredFlagErrors(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	tests := []struct {
		name string
		path []string
		flag map[string]string
		want string
	}{
		{name: "message list mutually exclusive targets", path: []string{"message", "list"}, flag: map[string]string{"conversation-id": "cid", "user": "u1", "time": "2026-01-01"}, want: "mutually exclusive"},
		{name: "message list missing target", path: []string{"message", "list"}, flag: map[string]string{"time": "2026-01-01"}, want: "--conversation-id, --user or --open-dingtalk-id is required"},
		{name: "topic replies missing conversation", path: []string{"message", "list-topic-replies"}, flag: map[string]string{"topic-id": "t1"}, want: "conversation-id"},
		{name: "read status missing conversation", path: []string{"message", "read-status"}, flag: map[string]string{"message-id": "m1"}, want: "conversation-id"},
		{name: "read status conflicting aliases", path: []string{"message", "read-status"}, flag: map[string]string{"conversation-id": "cid1", "group": "cid2", "message-id": "m1"}, want: "conflicts"},
		{name: "read status missing message", path: []string{"message", "read-status"}, flag: map[string]string{"conversation-id": "cid"}, want: "message-id"},
		{name: "update text emotion missing message", path: []string{"message", "update-text-emotion"}, flag: map[string]string{"conversation-id": "cid", "old-emotion-id": "e1", "emotion-id": "e2", "emotion-name": "n", "text": "t", "background-id": "b"}, want: "message-id"},
		{name: "update text emotion missing detail flag", path: []string{"message", "update-text-emotion"}, flag: map[string]string{"conversation-id": "cid", "message-id": "m1", "old-emotion-id": "e1", "emotion-id": "e2", "emotion-name": "n", "text": "t"}, want: "background-id"},
		{name: "transfer owner missing conversation", path: []string{"group", "transfer-owner"}, flag: map[string]string{"new-owner": "D1"}, want: "conversation-id"},
		{name: "invite url missing conversation", path: []string{"group", "invite-url"}, want: "conversation-id"},
		{name: "quit missing conversation", path: []string{"group", "quit"}, want: "conversation-id"},
		{name: "update icon missing conversation", path: []string{"group", "update-icon"}, flag: map[string]string{"icon-media-id": "@media"}, want: "conversation-id"},
		{name: "update settings missing conversation", path: []string{"group", "update-settings"}, flag: map[string]string{"setting-key": "searchable"}, want: "conversation-id"},
		{name: "set admin missing conversation", path: []string{"group", "set-admin"}, flag: map[string]string{"users": "D1"}, want: "conversation-id"},
		{name: "role list missing conversation", path: []string{"group-role", "list"}, want: "group"},
		{name: "role add missing conversation", path: []string{"group-role", "add"}, flag: map[string]string{"name": "role"}, want: "conversation-id"},
		{name: "role update missing conversation", path: []string{"group-role", "update"}, flag: map[string]string{"role-id": "r1", "name": "role"}, want: "conversation-id"},
		{name: "role remove missing conversation", path: []string{"group-role", "remove"}, flag: map[string]string{"role-id": "r1"}, want: "conversation-id"},
		{name: "role set user missing conversation", path: []string{"group-role", "set-user"}, flag: map[string]string{"user": "D1", "role-id": "r1"}, want: "conversation-id"},
		{name: "role remove user missing conversation", path: []string{"group-role", "remove-user"}, flag: map[string]string{"user": "D1", "role-ids": "r1"}, want: "conversation-id"},
		{name: "role query user missing conversation", path: []string{"group-role", "query-user"}, flag: map[string]string{"user": "D1"}, want: "conversation-id"},
		{name: "bots missing legacy group", path: []string{"group", "bots"}, want: "group"},
		{name: "bots rejects migrated conversation id", path: []string{"group", "bots"}, flag: map[string]string{"conversation-id": "cid"}, want: "no such flag"},
		{name: "dismiss missing conversation", path: []string{"group", "dismiss"}, flag: map[string]string{"yes": "true"}, want: "conversation-id"},
		{name: "set history missing conversation", path: []string{"group", "set-history"}, flag: map[string]string{"option": "ALL"}, want: "conversation-id"},
		{name: "set pin missing message", path: []string{"message", "set-pin-msg"}, flag: map[string]string{"open-conversation-id": "cid"}, want: "message-id"},
		{name: "unset pin missing message", path: []string{"message", "unset-pin-msg"}, flag: map[string]string{"open-conversation-id": "cid"}, want: "message-id"},
		{name: "audit join missing conversation", path: []string{"group", "audit-join-validation"}, flag: map[string]string{"record-id": "1", "applicant": "D1", "inviter": "D2", "status": "AuditApprove"}, want: "conversation-id"},
		{name: "set top missing message", path: []string{"message", "set-top-msg"}, flag: map[string]string{"open-conversation-id": "cid"}, want: "message-id"},
		{name: "unset top missing message", path: []string{"message", "unset-top-msg"}, flag: map[string]string{"open-conversation-id": "cid"}, want: "message-id"},
		{name: "update alias missing conversation", path: []string{"group", "update-alias"}, flag: map[string]string{"alias-title": "alias"}, want: "conversation-id"},
		{name: "notice create missing conversation", path: []string{"group", "notice", "create"}, flag: map[string]string{"content": "hello"}, want: "conversation-id"},
		{name: "notice edit missing conversation", path: []string{"group", "notice", "edit"}, flag: map[string]string{"notice-id": "n1", "content": "hello"}, want: "conversation-id"},
		{name: "notice get missing conversation", path: []string{"group", "notice", "get"}, flag: map[string]string{"notice-id": "n1"}, want: "conversation-id"},
		{name: "notice list missing conversation", path: []string{"group", "notice", "list"}, want: "conversation-id"},
	}

	probe := newChatCommand()
	messageList, _, err := probe.Find([]string{"message", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := chatConversationID(messageList); err != nil || got != "" {
		t.Fatalf("empty chatConversationID = %q, %v", got, err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runChatCoverageDirect(t, tc.path, tc.flag)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessageReadStatusConversationAliasesExecute(t *testing.T) {
	for _, alias := range []string{"group", "id", "chat", "open-conversation-id"} {
		t.Run(alias, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			if err := runChatCoverageCommand(t, caller, "message", "read-status", "--"+alias, "cid-1", "--message-id", "msg-1"); err != nil {
				t.Fatal(err)
			}
			if caller.server != "im" || caller.tool != "query_msg_read_status" {
				t.Fatalf("call = %s/%s, want im/query_msg_read_status", caller.server, caller.tool)
			}
			if got := caller.args["openConversationId"]; got != "cid-1" {
				t.Fatalf("openConversationId = %#v, want cid-1", got)
			}
			if got := caller.args["openMessageId"]; got != "msg-1" {
				t.Fatalf("openMessageId = %#v, want msg-1", got)
			}
		})
	}
}

func TestCrossPlatformCoverageChatWebhookReplyConversationAndDownloadEdges(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })

	for _, step := range []scriptedToolStep{{text: `{}`}, {text: `not-json`}, {text: `{"errcode":1,"errmsg":"bad"}`}, {err: errors.New("webhook")}} {
		_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{step}}, "message", "send-by-webhook", "--token=t", "--title=title", "--text=text")
	}
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "message", "reply", "--conversation-id=cid", "--ref-msg-id=mid", "--ref-sender=D1", "--text=reply", "--ai-tag", "--uuid=u")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"userId":"u1","openDingTalkId":"D1"}]}`}, {text: `{}`}}}, "message", "reply", "--conversation-id=cid", "--ref-msg-id=mid", "--ref-sender=u1", "--text=reply")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "conversation-info", "--open-dingtalk-id=D1")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "conversation-info", "--user=D1")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"userId":"u1","openDingTalkId":"D1"}]}`}, {text: `{}`}}}, "conversation-info", "--user=u1")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "message", "send-card", "--open-dingtalk-id=D1")
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "message", "send-card", "--receiver=D1")

	oldGet := httpGetFile
	t.Cleanup(func() { httpGetFile = oldGet })
	tmp := t.TempDir()
	base := []string{"message", "download-media", "--type=mediaId", "--resource-id=r", "--open-conversation-id=cid", "--message-id=mid"}
	_ = runChatCoverageCommand(t, &productExampleCaller{dry: true}, append(base, "--output="+filepath.Join(tmp, "dry"))...)
	for _, alias := range []string{"--msg-id=mid", "--open-message-id=mid"} {
		aliasArgs := []string{"message", "download-media", "--type=mediaId", "--resource-id=r", "--open-conversation-id=cid", alias, "--output=" + filepath.Join(tmp, "alias-dry")}
		_ = runChatCoverageCommand(t, &productExampleCaller{dry: true}, aliasArgs...)
	}
	missingMessageID := []string{"message", "download-media", "--type=mediaId", "--resource-id=r", "--open-conversation-id=cid", "--output=" + filepath.Join(tmp, "missing-id")}
	_ = runChatCoverageCommand(t, &productExampleCaller{dry: true}, missingMessageID...)
	for _, tc := range []struct {
		step scriptedToolStep
		out  string
		get  error
	}{
		{scriptedToolStep{err: errors.New("url")}, filepath.Join(tmp, "tool-error"), nil},
		{scriptedToolStep{text: `{}`}, filepath.Join(tmp, "parse-error"), nil},
		{scriptedToolStep{text: `{"resourceUrl":"https://example.test/file.txt"}`}, tmp + string(os.PathSeparator), nil},
		{scriptedToolStep{text: `{"resourceUrl":"https://example.test/file.txt"}`}, filepath.Join(tmp, "get-error"), errors.New("get")},
	} {
		httpGetFile = func(context.Context, string, map[string]string, string) error { return tc.get }
		_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{tc.step}}, append(base, "--output="+tc.out)...)
	}
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
	_ = runChatCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"resourceUrl":"https://example.test/file.txt"}`}}}, append(base, "--output="+filepath.Join(blocker, "child"))...)
}
