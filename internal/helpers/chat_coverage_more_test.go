package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

const (
	helperCurrentDOpenID  = "DAAAAAAAAAAAiE"
	helperCurrentDOpenID2 = "DAQEBAQEBAQEiE"
)

func newChatFlagTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "chat"}
	cmd.Flags().String("forward", "", "")
	cmd.Flags().String("direction", "", "")
	cmd.Flags().Int("limit", 1, "")
	cmd.Flags().Int("size", 2, "")
	cmd.Flags().String("group", "", "")
	cmd.Flags().String("conversation-id", "", "")
	cmd.Flags().String("id", "", "")
	cmd.Flags().String("chat", "", "")
	cmd.Flags().String("open-dingtalk-id", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("userId", "", "")
	cmd.Flags().String("agentCode", "agent", "")
	cmd.Flags().String("grant-type", "once", "")
	cmd.Flags().String("ttl", "", "")
	cmd.Flags().String("session-id", "", "")
	cmd.Flags().String("target-org-id", "", "")
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().StringArray("permParam", nil, "")
	return cmd
}

func TestNativeChatIDSplitUsesCurrentDFormat(t *testing.T) {
	userIDs, openIDs := splitChatIDValues([]string{
		helperCurrentDOpenID,
		"D-prefix-fixture-user",
		"d-prefix-fixture-user",
		"D-invalid",
		"fixture-user-id",
	})
	if len(openIDs) != 1 || openIDs[0] != helperCurrentDOpenID {
		t.Fatalf("open IDs=%#v", openIDs)
	}
	if got := strings.Join(userIDs, ","); got != "D-prefix-fixture-user,d-prefix-fixture-user,D-invalid,fixture-user-id" {
		t.Fatalf("user IDs=%#v", userIDs)
	}
}

func TestCrossPlatformCoverageChatDirectionAndScalarCoverage(t *testing.T) {
	search := &cobra.Command{Use: "search"}
	search.Flags().String("nicks", "", "")
	search.Flags().Int("limit", 20, "")
	search.Flags().Int("size", 20, "")
	search.Flags().String("cursor", "", "")
	search.Flags().String("match-mode", "", "")
	search.Flags().Bool("exclude-muted", false, "")
	if err := runChatSearchCommon(search, nil); err == nil {
		t.Fatal("missing nicks returned nil")
	}
	_ = search.Flags().Set("nicks", "One,Two")
	_ = search.Flags().Set("exclude-muted", "true")
	installScriptedCaller(t, &scriptedToolCaller{})
	if err := runChatSearchCommon(search, nil); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		forward, direction string
		changeForward      bool
		changeDirection    bool
		fallback           bool
	}{
		{fallback: true},
		{forward: "false", changeForward: true},
		{direction: "newer", changeDirection: true},
		{forward: "false", direction: "newer", changeForward: true, changeDirection: true},
		{direction: "older", changeDirection: true},
		{forward: "true", direction: "older", changeForward: true, changeDirection: true},
		{direction: " ", changeDirection: true, fallback: true},
		{direction: "sideways", changeDirection: true},
	} {
		cmd := newChatFlagTestCommand()
		if tc.changeForward {
			_ = cmd.Flags().Set("forward", tc.forward)
		}
		if tc.changeDirection {
			_ = cmd.Flags().Set("direction", tc.direction)
		}
		_, _ = resolveMessageForward(cmd, tc.fallback)
	}
	cmd := newChatFlagTestCommand()
	_ = cmd.Flags().Set("size", "9")
	if got := chatIntFlagOrFallback(cmd, "limit", "size"); got != 9 {
		t.Fatalf("alias int = %d", got)
	}
	_ = chatIntFlagOrFallback(newChatFlagTestCommand(), "limit", "size")

	for _, text := range []string{
		"", "https://example.test/a%20b", "prefix https://example.test", "prefix http://example.test",
		"contains%20escape", strings.Repeat("中", 50), "short",
	} {
		_ = sanitizeTitleFromText(text)
	}
	_ = truncateTitleToBytes("abcdef", 4, 100)
	_ = truncateTitleToBytes("abcdef", 100, 100)
	if _, err := marshalJSONRaw(map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("unsupported JSON value succeeded")
	}
	for _, raw := range []string{"", "1, 2", ",,", "1,bad"} {
		_, _ = parseCSVInt64(raw)
	}
	for _, value := range []string{"", "123", "12a"} {
		_ = isNumericUserID(value)
	}
	for _, raw := range []string{"{", `{}`, `{"errcode":0}`, `{"errcode":"0"}`, `{"errcode":1}`, `{"errcode":"2","errmsg":"failed"}`} {
		_, _, _ = webhookErrcodeFailure(raw)
	}
	_, _ = splitChatIDValues([]string{"", " user ", "D-open", "d-open"})
	args := map[string]any{"users": []string{"old"}}
	appendStringSliceArg(args, "users", nil)
	appendStringSliceArg(args, "users", []string{"new"})
	appendStringSliceArg(args, "open", []string{"D1"})
	_ = appendChatIDArgs(args, []string{"u1", "D1"}, "users", "open")
	for _, wrap := range []bool{true, false} {
		_ = normalizeAtPlaceholders("hello @u1 <@u2>", []string{"", "u1", "u2"}, wrap)
	}
	if got := NormalizeMessageMentions("hello @u1", []string{"u1"}, true, true); got != "<@all> hello <@u1>" {
		t.Fatalf("current-user mention normalization = %q", got)
	}
	if got := NormalizeMessageMentions("<@all> <@u1>", []string{"u1"}, true, false); got != "@all @u1" {
		t.Fatalf("bot mention normalization = %q", got)
	}
	if got := NormalizeMessageMentions("@alliance hello", nil, true, false); got != "@all @alliance hello" {
		t.Fatalf("bot @all token detection = %q", got)
	}
	if got := NormalizeMessageMentions("hello @all", nil, true, false); got != "hello @all" {
		t.Fatalf("trailing bot @all detection = %q", got)
	}
}

func TestCrossPlatformCoverageChatContactMappingCoverage(t *testing.T) {
	openIDs := map[string]string{}
	names := map[string]string{}
	collectContactUserMappings([]any{
		map[string]any{"userId": "u1", "openDingTalkId": "D1", "name": "One"},
		map[string]any{"employee": map[string]any{"staff_id": json.Number("2"), "open_dingtalk_id": "D2", "display_name": "Two"}},
		map[string]any{"ignored": true},
	}, openIDs, names)
	if openIDs["u1"] != "D1" || openIDs["2"] != "D2" {
		t.Fatalf("contact mappings = %#v", openIDs)
	}
	_ = stringForNestedJSONKeys(map[string]any{"employee": "bad"}, chatContactNestedUserKeys, chatUserIDJSONKeys)
	_ = stringForJSONKeys(map[string]any{"ignored": "x", "userId": ""}, chatUserIDJSONKeys)
	for _, value := range []any{" text ", json.Number("12"), float64(1.5), float32(2.5), int(3), int64(4), int32(5), true} {
		_ = stringFromJSONScalar(value)
	}
	if allUserIDsMapped([]string{"u1", "missing"}, openIDs) || !allUserIDsMapped([]string{"", "u1"}, openIDs) {
		t.Fatal("allUserIDsMapped result changed")
	}
}

func TestCrossPlatformCoverageResolveOpenDingTalkIDsCoverage(t *testing.T) {
	if id, err := resolveOpenDingTalkID(context.Background(), helperCurrentDOpenID); err != nil || id != helperCurrentDOpenID {
		t.Fatalf("direct ID = %q, %v", id, err)
	}
	if _, err := resolveOpenDingTalkID(context.Background(), ""); err == nil {
		t.Fatal("empty ID unexpectedly resolved")
	}
	if ids, err := resolveOpenDingTalkIDs(context.Background(), []string{" " + helperCurrentDOpenID + " ", ""}); err != nil || ids[0] != helperCurrentDOpenID {
		t.Fatalf("direct IDs = %#v, %v", ids, err)
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[{"userId":"u1","openDingTalkId":"` + helperCurrentDOpenID2 + `"}]}`}}}
	installScriptedCaller(t, caller)
	if ids, err := resolveOpenDingTalkIDs(context.Background(), []string{"u1", "u1"}); err != nil || ids[1] != helperCurrentDOpenID2 {
		t.Fatalf("resolved IDs = %#v, %v", ids, err)
	}
	caller.steps = []scriptedToolStep{{text: `{}`}, {text: `{}`}}
	caller.index = 0
	_, _ = resolveOpenDingTalkID(context.Background(), "missing")
	caller.steps = []scriptedToolStep{{err: errors.New("contact")}, {err: errors.New("fallback")}}
	caller.index = 0
	_, _ = resolveOpenDingTalkIDs(context.Background(), []string{"missing"})
	caller.steps = []scriptedToolStep{{text: `{`}}
	caller.index = 0
	_, _ = lookupOpenDingTalkIDsByUserID(context.Background(), []string{"u1"})

	caller.steps = []scriptedToolStep{
		{text: `{"result":[{"userId":"u2","name":"Alice"}]}`},
		{text: `{"result":[{"userId":"u2","openDingTalkId":"D2"}]}`},
	}
	caller.index = 0
	if got, err := lookupOpenDingTalkIDsByUserID(context.Background(), []string{"", "u2"}); err != nil || got["u2"] != "D2" {
		t.Fatalf("aisearch mapping = %#v, %v", got, err)
	}

	caller.steps = []scriptedToolStep{
		{text: `{"result":[{"userId":"u3","name":"Same"}]}`},
		{err: errors.New("aisearch")},
		{text: `{"result":[{"userId":"u3","openDingTalkId":"D3"}]}`},
	}
	caller.index = 0
	if got, err := lookupOpenDingTalkIDsByUserID(context.Background(), []string{"u3"}); err != nil || got["u3"] != "D3" {
		t.Fatalf("contact fallback mapping = %#v, %v", got, err)
	}

	caller.steps = []scriptedToolStep{{text: `{}`}, {text: `{`}}
	caller.index = 0
	if _, err := lookupOpenDingTalkIDsByUserID(context.Background(), []string{"u4"}); err == nil {
		t.Fatal("invalid fallback response returned nil")
	}

	caller.steps = []scriptedToolStep{
		{text: `{"result":[{"userId":"u5","name":"u5"}]}`},
		{text: `{}`},
		{text: `{}`},
	}
	caller.index = 0
	if got, err := lookupOpenDingTalkIDsByUserID(context.Background(), []string{"u5"}); err != nil || got["u5"] != "" {
		t.Fatalf("duplicate-keyword fallback = %#v, %v", got, err)
	}

	caller.steps = []scriptedToolStep{
		{text: `{"result":[{"userId":"u6","openDingTalkId":"D6"}]}`},
		{text: `{}`},
	}
	caller.index = 0
	if got, err := lookupOpenDingTalkIDsByUserID(context.Background(), []string{"u6", "u7"}); err != nil || got["u6"] != "D6" {
		t.Fatalf("partially mapped fallback = %#v, %v", got, err)
	}

	caller.steps = []scriptedToolStep{{text: `{`}}
	caller.index = 0
	if err := lookupOpenDingTalkIDsByAisearchPerson(context.Background(), "bad", map[string]string{}, map[string]string{}); err == nil {
		t.Fatal("invalid aisearch response returned nil")
	}
}

func TestCrossPlatformCoverageChatTargetAndGrantCoverage(t *testing.T) {
	for _, configure := range []func(*cobra.Command){
		func(c *cobra.Command) { _ = c.Flags().Set("group", "group") },
		func(c *cobra.Command) { _ = c.Flags().Set("open-dingtalk-id", "123") },
		func(c *cobra.Command) { _ = c.Flags().Set("user", "D-open") },
		func(c *cobra.Command) { _ = c.Flags().Set("user", "123") },
	} {
		cmd := newChatFlagTestCommand()
		configure(cmd)
		_, _ = buildConversationTargetArgs(cmd)
	}
	for _, scope := range []string{"bad", "chat.read"} {
		_ = validateChatScope(scope)
	}
	cmd := newChatFlagTestCommand()
	_ = cmd.Flags().Set("conversation-id", "cid")
	_ = cmd.Flags().Set("grant-type", "bad")
	_, _ = buildChatChmodArgs(cmd, "chat.read")
	for _, tc := range []struct{ grantType, ttl, session string }{
		{"bad", "", ""}, {"timed", "", ""}, {"timed", "1h", ""},
		{"session", "", ""}, {"session", "", "session"}, {"permanent", "", "extra"},
	} {
		cmd := newChatFlagTestCommand()
		_ = cmd.Flags().Set("grant-type", tc.grantType)
		_ = cmd.Flags().Set("ttl", tc.ttl)
		_ = cmd.Flags().Set("session-id", tc.session)
		_, _ = buildChatGrantBaseArgs(cmd, "chat.read")
	}

	for _, configure := range []func(*cobra.Command){
		func(c *cobra.Command) {},
		func(c *cobra.Command) { _ = c.Flags().Set("conversation-id", "cid") },
		func(c *cobra.Command) { _ = c.Flags().Set("open-dingtalk-id", "D1") },
		func(c *cobra.Command) { _ = c.Flags().Set("user", "u1") },
		func(c *cobra.Command) { _ = c.Flags().Set("permParam", "key=value") },
		func(c *cobra.Command) { _ = c.Flags().Set("permParam", "bad") },
		func(c *cobra.Command) { _ = c.Flags().Set("conversation-id", "cid"); _ = c.Flags().Set("user", "u1") },
	} {
		cmd := newChatFlagTestCommand()
		configure(cmd)
		_, _ = buildChatChmodArgs(cmd, "chat.read")
	}
	for _, values := range [][]string{nil, {"", "a=b", " a = override ", "bad"}} {
		_, _ = parseChatChmodParams(values)
	}
	for _, configure := range []func(*cobra.Command){
		func(c *cobra.Command) {},
		func(c *cobra.Command) { _ = c.Flags().Set("target-org-id", "org") },
		func(c *cobra.Command) { _ = c.Flags().Set("all", "true") },
		func(c *cobra.Command) { _ = c.Flags().Set("target-org-id", "org"); _ = c.Flags().Set("all", "true") },
	} {
		cmd := newChatFlagTestCommand()
		configure(cmd)
		_, _ = buildChatCrossOrgDataAuthArgs(cmd)
	}
	cmd = newChatFlagTestCommand()
	_ = cmd.Flags().Set("target-org-id", "org")
	_ = cmd.Flags().Set("grant-type", "bad")
	_, _ = buildChatCrossOrgDataAuthArgs(cmd)
	params := map[string]string{"existing": "value"}
	putChatChmodParam(params, "blank", " ")
	putChatChmodParam(params, "existing", "new")
}

func TestCrossPlatformCoverageChatFileUtilityCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fileMD5Hex(file)
	_, _ = fileMD5Hex(filepath.Join(t.TempDir(), "missing"))
	_, _ = fileMD5Hex(t.TempDir())
	for _, raw := range []string{
		`{`, `{}`,
		`{"resourceUrls":["https://upload"],"uploadKey":"key","headers":{"x-test":1,"blank":null},"ossHeaders":{"x-other":"yes"}}`,
		`{"result":{"resourceUrl":"https://upload","key":"key"}}`,
	} {
		_, _, _, _ = parseConversationFileUploadInfo(raw)
	}
	_, _ = buildConversationLocalFileMeta(filepath.Join(t.TempDir(), "missing"), "", "")
	_, _ = buildConversationLocalFileMeta(t.TempDir(), "", "")
	meta, err := buildConversationLocalFileMeta(file, "custom.bin", "provided")
	if err != nil || meta.FileType != "bin" {
		t.Fatalf("file meta = %#v, %v", meta, err)
	}
	_, _ = buildConversationLocalFileMeta(file, "", "")
	previousMD5 := chatFileMD5
	chatFileMD5 = func(string) (string, error) { return "", errors.New("md5") }
	t.Cleanup(func() { chatFileMD5 = previousMD5 })
	if _, err := buildConversationLocalFileMeta(file, "", ""); err == nil {
		t.Fatal("MD5 failure returned nil")
	}
	chatFileMD5 = previousMD5
	_, _ = buildConversationFileContent(1, 2, meta)
	for _, raw := range []string{`{`, `{}`, `{"result":{"dentryId":1,"spaceId":"2"}}`} {
		_, _, _ = parseConversationFileSendIDs(raw)
	}
	for _, value := range []any{
		map[string]any{"id": json.Number("1")},
		map[string]any{"nested": map[string]any{"id": "2"}},
		[]any{map[string]any{"id": 3}},
		`{"id":4}`, `[ {"id":5} ]`, "not-json", true,
	} {
		_, _ = findInt64Field(value, "id")
	}
	for _, value := range []any{json.Number("bad"), float64(-1), int64(3), int(4), "5", "bad", true} {
		_, _ = int64FromJSONScalar(value)
	}
	_ = cloneStringAnyMap(map[string]any{"key": "value"})
	_ = unmarshalJSONUseNumber(`{"n":1}`, &map[string]any{})
	_ = firstStringField(map[string]any{"one": "", "two": 2}, "one", "two")

	testseam.Protect(t, &deps)
	caller := &helpersCoreCaller{format: "json"}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	caller.format = "raw"
}

func TestCrossPlatformCoverageUploadConversationLocalFileCoverage(t *testing.T) {
	oldPut := httpPutFile
	t.Cleanup(func() { httpPutFile = oldPut })
	meta := conversationLocalFileMeta{LocalPath: "file", FileName: "file.txt", FileSize: 4, MD5: "md5"}
	for _, tc := range []struct {
		name  string
		steps []scriptedToolStep
		put   error
	}{
		{"success", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {text: `{"ok":true}`}}, nil},
		{"init-error", []scriptedToolStep{{err: errors.New("init")}}, nil},
		{"parse-error", []scriptedToolStep{{text: `{}`}}, nil},
		{"put-error", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}}, errors.New("put")},
		{"commit-error", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {err: errors.New("commit")}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return tc.put }
			_, _ = uploadConversationLocalFile(context.Background(), map[string]any{"target": "id"}, meta, "uuid")
		})
	}
}

func TestCrossPlatformCoverageChatBotRichMediaCoverage(t *testing.T) {
	testseam.Protect(t, &deps)
	testseam.Swap(t, &httpPutFile, func(context.Context, string, map[string]string, string, int64) error {
		return nil
	})

	filePath := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(filePath, []byte("report"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, response := range []string{`{`, `{"result":{}}`} {
		if _, err := parseConversationFileDownloadURL(response); err == nil {
			t.Fatalf("parseConversationFileDownloadURL(%q) returned nil", response)
		}
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "markdown requires title", args: []string{"--text", "message"}},
		{name: "markdown requires text", args: []string{"--title", "title"}},
		{name: "image requires URL", args: []string{"--msg-type", "image"}},
		{name: "file requires path", args: []string{"--msg-type", "file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"message", "send-by-bot", "--robot-code", "robot", "--group", "group"}
			if err := runChatCoverageCommand(t, &scriptedToolCaller{}, append(args, tc.args...)...); err == nil {
				t.Fatal("missing rich-media input returned nil")
			}
		})
	}

	t.Run("group and direct targets are mutually exclusive", func(t *testing.T) {
		err := runChatCoverageCommand(t, &scriptedToolCaller{},
			"message", "send-by-bot", "--robot-code", "robot",
			"--group", "group", "--users", "user", "--title", "title", "--text", "message",
		)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("mutually exclusive target error = %v", err)
		}
	})

	t.Run("direct markdown defaults to DX message type", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		if err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--users", "user",
			"--title", "title", "--text", "message",
		); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "batch_send_robot_msg_to_users" || caller.args["msgType"] != "sampleMarkdownDX" {
			t.Fatalf("direct markdown call = %s %#v", caller.tool, caller.args)
		}
		if _, exists := caller.args["referenceOpenMessageId"]; exists {
			t.Fatalf("ordinary direct message unexpectedly contains reply fields: %#v", caller.args)
		}
	})

	t.Run("group Markdown reference reply", func(t *testing.T) {
		const senderOpenDingTalkID = "DAAAAAAAAAAAiE"
		caller := &scriptedToolCaller{}
		if err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--conversation-id", "group",
			"--reply", "message-id", "--ref-sender", senderOpenDingTalkID,
			"--text", "received",
		); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "send_robot_group_message" ||
			caller.args["referenceOpenMessageId"] != "message-id" ||
			caller.args["srcMsgSendOpenDingTalkId"] != senderOpenDingTalkID ||
			caller.args["title"] != "received" {
			t.Fatalf("group reference reply call = %s %#v", caller.tool, caller.args)
		}
	})

	t.Run("group reference reply resolves sender user ID", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"result":[{"userId":"sender-user","openDingTalkId":"D-sender"}]}`},
			{text: `{}`},
		}}
		if err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--conversation-id", "group",
			"--reply", "message-id", "--ref-sender", "sender-user",
			"--title", "reply", "--text", "received",
		); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "send_robot_group_message" || caller.args["srcMsgSendOpenDingTalkId"] != "D-sender" {
			t.Fatalf("resolved group reference reply = %s %#v", caller.tool, caller.args)
		}
	})

	t.Run("direct RunE enforces paired reply flags", func(t *testing.T) {
		err := runChatCoverageDirect(t, []string{"message", "send-by-bot"}, map[string]string{
			"robot-code":      "robot",
			"conversation-id": "group",
			"reply":           "message-id",
			"title":           "reply",
			"text":            "received",
		})
		if err == nil || !strings.Contains(err.Error(), "must be specified together") {
			t.Fatalf("direct RunE paired reply flags error = %v", err)
		}
	})

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "reply requires sender", args: []string{"--reply", "message-id"}, want: "ref-sender"},
		{name: "sender requires reply", args: []string{"--ref-sender", "D-sender"}, want: "reply"},
		{name: "paired reply flags reject empty values", args: []string{"--reply=", "--ref-sender="}, want: "specified together"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"message", "send-by-bot", "--robot-code", "robot", "--conversation-id", "group",
				"--title", "reply", "--text", "received",
			}
			err := runChatCoverageCommand(t, &scriptedToolCaller{}, append(args, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("paired reply flags error = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("reference reply is group only", func(t *testing.T) {
		err := runChatCoverageCommand(t, &scriptedToolCaller{},
			"message", "send-by-bot", "--robot-code", "robot", "--users", "user",
			"--reply", "message-id", "--ref-sender", "D-sender",
			"--title", "reply", "--text", "received",
		)
		if err == nil || !strings.Contains(err.Error(), "only supported with --conversation-id") {
			t.Fatalf("direct reference reply error = %v", err)
		}
	})

	t.Run("reference reply rejects rich media", func(t *testing.T) {
		err := runChatCoverageCommand(t, &scriptedToolCaller{},
			"message", "send-by-bot", "--robot-code", "robot", "--conversation-id", "group",
			"--reply", "message-id", "--ref-sender", "D-sender",
			"--msg-type", "image", "--image-url", "https://example.test/image.png",
		)
		if err == nil || !strings.Contains(err.Error(), "only support Markdown") {
			t.Fatalf("rich-media reference reply error = %v", err)
		}
	})

	t.Run("direct image", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		if err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--users", "user",
			"--msg-type", "image", "--image-url", "https://example.test/image.png", "--at-all",
		); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "batch_send_robot_msg_to_users" || caller.args["msgType"] != "sampleImageMsg" || caller.args["isAtAll"] != "true" {
			t.Fatalf("direct image call = %s %#v", caller.tool, caller.args)
		}
	})

	for _, tc := range []struct {
		name   string
		target []string
	}{
		{name: "user file", target: []string{"--users", "user"}},
		{name: "open DingTalk ID file", target: []string{"--open-dingtalk-ids", "D-user"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{
				{text: `{"resourceUrl":"https://upload.example/file","uploadKey":"key"}`},
				{text: `{"result":{"downloadUrl":"https://download.example/report.pdf"}}`},
				{text: `{"success":true}`},
			}}
			args := []string{"message", "send-by-bot", "--robot-code", "robot", "--msg-type", "file", "--file-path", filePath}
			if err := runChatCoverageCommand(t, caller, append(args, tc.target...)...); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 3 || caller.tool != "batch_send_robot_msg_to_users" || caller.args["msgType"] != "sampleDingtalkDriveFile" || caller.args["fileUrl"] != "https://download.example/report.pdf" {
				t.Fatalf("direct file call = %d %s %#v", caller.calls, caller.tool, caller.args)
			}
		})
	}

	t.Run("file dry run", func(t *testing.T) {
		caller := &scriptedToolCaller{dry: true}
		if err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--group", "group",
			"--msg-type", "file", "--file-path", filePath,
		); err != nil || caller.calls != 0 {
			t.Fatalf("dry run error = %v, calls = %d", err, caller.calls)
		}
	})

	t.Run("file rejects multiple recipients", func(t *testing.T) {
		err := runChatCoverageCommand(t, &scriptedToolCaller{},
			"message", "send-by-bot", "--robot-code", "robot", "--users", "one,two",
			"--msg-type", "file", "--file-path", filePath,
		)
		if err == nil || !strings.Contains(err.Error(), "requires exactly one") {
			t.Fatalf("multiple-recipient error = %v", err)
		}
	})

	t.Run("file upload failure", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("upload failed")}}}
		err := runChatCoverageCommand(t, caller,
			"message", "send-by-bot", "--robot-code", "robot", "--group", "group",
			"--msg-type", "file", "--file-path", filePath,
		)
		if err == nil || !strings.Contains(err.Error(), "upload failed") {
			t.Fatalf("upload error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageGuardGroupOwnerRemovalCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() { os.Args = oldArgs })
	for _, tc := range []struct {
		name   string
		remove []string
		steps  []scriptedToolStep
	}{
		{"owner-open", []string{"D-owner"}, []scriptedToolStep{{text: `{"result":{"list":[{"memberRoleType":1,"openDingtalkId":"D-owner"}]}}`}}},
		{"other-open", []string{"D-other"}, []scriptedToolStep{{text: `{"result":{"list":[{"memberRoleType":1,"openDingtalkId":"D-owner"}]}}`}}},
		{"owner-user", []string{"u1"}, []scriptedToolStep{
			{text: `{"result":{"hasMore":true,"nextCursor":"next","list":[]}}`},
			{text: `{"result":{"list":[{"memberRoleType":1,"openDingtalkId":"D-owner"}]}}`},
			{text: `{"result":[{"userId":"u1","openDingTalkId":"D-owner"}]}`},
		}},
		{"group-error", []string{"u1"}, []scriptedToolStep{{err: errors.New("group")}}},
		{"group-invalid", []string{"u1"}, []scriptedToolStep{{text: `{`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			_ = guardGroupOwnerRemoval(context.Background(), "group", tc.remove)
		})
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
	installScriptedCaller(t, caller)
	if owner, err := groupOwnerOpenDingTalkID(context.Background(), "group"); err != nil || owner != "" {
		t.Fatalf("missing owner = %q, %v", owner, err)
	}
}
