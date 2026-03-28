package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

func TestChatMessageCompatCommandsDispatchToCanonicalTools(t *testing.T) {
	setupRuntimeCommandTest(t)
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "*")

	server, calls := newChatCompatMCPServer(t)
	defer server.Close()

	t.Setenv(cli.CatalogFixtureEnv, writeChatCompatCatalogFixture(t, server.URL))

	testCases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "list group",
			args:     []string{"-f", "json", "chat", "message", "list", "--group", "cid-1", "--time", "2025-03-01 00:00:00", "--yes"},
			wantTool: "list_conversation_message_v2",
			wantParams: map[string]any{
				"openconversation_id": "cid-1",
				"time":                "2025-03-01 00:00:00",
				"forward":             true,
			},
		},
		{
			name:     "send user",
			args:     []string{"-f", "json", "chat", "message", "send", "--user", "user-1", "hi", "--yes"},
			wantTool: "send_direct_message_as_user",
			wantParams: map[string]any{
				"receiverUserId": "user-1",
				"title":          "消息",
				"text":           "hi",
			},
		},
		{
			name:     "send by bot users",
			args:     []string{"-f", "json", "chat", "message", "send-by-bot", "--robot-code", "robot-1", "--users", "u1,u2", "--title", "提醒", "--text", "内容", "--yes"},
			wantTool: "batch_send_robot_msg_to_users",
			wantParams: map[string]any{
				"robotCode": "robot-1",
				"userIds":   []any{"u1", "u2"},
				"title":     "提醒",
				"markdown":  "内容",
			},
		},
		{
			name:     "send by bot raw alias",
			args:     []string{"-f", "json", "chat", "message", "send_robot_group_message", "--robot-code", "robot-1", "--open-conversation-id", "cid-1", "--title", "提醒", "--markdown", "内容", "--yes"},
			wantTool: "send_robot_group_message",
			wantParams: map[string]any{
				"robotCode":          "robot-1",
				"openConversationId": "cid-1",
				"title":              "提醒",
				"markdown":           "内容",
			},
		},
		{
			name:     "send by bot raw json alias",
			args:     []string{"-f", "json", "chat", "message", "send_robot_group_message", "--json", `{"robotCode":"robot-1","openConversationId":"cid-1","title":"提醒","markdown":"内容"}`, "--yes"},
			wantTool: "send_robot_group_message",
			wantParams: map[string]any{
				"robotCode":          "robot-1",
				"openConversationId": "cid-1",
				"title":              "提醒",
				"markdown":           "内容",
			},
		},
		{
			name:     "recall by bot group",
			args:     []string{"-f", "json", "chat", "message", "recall-by-bot", "--robot-code", "robot-1", "--group", "cid-1", "--keys", "k1,k2", "--yes"},
			wantTool: "recall_robot_group_message",
			wantParams: map[string]any{
				"robotCode":          "robot-1",
				"openConversationId": "cid-1",
				"processQueryKeys":   []any{"k1", "k2"},
			},
		},
		{
			name:     "recall by bot raw alias",
			args:     []string{"-f", "json", "chat", "message", "recall_robot_group_message", "--robot-code", "robot-1", "--open-conversation-id", "cid-1", "--process-query-keys", "k1,k2", "--yes"},
			wantTool: "recall_robot_group_message",
			wantParams: map[string]any{
				"robotCode":          "robot-1",
				"openConversationId": "cid-1",
				"processQueryKeys":   []any{"k1", "k2"},
			},
		},
		{
			name:     "batch recall by bot raw json alias",
			args:     []string{"-f", "json", "chat", "message", "batch_recall_robot_users_msg", "--json", `{"robotCode":"robot-1","processQueryKeys":["k1","k2"]}`, "--yes"},
			wantTool: "batch_recall_robot_users_msg",
			wantParams: map[string]any{
				"robotCode":        "robot-1",
				"processQueryKeys": []any{"k1", "k2"},
			},
		},
		{
			name:     "send by webhook",
			args:     []string{"-f", "json", "chat", "message", "send-by-webhook", "--token", "token-1", "--title", "告警", "--text", "body", "--at-users", "u1,u2", "--yes"},
			wantTool: "send_message_by_custom_robot",
			wantParams: map[string]any{
				"robotToken": "token-1",
				"title":      "告警",
				"text":       "body",
				"atUserIds":  []any{"u1", "u2"},
			},
		},
		{
			name:     "send by webhook raw alias",
			args:     []string{"-f", "json", "chat", "message", "send_message_by_custom_robot", "--robot-token", "token-1", "--title", "告警", "--text", "body", "--at-user-ids", "u1,u2", "--is-at-all", "--yes"},
			wantTool: "send_message_by_custom_robot",
			wantParams: map[string]any{
				"robotToken": "token-1",
				"title":      "告警",
				"text":       "body",
				"atUserIds":  []any{"u1", "u2"},
				"isAtAll":    true,
			},
		},
		{
			name:     "send by webhook raw json alias",
			args:     []string{"-f", "json", "chat", "message", "send_message_by_custom_robot", "--json", `{"robotToken":"token-1","title":"告警","text":"body","atUserIds":["u1","u2"],"isAtAll":true}`, "--yes"},
			wantTool: "send_message_by_custom_robot",
			wantParams: map[string]any{
				"robotToken": "token-1",
				"title":      "告警",
				"text":       "body",
				"atUserIds":  []any{"u1", "u2"},
				"isAtAll":    true,
			},
		},
		{
			name:     "list topic replies",
			args:     []string{"-f", "json", "chat", "message", "list-topic-replies", "--group", "cid-1", "--topic-id", "topic-1", "--yes"},
			wantTool: "list_topic_replies",
			wantParams: map[string]any{
				"openconversationId": "cid-1",
				"topicId":            "topic-1",
				"pageSize":           float64(50),
				"forward":            false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			calls.Reset()

			cmd := NewRootCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%s) error = %v", strings.Join(tc.args, " "), err)
			}

			gotTool, gotParams := calls.LastToolCall(t)
			if gotTool != tc.wantTool {
				t.Fatalf("tool = %q, want %q", gotTool, tc.wantTool)
			}
			if !reflect.DeepEqual(gotParams, tc.wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, tc.wantParams)
			}
		})
	}
}

type chatCompatCalls struct {
	mu     sync.Mutex
	names  []string
	params []map[string]any
}

func (c *chatCompatCalls) Record(name string, params map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
	c.params = append(c.params, params)
}

func (c *chatCompatCalls) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = nil
	c.params = nil
}

func (c *chatCompatCalls) LastToolCall(t *testing.T) (string, map[string]any) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.names) != 1 || len(c.params) != 1 {
		t.Fatalf("recorded tool calls = %#v / %#v, want exactly one", c.names, c.params)
	}
	return c.names[0], c.params[0]
}

func newChatCompatMCPServer(t *testing.T) (*httptest.Server, *chatCompatCalls) {
	t.Helper()

	calls := &chatCompatCalls{}
	tools := []map[string]any{
		{"name": "list_conversation_message_v2", "inputSchema": map[string]any{"type": "object"}},
		{"name": "list_individual_chat_message", "inputSchema": map[string]any{"type": "object"}},
		{"name": "send_message_as_user", "inputSchema": map[string]any{"type": "object"}},
		{"name": "send_direct_message_as_user", "inputSchema": map[string]any{"type": "object"}},
		{"name": "list_topic_replies", "inputSchema": map[string]any{"type": "object"}},
		{"name": "send_robot_group_message", "inputSchema": map[string]any{"type": "object"}},
		{"name": "batch_send_robot_msg_to_users", "inputSchema": map[string]any{"type": "object"}},
		{"name": "recall_robot_group_message", "inputSchema": map[string]any{"type": "object"}},
		{"name": "batch_recall_robot_users_msg", "inputSchema": map[string]any{"type": "object"}},
		{"name": "send_message_by_custom_robot", "inputSchema": map[string]any{"type": "object"}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		method, _ := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
					"serverInfo":      map[string]any{"name": "chat-compat", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result":  map[string]any{"tools": tools},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			name, _ := params["name"].(string)
			arguments, _ := params["arguments"].(map[string]any)
			calls.Record(name, arguments)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"content": map[string]any{"ok": true},
				},
			})
		default:
			http.Error(w, "unsupported method", http.StatusBadRequest)
		}
	}))

	return server, calls
}

func writeChatCompatCatalogFixture(t *testing.T, endpoint string) string {
	t.Helper()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:          "group-chat",
				DisplayName: "钉钉群聊",
				Description: "群聊 / 会话 / 群组管理",
				ServerKey:   "group-chat-fixture",
				Endpoint:    endpoint,
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
					Groups: map[string]ir.CLIGroupDef{
						"message": {Description: "会话消息管理"},
					},
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "list_conversation_message_v2",
						CLIName:       "list",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "group-chat.list_conversation_message_v2",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"openconversation_id", "time"},
							"properties": map[string]any{
								"openconversation_id": map[string]any{"type": "string"},
								"time":                map[string]any{"type": "string"},
								"forward":             map[string]any{"type": "boolean"},
								"limit":               map[string]any{"type": "integer"},
							},
						},
					},
					{
						RPCName:       "list_individual_chat_message",
						CLIName:       "list_individual_chat_message",
						Hidden:        true,
						CanonicalPath: "group-chat.list_individual_chat_message",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"userId", "time"},
							"properties": map[string]any{
								"userId":  map[string]any{"type": "string"},
								"time":    map[string]any{"type": "string"},
								"forward": map[string]any{"type": "boolean"},
								"limit":   map[string]any{"type": "integer"},
							},
						},
					},
					{
						RPCName:       "send_message_as_user",
						CLIName:       "send",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "group-chat.send_message_as_user",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"openConversation_id", "title", "text"},
							"properties": map[string]any{
								"openConversation_id": map[string]any{"type": "string"},
								"title":               map[string]any{"type": "string"},
								"text":                map[string]any{"type": "string"},
								"atAll":               map[string]any{"type": "boolean"},
								"atUserIds": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
					{
						RPCName:       "send_direct_message_as_user",
						CLIName:       "send_direct_message_as_user",
						Hidden:        true,
						CanonicalPath: "group-chat.send_direct_message_as_user",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"receiverUserId", "title", "text"},
							"properties": map[string]any{
								"receiverUserId": map[string]any{"type": "string"},
								"title":          map[string]any{"type": "string"},
								"text":           map[string]any{"type": "string"},
							},
						},
					},
					{
						RPCName:       "list_topic_replies",
						CLIName:       "list-topic-replies",
						Group:         "message",
						CanonicalPath: "group-chat.list_topic_replies",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"openconversationId", "topicId"},
							"properties": map[string]any{
								"openconversationId": map[string]any{"type": "string"},
								"topicId":            map[string]any{"type": "string"},
								"startTime":          map[string]any{"type": "string"},
								"pageSize":           map[string]any{"type": "integer"},
								"forward":            map[string]any{"type": "boolean"},
							},
						},
					},
				},
			},
			{
				ID:          "bot",
				DisplayName: "机器人消息",
				Description: "机器人消息",
				ServerKey:   "bot-fixture",
				Endpoint:    endpoint,
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
					Groups: map[string]ir.CLIGroupDef{
						"message": {Description: "会话消息管理"},
					},
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "send_robot_group_message",
						CLIName:       "send_robot_group_message",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "bot.send_robot_group_message",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"robotCode", "openConversationId", "title", "markdown"},
							"properties": map[string]any{
								"robotCode":          map[string]any{"type": "string"},
								"openConversationId": map[string]any{"type": "string"},
								"title":              map[string]any{"type": "string"},
								"markdown":           map[string]any{"type": "string"},
							},
						},
					},
					{
						RPCName:       "batch_send_robot_msg_to_users",
						CLIName:       "batch_send_robot_msg_to_users",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "bot.batch_send_robot_msg_to_users",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"robotCode", "userIds", "title", "markdown"},
							"properties": map[string]any{
								"robotCode": map[string]any{"type": "string"},
								"userIds": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"title":    map[string]any{"type": "string"},
								"markdown": map[string]any{"type": "string"},
							},
						},
					},
					{
						RPCName:       "recall_robot_group_message",
						CLIName:       "recall_robot_group_message",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "bot.recall_robot_group_message",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"robotCode", "openConversationId", "processQueryKeys"},
							"properties": map[string]any{
								"robotCode":          map[string]any{"type": "string"},
								"openConversationId": map[string]any{"type": "string"},
								"processQueryKeys": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
					{
						RPCName:       "batch_recall_robot_users_msg",
						CLIName:       "batch_recall_robot_users_msg",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "bot.batch_recall_robot_users_msg",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"robotCode", "processQueryKeys"},
							"properties": map[string]any{
								"robotCode": map[string]any{"type": "string"},
								"processQueryKeys": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
					{
						RPCName:       "send_message_by_custom_robot",
						CLIName:       "send_message_by_custom_robot",
						Group:         "message",
						Hidden:        true,
						CanonicalPath: "bot.send_message_by_custom_robot",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"robotToken", "title", "text"},
							"properties": map[string]any{
								"robotToken": map[string]any{"type": "string"},
								"title":      map[string]any{"type": "string"},
								"text":       map[string]any{"type": "string"},
								"isAtAll":    map[string]any{"type": "boolean"},
								"atMobiles": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"atUserIds": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "chat-compat-catalog.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}
