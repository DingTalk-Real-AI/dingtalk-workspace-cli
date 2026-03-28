package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/spf13/cobra"
)

func TestRootCommandDoesNotInjectPatchedHelpCommands(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newDiscoveryRuntimeServer(t,
		discoveryRuntimeFixture{
			Command:     "doc",
			Description: "文档管理",
			ToolOverrides: map[string]any{
				"search_docs": map[string]any{
					"cliName": "search",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{{
				Name:        "search_docs",
				Title:       "搜索文档",
				Description: "搜索文档",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		discoveryRuntimeFixture{
			Command:     "chat",
			Description: "聊天管理",
			Groups: map[string]any{
				"message": map[string]any{"description": "消息管理"},
			},
			ToolOverrides: map[string]any{
				"list_messages": map[string]any{
					"cliName": "list",
					"group":   "message",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{{
				Name:        "list_messages",
				Title:       "消息列表",
				Description: "消息列表",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		discoveryRuntimeFixture{
			Command:     "minutes",
			Description: "听记管理",
			Groups: map[string]any{
				"list": map[string]any{"description": "列表"},
			},
			ToolOverrides: map[string]any{
				"list_minutes_mine": map[string]any{
					"cliName": "mine",
					"group":   "list",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{{
				Name:        "list_minutes_mine",
				Title:       "我的听记",
				Description: "我的听记",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
	)
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	for _, path := range []string{
		"doc upload",
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

	srv := newDiscoveryRuntimeServer(t, discoveryRuntimeFixture{
		Command:     "aiapp",
		Description: "AI应用管理",
		ToolOverrides: map[string]any{
			"create_ai_app": map[string]any{
				"cliName": "create",
				"flags": map[string]any{
					"prompt": map[string]any{
						"alias": "prompt",
					},
				},
			},
		},
		Tools: []transport.ToolDescriptor{{
			Name:        "create_ai_app",
			Title:       "创建 AI 应用",
			Description: "创建 AI 应用",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{"type": "string"},
				},
			},
		}},
	})
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

	srv := newDiscoveryRuntimeServer(t,
		discoveryRuntimeFixture{
			Command:     "aiapp",
			Description: "AI应用管理",
			ToolOverrides: map[string]any{
				"create_ai_app": map[string]any{
					"cliName": "create",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{{
				Name:        "create_ai_app",
				Title:       "创建 AI 应用",
				Description: "创建 AI 应用",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		discoveryRuntimeFixture{
			Command:     "aitable",
			Description: "多维表管理",
			ToolOverrides: map[string]any{
				"list_bases": map[string]any{
					"cliName": "list",
					"flags":   map[string]any{},
				},
			},
			Tools: []transport.ToolDescriptor{{
				Name:        "list_bases",
				Title:       "列出 Base",
				Description: "列出 Base",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
	)
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

	srv := newDiscoveryRuntimeServer(t, discoveryRuntimeFixture{
		Command:     "aiapp",
		Description: "AI应用管理",
		ToolOverrides: map[string]any{
			"create_ai_app": map[string]any{
				"cliName": "create",
				"flags": map[string]any{
					"prompt": map[string]any{
						"alias": "prompt",
					},
				},
			},
		},
		Tools: []transport.ToolDescriptor{{
			Name:        "create_ai_app",
			Title:       "创建 AI 应用",
			Description: "创建 AI 应用",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{"type": "string"},
				},
			},
		}},
	})
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

func TestRootHelpDeduplicatesVisibleServiceNames(t *testing.T) {
	originalProducts := func() map[string]bool {
		dynamicMu.RLock()
		defer dynamicMu.RUnlock()

		cloned := make(map[string]bool, len(dynamicProducts))
		for key, value := range dynamicProducts {
			cloned[key] = value
		}
		return cloned
	}()

	dynamicMu.Lock()
	dynamicProducts = map[string]bool{"aitable": true}
	dynamicMu.Unlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicProducts = originalProducts
		dynamicMu.Unlock()
	})

	root := &cobra.Command{Use: "dws"}
	root.AddCommand(
		&cobra.Command{Use: "aitable", Short: "AI 表格操作"},
		&cobra.Command{Use: "aitable", Short: "AI 表格操作"},
	)

	var out bytes.Buffer
	root.SetOut(&out)

	renderRootHelp(root)

	if got := strings.Count(out.String(), "  aitable"); got != 1 {
		t.Fatalf("root help listed duplicate visible services %d times:\n%s", got, out.String())
	}
}

func TestRootCommandDoesNotRegisterUpgradeCommand(t *testing.T) {
	root := NewRootCommand()
	if cmd := lookupCommand(root, "upgrade"); cmd != nil {
		t.Fatalf("findCommand(upgrade) = %q, want nil", cmd.CommandPath())
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
