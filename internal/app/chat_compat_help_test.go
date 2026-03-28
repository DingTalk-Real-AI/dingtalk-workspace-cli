package app

import (
	"bytes"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	probesurface "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/surface"
)

func TestRootCommandRegistersChatMessageCompatCommands(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newChatCompatDiscoveryServer(t)
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	message := lookupCommand(root, "chat message")
	if message == nil {
		t.Fatal("lookupCommand(chat message) = nil")
	}

	var got []string
	for _, child := range message.Commands() {
		if child.Hidden {
			continue
		}
		got = append(got, child.Name())
	}

	want := []string{
		"list",
		"recall-by-bot",
		"send",
		"send-by-bot",
		"send-by-webhook",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible chat message commands = %#v, want %#v", got, want)
	}
}

func TestChatMessageCompatHelpMatchesTruthFlagSurface(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newChatCompatDiscoveryServer(t)
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cases := []struct {
		args []string
		want [][]string
	}{
		{
			args: []string{"chat", "message", "list", "--help"},
			want: [][]string{{"--forward"}, {"--group"}, {"-h", "--help"}, {"--limit"}, {"--time"}, {"--user"}},
		},
		{
			args: []string{"chat", "message", "recall-by-bot", "--help"},
			want: [][]string{{"--group"}, {"-h", "--help"}, {"--keys"}, {"--robot-code"}},
		},
		{
			args: []string{"chat", "message", "send", "--help"},
			want: [][]string{{"--at-all"}, {"--at-users"}, {"--group"}, {"-h", "--help"}, {"--title"}, {"--user"}},
		},
		{
			args: []string{"chat", "message", "send-by-bot", "--help"},
			want: [][]string{{"--group"}, {"-h", "--help"}, {"--robot-code"}, {"--text"}, {"--title"}, {"--users"}},
		},
		{
			args: []string{"chat", "message", "send-by-webhook", "--help"},
			want: [][]string{{"--at-all"}, {"--at-mobiles"}, {"--at-users"}, {"-h", "--help"}, {"--text"}, {"--title"}, {"--token"}},
		},
	}

	for _, tc := range cases {
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(tc.args)

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", tc.args, err)
		}

		page := probesurface.ParsePage(tc.args[:len(tc.args)-1], out.String())
		got := localSurfaceFlagNames(page.LocalFlags)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s flags = %#v, want %#v\nhelp:\n%s", tc.args[2], got, tc.want, out.String())
		}
	}
}

func localSurfaceFlagNames(entries []probesurface.FlagEntry) [][]string {
	out := make([][]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, append([]string{}, entry.Names...))
	}
	return out
}

func newChatCompatDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newDiscoveryRuntimeServer(t,
		discoveryRuntimeFixture{
			ID:           "group-chat",
			Command:      "chat",
			EndpointPath: "group-chat",
			Description:  "群聊 / 会话 / 群组管理",
			Groups: map[string]any{
				"message": map[string]any{"description": "会话消息管理"},
			},
			ToolOverrides: map[string]any{
				"list_conversation_message_v2": map[string]any{
					"cliName": "list",
					"group":   "message",
					"hidden":  true,
					"flags":   map[string]any{},
				},
				"send_message_as_user": map[string]any{
					"cliName": "send",
					"group":   "message",
					"hidden":  true,
					"flags":   map[string]any{},
				},
				"send_direct_message_as_user": map[string]any{
					"hidden": true,
					"flags":  map[string]any{},
				},
				"list_individual_chat_message": map[string]any{
					"hidden": true,
					"flags":  map[string]any{},
				},
				"list_topic_replies": map[string]any{
					"cliName": "list-topic-replies",
					"group":   "message",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "list_conversation_message_v2", Title: "拉取群聊消息", Description: "拉取群聊消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "send_message_as_user", Title: "发送群消息", Description: "发送群消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "send_direct_message_as_user", Title: "发送单聊消息", Description: "发送单聊消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "list_individual_chat_message", Title: "拉取单聊消息", Description: "拉取单聊消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "list_topic_replies", Title: "拉取话题回复", Description: "拉取话题回复", InputSchema: map[string]any{"type": "object"}},
			},
		},
		discoveryRuntimeFixture{
			ID:           "bot",
			Command:      "chat",
			EndpointPath: "bot",
			Description:  "机器人消息",
			Groups: map[string]any{
				"message": map[string]any{"description": "会话消息管理"},
			},
			ToolOverrides: map[string]any{
				"send_robot_group_message": map[string]any{
					"group":  "message",
					"hidden": true,
					"flags":  map[string]any{},
				},
				"batch_send_robot_msg_to_users": map[string]any{
					"group":  "message",
					"hidden": true,
					"flags":  map[string]any{},
				},
				"recall_robot_group_message": map[string]any{
					"group":  "message",
					"hidden": true,
					"flags":  map[string]any{},
				},
				"batch_recall_robot_users_msg": map[string]any{
					"group":  "message",
					"hidden": true,
					"flags":  map[string]any{},
				},
				"send_message_by_custom_robot": map[string]any{
					"group":  "message",
					"hidden": true,
					"flags":  map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "send_robot_group_message", Title: "机器人发群消息", Description: "机器人发群消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "batch_send_robot_msg_to_users", Title: "机器人发单聊消息", Description: "机器人发单聊消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "recall_robot_group_message", Title: "机器人撤回群消息", Description: "机器人撤回群消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "batch_recall_robot_users_msg", Title: "机器人撤回单聊消息", Description: "机器人撤回单聊消息", InputSchema: map[string]any{"type": "object"}},
				{Name: "send_message_by_custom_robot", Title: "Webhook 发消息", Description: "Webhook 发消息", InputSchema: map[string]any{"type": "object"}},
			},
		},
	)
}
