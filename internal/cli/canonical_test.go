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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/spf13/cobra"
)

func TestBuildFlagSpecsGeneratesOnlySupportedTopLevelFlags(t *testing.T) {
	t.Parallel()

	specs := BuildFlagSpecs(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Document title",
			},
			"notify": map[string]any{
				"type": "boolean",
			},
			"metadata": map[string]any{
				"type": "object",
			},
			"tags": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}, map[string]ir.CLIFlagHint{
		"title": {
			Shorthand: "t",
			Alias:     "name",
		},
	})

	if len(specs) != 4 {
		t.Fatalf("BuildFlagSpecs() len = %d, want 4", len(specs))
	}
	if specs[0].PropertyName != "metadata" || specs[1].PropertyName != "notify" || specs[2].PropertyName != "tags" || specs[3].PropertyName != "title" {
		t.Fatalf("BuildFlagSpecs() unexpected order = %#v", specs)
	}
	if specs[0].Kind != "json" {
		t.Fatalf("BuildFlagSpecs() metadata kind = %q, want json", specs[0].Kind)
	}
	if specs[3].Alias != "name" || specs[3].Shorthand != "t" {
		t.Fatalf("BuildFlagSpecs() title hints = %#v, want alias=name shorthand=t", specs[3])
	}
}

func TestFixtureLoaderLoadsCatalog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "catalog.json")
	data := []byte(`{"products":[{"id":"doc","display_name":"文档","server_key":"doc-key","endpoint":"https://example.com/server/doc","tools":[{"rpc_name":"create_document","canonical_path":"doc.create_document"}]}]}`)
	if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	catalog, err := FixtureLoader{Path: fixturePath}.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(catalog.Products) != 1 || catalog.Products[0].ID != "doc" {
		t.Fatalf("Load() catalog = %#v, want doc product", catalog)
	}
}

func TestSchemaPayloadFindsTool(t *testing.T) {
	t.Parallel()

	payload, err := schemaPayload(ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_document",
						CanonicalPath: "doc.create_document",
						InputSchema: map[string]any{
							"type": "object",
							"required": []any{
								"title",
							},
						},
					},
				},
			},
		},
	}, []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("schemaPayload() error = %v", err)
	}
	if payload["kind"] != "schema" {
		t.Fatalf("schemaPayload() kind = %#v, want schema", payload["kind"])
	}
}

func TestNewMCPCommandReturnsLoaderErrorForInvocations(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("fixture missing")
	cmd := NewMCPCommand(context.Background(), errorLoader{err: wantErr}, executor.EchoRunner{}, nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create_document"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestNewMCPCommandSkipsProductsMarkedSkip(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Skip: true,
					},
				},
				{
					ID: "drive",
				},
			},
		},
	}, executor.EchoRunner{}, nil)

	if got := cmd.Commands(); len(got) != 1 || got[0].Name() != "drive" {
		t.Fatalf("mcp commands = %#v, want only drive", got)
	}
}

func TestProductCommandUsesCLICommandAlias(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "create_document"},
					},
				},
			},
		},
	}, runner, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"documents", "create_document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("runner.last.CanonicalProduct = %q, want doc", runner.last.CanonicalProduct)
	}
}

func TestNewMCPCommandAddsGroupedRoutesFromCLIMetadata(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					CLI: &ir.ProductCLIMetadata{
						Command: "documents",
						Group:   "office/collab",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "create_document"},
					},
				},
			},
		},
	}, runner, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"office", "collab", "documents", "create_document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("runner.last.CanonicalProduct = %q, want doc", runner.last.CanonicalProduct)
	}
}

func TestToolCommandUsesCLINameAndFlagHints(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "create_document",
							CLIName: "create",
							InputSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"title": map[string]any{"type": "string"},
								},
							},
							FlagHints: map[string]ir.CLIFlagHint{
								"title": {
									Alias:     "name",
									Shorthand: "t",
								},
							},
						},
					},
				},
			},
		},
	}, runner, nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create", "--name", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Tool != "create_document" {
		t.Fatalf("runner.last.Tool = %q, want create_document", runner.last.Tool)
	}
	if runner.last.Params["title"] != "hello" {
		t.Fatalf("runner.last.Params[title] = %#v, want hello", runner.last.Params["title"])
	}
}

func TestToolCommandValidatesInputSchemaBeforeRun(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "create_document",
							InputSchema: map[string]any{
								"type": "object",
								"required": []any{
									"title",
								},
								"properties": map[string]any{
									"title": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}, runner, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doc", "create_document", "--params", `{"title":"ok","unknown":"x"}`})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want schema validation error")
	}
	if !strings.Contains(err.Error(), "$.unknown is not allowed") {
		t.Fatalf("Execute() error = %v, want unknown-property validation", err)
	}
	if runner.called != 0 {
		t.Fatalf("runner called = %d, want 0", runner.called)
	}
}

func TestToolCommandSupportsDryRunWithoutSensitiveConfirmation(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName:   "create_document",
							Sensitive: true,
							InputSchema: map[string]any{
								"type": "object",
							},
						},
					},
				},
			},
		},
	}, runner, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.PersistentFlags().Bool("dry-run", false, "Preview the operation without executing it")
	cmd.SetArgs([]string{"doc", "create_document", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !runner.last.DryRun {
		t.Fatalf("runner.last.DryRun = %t, want true", runner.last.DryRun)
	}
	if runner.called != 1 {
		t.Fatalf("runner called = %d, want 1", runner.called)
	}
}

func TestDeprecatedLifecycleAddsWarningToResult(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-doc",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedBy:    9527,
						DeprecationDate: "2026-04-01T00:00:00Z",
						MigrationURL:    "https://example.com/migration",
					},
					Tools: []ir.ToolDescriptor{
						{
							RPCName: "search_documents",
						},
					},
				},
			},
		},
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-doc", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload struct {
		Response map[string]any `json:"response"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if payload.Response["warning"] == "" {
		t.Fatalf("warning is empty, payload=%#v", payload.Response)
	}
	warning, _ := payload.Response["warning"].(string)
	if !strings.Contains(warning, "deprecated_by_mcpId=9527") {
		t.Fatalf("warning = %q, want deprecated_by_mcpId=9527", warning)
	}
}

func TestDeprecatedLifecyclePrintsWarningToStderr(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-doc",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedBy: 9527,
						MigrationURL: "https://example.com/migration",
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "search_documents"},
					},
				},
			},
		},
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-doc", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "warning: product legacy-doc is deprecated") {
		t.Fatalf("stderr = %q, want deprecation warning", stderr)
	}
	if !strings.Contains(stderr, "migration=https://example.com/migration") {
		t.Fatalf("stderr = %q, want migration hint", stderr)
	}
}

func TestSensitiveToolConfirmationWorksWithoutYesFlag(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "doc",
					Tools: []ir.ToolDescriptor{
						{
							RPCName:   "create_document",
							CLIName:   "create-document",
							Sensitive: true,
						},
					},
				},
			},
		},
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"doc", "create-document"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestLegacyCandidateLifecycleAddsWarningToResult(t *testing.T) {
	t.Parallel()

	cmd := NewMCPCommand(context.Background(), StaticLoader{
		Catalog: ir.Catalog{
			Products: []ir.CanonicalProduct{
				{
					ID: "legacy-candidate",
					Lifecycle: &ir.LifecycleInfo{
						DeprecatedCandidate: true,
					},
					Tools: []ir.ToolDescriptor{
						{RPCName: "search_documents"},
					},
				},
			},
		},
	}, executor.EchoRunner{}, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"legacy-candidate", "search_documents"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload struct {
		Response map[string]any `json:"response"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	warning, _ := payload.Response["warning"].(string)
	if !strings.Contains(warning, "legacy candidate") {
		t.Fatalf("warning = %q, want legacy candidate marker", warning)
	}
}

// ---------------------------------------------------------------------------
// Input source resolution: @file for string flags
// ---------------------------------------------------------------------------

func TestToolCommandResolvesAtFileForStringFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "msg.md")
	if err := os.WriteFile(filePath, []byte("Hello from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@" + filePath, "--user-id", "u001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "Hello from file" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "Hello from file")
	}
	if runner.last.Params["user_id"] != "u001" {
		t.Errorf("params[user_id] = %q, want %q", runner.last.Params["user_id"], "u001")
	}
}

func TestToolCommandResolvesAtFileForJsonFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "payload.json")
	payload := `{"text":"from json file","user_id":"u002"}`
	if err := os.WriteFile(filePath, []byte(payload), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--json", "@" + filePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "from json file" {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], "from json file")
	}
	if runner.last.Params["user_id"] != "u002" {
		t.Errorf("params[user_id] = %q, want %q", runner.last.Params["user_id"], "u002")
	}
}

func TestToolCommandMultipleAtFileFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	titlePath := filepath.Join(dir, "title.txt")
	bodyPath := filepath.Join(dir, "body.md")
	if err := os.WriteFile(titlePath, []byte("My Title"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(bodyPath, []byte("# Body\n\nContent here"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "create_document",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title": map[string]any{"type": "string"},
								"body":  map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"doc", "create_document", "--title", "@" + titlePath, "--body", "@" + bodyPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["title"] != "My Title" {
		t.Errorf("params[title] = %q, want %q", runner.last.Params["title"], "My Title")
	}
	if runner.last.Params["body"] != "# Body\n\nContent here" {
		t.Errorf("params[body] = %q, want %q", runner.last.Params["body"], "# Body\n\nContent here")
	}
}

func TestToolCommandAtFileMissingReturnsError(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@/nonexistent/file.txt"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail for missing @file")
	}
	if !strings.Contains(err.Error(), "--text") {
		t.Errorf("error should mention flag name, got: %v", err)
	}
	if runner.called != 0 {
		t.Error("runner should not be called on @file error")
	}
}

func TestToolCommandAtFileForJsonMissingReturnsError(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--json", "@/nonexistent/payload.json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail for missing @file on --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json, got: %v", err)
	}
}

func TestToolCommandAtFileUTF8ContentPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "chinese.txt")
	content := "你好世界 🌍\n第二行"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"chat", "send_message", "--text", "@" + filePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != content {
		t.Errorf("params[text] = %q, want %q", runner.last.Params["text"], content)
	}
}

func TestToolCommandPlainAtValueNotResolvedForNonStringFlags(t *testing.T) {
	t.Parallel()

	// Integer and boolean flags should NOT resolve @file syntax.
	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "todo",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "create_task",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title":    map[string]any{"type": "string"},
								"priority": map[string]any{"type": "integer"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"todo", "create_task", "--title", "test", "--priority", "3"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["priority"] != 3 {
		t.Errorf("params[priority] = %v, want 3", runner.last.Params["priority"])
	}
}

// ---------------------------------------------------------------------------
// Input source resolution: --json @file override priority
// ---------------------------------------------------------------------------

func TestToolCommandJsonFlagOverridesOverrideFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "base.json")
	if err := os.WriteFile(filePath, []byte(`{"text":"from-json","user_id":"json-user"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "chat",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "send_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text":    map[string]any{"type": "string"},
								"user_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	// --text override should win over --json base payload.
	cmd.SetArgs([]string{"chat", "send_message", "--json", "@" + filePath, "--text", "override"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["text"] != "override" {
		t.Errorf("params[text] = %q, want %q (override should win)", runner.last.Params["text"], "override")
	}
	if runner.last.Params["user_id"] != "json-user" {
		t.Errorf("params[user_id] = %q, want %q (from json base)", runner.last.Params["user_id"], "json-user")
	}
}

// ---------------------------------------------------------------------------
// Sensitive tool + stdin guard interaction
// ---------------------------------------------------------------------------

func TestSensitiveToolWithStdinClaimedRequiresYes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Sensitive tool + @file (does NOT claim stdin) → should still prompt.
	// We provide "yes" on stdin to pass confirmation.
	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.called != 1 {
		t.Errorf("runner called = %d, want 1", runner.called)
	}
}

func TestSensitiveToolDeniedOnStdinWithNoYesFlag(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() should fail when user denies confirmation")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancellation, got: %v", err)
	}
	if runner.called != 0 {
		t.Error("runner should not be called when confirmation denied")
	}
}

func TestSensitiveToolWithYesFlagSkipsConfirmation(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "doc",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:   "delete_document",
						Sensitive: true,
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"doc_id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.PersistentFlags().Bool("yes", false, "Skip confirmation")
	cmd.SetArgs([]string{"doc", "delete_document", "--doc-id", "DOC001", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.called != 1 {
		t.Errorf("runner called = %d, want 1", runner.called)
	}
}

// ---------------------------------------------------------------------------
// collectOverrides: @file does not affect non-string flag types
// ---------------------------------------------------------------------------

func TestCollectOverridesResolvesAtFileOnlyForStringKind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "name.txt")
	if err := os.WriteFile(filePath, []byte("resolved name"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &captureRunner{}
	cmd := newTestMCPCommand(t, ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID: "contact",
				Tools: []ir.ToolDescriptor{
					{
						RPCName: "search_user",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"keyword": map[string]any{"type": "string"},
								"active":  map[string]any{"type": "boolean"},
								"limit":   map[string]any{"type": "integer"},
							},
						},
					},
				},
			},
		},
	}, runner)

	cmd.SetArgs([]string{"contact", "search_user",
		"--keyword", "@" + filePath,
		"--active=true",
		"--limit", "10",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.last.Params["keyword"] != "resolved name" {
		t.Errorf("params[keyword] = %q, want %q", runner.last.Params["keyword"], "resolved name")
	}
	if runner.last.Params["active"] != true {
		t.Errorf("params[active] = %v, want true", runner.last.Params["active"])
	}
	if runner.last.Params["limit"] != 10 {
		t.Errorf("params[limit] = %v, want 10", runner.last.Params["limit"])
	}
}

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

func newTestMCPCommand(t *testing.T, catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	t.Helper()
	cmd := NewMCPCommand(context.Background(), StaticLoader{Catalog: catalog}, runner, nil)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	return cmd
}

type errorLoader struct {
	err error
}

func (l errorLoader) Load(context.Context) (ir.Catalog, error) {
	return ir.Catalog{}, l.err
}

type captureRunner struct {
	last   executor.Invocation
	called int
}

func (r *captureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	r.called++
	return executor.Result{Invocation: invocation}, nil
}
