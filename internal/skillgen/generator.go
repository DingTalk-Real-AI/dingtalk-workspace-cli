package skillgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

type Artifact struct {
	Path    string
	Content []byte
}

type skillIndexEntry struct {
	Name        string
	Description string
	Category    string
	Path        string
}

type helperSkillRef struct {
	Name string
	Path string
	Tool ir.ToolDescriptor
}

type frontmatterSpec struct {
	Name           string
	Description    string
	Category       string
	CLIHelp        string
	Domain         string
	RequiresSkills []string
}

const (
	skillVersion                = "1.1.0"
	skillCategoryService        = "service"
	skillCategoryHelper         = "helper"
	frontmatterDescriptionLimit = 120
	topLevelDWSDescription      = "管理钉钉产品能力(AI表格/日历/通讯录/文档/机器人/待办/邮箱/听记/AI应用/审批/日志/钉盘等)。当用户需要操作表格数据、管理日程会议、查询通讯录、发送消息通知、处理审批流程、查看听记摘要、创建应用/系统/管理后台/业务工具、查看或提交日报周报（钉钉日志模版）、管理钉盘文件时使用、钉钉技能市场搜索及下载。"
	generatedDocsRoot           = "skills/generated/docs"
	generatedDocsCLIRoot        = generatedDocsRoot + "/cli"
	generatedDocsSchemaRoot     = generatedDocsRoot + "/schema"
	generatedDocsReadmePath     = generatedDocsRoot + "/README.md"
	generatedDocsCoveragePath   = generatedDocsRoot + "/skills-coverage.md"
	legacyGeneratedDocsRoot     = "docs/generated"
)

var writeOperationTokens = map[string]struct{}{
	"add":      {},
	"append":   {},
	"approve":  {},
	"batch":    {},
	"commit":   {},
	"create":   {},
	"delete":   {},
	"done":     {},
	"insert":   {},
	"issue":    {},
	"mkdir":    {},
	"modify":   {},
	"patch":    {},
	"reject":   {},
	"remove":   {},
	"replace":  {},
	"revoke":   {},
	"send":     {},
	"submit":   {},
	"sync":     {},
	"transfer": {},
	"update":   {},
	"upload":   {},
	"write":    {},
}

func Generate(catalog ir.Catalog) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(catalog.Products)+12)

	catalogJSON, err := marshalJSON(CatalogSnapshotPayload(catalog))
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{
		Path:    generatedDocsPath("schema", "catalog.json"),
		Content: catalogJSON,
	})

	for _, product := range catalog.Products {
		for _, tool := range product.Tools {
			payload := generatedToolSchemaPayload(product, tool)
			data, err := marshalJSON(payload)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, Artifact{
				Path:    generatedDocsPath("schema", tool.CanonicalPath+".json"),
				Content: data,
			})
			artifacts = append(artifacts, Artifact{
				Path:    generatedDocsPath("schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json"),
				Content: data,
			})
		}
	}

	cliDoc, err := renderCanonicalCLI(catalog)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{
		Path:    generatedDocsPath("cli", "canonical-cli.md"),
		Content: []byte(cliDoc),
	})
	for _, product := range catalog.Products {
		artifacts = append(artifacts, Artifact{
			Path:    generatedDocsPath("cli", safeDocSegment(product.ID)+".md"),
			Content: []byte(renderProductCLI(product)),
		})
	}

	readme := strings.Join([]string{
		"# Generated Docs",
		"",
		"These artifacts are generated from the shared canonical Tool IR. Do not edit them by hand.",
		"",
		"- `skills/generated/docs/cli/canonical-cli.md`: canonical command surface summary",
		"- `skills/generated/docs/cli/<product>.md`: per-product canonical command summary",
		"- `skills/generated/docs/schema/catalog.json`: full catalog snapshot",
		"- `skills/generated/docs/schema/<product>.<tool>.json`: per-tool schema payloads",
		"- `skills/generated/docs/schema/<product>/<tool>.json`: per-tool schema payloads in hierarchical layout",
		"- `skills/index.md`: top-level skills index for generated docs navigation",
		"- `skills/generated/docs/skills-coverage.md`: coverage report against legacy17/extended22 targets",
		"",
	}, "\n")
	artifacts = append(artifacts, Artifact{
		Path:    generatedDocsReadmePath,
		Content: []byte(readme),
	})

	indexEntries := make([]skillIndexEntry, 0, len(catalog.Products)*4)

	sharedPath := filepath.ToSlash("skills/generated/dws-shared/SKILL.md")
	artifacts = append(artifacts, Artifact{
		Path:    sharedPath,
		Content: []byte(renderSharedSkill(catalog)),
	})
	indexEntries = append(indexEntries, skillIndexEntry{
		Name:        "dws-shared",
		Description: truncateSkillDescription("DWS shared reference for authentication, command patterns, and safety rules."),
		Category:    skillCategoryService,
		Path:        sharedPath,
	})
	helperCountsByService := map[string]int{}

	for _, product := range catalog.Products {
		helperRefs := make([]helperSkillRef, 0, len(product.Tools))
		helperNames := map[string]int{}
		serviceName := "dws-" + safeSkillSegment(product.ID)
		serviceDir := filepath.ToSlash(filepath.Join("skills/generated", serviceName))

		for _, tool := range product.Tools {
			if isBlockedTool(product, tool) {
				continue
			}
			routeSegments := skillRouteSegments(product, tool)
			helperName := helperSkillName(product, tool, routeSegments, helperNames)
			toolSlug := skillToolSlug(tool, routeSegments)
			helperDir := serviceDir
			if len(routeSegments) > 1 {
				// Nest helper skill files under the resolved CLI route hierarchy,
				// so disambiguated tool routes get distinct files.
				for _, segment := range routeSegments[:len(routeSegments)-1] {
					helperDir = filepath.ToSlash(filepath.Join(helperDir, safeSkillSegment(segment)))
				}
			}
			helperPath := filepath.ToSlash(filepath.Join(helperDir, toolSlug+".md"))
			helperRefs = append(helperRefs, helperSkillRef{
				Name: helperName,
				Path: helperPath,
				Tool: tool,
			})
			helperMD := renderHelperSkill(product, tool, helperName, helperPath)
			artifacts = append(artifacts, Artifact{Path: helperPath, Content: []byte(helperMD)})
			indexEntries = append(indexEntries, skillIndexEntry{
				Name:        helperName,
				Description: truncateSkillDescription(helperDescription(product, tool)),
				Category:    skillCategoryHelper,
				Path:        helperPath,
			})
		}

		servicePath := filepath.ToSlash(filepath.Join(serviceDir, "SKILL.md"))
		serviceMD := renderServiceSkill(product, helperRefs)
		artifacts = append(artifacts, Artifact{Path: servicePath, Content: []byte(serviceMD)})
		indexEntries = append(indexEntries, skillIndexEntry{
			Name:        serviceName,
			Description: truncateSkillDescription(serviceDescription(product)),
			Category:    skillCategoryService,
			Path:        servicePath,
		})
		helperCountsByService[safeSkillSegment(product.ID)] = len(helperRefs)
	}

	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("skills/index.md"),
		Content: []byte(renderSkillsIndex(indexEntries, "skills/")),
	})
	artifacts = append(artifacts, Artifact{
		Path:    generatedDocsCoveragePath,
		Content: []byte(renderSkillsCoverageReport(catalog, helperCountsByService)),
	})

	slices.SortFunc(artifacts, func(left, right Artifact) int {
		return strings.Compare(left.Path, right.Path)
	})
	return artifacts, nil
}

func renderCanonicalCLI(catalog ir.Catalog) (string, error) {
	var builder strings.Builder
	builder.WriteString("# Canonical CLI Surface\n\n")
	builder.WriteString("Generated from the shared Tool IR. Do not edit by hand.\n\n")
	builder.WriteString("## Command Pattern\n\n")
	builder.WriteString("- `dws <product> <tool> --json '{...}'`\n")
	builder.WriteString("- `dws schema <product>.<tool>`\n\n")
	builder.WriteString("## Products\n\n")

	for _, product := range catalog.Products {
		builder.WriteString(fmt.Sprintf("### `%s`\n\n", product.ID))
		builder.WriteString(fmt.Sprintf("- Display name: %s\n", safeValue(product.DisplayName, product.ID)))
		if description := strings.TrimSpace(productServiceDescription(product)); description != "" {
			builder.WriteString(fmt.Sprintf("- Description: %s\n", description))
		}
		builder.WriteString(fmt.Sprintf("- Server key: `%s`\n", product.ServerKey))
		builder.WriteString(fmt.Sprintf("- Protocol: `%s`\n", safeValue(product.NegotiatedProtocolVersion, "unknown")))
		builder.WriteString(fmt.Sprintf("- Degraded: `%t`\n", product.Degraded))
		builder.WriteString("- Tools:\n")
		for _, tool := range product.Tools {
			flags := renderFlags(cli.VisibleFlagSpecs(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints)))
			builder.WriteString(fmt.Sprintf("  - `%s`: %s\n", tool.CanonicalPath, safeValue(tool.Description, tool.Title)))
			builder.WriteString(fmt.Sprintf("    CLI route: `%s`\n", cliInvocation(product, tool)))
			builder.WriteString(fmt.Sprintf("    Flags: %s\n", flags))
			builder.WriteString(fmt.Sprintf("    Schema: `%s`\n", generatedDocsPath("schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json")))
		}
		builder.WriteString("\n")
	}

	return finalizeMarkdown(builder.String()), nil
}

func renderSharedSkill(catalog ir.Catalog) string {
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        "dws-shared",
		Description: "DWS shared reference for authentication, command patterns, and safety rules.",
		Category:    "productivity",
	}))
	builder.WriteString("# dws - Shared Reference\n\n")
	builder.WriteString("## Installation\n\n")
	builder.WriteString("Ensure `dws` is installed and accessible from `$PATH`.\n\n")
	builder.WriteString("## Authentication\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("dws auth login\n")
	builder.WriteString("dws auth status\n")
	builder.WriteString("```\n\n")
	builder.WriteString("## Global Flags\n\n")
	builder.WriteString("| Flag | Description |\n")
	builder.WriteString("|------|-------------|\n")
	builder.WriteString("| `--format <FORMAT>` | Output format: `json`, `table`, `raw` |\n")
	builder.WriteString("| `--dry-run` | Preview the operation without executing it |\n")
	builder.WriteString("| `--verbose` | Show verbose logs |\n")
	builder.WriteString("| `--yes` | Skip confirmation prompts for sensitive operations |\n\n")
	builder.WriteString("## Global Rules\n\n")
	builder.WriteString("- Output defaults to JSON. Use `--format table` for human-readable output.\n")
	builder.WriteString("- Confirm with user before any write/delete/revoke action.\n")
	builder.WriteString("- Never fabricate IDs; always extract from command output.\n")
	builder.WriteString("- For risky operations, run a read/list check before executing write operations.\n\n")
	builder.WriteString("## Command Pattern\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("dws <product> <tool> --json '{...}'\n")
	builder.WriteString("dws schema <product>\n")
	builder.WriteString("dws schema <product>.<tool>\n")
	builder.WriteString("```\n\n")
	builder.WriteString("## Services\n\n")
	for _, product := range catalog.Products {
		builder.WriteString(fmt.Sprintf("- `dws-%s`\n", safeSkillSegment(product.ID)))
	}
	builder.WriteString("\n")
	return finalizeMarkdown(builder.String())
}

func renderServiceSkill(product ir.CanonicalProduct, helpers []helperSkillRef) string {
	serviceName := "dws-" + safeSkillSegment(product.ID)
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        serviceName,
		Description: serviceDescription(product),
		Category:    "productivity",
		CLIHelp:     fmt.Sprintf("dws %s --help", cliProductCommand(product)),
	}))
	builder.WriteString(fmt.Sprintf("# %s\n\n", product.ID))
	builder.WriteString("> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.\n\n")
	builder.WriteString(fmt.Sprintf("- Display name: %s\n", safeValue(product.DisplayName, product.ID)))
	if description := strings.TrimSpace(productServiceDescription(product)); description != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", description))
	}
	builder.WriteString(fmt.Sprintf("- Endpoint: `%s`\n", safeValue(product.Endpoint, "unknown")))
	builder.WriteString(fmt.Sprintf("- Protocol: `%s`\n", safeValue(product.NegotiatedProtocolVersion, "unknown")))
	builder.WriteString(fmt.Sprintf("- Degraded: `%t`\n\n", product.Degraded))

	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("%s --json '{...}'\n", cliCommandPattern(product, helpers)))
	builder.WriteString("```\n\n")

	if len(helpers) > 0 {
		builder.WriteString("## Helper Commands\n\n")
		builder.WriteString("| Command | Tool | Description |\n")
		builder.WriteString("|---------|------|-------------|\n")
		for _, helper := range helpers {
			builder.WriteString(fmt.Sprintf("| [`%s`](%s) | `%s` | %s |\n",
				helper.Name,
				relativeMarkdownPath(filepath.ToSlash(filepath.Join("skills/generated", serviceName, "SKILL.md")), helper.Path),
				helper.Tool.RPCName,
				safeValue(helper.Tool.Description, helper.Tool.Title),
			))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## API Tools\n\n")
	if len(helpers) == 0 {
		builder.WriteString("No tools available.\n\n")
	} else {
		for _, helper := range helpers {
			required := toolRequiredFields(helper.Tool)
			requiredText := "none"
			if len(required) > 0 {
				requiredText = strings.Join(wrapTicks(required), ", ")
			}
			builder.WriteString(fmt.Sprintf("### `%s`\n\n", helper.Tool.RPCName))
			builder.WriteString(fmt.Sprintf("- Canonical path: `%s`\n", helper.Tool.CanonicalPath))
			builder.WriteString(fmt.Sprintf("- CLI route: `%s`\n", cliInvocation(product, helper.Tool)))
			builder.WriteString(fmt.Sprintf("- Description: %s\n", safeValue(helper.Tool.Description, helper.Tool.Title)))
			builder.WriteString(fmt.Sprintf("- Required fields: %s\n", requiredText))
			builder.WriteString(fmt.Sprintf("- Sensitive: `%t`\n\n", helper.Tool.Sensitive))
		}
	}

	builder.WriteString("## Discovering Commands\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("dws schema                       # list available products (JSON)\ndws schema %s                     # inspect product tools (JSON)\ndws schema %s.<tool>              # inspect tool schema (JSON)\n", product.ID, product.ID))
	builder.WriteString("```\n")
	return finalizeMarkdown(builder.String())
}

func renderHelperSkill(product ir.CanonicalProduct, tool ir.ToolDescriptor, helperName string, helperPath string) string {
	var builder strings.Builder
	sharedPath := relativeMarkdownPath(helperPath, filepath.ToSlash(filepath.Join("skills/generated", "dws-shared", "SKILL.md")))
	servicePath := relativeMarkdownPath(helperPath, filepath.ToSlash(filepath.Join("skills/generated", "dws-"+safeSkillSegment(product.ID), "SKILL.md")))
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        helperName,
		Description: helperDescription(product, tool),
		Category:    "productivity",
		CLIHelp:     fmt.Sprintf("%s --help", cliInvocation(product, tool)),
	}))
	builder.WriteString(fmt.Sprintf("# %s %s\n\n", product.ID, helperDisplayName(product, tool)))
	builder.WriteString(fmt.Sprintf("> **PREREQUISITE:** Read `%s` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.\n\n", sharedPath))
	builder.WriteString(fmt.Sprintf("%s\n\n", safeValue(tool.Description, tool.Title)))

	builder.WriteString("## Usage\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("%s --json '{...}'\n", cliInvocation(product, tool)))
	builder.WriteString("```\n\n")

	specs := cli.VisibleFlagSpecs(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
	required := toolRequiredFields(tool)
	if len(specs) > 0 {
		builder.WriteString("## Flags\n\n")
		builder.WriteString("| Flag | Required | Default | Description |\n")
		builder.WriteString("|------|----------|---------|-------------|\n")
		requiredSet := make(map[string]struct{}, len(required))
		for _, field := range required {
			requiredSet[field] = struct{}{}
		}
		for _, spec := range specs {
			requiredMark := "—"
			if _, ok := requiredSet[spec.PropertyName]; ok {
				requiredMark = "✓"
			}
			builder.WriteString(fmt.Sprintf("| %s | %s | — | %s |\n", flagDisplayLabel(spec), requiredMark, safeValue(spec.Description, "-")))
		}
		builder.WriteString("\n")
	}

	if len(required) > 0 {
		builder.WriteString("## Required Fields\n\n")
		for _, field := range required {
			builder.WriteString(fmt.Sprintf("- `%s`\n", field))
		}
		builder.WriteString("\n")
	}

	if isWriteTool(tool) {
		builder.WriteString("> [!CAUTION]\n")
		builder.WriteString("> This is a **write** command — confirm with the user before executing.\n\n")
	}

	serviceName := "dws-" + safeSkillSegment(product.ID)
	builder.WriteString("## See Also\n\n")
	builder.WriteString(fmt.Sprintf("- [dws-shared](%s) — Global rules and auth\n- [%s](%s) — Product skill\n", sharedPath, serviceName, servicePath))
	return finalizeMarkdown(builder.String())
}

func renderSkillsIndex(entries []skillIndexEntry, linkBase string) string {
	slices.SortFunc(entries, func(left, right skillIndexEntry) int {
		if left.Category != right.Category {
			return strings.Compare(left.Category, right.Category)
		}
		return strings.Compare(left.Name, right.Name)
	})

	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        "dws",
		Description: topLevelDWSDescription,
		Category:    "productivity",
		CLIHelp:     "dws --help",
	}))
	builder.WriteString("# Skills Index\n\n")
	builder.WriteString("> Auto-generated by `dws generate-skills`. Do not edit manually.\n\n")

	sections := []struct {
		Category string
		Heading  string
		SubTitle string
	}{
		{Category: skillCategoryService, Heading: "## Services", SubTitle: "Core DWS product and shared skills."},
		// {Category: skillCategoryHelper, Heading: "## Helpers", SubTitle: "Tool-level execution skills."},
	}

	for _, section := range sections {
		items := make([]skillIndexEntry, 0)
		for _, entry := range entries {
			if entry.Category == section.Category {
				items = append(items, entry)
			}
		}
		if len(items) == 0 {
			continue
		}
		builder.WriteString(section.Heading)
		builder.WriteString("\n\n")
		builder.WriteString(section.SubTitle)
		builder.WriteString("\n\n")
		builder.WriteString("| Skill | Description |\n")
		builder.WriteString("|-------|-------------|\n")
		for _, entry := range items {
			builder.WriteString(fmt.Sprintf("| [%s](%s) | %s |\n", entry.Name, indexRelativeLink(entry.Path, linkBase), entry.Description))
		}
		builder.WriteString("\n")
	}

	return finalizeMarkdown(builder.String())
}

func renderSkillsCoverageReport(catalog ir.Catalog, helperCounts map[string]int) string {
	var builder strings.Builder
	builder.WriteString("# Skills Coverage Report\n\n")
	builder.WriteString("> Auto-generated by `dws generate-skills`. Do not edit manually.\n\n")
	builder.WriteString(fmt.Sprintf("- Catalog products: `%d`\n", len(catalog.Products)))
	builder.WriteString(fmt.Sprintf("- Generated services: `%d`\n", len(catalog.Products)))

	totalHelpers := 0
	for _, count := range helperCounts {
		totalHelpers += count
	}
	builder.WriteString(fmt.Sprintf("- Generated helpers: `%d`\n\n", totalHelpers))

	services := make([]string, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		services = append(services, safeSkillSegment(product.ID))
	}
	services = uniqueSkillProducts(services)

	if len(services) == 0 {
		builder.WriteString("No catalog-backed services were generated.\n")
		return finalizeMarkdown(builder.String())
	}

	builder.WriteString("## Services\n\n")
	builder.WriteString("| Service | Helpers |\n")
	builder.WriteString("|---------|---------|\n")
	for _, service := range services {
		builder.WriteString(fmt.Sprintf("| `%s` | `%d` |\n", service, helperCounts[service]))
	}
	builder.WriteString("\n")
	return finalizeMarkdown(builder.String())
}

func renderProductCLI(product ir.CanonicalProduct) string {
	lines := []string{
		fmt.Sprintf("# Canonical Product: %s", product.ID),
		"",
		"Generated from shared Tool IR. Do not edit by hand.",
		"",
		fmt.Sprintf("- Display name: %s", safeValue(product.DisplayName, product.ID)),
	}
	if description := strings.TrimSpace(productServiceDescription(product)); description != "" {
		lines = append(lines, fmt.Sprintf("- Description: %s", description))
	}
	lines = append(lines,
		fmt.Sprintf("- Server key: `%s`", product.ServerKey),
		fmt.Sprintf("- Endpoint: `%s`", product.Endpoint),
		fmt.Sprintf("- Protocol: `%s`", safeValue(product.NegotiatedProtocolVersion, "unknown")),
		fmt.Sprintf("- Degraded: `%t`", product.Degraded),
		"",
		"## Tools",
		"",
	)
	for _, tool := range product.Tools {
		lines = append(lines, fmt.Sprintf("- `%s`", helperDisplayName(product, tool)))
		lines = append(lines, fmt.Sprintf("  - Path: `%s`", tool.CanonicalPath))
		lines = append(lines, fmt.Sprintf("  - CLI route: `%s`", cliInvocation(product, tool)))
		lines = append(lines, fmt.Sprintf("  - Description: %s", safeValue(tool.Description, tool.Title)))
		lines = append(lines, fmt.Sprintf("  - Flags: %s", renderFlags(cli.VisibleFlagSpecs(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints)))))
		lines = append(lines, fmt.Sprintf("  - Schema: `%s`", generatedDocsPath("schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json")))
	}
	lines = append(lines, "")
	return finalizeMarkdown(strings.Join(lines, "\n"))
}

func CatalogSnapshotPayload(catalog ir.Catalog) map[string]any {
	products := make([]map[string]any, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		products = append(products, generatedCatalogProduct(product))
	}
	return map[string]any{
		"products": products,
	}
}

func generatedCatalogProduct(product ir.CanonicalProduct) map[string]any {
	tools := make([]map[string]any, 0, len(product.Tools))
	for _, tool := range product.Tools {
		tools = append(tools, generatedCatalogTool(product, tool))
	}

	payload := map[string]any{
		"id":           product.ID,
		"display_name": product.DisplayName,
		"server_key":   product.ServerKey,
		"endpoint":     product.Endpoint,
		"degraded":     product.Degraded,
		"tools":        tools,
	}
	if strings.TrimSpace(product.Description) != "" {
		payload["description"] = product.Description
	}
	if strings.TrimSpace(product.ServiceDescription) != "" {
		payload["service_description"] = product.ServiceDescription
	}
	if strings.TrimSpace(product.SchemaURI) != "" {
		payload["schema_uri"] = product.SchemaURI
	}
	if strings.TrimSpace(product.NegotiatedProtocolVersion) != "" {
		payload["negotiated_protocol_version"] = product.NegotiatedProtocolVersion
	}
	if strings.TrimSpace(product.Source) != "" {
		payload["source"] = product.Source
	}
	if product.Lifecycle != nil {
		payload["lifecycle"] = product.Lifecycle
	}
	if product.CLI != nil {
		payload["cli"] = product.CLI
	}
	return payload
}

func generatedCatalogTool(product ir.CanonicalProduct, tool ir.ToolDescriptor) map[string]any {
	flags := cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints)
	flagHints := cli.BuildEffectiveFlagHints(tool.InputSchema, tool.FlagHints)

	payload := map[string]any{
		"rpc_name":          tool.RPCName,
		"cli_name":          tool.CLIName,
		"title":             tool.Title,
		"description":       tool.Description,
		"input_schema":      tool.InputSchema,
		"sensitive":         tool.Sensitive,
		"source_server_key": tool.SourceServerKey,
		"canonical_path":    tool.CanonicalPath,
		"cli_path":          generatedCLIPath(product, tool),
		"required":          cli.RequiredFlagProperties(flagHints),
		"flags":             flags,
	}
	if len(tool.Aliases) > 0 {
		payload["aliases"] = tool.Aliases
	}
	if len(tool.OutputSchema) > 0 {
		payload["output_schema"] = tool.OutputSchema
	}
	if tool.Hidden {
		payload["hidden"] = true
	}
	if strings.TrimSpace(tool.Group) != "" {
		payload["group"] = tool.Group
	}
	if len(flagHints) > 0 {
		payload["flag_hints"] = flagHints
	} else if len(tool.FlagHints) > 0 {
		payload["flag_hints"] = tool.FlagHints
	}
	return payload
}

func generatedToolSchemaPayload(product ir.CanonicalProduct, tool ir.ToolDescriptor) map[string]any {
	flags := cli.VisibleFlagSpecs(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
	flagHints := visibleGeneratedFlagHints(flags, cli.BuildEffectiveFlagHints(tool.InputSchema, tool.FlagHints))
	payload := map[string]any{
		"kind":         "generated_schema",
		"path":         tool.CanonicalPath,
		"product_id":   product.ID,
		"display":      product.DisplayName,
		"tool":         generatedToolSummary(tool),
		"cli_path":     generatedCLIPath(product, tool),
		"input_schema": tool.InputSchema,
		"required":     cli.RequiredFlagProperties(flagHints),
		"flags":        flags,
	}
	if len(flagHints) > 0 {
		payload["flag_hints"] = flagHints
	}
	if len(tool.OutputSchema) > 0 {
		payload["output_schema"] = tool.OutputSchema
	}
	return payload
}

func generatedCLIPath(product ir.CanonicalProduct, tool ir.ToolDescriptor) []string {
	parts := strings.Fields(cliInvocation(product, tool))
	if len(parts) == 0 {
		return nil
	}
	if parts[0] == "dws" {
		return append([]string{}, parts[1:]...)
	}
	return parts
}

func generatedToolSummary(tool ir.ToolDescriptor) map[string]any {
	summary := map[string]any{
		"rpc_name":          tool.RPCName,
		"cli_name":          tool.CLIName,
		"aliases":           tool.Aliases,
		"title":             tool.Title,
		"description":       tool.Description,
		"sensitive":         tool.Sensitive,
		"canonical_path":    tool.CanonicalPath,
		"source_server_key": tool.SourceServerKey,
	}
	if strings.TrimSpace(tool.Group) != "" {
		summary["group"] = tool.Group
	}
	if tool.Hidden {
		summary["hidden"] = true
	}
	return summary
}

func renderFlags(specs []cli.FlagSpec) string {
	specs = cli.VisibleFlagSpecs(specs)
	if len(specs) == 0 {
		return "none"
	}
	flags := make([]string, 0, len(specs))
	for _, spec := range specs {
		label := flagDisplayLabel(spec)
		if spec.Shorthand != "" {
			label = label + fmt.Sprintf(" (`-%s`)", spec.Shorthand)
		}
		flags = append(flags, label)
	}
	return strings.Join(flags, ", ")
}

func visibleGeneratedFlagHints(specs []cli.FlagSpec, hints map[string]ir.CLIFlagHint) map[string]ir.CLIFlagHint {
	if len(specs) == 0 || len(hints) == 0 {
		return nil
	}
	visibleProps := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Hidden {
			continue
		}
		visibleProps[spec.PropertyName] = struct{}{}
	}
	if len(visibleProps) == 0 {
		return nil
	}
	out := make(map[string]ir.CLIFlagHint)
	for property, hint := range hints {
		if _, ok := visibleProps[property]; !ok {
			continue
		}
		if hint.Hidden {
			continue
		}
		out[property] = hint
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// flagDisplayLabel returns the public Markdown label for a flag in skill documentation.
func flagDisplayLabel(spec cli.FlagSpec) string {
	primary := strings.TrimSpace(spec.FlagName)
	return fmt.Sprintf("`--%s`", primary)
}

func safeSkillSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func safeDocSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '/', '\\', ':':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func toolRequiredFields(tool ir.ToolDescriptor) []string {
	specs := cli.VisibleFlagSpecs(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
	hints := visibleGeneratedFlagHints(specs, cli.BuildEffectiveFlagHints(tool.InputSchema, tool.FlagHints))
	return cli.RequiredFlagProperties(hints)
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func safeValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func serviceDescription(product ir.CanonicalProduct) string {
	base := safeValue(product.DisplayName, product.ID)
	desc := strings.TrimSpace(productServiceDescription(product))
	if desc == "" {
		desc = fmt.Sprintf("DWS %s service.", product.ID)
	}
	if strings.Contains(strings.ToLower(desc), strings.ToLower(base)) {
		return truncateSkillDescription(desc)
	}
	return truncateSkillDescription(fmt.Sprintf("%s: %s", base, desc))
}

func productServiceDescription(product ir.CanonicalProduct) string {
	if description := strings.TrimSpace(product.ServiceDescription); description != "" {
		return description
	}
	return strings.TrimSpace(product.Description)
}

func helperDescription(product ir.CanonicalProduct, tool ir.ToolDescriptor) string {
	base := safeValue(product.DisplayName, product.ID)
	desc := safeValue(tool.Description, tool.Title)
	if desc == "" {
		desc = tool.RPCName
	}
	return truncateSkillDescription(fmt.Sprintf("%s: %s", base, desc))
}

func helperSkillName(product ir.CanonicalProduct, tool ir.ToolDescriptor, routeSegments []string, seen map[string]int) string {
	parts := []string{safeSkillSegment(product.ID)}
	for _, segment := range routeSegments {
		token := safeSkillSegment(segment)
		if token == "unknown" {
			continue
		}
		parts = append(parts, token)
	}
	token := ""
	if len(parts) > 1 {
		token = strings.Join(parts, "-")
	} else {
		token = safeSkillSegment(tool.CLIName)
	}
	if token == "unknown" {
		token = safeSkillSegment(tool.RPCName)
	}
	name := fmt.Sprintf("dws-%s", token)
	if _, ok := seen[name]; !ok {
		seen[name] = 1
		return name
	}
	seen[name]++
	return fmt.Sprintf("%s-%d", name, seen[name])
}

func isBlockedTool(_ ir.CanonicalProduct, _ ir.ToolDescriptor) bool {
	return false
}

func helperDisplayName(product ir.CanonicalProduct, tool ir.ToolDescriptor) string {
	segments := skillRouteSegments(product, tool)
	if len(segments) == 0 {
		return "unknown"
	}
	return strings.Join(segments, " ")
}

func cliInvocation(product ir.CanonicalProduct, tool ir.ToolDescriptor) string {
	segments := append([]string{"dws"}, cli.ResolveToolCLIPath(product, tool)...)
	return strings.Join(compactStrings(segments), " ")
}

func cliCommandPattern(product ir.CanonicalProduct, helpers []helperSkillRef) string {
	hasGroups := false
	for _, helper := range helpers {
		if strings.TrimSpace(helper.Tool.Group) != "" {
			hasGroups = true
			break
		}
	}
	if hasGroups {
		return fmt.Sprintf("dws %s <group> <command>", cliProductCommand(product))
	}
	return fmt.Sprintf("dws %s <command>", cliProductCommand(product))
}

func cliProductCommand(product ir.CanonicalProduct) string {
	if product.CLI != nil {
		if command := strings.TrimSpace(product.CLI.Command); command != "" {
			return command
		}
	}
	return strings.TrimSpace(product.ID)
}

func cliRouteSegments(tool ir.ToolDescriptor) []string {
	segments := make([]string, 0, 2)
	if group := strings.TrimSpace(tool.Group); group != "" {
		for _, segment := range strings.Split(group, ".") {
			segment = strings.TrimSpace(segment)
			if segment != "" {
				segments = append(segments, segment)
			}
		}
	}
	name := strings.TrimSpace(tool.CLIName)
	if name == "" {
		name = strings.TrimSpace(tool.RPCName)
	}
	if name != "" {
		segments = append(segments, name)
	}
	return segments
}

func skillRouteSegments(product ir.CanonicalProduct, tool ir.ToolDescriptor) []string {
	path := cli.ResolveToolCLIPath(product, tool)
	if len(path) > 1 {
		return append([]string(nil), path[1:]...)
	}
	return cliRouteSegments(tool)
}

func skillToolSlug(tool ir.ToolDescriptor, routeSegments []string) string {
	if len(routeSegments) > 0 {
		slug := safeSkillSegment(routeSegments[len(routeSegments)-1])
		if slug != "unknown" {
			return slug
		}
	}
	slug := safeSkillSegment(tool.CLIName)
	if slug == "unknown" {
		slug = strings.ReplaceAll(safeSkillSegment(tool.RPCName), "_", "-")
	}
	return slug
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func relativeMarkdownPath(fromFilePath, toFilePath string) string {
	from := filepath.FromSlash(strings.TrimSpace(fromFilePath))
	to := filepath.FromSlash(strings.TrimSpace(toFilePath))
	if from == "" || to == "" {
		return ""
	}
	rel, err := filepath.Rel(filepath.Dir(from), to)
	if err != nil {
		return filepath.ToSlash(to)
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		return "./"
	}
	if strings.HasPrefix(rel, ".") {
		return rel
	}
	return "./" + rel
}

func isWriteTool(tool ir.ToolDescriptor) bool {
	if tool.Sensitive {
		return true
	}
	parts := strings.FieldsFunc(strings.ToLower(tool.RPCName), func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	for _, part := range parts {
		if _, ok := writeOperationTokens[part]; ok {
			return true
		}
	}
	return false
}

func renderFrontmatter(spec frontmatterSpec) string {
	category := strings.TrimSpace(spec.Category)
	if category == "" {
		category = "productivity"
	}
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString(fmt.Sprintf("name: %s\n", strings.TrimSpace(spec.Name)))
	builder.WriteString(fmt.Sprintf("description: \"%s\"\n", sanitizeSkillDescription(spec.Description)))
	builder.WriteString("metadata:\n")
	builder.WriteString(fmt.Sprintf("  version: %s\n", skillVersion))
	builder.WriteString("  openclaw:\n")
	builder.WriteString(fmt.Sprintf("    category: \"%s\"\n", category))
	if strings.TrimSpace(spec.Domain) != "" {
		builder.WriteString(fmt.Sprintf("    domain: \"%s\"\n", safeSkillSegment(spec.Domain)))
	}
	builder.WriteString("    requires:\n")
	builder.WriteString("      bins:\n")
	builder.WriteString("        - dws\n")
	if len(spec.RequiresSkills) > 0 {
		builder.WriteString("      skills:\n")
		for _, skill := range spec.RequiresSkills {
			skill = strings.TrimSpace(skill)
			if skill == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("        - %s\n", skill))
		}
	}
	if strings.TrimSpace(spec.CLIHelp) != "" {
		builder.WriteString(fmt.Sprintf("    cliHelp: \"%s\"\n", strings.TrimSpace(spec.CLIHelp)))
	}
	builder.WriteString("---\n\n")
	return builder.String()
}

func finalizeMarkdown(markdown string) string {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")
	lines := strings.Split(markdown, "\n")
	for idx, line := range lines {
		lines[idx] = strings.TrimRight(line, " \t")
	}
	markdown = strings.Join(lines, "\n")
	return strings.TrimRight(markdown, "\n") + "\n"
}

func sanitizeSkillDescription(description string) string {
	description = strings.ReplaceAll(description, "\"", "'")
	description = strings.TrimSpace(description)
	if description == "" {
		description = "DWS generated skill."
	}
	if description != topLevelDWSDescription {
		description = truncateSkillDescription(description)
	}
	if !hasTerminalPunctuation(description) {
		description += "."
	}
	return description
}

func truncateSkillDescription(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}
	runes := []rune(description)
	if len(runes) <= frontmatterDescriptionLimit {
		return description
	}
	return strings.TrimSpace(string(runes[:frontmatterDescriptionLimit-1])) + "…"
}

func hasTerminalPunctuation(value string) bool {
	for _, suffix := range []string{".", "…", "!", "?", "。", "！", "？"} {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func indexRelativeLink(entryPath, linkBase string) string {
	entryPath = filepath.ToSlash(strings.TrimSpace(entryPath))
	if entryPath == "" {
		return ""
	}
	linkBase = filepath.ToSlash(strings.TrimSpace(linkBase))
	return strings.TrimPrefix(entryPath, linkBase)
}

func uniqueSkillProducts(products []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(products))
	for _, product := range products {
		product = strings.TrimSpace(strings.ToLower(product))
		if product == "" {
			continue
		}
		product = safeSkillSegment(product)
		if product == "unknown" {
			continue
		}
		if _, ok := seen[product]; ok {
			continue
		}
		seen[product] = struct{}{}
		out = append(out, product)
	}
	slices.Sort(out)
	return out
}

func wrapTicks(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, fmt.Sprintf("`%s`", value))
	}
	return out
}

func WriteArtifacts(root string, artifacts []Artifact) error {
	for _, artifact := range artifacts {
		if err := removeLegacyGeneratedDocsArtifact(root, artifact.Path); err != nil {
			return err
		}
		target := filepath.Join(root, artifact.Path)
		if err := writeFile(target, artifact.Content); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	var buffer bytes.Buffer
	buffer.Write(content)
	return writeFileBytes(path, buffer.Bytes())
}

func generatedDocsPath(parts ...string) string {
	segments := append([]string{generatedDocsRoot}, parts...)
	return filepath.ToSlash(filepath.Join(segments...))
}

func removeLegacyGeneratedDocsArtifact(root, path string) error {
	legacyPath, ok := legacyGeneratedDocsTwin(path)
	if !ok {
		return nil
	}
	target := filepath.Join(root, filepath.FromSlash(legacyPath))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func legacyGeneratedDocsTwin(path string) (string, bool) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	prefix := generatedDocsRoot + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return legacyGeneratedDocsRoot + "/" + strings.TrimPrefix(path, prefix), true
}
