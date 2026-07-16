package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func runChatCoverageDirect(t *testing.T, path []string, flags map[string]string) error {
	t.Helper()
	command, _, err := newChatCommand().Find(path)
	if err != nil {
		return err
	}
	for name, value := range flags {
		if err := command.Flags().Set(name, value); err != nil {
			return err
		}
	}
	return command.RunE(command, nil)
}

func TestCrossPlatformCoverageChatCommandValidationAndSuccessEdges(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })
	caller := &productExampleCaller{}

	commands := [][]string{
		{"chmod", "chat.read", "--ttl="},
		{"chmod", "chat.read", "--conversation-id=cid"},
		{"message", "list", "--group=cid", "--time=2026-01-01", "--limit=1"},
		{"message", "list", "--user=D-user", "--time=2026-01-01", "--limit=1"},
		{"message", "list", "--open-dingtalk-id=D-user", "--time=2026-01-01", "--direction=sideways"},
		{"message", "list-direct", "--user=D-user", "--limit=1"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=bad"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=2026-01-01T00:00:00Z", "--end=bad"},
		{"message", "list-by-sender", "--sender-user-id=u1", "--start=2026-01-02T00:00:00Z", "--end=2026-01-01T00:00:00Z"},
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
		{"category", "rename", "--category-id=1", "--title=renamed"},
		{"category", "add-conv", "--group=cid", "--category-ids=1,2"},
		{"category", "remove-conv", "--group=cid", "--category-ids=1,2"},
		{"message", "list-by-ids", "--msg-ids=" + strings.Repeat("id,", 51) + "last"},
		{"group", "transfer-owner", "--group=cid", "--new-owner=D-owner"},
		{"group", "update-icon", "--group=cid", "--icon-media-id=@valid"},
		{"group", "set-history", "--group=cid", "--option=ALL"},
		{"group", "audit-join-validation", "--group=cid", "--record-id=1", "--applicant=D1", "--inviter=D2", "--status=AuditApprove", "--description=ok"},
		{"mark-read", "--conversation-id=cid", "--message-id=mid"},
		{"text", "translate", "--query=hello", "--to=zh_CN"},
		{"group-role", "set-user", "--group=cid", "--user=D1", "--role-ids=r1"},
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
	_ = runChatCoverageCommand(t, &scriptedToolCaller{}, "message", "send-card", "--receiver=D1")

	oldGet := httpGetFile
	t.Cleanup(func() { httpGetFile = oldGet })
	tmp := t.TempDir()
	base := []string{"message", "download-media", "--type=mediaId", "--resource-id=r", "--open-conversation-id=cid", "--message-id=mid"}
	_ = runChatCoverageCommand(t, &productExampleCaller{dry: true}, append(base, "--output="+filepath.Join(tmp, "dry"))...)
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
