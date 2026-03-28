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

package skillgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
)

func TestWriteArtifactsMaterializesFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacyPath := filepath.Join(root, "docs/generated/example.txt")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := WriteArtifacts(root, []Artifact{
		{Path: "skills/generated/docs/example.txt", Content: []byte("example\n")},
	}); err != nil {
		t.Fatalf("WriteArtifacts() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "skills/generated/docs/example.txt")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy docs/generated artifact still exists: %v", err)
	}
}

func TestUniqueSkillProducts(t *testing.T) {
	t.Parallel()

	got := uniqueSkillProducts([]string{"doc", "drive", "doc", " DRIVE ", ""})
	want := []string{"doc", "drive"}
	if len(got) != len(want) {
		t.Fatalf("uniqueSkillProducts() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniqueSkillProducts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFinalizeMarkdownNormalizesTrailingWhitespace(t *testing.T) {
	t.Parallel()

	input := "line one  \r\nline two\t \n\n"
	got := finalizeMarkdown(input)
	want := "line one\nline two\n"
	if got != want {
		t.Fatalf("finalizeMarkdown() = %q, want %q", got, want)
	}
}

func TestGenerateUsesFlattenedSkillsPaths(t *testing.T) {
	t.Parallel()

	artifacts, err := Generate(ir.Catalog{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	for _, want := range []string{
		"skills/index.md",
		"skills/generated/dws-shared/SKILL.md",
	} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("Generate() missing artifact %q", want)
		}
	}

	for _, oldPath := range []string{
		"skills/generated/apis.md",
		"skills/dws/generated/apis.md",
		"skills/dws/generated/dws-shared/SKILL.md",
		"skills/generated/canonical-surface/SKILL.md",
		"skills/generated/canonical-surface/api.md",
		"skills/generated/dws-shared/api.md",
	} {
		if _, ok := paths[oldPath]; ok {
			t.Fatalf("Generate() still emitted legacy artifact %q", oldPath)
		}
	}

	readme, ok := paths["skills/generated/docs/README.md"]
	if !ok {
		t.Fatal("Generate() missing skills/generated/docs/README.md")
	}
	if !strings.Contains(readme, "`skills/index.md`") {
		t.Fatalf("skills/generated/docs/README.md missing flattened skills path:\n%s", readme)
	}
	if strings.Contains(readme, "`skills/dws/generated/apis.md`") {
		t.Fatalf("skills/generated/docs/README.md still references legacy skills path:\n%s", readme)
	}

	for path := range paths {
		if strings.HasPrefix(path, "docs/generated/") {
			t.Fatalf("Generate() still emitted legacy docs artifact %q", path)
		}
	}
}

func TestGenerateOmitsPersonaAndRecipeArtifacts(t *testing.T) {
	t.Parallel()

	artifacts, err := Generate(ir.Catalog{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
		if strings.Contains(artifact.Path, "skills/generated/persona-") {
			t.Fatalf("Generate() emitted persona artifact %q", artifact.Path)
		}
		if strings.Contains(artifact.Path, "skills/generated/recipe-") {
			t.Fatalf("Generate() emitted recipe artifact %q", artifact.Path)
		}
	}

	index, ok := paths["skills/index.md"]
	if !ok {
		t.Fatal("Generate() missing skills/index.md")
	}
	for _, snippet := range []string{
		"## Personas",
		"## Recipes",
		"## Helpers",
		"Tool-level execution skills.",
		"| [persona-",
		"| [recipe-",
	} {
		if strings.Contains(index, snippet) {
			t.Fatalf("skills/index.md still contains %q:\n%s", snippet, index)
		}
	}
}

func TestGenerateEmitsUsableSkillFiles(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "contact",
				DisplayName:               "钉钉通讯录",
				Description:               "搜索和查询组织成员",
				Endpoint:                  "https://example.invalid/contact",
				NegotiatedProtocolVersion: "2025-03-26",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_user_by_keyword",
						CLIName:       "search-user",
						Title:         "搜索成员",
						Description:   "按关键词搜索组织成员",
						CanonicalPath: "contact.search_user_by_keyword",
						InputSchema: map[string]any{
							"type":     "object",
							"required": []any{"keyword"},
							"properties": map[string]any{
								"keyword": map[string]any{
									"type":        "string",
									"description": "搜索关键词",
								},
							},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	for _, want := range []string{
		"skills/generated/dws-shared/SKILL.md",
		"skills/generated/dws-contact/SKILL.md",
		"skills/generated/dws-contact/search-user.md",
		"skills/generated/docs/cli/contact.md",
		"skills/generated/docs/schema/contact/search_user_by_keyword.json",
	} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("Generate() missing usable skill artifact %q", want)
		}
	}

	helperSkill := paths["skills/generated/dws-contact/search-user.md"]
	for _, snippet := range []string{
		"name: dws-contact-search-user",
		"metadata:",
		"openclaw:",
		"bins:",
		"- dws",
		"../dws-shared/SKILL.md",
		"./SKILL.md",
	} {
		if !strings.Contains(helperSkill, snippet) {
			t.Fatalf("helper skill missing %q:\n%s", snippet, helperSkill)
		}
	}

	index, ok := paths["skills/index.md"]
	if !ok {
		t.Fatal("Generate() missing skills/index.md")
	}
	if !strings.Contains(index, topLevelDWSDescription) {
		t.Fatalf("skills/index.md missing top-level dws description:\n%s", index)
	}
	for _, forbidden := range []string{
		"## Helpers",
		"Tool-level execution skills.",
		"| [dws-contact-search-user]",
	} {
		if strings.Contains(index, forbidden) {
			t.Fatalf("skills/index.md unexpectedly contains %q:\n%s", forbidden, index)
		}
	}

	productDoc := paths["skills/generated/docs/cli/contact.md"]
	if !strings.Contains(productDoc, "`skills/generated/docs/schema/contact/search_user_by_keyword.json`") {
		t.Fatalf("product cli doc missing migrated schema reference:\n%s", productDoc)
	}

	for _, oldPath := range []string{
		"skills/generated/canonical-surface/SKILL.md",
		"skills/generated/canonical-surface/api.md",
		"skills/generated/dws-shared/api.md",
		"skills/generated/dws-contact/api.md",
		"skills/generated/dws-contact-search-user/SKILL.md",
		"skills/generated/dws-contact-search-user/api.md",
		"docs/generated/README.md",
		"docs/generated/cli/contact.md",
		"docs/generated/schema/contact.search_user_by_keyword.json",
	} {
		if _, ok := paths[oldPath]; ok {
			t.Fatalf("Generate() still emitted deprecated artifact %q", oldPath)
		}
	}
}

func TestGeneratePrefersServiceDescriptionForServiceSkillFrontmatter(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                 "aitable",
				DisplayName:        "钉钉 AI 表格",
				Description:        "AI 表格操作",
				ServiceDescription: "钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段，快速打通查询、维护与自动化办公流程。",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_base",
						CLIName:       "create",
						Group:         "base",
						Title:         "创建 AI 表格",
						Description:   "创建一个新的 AI 表格 Base",
						CanonicalPath: "aitable.create_base",
						InputSchema:   map[string]any{"type": "object"},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	serviceSkill, ok := paths["skills/generated/dws-aitable/SKILL.md"]
	if !ok {
		t.Fatal("Generate() missing aitable service skill")
	}
	if !strings.Contains(serviceSkill, `description: "钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段`) {
		t.Fatalf("service skill frontmatter did not use service description:\n%s", serviceSkill)
	}
	if !strings.Contains(serviceSkill, "- Description: 钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段，快速打通查询、维护与自动化办公流程。") {
		t.Fatalf("service skill body missing service description:\n%s", serviceSkill)
	}

	index, ok := paths["skills/index.md"]
	if !ok {
		t.Fatal("Generate() missing skills/index.md")
	}
	if !strings.Contains(index, "钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段") {
		t.Fatalf("skills index did not use service description:\n%s", index)
	}
}

func TestGenerateOmitsHiddenFlagsFromHelperSkillDocs(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:          "contact",
				DisplayName: "钉钉通讯录",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_user_by_keyword",
						CLIName:       "search-user",
						Title:         "搜索成员",
						Description:   "按关键词搜索组织成员",
						CanonicalPath: "contact.search_user_by_keyword",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"keyword": map[string]any{"type": "string"},
								"debug":   map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"debug": {Hidden: true},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	helper, ok := paths["skills/generated/dws-contact/search-user.md"]
	if !ok {
		t.Fatal("Generate() missing helper skill")
	}
	if strings.Contains(helper, "--debug") {
		t.Fatalf("helper skill unexpectedly documented hidden flag:\n%s", helper)
	}
}

func TestGenerateUsesGroupedCLIPathsForAITableSkills(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "aitable",
				DisplayName:               "钉钉 AI 表格",
				Description:               "AI 表格操作",
				Endpoint:                  "https://example.invalid/aitable",
				NegotiatedProtocolVersion: "2025-03-26",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "create_base",
						CLIName:       "create",
						Group:         "base",
						Title:         "创建 Base",
						Description:   "创建 AI 表格 Base",
						CanonicalPath: "aitable.create_base",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"baseName": map[string]any{
									"type": "string",
								},
								"templateId": map[string]any{
									"type": "string",
								},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"baseName":   {Alias: "name"},
							"templateId": {Alias: "template-id"},
						},
					},
					{
						RPCName:       "create_table",
						CLIName:       "create",
						Group:         "table",
						Title:         "创建数据表",
						Description:   "创建 AI 表格数据表",
						CanonicalPath: "aitable.create_table",
						InputSchema: map[string]any{
							"type": "object",
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	serviceSkill, ok := paths["skills/generated/dws-aitable/SKILL.md"]
	if !ok {
		t.Fatal("Generate() missing aitable service skill")
	}
	for _, snippet := range []string{
		"[`dws-aitable-base-create`](./base/create.md)",
		"[`dws-aitable-table-create`](./table/create.md)",
	} {
		if !strings.Contains(serviceSkill, snippet) {
			t.Fatalf("aitable service skill missing %q:\n%s", snippet, serviceSkill)
		}
	}
	for _, forbidden := range []string{
		"dws-aitable-create-2",
		"](./create.md)",
	} {
		if strings.Contains(serviceSkill, forbidden) {
			t.Fatalf("aitable service skill unexpectedly contains %q:\n%s", forbidden, serviceSkill)
		}
	}

	baseHelper, ok := paths["skills/generated/dws-aitable/base/create.md"]
	if !ok {
		t.Fatal("Generate() missing aitable base helper skill")
	}
	for _, snippet := range []string{
		"name: dws-aitable-base-create",
		"cliHelp: \"dws aitable base create --help\"",
		"dws aitable base create --json '{...}'",
		"../../dws-shared/SKILL.md",
		"../SKILL.md",
		"| `--name` |",
	} {
		if !strings.Contains(baseHelper, snippet) {
			t.Fatalf("aitable base helper missing %q:\n%s", snippet, baseHelper)
		}
	}
	for _, forbidden := range []string{
		"dws schema aitable.create_base --json",
	} {
		if strings.Contains(baseHelper, forbidden) {
			t.Fatalf("aitable base helper unexpectedly contains %q:\n%s", forbidden, baseHelper)
		}
	}
}

func TestGenerateFlattensNestedWrapperSchemaArtifacts(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "group-chat",
				DisplayName:               "钉钉群聊",
				Description:               "钉钉群聊",
				Endpoint:                  "https://example.invalid/group-chat",
				NegotiatedProtocolVersion: "2025-03-26",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_groups_by_keyword",
						CLIName:       "search",
						Title:         "搜索群聊",
						Description:   "根据关键词搜索群聊",
						CanonicalPath: "group-chat.search_groups_by_keyword",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"OpenSearchRequest": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"cursor": map[string]any{"type": "string"},
										"query":  map[string]any{"type": "string"},
									},
									"required": []any{"query"},
								},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"OpenSearchRequest.cursor": {Alias: "cursor"},
							"OpenSearchRequest.query":  {Alias: "query"},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	schemaDoc, ok := paths["skills/generated/docs/schema/group-chat.search_groups_by_keyword.json"]
	if !ok {
		t.Fatal("Generate() missing flattened group-chat schema artifact")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(schemaDoc), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	required, _ := payload["required"].([]any)
	if len(required) != 1 || required[0] != "OpenSearchRequest.query" {
		t.Fatalf("generated required = %#v, want [OpenSearchRequest.query]", required)
	}

	flags, _ := payload["flags"].([]any)
	if len(flags) != 2 {
		t.Fatalf("generated flags len = %d, want 2", len(flags))
	}
	if !containsGeneratedFlag(flags, "OpenSearchRequest.cursor", "cursor") {
		t.Fatalf("generated flags missing cursor flattening: %#v", flags)
	}
	if !containsGeneratedFlag(flags, "OpenSearchRequest.query", "query") {
		t.Fatalf("generated flags missing query flattening: %#v", flags)
	}

	flagHints, _ := payload["flag_hints"].(map[string]any)
	queryHint, _ := flagHints["OpenSearchRequest.query"].(map[string]any)
	if alias, _ := queryHint["alias"].(string); alias != "query" {
		t.Fatalf("generated query alias = %q, want query", alias)
	}
	if requiredValue, _ := queryHint["required"].(bool); !requiredValue {
		t.Fatalf("generated query hint required = %#v, want true", queryHint["required"])
	}

	helperSkill, ok := paths["skills/generated/dws-group-chat/search.md"]
	if !ok {
		t.Fatal("Generate() missing flattened group-chat helper skill")
	}
	for _, snippet := range []string{
		"| `--cursor` |",
		"| `--query` | ✓ |",
		"- `OpenSearchRequest.query`",
	} {
		if !strings.Contains(helperSkill, snippet) {
			t.Fatalf("group-chat helper missing %q:\n%s", snippet, helperSkill)
		}
	}
	for _, forbidden := range []string{
		"`--OpenSearchRequest`",
		"- `OpenSearchRequest`",
	} {
		if strings.Contains(helperSkill, forbidden) {
			t.Fatalf("group-chat helper unexpectedly contains %q:\n%s", forbidden, helperSkill)
		}
	}
}

func containsGeneratedFlag(flags []any, propertyName, flagName string) bool {
	for _, entry := range flags {
		flag, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if flag["property_name"] == propertyName && flag["flag_name"] == flagName {
			return true
		}
	}
	return false
}

func TestGenerateCatalogSnapshotCarriesFlattenedToolSurface(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "group-chat",
				DisplayName:               "钉钉群聊",
				Description:               "钉钉群聊",
				ServerKey:                 "group-chat-server",
				Endpoint:                  "https://example.invalid/group-chat",
				NegotiatedProtocolVersion: "2025-03-26",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_groups_by_keyword",
						CLIName:       "search",
						Title:         "搜索群聊",
						Description:   "根据关键词搜索群聊",
						CanonicalPath: "group-chat.search_groups_by_keyword",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"OpenSearchRequest": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"cursor": map[string]any{"type": "string"},
										"query":  map[string]any{"type": "string"},
									},
									"required": []any{"query"},
								},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"OpenSearchRequest.cursor": {Alias: "cursor"},
							"OpenSearchRequest.query":  {Alias: "query"},
						},
						SourceServerKey: "group-chat-server",
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	catalogDoc, ok := paths["skills/generated/docs/schema/catalog.json"]
	if !ok {
		t.Fatal("Generate() missing catalog snapshot artifact")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(catalogDoc), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	products, _ := payload["products"].([]any)
	if len(products) != 1 {
		t.Fatalf("catalog products len = %d, want 1", len(products))
	}
	product, _ := products[0].(map[string]any)
	tools, _ := product["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("catalog tools len = %d, want 1", len(tools))
	}
	tool, _ := tools[0].(map[string]any)

	cliPath, _ := tool["cli_path"].([]any)
	if len(cliPath) != 2 || cliPath[0] != "chat" || cliPath[1] != "search" {
		t.Fatalf("catalog cli_path = %#v, want [chat search]", cliPath)
	}

	required, _ := tool["required"].([]any)
	if len(required) != 1 || required[0] != "OpenSearchRequest.query" {
		t.Fatalf("catalog required = %#v, want [OpenSearchRequest.query]", required)
	}

	flags, _ := tool["flags"].([]any)
	if len(flags) != 2 {
		t.Fatalf("catalog flags len = %d, want 2", len(flags))
	}
	if !containsGeneratedFlag(flags, "OpenSearchRequest.cursor", "cursor") {
		t.Fatalf("catalog flags missing cursor flattening: %#v", flags)
	}
	if !containsGeneratedFlag(flags, "OpenSearchRequest.query", "query") {
		t.Fatalf("catalog flags missing query flattening: %#v", flags)
	}

	flagHints, _ := tool["flag_hints"].(map[string]any)
	queryHint, _ := flagHints["OpenSearchRequest.query"].(map[string]any)
	if alias, _ := queryHint["alias"].(string); alias != "query" {
		t.Fatalf("catalog query alias = %q, want query", alias)
	}
	if requiredValue, _ := queryHint["required"].(bool); !requiredValue {
		t.Fatalf("catalog query hint required = %#v, want true", queryHint["required"])
	}
}

func TestGenerateUsesProductCLICommandForSkillRoutes(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "bot",
				DisplayName:               "机器人消息",
				Description:               "机器人消息",
				Endpoint:                  "https://example.invalid/bot",
				NegotiatedProtocolVersion: "2025-03-26",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_groups_by_keyword",
						CLIName:       "search_groups_by_keyword",
						Title:         "搜索群聊",
						Description:   "根据关键词搜索群聊",
						CanonicalPath: "bot.search_groups_by_keyword",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"keyword": map[string]any{
									"type": "string",
								},
							},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	serviceSkill, ok := paths["skills/generated/dws-bot/SKILL.md"]
	if !ok {
		t.Fatal("Generate() missing bot service skill")
	}
	for _, snippet := range []string{
		`cliHelp: "dws chat --help"`,
		"dws chat <command> --json '{...}'",
		"- CLI route: `dws chat search_groups_by_keyword`",
	} {
		if !strings.Contains(serviceSkill, snippet) {
			t.Fatalf("bot service skill missing %q:\n%s", snippet, serviceSkill)
		}
	}
	for _, forbidden := range []string{
		`cliHelp: "dws bot --help"`,
		"dws bot <command> --json '{...}'",
		"- CLI route: `dws bot search_groups_by_keyword`",
	} {
		if strings.Contains(serviceSkill, forbidden) {
			t.Fatalf("bot service skill unexpectedly contains %q:\n%s", forbidden, serviceSkill)
		}
	}

	helperSkill, ok := paths["skills/generated/dws-bot/search-groups-by-keyword.md"]
	if !ok {
		t.Fatal("Generate() missing bot helper skill")
	}
	for _, snippet := range []string{
		`cliHelp: "dws chat search_groups_by_keyword --help"`,
		"dws chat search_groups_by_keyword --json '{...}'",
	} {
		if !strings.Contains(helperSkill, snippet) {
			t.Fatalf("bot helper skill missing %q:\n%s", snippet, helperSkill)
		}
	}
	for _, forbidden := range []string{
		`cliHelp: "dws bot search_groups_by_keyword --help"`,
		"dws bot search_groups_by_keyword --json '{...}'",
	} {
		if strings.Contains(helperSkill, forbidden) {
			t.Fatalf("bot helper skill unexpectedly contains %q:\n%s", forbidden, helperSkill)
		}
	}

	productDoc, ok := paths["skills/generated/docs/cli/bot.md"]
	if !ok {
		t.Fatal("Generate() missing bot CLI doc")
	}
	if !strings.Contains(productDoc, "  - CLI route: `dws chat search_groups_by_keyword`") {
		t.Fatalf("bot CLI doc missing routed command path:\n%s", productDoc)
	}
	if strings.Contains(productDoc, "  - CLI route: `dws bot search_groups_by_keyword`") {
		t.Fatalf("bot CLI doc unexpectedly contains product id route:\n%s", productDoc)
	}
}

func TestGenerateProductCLIDocPrefersPublicRouteAndFlagNames(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "bot",
				DisplayName:               "机器人消息",
				Description:               "机器人消息",
				Endpoint:                  "https://example.invalid/bot",
				NegotiatedProtocolVersion: "2025-03-26",
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "search_my_robots",
						CLIName:       "search",
						Group:         "bot",
						Title:         "搜索我创建的企业机器人",
						Description:   "搜索我创建的机器人，可获取机器人robotCode等信息。",
						CanonicalPath: "bot.search_my_robots",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"currentPage": map[string]any{"type": "number"},
								"pageSize":    map[string]any{"type": "number"},
								"robotName":   map[string]any{"type": "string"},
							},
						},
						FlagHints: map[string]ir.CLIFlagHint{
							"currentPage": {Alias: "page"},
							"pageSize":    {Alias: "size"},
							"robotName":   {Alias: "name"},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	productDoc, ok := paths["skills/generated/docs/cli/bot.md"]
	if !ok {
		t.Fatal("Generate() missing bot CLI doc")
	}
	for _, snippet := range []string{
		"- `bot search`",
		"  - CLI route: `dws bot bot search`",
		"  - Flags: `--page`, `--size`, `--name`",
	} {
		if !strings.Contains(productDoc, snippet) {
			t.Fatalf("bot CLI doc missing %q:\n%s", snippet, productDoc)
		}
	}
	for _, forbidden := range []string{
		"- `search_my_robots`",
		"`--currentPage`",
		"`--pageSize`",
		"`--robotName`",
		"/ `--page`",
	} {
		if strings.Contains(productDoc, forbidden) {
			t.Fatalf("bot CLI doc unexpectedly contains %q:\n%s", forbidden, productDoc)
		}
	}
}

func TestGenerateDisambiguatesDuplicateToolRoutesInHelperArtifacts(t *testing.T) {
	t.Parallel()

	catalog := ir.Catalog{
		Products: []ir.CanonicalProduct{
			{
				ID:                        "bot",
				DisplayName:               "机器人消息",
				Description:               "机器人消息",
				Endpoint:                  "https://example.invalid/bot",
				NegotiatedProtocolVersion: "2025-03-26",
				CLI: &ir.ProductCLIMetadata{
					Command: "chat",
				},
				Tools: []ir.ToolDescriptor{
					{
						RPCName:       "batch_send_robot_msg_to_users",
						CLIName:       "send-by-bot",
						Group:         "message",
						Title:         "单聊发送",
						Description:   "批量发送单聊消息",
						CanonicalPath: "bot.batch_send_robot_msg_to_users",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"userIds": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
					{
						RPCName:       "send_robot_group_message",
						CLIName:       "send-by-bot",
						Group:         "message",
						Title:         "群聊发送",
						Description:   "发送群聊消息",
						CanonicalPath: "bot.send_robot_group_message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"openConversationId": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}

	artifacts, err := Generate(catalog)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	paths := map[string]string{}
	for _, artifact := range artifacts {
		paths[artifact.Path] = string(artifact.Content)
	}

	serviceSkill := paths["skills/generated/dws-bot/SKILL.md"]
	for _, snippet := range []string{
		"[`dws-bot-message-send-by-bot`](./message/send-by-bot.md)",
		"[`dws-bot-message-send-robot-group-message`](./message/send-robot-group-message.md)",
		"- CLI route: `dws chat message send_robot_group_message`",
	} {
		if !strings.Contains(serviceSkill, snippet) {
			t.Fatalf("bot service skill missing %q:\n%s", snippet, serviceSkill)
		}
	}

	groupHelper, ok := paths["skills/generated/dws-bot/message/send-robot-group-message.md"]
	if !ok {
		t.Fatal("Generate() missing disambiguated group-message helper skill")
	}
	for _, snippet := range []string{
		"name: dws-bot-message-send-robot-group-message",
		`cliHelp: "dws chat message send_robot_group_message --help"`,
		"dws chat message send_robot_group_message --json '{...}'",
	} {
		if !strings.Contains(groupHelper, snippet) {
			t.Fatalf("group helper missing %q:\n%s", snippet, groupHelper)
		}
	}
	for _, forbidden := range []string{
		`cliHelp: "dws chat message send-by-bot --help"`,
		"dws chat message send-by-bot --json '{...}'",
	} {
		if strings.Contains(groupHelper, forbidden) {
			t.Fatalf("group helper unexpectedly contains %q:\n%s", forbidden, groupHelper)
		}
	}
}
