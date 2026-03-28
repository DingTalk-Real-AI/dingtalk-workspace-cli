package extensions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mock_mcp"
)

func TestCLIMetadataDrivesCanonicalCommandShape(t *testing.T) {
	t.Parallel()

	fixture := mockmcp.DefaultFixture()
	fixture.Servers = []mockmcp.ServerFixture{fixture.Servers[0]}
	fixture.Servers[0].CLI.ID = "knowledge"
	fixture.Servers[0].CLI.Command = "knowledge"
	fixture.Servers[0].CLI.Group = "office"
	fixture.Servers[0].CLI.Hidden = true
	fixture.Servers[0].CLI.Tools = []market.CLITool{
		{
			Name:        "create_document",
			CLIName:     "make-doc",
			Title:       "创建文档",
			Description: "创建文档",
			IsSensitive: boolPtr(true),
			Hidden:      boolPtr(true),
			Flags: map[string]market.CLIFlagHint{
				"title": {Alias: "name", Shorthand: "t"},
			},
		},
	}
	fixture.Servers[0].MCP.Tools = []transport.ToolDescriptor{
		{
			Name:        "create_document",
			Title:       "创建文档",
			Description: "创建文档",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string"},
				},
			},
		},
	}

	srv := newRuntimeMarketServer(t, fixture)
	defer srv.Close()

	service := discovery.NewService(
		market.NewClient(srv.URL, srv.Client()),
		transport.NewClient(srv.Client()),
		cache.NewStore(t.TempDir()),
	)
	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}
	runtimeServers, failures := service.DiscoverAllRuntime(context.Background(), servers)
	if len(failures) > 0 {
		t.Fatalf("DiscoverAllRuntime() failures = %v", failures[0].Err)
	}

	catalog := ir.BuildCatalog(runtimeServers)
	if len(catalog.Products) != 1 {
		t.Fatalf("catalog products len = %d, want 1", len(catalog.Products))
	}
	product := catalog.Products[0]
	if product.ID != "knowledge" {
		t.Fatalf("product.ID = %q, want knowledge", product.ID)
	}
	if product.CLI == nil || !product.CLI.Hidden || product.CLI.Group != "office" {
		t.Fatalf("product.CLI = %#v, want hidden office", product.CLI)
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if tool.CLIName != "make-doc" || !tool.Hidden {
		t.Fatalf("tool metadata = %#v, want make-doc hidden", tool)
	}

	cmd := cli.NewMCPCommand(cli.StaticLoader{Catalog: catalog}, executor.EchoRunner{})
	module := cmd.Commands()[0]
	if !module.Hidden || module.Name() != "knowledge" {
		t.Fatalf("module hidden/name = (%t,%s), want (true,knowledge)", module.Hidden, module.Name())
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"-f", "json", "knowledge", "make-doc", "--name", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload struct {
		Invocation struct {
			Tool   string         `json:"tool"`
			Params map[string]any `json:"params"`
		} `json:"invocation"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if payload.Invocation.Tool != "create_document" {
		t.Fatalf("invocation.tool = %q, want create_document", payload.Invocation.Tool)
	}
	if payload.Invocation.Params["title"] != "hello" {
		t.Fatalf("invocation.params.title = %#v, want hello", payload.Invocation.Params["title"])
	}
}

func boolPtr(value bool) *bool {
	return &value
}
