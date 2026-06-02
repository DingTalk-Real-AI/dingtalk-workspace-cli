package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestRootCommandDoesNotInjectPatchedHelpCommands(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 3, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("doc", "文档管理", nil, map[string]any{
				"search_docs": map[string]any{
					"cliName": "search",
					"flags":   map[string]any{},
				},
			}),
			discoveryServerEntry("chat", "聊天管理", map[string]any{
				"message": map[string]any{"description": "消息管理"},
			}, map[string]any{
				"list_messages": map[string]any{
					"cliName": "list",
					"group":   "message",
					"flags":   map[string]any{},
				},
			}),
			discoveryServerEntry("minutes", "听记管理", map[string]any{
				"list": map[string]any{"description": "列表"},
			}, map[string]any{
				"list_minutes_mine": map[string]any{
					"cliName": "mine",
					"group":   "list",
					"flags":   map[string]any{},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	for _, path := range []string{
		"chat message list-topic-replies",
		"minutes list all",
	} {
		if cmd := lookupCommand(root, path); cmd != nil {
			t.Fatalf("findCommand(%q) = %q, want nil", path, cmd.CommandPath())
		}
	}
}

func TestDynamicLeafHelpDoesNotUsePatchedExamplesOrFlagText(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("aiapp", "AI应用管理", nil, map[string]any{
				"create_ai_app": map[string]any{
					"cliName": "create",
					"flags": map[string]any{
						"prompt": map[string]any{
							"alias": "prompt",
						},
					},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"aiapp", "create", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(aiapp create --help) error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "创建一个天气查询应用") {
		t.Fatalf("leaf help still contains patched example:\n%s", got)
	}
	if strings.Contains(got, "创建 AI 应用的 prompt（必填）") {
		t.Fatalf("leaf help still contains patched flag usage:\n%s", got)
	}
	if !strings.Contains(got, "--prompt string") {
		t.Fatalf("leaf help missing dynamic prompt flag:\n%s", got)
	}
}

func TestRootHelpUsesMCPOnlySummary(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 2, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("aiapp", "AI应用管理", nil, map[string]any{
				"create_ai_app": map[string]any{
					"cliName": "create",
					"flags":   map[string]any{},
				},
			}),
			discoveryServerEntry("aitable", "多维表管理", nil, map[string]any{
				"list_bases": map[string]any{
					"cliName": "list",
					"flags":   map[string]any{},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--help) error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"Discovered MCP Services:", "aiapp", "AI应用管理", "aitable", "多维表管理"} {
		if !strings.Contains(got, want) {
			t.Fatalf("root help missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"快速开始:", "更多信息:", "auth            认证管理", "Flags:"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("root help unexpectedly contains %q:\n%s", unwanted, got)
		}
	}
}

func TestRootHelpCustomizationDoesNotAffectSubcommandHelp(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("aiapp", "AI应用管理", nil, map[string]any{
				"create_ai_app": map[string]any{
					"cliName": "create",
					"flags": map[string]any{
						"prompt": map[string]any{
							"alias": "prompt",
						},
					},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"aiapp", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(aiapp --help) error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Usage:") || !strings.Contains(got, "Available Commands:") || !strings.Contains(got, "Flags:") {
		t.Fatalf("subcommand help should still use cobra default sections:\n%s", got)
	}
	if strings.Contains(got, "Discovered MCP Services:") {
		t.Fatalf("subcommand help should not render root-only MCP summary:\n%s", got)
	}
}

func TestRootCommandRegistersUpgradeCommand(t *testing.T) {
	root := NewRootCommand()
	if cmd := lookupCommand(root, "upgrade"); cmd == nil {
		t.Fatal("upgrade command should be registered on root, but was not found")
	}
}

func discoveryServerEntry(command, description string, groups, toolOverrides map[string]any) map[string]any {
	cliMeta := map[string]any{
		"id":            command,
		"command":       command,
		"description":   description,
		"toolOverrides": toolOverrides,
	}
	if len(groups) > 0 {
		cliMeta["groups"] = groups
	}

	return map[string]any{
		"server": map[string]any{
			"name":        command,
			"description": description,
			"remotes": []any{
				map[string]any{
					"type": "streamable-http",
					"url":  "https://mcp.dingtalk.com/" + command,
				},
			},
		},
		"_meta": map[string]any{
			"com.dingtalk.mcp.registry/metadata": map[string]any{
				"status":   "active",
				"isLatest": true,
			},
			"com.dingtalk.mcp.registry/cli": cliMeta,
		},
	}
}

func lookupCommand(root *cobra.Command, path string) *cobra.Command {
	if root == nil || path == "" {
		return root
	}

	cmd := root
	for _, part := range strings.Fields(path) {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == part {
				cmd = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}
