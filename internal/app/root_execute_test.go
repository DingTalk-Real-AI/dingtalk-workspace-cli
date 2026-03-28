// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	mockmcp "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mock_mcp"
	"github.com/spf13/cobra"
)

func TestPrintExecutionErrorDefaultsToHumanReadable(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := printExecutionError(root, &stdout, &stderr, apperrors.NewValidation(
		"bad flag",
		apperrors.WithHint("Pass the required flag and retry."),
	))
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty for human-readable error output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Error: [VALIDATION] bad flag") {
		t.Fatalf("stderr = %q, want human-readable header", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Hint: Pass the required flag and retry.") {
		t.Fatalf("stderr = %q, want hint line", stderr.String())
	}
}

func TestPrintExecutionErrorUsesJSONWhenFormatIsJSON(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	if err := root.PersistentFlags().Set("format", "json"); err != nil {
		t.Fatalf("Set(format) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := printExecutionError(root, &stdout, &stderr, apperrors.NewValidation("bad flag"))
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty for JSON error output", stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"category\": \"validation\"") {
		t.Fatalf("stdout = %q, want JSON error payload", stdout.String())
	}
}

func TestPrintExecutionErrorUsesJSONWhenCommandSetsJSONFlag(t *testing.T) {
	setupRuntimeCommandTest(t)

	server := mockmcp.DefaultServer()
	defer server.Close()
	t.Setenv(cli.CatalogFixtureEnv, writeDocCatalogFixture(t, server.RemoteURL("/server/doc"), false))

	root := NewRootCommand()
	root.SetArgs([]string{"doc", "search_documents", "--json", "{"})

	executed, execErr := root.ExecuteC()
	if execErr == nil {
		t.Fatal("ExecuteC() error = nil, want validation error")
	}
	if executed == nil {
		t.Fatal("ExecuteC() returned nil command")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := printExecutionError(executed, &stdout, &stderr, execErr)
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty for JSON error output", stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"category\": \"validation\"") {
		t.Fatalf("stdout = %q, want JSON error payload", stdout.String())
	}
}

func TestCompletionCommandUsesConfiguredWriter(t *testing.T) {
	setupRuntimeCommandTest(t)

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "bash"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "bash completion for dws") {
		t.Fatalf("output = %q, want completion script in configured writer", out.String())
	}
}

func TestGenerateSkillsWritesSkillsFromFixtureCatalog(t *testing.T) {
	setupRuntimeCommandTest(t)
	server := mockmcp.DefaultServer()
	defer server.Close()

	fixture := writeDocCatalogFixture(t, server.RemoteURL("/server/doc"), false)

	outputRoot := t.TempDir()
	legacySkillDir := filepath.Join(outputRoot, "skills", "generated", "dws-doc")
	if err := os.MkdirAll(legacySkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacySkill := filepath.Join(legacySkillDir, "api.md")
	if err := os.WriteFile(legacySkill, []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"generate-skills", "--source", "fixture", "--fixture", fixture, "--output-root", outputRoot, "--with-docs=false"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(generate-skills) error = %v\noutput:\n%s", err, out.String())
	}

	if !strings.Contains(out.String(), "generated") {
		t.Fatalf("output = %q, want generation summary", out.String())
	}
	generatedSkill := filepath.Join(outputRoot, "skills", "generated", "dws-doc", "SKILL.md")
	content, err := os.ReadFile(generatedSkill)
	if err != nil {
		t.Fatalf("generated skill %s not found: %v", generatedSkill, err)
	}
	text := string(content)
	for _, snippet := range []string{
		"name: dws-doc",
		"metadata:",
		"openclaw:",
		"bins:",
		"- dws",
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("generated skill missing %q:\n%s", snippet, text)
		}
	}
	if _, err := os.Stat(legacySkill); !os.IsNotExist(err) {
		t.Fatalf("legacy api.md still exists after generation: %v", err)
	}
}

func TestGenerateSkillsWritesDocsUnderSkillsGeneratedDocs(t *testing.T) {
	setupRuntimeCommandTest(t)
	server := mockmcp.DefaultServer()
	defer server.Close()

	fixture := writeDocCatalogFixture(t, server.RemoteURL("/server/doc"), false)

	outputRoot := t.TempDir()
	legacyDocsReadme := filepath.Join(outputRoot, "docs", "generated", "README.md")
	if err := os.MkdirAll(filepath.Dir(legacyDocsReadme), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(legacyDocsReadme, []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"generate-skills", "--source", "fixture", "--fixture", fixture, "--output-root", outputRoot})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(generate-skills --with-docs) error = %v\noutput:\n%s", err, out.String())
	}

	if !strings.Contains(out.String(), "generated") {
		t.Fatalf("output = %q, want generation summary", out.String())
	}
	for _, want := range []string{
		filepath.Join(outputRoot, "skills", "generated", "docs", "README.md"),
		filepath.Join(outputRoot, "skills", "generated", "docs", "schema", "catalog.json"),
		filepath.Join(outputRoot, "skills", "generated", "docs", "cli", "canonical-cli.md"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("Stat(%s) error = %v", want, err)
		}
	}
	if _, err := os.Stat(legacyDocsReadme); !os.IsNotExist(err) {
		t.Fatalf("legacy docs/generated/README.md still exists after generation: %v", err)
	}
}

func TestUnknownSubcommandShowsHelp(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"cache", "nonexistent-cmd"})

	executed, err := root.ExecuteC()
	if err == nil {
		t.Fatal("ExecuteC() error = nil, want unknown command error")
	}
	if !isUnknownCommandError(err) {
		t.Fatalf("isUnknownCommandError() = false for error: %v", err)
	}

	// Simulate what Execute() does: redirect output to stderr and print help
	if executed == nil {
		executed = root
	}
	executed.SetOut(&out)
	_ = executed.Help()

	combined := out.String()
	// Help text should include the parent command's usage
	if !strings.Contains(combined, "cache") {
		t.Fatalf("output should contain parent command name 'cache', got:\n%s", combined)
	}
	// Help text should list available subcommands
	if !strings.Contains(combined, "Available Commands") {
		t.Fatalf("output should contain 'Available Commands', got:\n%s", combined)
	}
	if !strings.Contains(combined, "refresh") {
		t.Fatalf("output should list 'refresh' subcommand, got:\n%s", combined)
	}
}

func TestVersionCommandDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_PIN", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	if !strings.Contains(out.String(), "版本:") {
		t.Fatalf("version output missing version header:\n%s", out.String())
	}
}

func TestVersionCommandBlocksOnAgedDiscoveryRevalidation(t *testing.T) {
	t.Setenv("DWS_PIN", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC().Add(-2 * time.Hour),
		Servers: []market.ServerDescriptor{minimalCLIServer("cached", "https://mcp.dingtalk.com/cached/v1")},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		response := map[string]any{
			"metadata": map[string]any{"count": 1, "nextCursor": ""},
			"servers": []any{
				discoveryRuntimeServerEntry("http://"+r.Host, discoveryRuntimeFixture{
					Command:     "network-server",
					Description: "Network Server",
					ToolOverrides: map[string]any{
						"test_tool": map[string]any{
							"cliName": "test",
							"flags":   map[string]any{},
						},
					},
				}),
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/network-server", func(w http.ResponseWriter, r *http.Request) {
		serveAppRuntimeToolList(t, w, r, "network-server", transport.ToolDescriptor{
			Name:        "test_tool",
			Title:       "Test Tool",
			Description: "Test Tool",
			InputSchema: map[string]any{"type": "object"},
		})
	})

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	start := time.Now()
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("Execute(version) took %v, want synchronous revalidation to block on live discovery", elapsed)
	}
	if !strings.Contains(out.String(), "版本:") {
		t.Fatalf("version output missing version header:\n%s", out.String())
	}
}

func TestRootHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_PIN", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newDiscoveryRuntimeServer(t, discoveryRuntimeFixture{
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
	})
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
	if !strings.Contains(out.String(), "Discovered MCP Services:") {
		t.Fatalf("root help output missing MCP summary:\n%s", out.String())
	}
}

func TestRootShortHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_PIN", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newDiscoveryRuntimeServer(t, discoveryRuntimeFixture{
		Command:     "devdoc",
		Description: "开放平台文档搜索",
		Groups: map[string]any{
			"article": map[string]any{"description": "文档文章"},
		},
		ToolOverrides: map[string]any{
			"search_article": map[string]any{
				"cliName": "search",
				"group":   "article",
				"flags":   map[string]any{},
			},
		},
		Tools: []transport.ToolDescriptor{{
			Name:        "search_article",
			Title:       "搜索文章",
			Description: "搜索文章",
			InputSchema: map[string]any{"type": "object"},
		}},
	})
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(-h) error = %v", err)
	}
	if !strings.Contains(out.String(), "Discovered MCP Services:") {
		t.Fatalf("root short help output missing MCP summary:\n%s", out.String())
	}
}

func TestNestedShortHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_PIN", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newDiscoveryRuntimeServer(t, discoveryRuntimeFixture{
		Command:     "devdoc",
		Description: "开放平台文档搜索",
		Groups: map[string]any{
			"article": map[string]any{"description": "文档文章"},
		},
		ToolOverrides: map[string]any{
			"search_article": map[string]any{
				"cliName": "search",
				"group":   "article",
				"flags": map[string]any{
					"keyword": map[string]any{"alias": "keyword"},
				},
			},
		},
		Tools: []transport.ToolDescriptor{{
			Name:        "search_article",
			Title:       "搜索文章",
			Description: "搜索文章",
			InputSchema: map[string]any{"type": "object"},
		}},
	})
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"devdoc", "article", "search", "-h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(devdoc article search -h) error = %v", err)
	}
	if !strings.Contains(out.String(), "Usage:\n  dws devdoc article search [flags]") {
		t.Fatalf("nested short help output missing canonical usage block:\n%s", out.String())
	}
}

func TestChatGroupMembersRemoveCommandRemainsReachableWithExecutableParent(t *testing.T) {
	setupRuntimeCommandTest(t)
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	srv := newDiscoveryRuntimeServer(t,
		discoveryRuntimeFixture{
			ID:           "bot",
			Command:      "bot-runtime",
			CLICommand:   "chat",
			EndpointPath: "bot-runtime",
			Description:  "机器人消息",
			Groups: map[string]any{
				"bot":           map[string]any{"description": "机器人管理"},
				"group.members": map[string]any{"description": "群成员管理"},
			},
			ToolOverrides: map[string]any{
				"search_my_robots": map[string]any{
					"cliName": "search",
					"group":   "bot",
					"flags": map[string]any{
						"robotName": map[string]any{"alias": "name"},
					},
				},
				"add_robot_to_group": map[string]any{
					"cliName": "add-bot",
					"group":   "group.members",
					"flags": map[string]any{
						"openConversationId": map[string]any{"alias": "id"},
						"robotCode":          map[string]any{"alias": "robot-code"},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{
					Name:        "search_my_robots",
					Title:       "搜索机器人",
					Description: "搜索机器人",
					InputSchema: map[string]any{"type": "object"},
				},
				{
					Name:        "add_robot_to_group",
					Title:       "添加机器人",
					Description: "添加机器人",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
		discoveryRuntimeFixture{
			ID:           "group-chat",
			Command:      "group-chat-runtime",
			CLICommand:   "chat",
			EndpointPath: "group-chat-runtime",
			Description:  "群聊管理",
			Groups: map[string]any{
				"group":         map[string]any{"description": "群组管理"},
				"group.members": map[string]any{"description": "群成员管理"},
				"message":       map[string]any{"description": "消息管理"},
			},
			ToolOverrides: map[string]any{
				"search_groups_by_keyword": map[string]any{
					"cliName": "search",
					"flags": map[string]any{
						"OpenSearchRequest.cursor":  map[string]any{"alias": "cursor"},
						"OpenSearchRequest.keyword": map[string]any{"alias": "query"},
					},
				},
				"create_internal_group": map[string]any{
					"cliName": "create",
					"group":   "group",
				},
				"update_group_name": map[string]any{
					"cliName": "rename",
					"group":   "group",
				},
				"get_group_members": map[string]any{
					"cliName": "members",
					"group":   "group",
					"flags": map[string]any{
						"cursor":              map[string]any{"alias": "cursor"},
						"openconversation_id": map[string]any{"alias": "id"},
					},
				},
				"add_group_member": map[string]any{
					"cliName": "add",
					"group":   "group.members",
					"flags": map[string]any{
						"openconversation_id": map[string]any{"alias": "id"},
						"userId":              map[string]any{"alias": "users"},
					},
				},
				"remove_group_member": map[string]any{
					"cliName": "remove",
					"group":   "group.members",
					"flags": map[string]any{
						"openconversationId": map[string]any{"alias": "id"},
						"userIdList": map[string]any{
							"alias":     "users",
							"transform": "csv_to_array",
						},
					},
				},
			},
			Tools: []transport.ToolDescriptor{
				{Name: "search_groups_by_keyword", Title: "搜索群聊", Description: "搜索群聊", InputSchema: map[string]any{"type": "object"}},
				{Name: "create_internal_group", Title: "创建群聊", Description: "创建群聊", InputSchema: map[string]any{"type": "object"}},
				{Name: "update_group_name", Title: "修改群名", Description: "修改群名", InputSchema: map[string]any{"type": "object"}},
				{
					Name:        "get_group_members",
					Title:       "查群成员列表",
					Description: "查群成员列表",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"openconversation_id": map[string]any{"type": "string"},
							"cursor":              map[string]any{"type": "string"},
						},
					},
				},
				{
					Name:        "add_group_member",
					Title:       "添加群成员",
					Description: "添加群成员",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"openconversation_id": map[string]any{"type": "string"},
							"userId":              map[string]any{"type": "string"},
						},
					},
				},
				{
					Name:        "remove_group_member",
					Title:       "移除群成员",
					Description: "移除群成员",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"openconversationId": map[string]any{"type": "string"},
							"userIdList": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	)
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	if cmd := lookupCommand(root, "chat group members remove"); cmd == nil {
		members := lookupCommand(root, "chat group members")
		var children []string
		if members != nil {
			for _, child := range members.Commands() {
				children = append(children, child.Name())
			}
		}
		loader := cli.NewEnvironmentLoader()
		loader.CatalogBaseURLOverride = DiscoveryBaseURL()
		catalog, err := loader.Load(context.Background())
		if err != nil {
			t.Fatalf("lookupCommand(chat group members remove) = nil; members children = %v; catalog load error = %v", children, err)
		}
		var chatTools []string
		if product, ok := catalog.FindProduct("group-chat"); ok {
			for _, tool := range product.Tools {
				chatTools = append(chatTools, strings.Join(append([]string{tool.CLIName}, tool.Group), "@"))
			}
		}
		bareRoot := &cobra.Command{Use: "dws"}
		if err := cli.AddCanonicalProducts(bareRoot, cli.StaticLoader{Catalog: catalog}, executor.EchoRunner{}); err != nil {
			t.Fatalf("lookupCommand(chat group members remove) = nil; members children = %v; group-chat tools = %v; AddCanonicalProducts error = %v", children, chatTools, err)
		}
		var bareChildren []string
		if bareMembers := lookupCommand(bareRoot, "chat group members"); bareMembers != nil {
			for _, child := range bareMembers.Commands() {
				bareChildren = append(bareChildren, child.Name())
			}
		}
		t.Fatalf("lookupCommand(chat group members remove) = nil; members children = %v; group-chat tools = %v; bare members children = %v", children, chatTools, bareChildren)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"chat", "group", "members", "-h"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(chat group members -h) error = %v", err)
	}
	if !strings.Contains(out.String(), "  remove      ") && !strings.Contains(out.String(), "  remove  ") {
		t.Fatalf("group members help missing remove subcommand:\n%s", out.String())
	}

	out.Reset()
	root.SetArgs([]string{"chat", "group", "members", "remove", "-h"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(chat group members remove -h) error = %v", err)
	}
	if !strings.Contains(out.String(), "Usage:\n  dws chat group members remove [flags]") {
		t.Fatalf("remove help missing canonical usage block:\n%s", out.String())
	}
}
