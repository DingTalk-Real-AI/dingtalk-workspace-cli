package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
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
	skillCategoryPersona        = "persona"
	skillCategoryRecipe         = "recipe"
	frontmatterDescriptionLimit = 120
)

var blockedCanonicalTools = map[string]struct{}{
	"drive.empty_trash": {},
	"drive.delete":      {},
}

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

var legacy17CoverageTargets = []string{
	"aiapp",
	"aitable",
	"attendance",
	"calendar",
	"chat",
	"conference",
	"contact",
	"devdoc",
	"ding",
	"doc",
	"drive",
	"live",
	"mail",
	"minutes",
	"oa",
	"report",
	"todo",
}

var extended22CoverageTargets = []string{
	"aiapp",
	"aitable",
	"attendance",
	"calendar",
	"chat",
	"conference",
	"contact",
	"devdoc",
	"ding",
	"doc",
	"drive",
	"live",
	"mail",
	"minutes",
	"oa",
	"report",
	"todo",
	"aidesign",
	"finance",
	"law",
	"docparse",
}

func Generate(catalog ir.Catalog) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(catalog.Products)+12)

	catalogJSON, err := marshalJSON(catalog)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("docs/generated/schema/catalog.json"),
		Content: catalogJSON,
	})

	for _, product := range catalog.Products {
		for _, tool := range product.Tools {
			payload := map[string]any{
				"kind":       "generated_schema",
				"path":       tool.CanonicalPath,
				"product_id": product.ID,
				"display":    product.DisplayName,
				"tool":       tool,
				"required":   requiredFields(tool.InputSchema),
			}
			data, err := marshalJSON(payload)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, Artifact{
				Path:    filepath.ToSlash(filepath.Join("docs/generated/schema", tool.CanonicalPath+".json")),
				Content: data,
			})
			artifacts = append(artifacts, Artifact{
				Path:    filepath.ToSlash(filepath.Join("docs/generated/schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json")),
				Content: data,
			})
		}
	}

	cliDoc, err := renderCanonicalCLI(catalog)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("docs/generated/cli/canonical-cli.md"),
		Content: []byte(cliDoc),
	})
	for _, product := range catalog.Products {
		artifacts = append(artifacts, Artifact{
			Path:    filepath.ToSlash(filepath.Join("docs/generated/cli", safeDocSegment(product.ID)+".md")),
			Content: []byte(renderProductCLI(product)),
		})
	}

	readme := strings.Join([]string{
		"# Generated Docs",
		"",
		"These artifacts are generated from the shared canonical Tool IR. Do not edit them by hand.",
		"",
		"- `docs/generated/cli/canonical-cli.md`: canonical command surface summary",
		"- `docs/generated/cli/<product>.md`: per-product canonical command summary",
		"- `docs/generated/schema/catalog.json`: full catalog snapshot",
		"- `docs/generated/schema/<product>.<tool>.json`: per-tool schema payloads",
		"- `docs/generated/schema/<product>/<tool>.json`: per-tool schema payloads in hierarchical layout",
		"- `skills/generated/apis.md`: top-level skills index for generated docs navigation",
		"- `docs/generated/skills-coverage.md`: coverage report against legacy17/extended22 targets",
		"",
	}, "\n")
	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("docs/generated/README.md"),
		Content: []byte(readme),
	})

	indexEntries := make([]skillIndexEntry, 0, len(catalog.Products)*4)

	canonicalSkill, err := renderCanonicalSurfaceSkill(catalog)
	if err != nil {
		return nil, err
	}
	canonicalPath := filepath.ToSlash("skills/generated/canonical-surface/api.md")
	artifacts = append(artifacts, Artifact{
		Path:    canonicalPath,
		Content: []byte(canonicalSkill),
	})
	indexEntries = append(indexEntries, skillIndexEntry{
		Name:        "canonical-surface",
		Description: truncateSkillDescription("DWS canonical MCP surface and schema discovery entrypoint."),
		Category:    skillCategoryService,
		Path:        canonicalPath,
	})

	sharedPath := filepath.ToSlash("skills/generated/dws-shared/api.md")
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
	availableServices := map[string]struct{}{}
	helperCountsByService := map[string]int{}

	for _, product := range catalog.Products {
		helperRefs := make([]helperSkillRef, 0, len(product.Tools))
		helperNames := map[string]int{}

		for _, tool := range product.Tools {
			if isBlockedTool(product, tool) {
				continue
			}
			helperName := helperSkillName(product, tool, helperNames)
			helperPath := filepath.ToSlash(filepath.Join("skills/generated", helperName, "api.md"))
			helperRefs = append(helperRefs, helperSkillRef{
				Name: helperName,
				Path: helperPath,
				Tool: tool,
			})
			helperMD := renderHelperSkill(product, tool, helperName)
			artifacts = append(artifacts, Artifact{Path: helperPath, Content: []byte(helperMD)})
			indexEntries = append(indexEntries, skillIndexEntry{
				Name:        helperName,
				Description: truncateSkillDescription(helperDescription(product, tool)),
				Category:    skillCategoryHelper,
				Path:        helperPath,
			})
		}

		serviceName := "dws-" + safeSkillSegment(product.ID)
		servicePath := filepath.ToSlash(filepath.Join("skills/generated", serviceName, "api.md"))
		serviceMD := renderServiceSkill(product, helperRefs)
		artifacts = append(artifacts, Artifact{Path: servicePath, Content: []byte(serviceMD)})
		indexEntries = append(indexEntries, skillIndexEntry{
			Name:        serviceName,
			Description: truncateSkillDescription(serviceDescription(product)),
			Category:    skillCategoryService,
			Path:        servicePath,
		})
		serviceToken := safeSkillSegment(product.ID)
		availableServices[serviceToken] = struct{}{}
		helperCountsByService[serviceToken] = len(helperRefs)
	}

	personaRegistry, err := loadPersonaRegistry()
	if err != nil {
		return nil, err
	}
	recipeRegistry, err := loadRecipeRegistry()
	if err != nil {
		return nil, err
	}
	if err := validateRegistryReferences(personaRegistry, recipeRegistry); err != nil {
		return nil, err
	}

	for _, persona := range personaRegistry.Personas {
		safeName := "persona-" + safeSkillSegment(persona.Name)
		path := filepath.ToSlash(filepath.Join("skills/generated", safeName, "api.md"))
		artifacts = append(artifacts, Artifact{Path: path, Content: []byte(renderPersonaSkill(persona, safeName, availableServices))})
		indexEntries = append(indexEntries, skillIndexEntry{
			Name:        safeName,
			Description: truncateSkillDescription(persona.Description),
			Category:    skillCategoryPersona,
			Path:        path,
		})
	}

	for _, recipe := range recipeRegistry.Recipes {
		safeName := "recipe-" + safeSkillSegment(recipe.Name)
		path := filepath.ToSlash(filepath.Join("skills/generated", safeName, "api.md"))
		artifacts = append(artifacts, Artifact{Path: path, Content: []byte(renderRecipeSkill(recipe, safeName, availableServices))})
		indexEntries = append(indexEntries, skillIndexEntry{
			Name:        safeName,
			Description: truncateSkillDescription(recipe.Description),
			Category:    skillCategoryRecipe,
			Path:        path,
		})
	}

	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("skills/generated/apis.md"),
		Content: []byte(renderSkillsIndex(indexEntries, "skills/generated/")),
	})
	artifacts = append(artifacts, Artifact{
		Path:    filepath.ToSlash("docs/generated/skills-coverage.md"),
		Content: []byte(renderSkillsCoverageReport(catalog, availableServices, helperCountsByService)),
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
	builder.WriteString("- `dws mcp <canonical-product> <tool> --json '{...}'`\n")
	builder.WriteString("- `dws schema <canonical-product>.<tool>`\n\n")
	builder.WriteString("## Products\n\n")

	for _, product := range catalog.Products {
		_, _ = fmt.Fprintf(&builder, "### `%s`\n\n", product.ID)
		_, _ = fmt.Fprintf(&builder, "- Display name: %s\n", safeValue(product.DisplayName, product.ID))
		_, _ = fmt.Fprintf(&builder, "- Server key: `%s`\n", product.ServerKey)
		_, _ = fmt.Fprintf(&builder, "- Protocol: `%s`\n", safeValue(product.NegotiatedProtocolVersion, "unknown"))
		_, _ = fmt.Fprintf(&builder, "- Degraded: `%t`\n", product.Degraded)
		builder.WriteString("- Tools:\n")
		for _, tool := range product.Tools {
			flags := renderFlags(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
			_, _ = fmt.Fprintf(&builder, "  - `%s`: %s\n", tool.CanonicalPath, safeValue(tool.Description, tool.Title))
			_, _ = fmt.Fprintf(&builder, "    Flags: %s\n", flags)
			_, _ = fmt.Fprintf(&builder, "    Schema: `%s`\n", filepath.ToSlash(filepath.Join("docs/generated/schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json")))
		}
		builder.WriteString("\n")
	}

	return finalizeMarkdown(builder.String()), nil
}

func renderCanonicalSurfaceSkill(catalog ir.Catalog) (string, error) {
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        "canonical-surface",
		Description: "DWS canonical MCP surface and schema discovery entrypoint.",
		Category:    "productivity",
		CLIHelp:     "dws mcp --help",
	}))
	builder.WriteString("# DWS Canonical Surface\n\n")
	builder.WriteString("Use the canonical `dws mcp` surface generated from the shared Tool IR.\n\n")
	builder.WriteString("## Command Pattern\n\n")
	builder.WriteString("- `dws mcp <canonical-product> <tool> --json '{...}'`\n")
	builder.WriteString("- `dws schema <canonical-product>.<tool>`\n")
	builder.WriteString("- Top-level scalar and scalar-array fields may also be passed as convenience flags.\n")
	builder.WriteString("- Nested objects stay in `--json` or `--params`.\n\n")
	builder.WriteString("## Known Products\n\n")
	for _, product := range catalog.Products {
		builder.WriteString(fmt.Sprintf("- `%s`: ", product.ID))
		toolNames := make([]string, 0, len(product.Tools))
		for _, tool := range product.Tools {
			toolNames = append(toolNames, fmt.Sprintf("`%s`", tool.RPCName))
		}
		builder.WriteString(strings.Join(toolNames, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString("\n## References\n\n")
	builder.WriteString("- `docs/generated/cli/canonical-cli.md`\n")
	builder.WriteString("- `docs/generated/schema/catalog.json`\n")
	return finalizeMarkdown(builder.String()), nil
}

func renderSharedSkill(catalog ir.Catalog) string {
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        "dws-shared",
		Description: "DWS shared reference for authentication, command patterns, and safety rules.",
		Category:    "productivity",
	}))
	builder.WriteString("# DWS Shared Reference\n\n")
	builder.WriteString("## Installation\n\n")
	builder.WriteString("Ensure `dws` is installed and accessible from `$PATH`.\n\n")
	builder.WriteString("## Authentication\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("dws auth login\n")
	builder.WriteString("dws auth status\n")
	builder.WriteString("```\n\n")
	builder.WriteString("## Global Rules\n\n")
	builder.WriteString("- Output defaults to JSON. Use `--format table` for human-readable output.\n")
	builder.WriteString("- Confirm with user before any write/delete/revoke action.\n")
	builder.WriteString("- Never fabricate IDs; always extract from command output.\n")
	builder.WriteString("- For risky operations, run a read/list check before executing write operations.\n\n")
	builder.WriteString("## Command Pattern\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("dws mcp <product> <tool> --json '{...}'\n")
	builder.WriteString("dws schema <product>.<tool> --json\n")
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
		CLIHelp:     fmt.Sprintf("dws mcp %s --help", product.ID),
	}))
	builder.WriteString(fmt.Sprintf("# DWS %s\n\n", product.ID))
	builder.WriteString("> **PREREQUISITE:** Read `../dws-shared/api.md` for auth, command patterns, and security rules.\n\n")
	builder.WriteString(fmt.Sprintf("- Display name: %s\n", safeValue(product.DisplayName, product.ID)))
	builder.WriteString(fmt.Sprintf("- Endpoint: `%s`\n", safeValue(product.Endpoint, "unknown")))
	builder.WriteString(fmt.Sprintf("- Protocol: `%s`\n", safeValue(product.NegotiatedProtocolVersion, "unknown")))
	builder.WriteString(fmt.Sprintf("- Degraded: `%t`\n\n", product.Degraded))

	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("dws mcp %s <tool> --json '{...}'\n", product.ID))
	builder.WriteString("```\n\n")

	if len(helpers) > 0 {
		builder.WriteString("## Helper Skills\n\n")
		builder.WriteString("| Skill | Tool | Description |\n")
		builder.WriteString("|-------|------|-------------|\n")
		for _, helper := range helpers {
			builder.WriteString(fmt.Sprintf("| [`%s`](../%s/api.md) | `%s` | %s |\n",
				helper.Name,
				filepath.Base(filepath.Dir(helper.Path)),
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
			required := requiredFields(helper.Tool.InputSchema)
			requiredText := "none"
			if len(required) > 0 {
				requiredText = strings.Join(wrapTicks(required), ", ")
			}
			builder.WriteString(fmt.Sprintf("### `%s`\n\n", helper.Tool.RPCName))
			builder.WriteString(fmt.Sprintf("- Canonical path: `%s`\n", helper.Tool.CanonicalPath))
			builder.WriteString(fmt.Sprintf("- Description: %s\n", safeValue(helper.Tool.Description, helper.Tool.Title)))
			builder.WriteString(fmt.Sprintf("- Required fields: %s\n", requiredText))
			builder.WriteString(fmt.Sprintf("- Sensitive: `%t`\n\n", helper.Tool.Sensitive))
		}
	}

	builder.WriteString("## Discovering Commands\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("dws schema --json                # list available products\ndws schema %s.<tool> --json       # inspect schema\n", product.ID))
	builder.WriteString("```\n")
	return finalizeMarkdown(builder.String())
}

func renderHelperSkill(product ir.CanonicalProduct, tool ir.ToolDescriptor, helperName string) string {
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:        helperName,
		Description: helperDescription(product, tool),
		Category:    "productivity",
		CLIHelp:     fmt.Sprintf("dws schema %s --json", tool.CanonicalPath),
	}))
	builder.WriteString(fmt.Sprintf("# DWS %s %s\n\n", product.ID, tool.RPCName))
	builder.WriteString("> **PREREQUISITE:** Read `../dws-shared/api.md` for auth, command patterns, and security rules.\n\n")
	builder.WriteString(fmt.Sprintf("%s\n\n", safeValue(tool.Description, tool.Title)))

	builder.WriteString("## Usage\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString(fmt.Sprintf("dws mcp %s %s --json '{...}'\n", product.ID, tool.RPCName))
	builder.WriteString("```\n\n")

	specs := cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints)
	required := requiredFields(tool.InputSchema)
	if len(specs) > 0 {
		builder.WriteString("## Flags\n\n")
		builder.WriteString("| Flag | Required | Description |\n")
		builder.WriteString("|------|----------|-------------|\n")
		requiredSet := make(map[string]struct{}, len(required))
		for _, field := range required {
			requiredSet[field] = struct{}{}
		}
		for _, spec := range specs {
			requiredMark := "—"
			if _, ok := requiredSet[spec.PropertyName]; ok {
				requiredMark = "✓"
			}
			builder.WriteString(fmt.Sprintf("| `--%s` | %s | %s |\n", spec.FlagName, requiredMark, safeValue(spec.Description, "-")))
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
	builder.WriteString(fmt.Sprintf("- [dws-shared](../dws-shared/api.md) — Global rules\n- [%s](../%s/api.md) — Product skill\n", serviceName, serviceName))
	return finalizeMarkdown(builder.String())
}

func renderPersonaSkill(persona PersonaEntry, skillName string, availableServices map[string]struct{}) string {
	var builder strings.Builder
	required, missing := resolveRequiredServiceSkills(persona.serviceRefs(), availableServices)
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:           skillName,
		Description:    persona.Description,
		Category:       skillCategoryPersona,
		RequiresSkills: required,
	}))
	builder.WriteString(fmt.Sprintf("# %s\n\n", persona.Title))
	if len(required) > 0 {
		builder.WriteString(fmt.Sprintf("> **PREREQUISITE:** Load the following skills before using this persona: %s\n\n", strings.Join(wrapTicks(required), ", ")))
	}
	if len(missing) > 0 {
		builder.WriteString(fmt.Sprintf("> **NOTE:** Current catalog snapshot does not expose these referenced services yet: %s\n\n", strings.Join(wrapTicks(missing), ", ")))
	}
	builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(persona.Description)))
	if len(persona.Workflows) > 0 {
		builder.WriteString("## Relevant Workflows\n\n")
		for _, workflow := range persona.Workflows {
			workflowSkill := workflowSkillName(workflow)
			if workflowSkill == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("- [`%s`](../%s/api.md)\n", workflowSkill, workflowSkill))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Instructions\n")
	for _, instruction := range persona.Instructions {
		builder.WriteString(fmt.Sprintf("- %s\n", instruction))
	}
	builder.WriteString("\n")
	if len(persona.Tips) > 0 {
		builder.WriteString("## Tips\n")
		for _, tip := range persona.Tips {
			builder.WriteString(fmt.Sprintf("- %s\n", tip))
		}
		builder.WriteString("\n")
	}
	return finalizeMarkdown(builder.String())
}

func renderRecipeSkill(recipe RecipeEntry, skillName string, availableServices map[string]struct{}) string {
	var builder strings.Builder
	required, missing := resolveRequiredServiceSkills(recipe.serviceRefs(), availableServices)
	builder.WriteString(renderFrontmatter(frontmatterSpec{
		Name:           skillName,
		Description:    recipe.Description,
		Category:       skillCategoryRecipe,
		Domain:         safeSkillSegment(recipe.Category),
		RequiresSkills: required,
	}))
	builder.WriteString(fmt.Sprintf("# %s\n\n", recipe.Title))
	if len(required) > 0 {
		builder.WriteString(fmt.Sprintf("> **PREREQUISITE:** Load the following skills before executing this recipe: %s\n\n", strings.Join(wrapTicks(required), ", ")))
	}
	if len(missing) > 0 {
		builder.WriteString(fmt.Sprintf("> **NOTE:** Current catalog snapshot does not expose these referenced services yet: %s\n\n", strings.Join(wrapTicks(missing), ", ")))
	}
	builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(recipe.Description)))
	if strings.TrimSpace(recipe.Caution) != "" {
		builder.WriteString(fmt.Sprintf("> [!CAUTION]\n> %s\n\n", strings.TrimSpace(recipe.Caution)))
	}
	builder.WriteString("## Steps\n\n")
	for i, step := range recipe.Steps {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(step)))
	}
	builder.WriteString("\n")
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
	builder.WriteString("# Skills Index\n\n")
	builder.WriteString("> Auto-generated by `dws generate-skills`. Do not edit manually.\n\n")

	sections := []struct {
		Category string
		Heading  string
		SubTitle string
	}{
		{Category: skillCategoryService, Heading: "## Services", SubTitle: "Core DWS product and shared skills."},
		{Category: skillCategoryHelper, Heading: "## Helpers", SubTitle: "Tool-level execution skills."},
		{Category: skillCategoryPersona, Heading: "## Personas", SubTitle: "Role-based skill bundles."},
		{Category: skillCategoryRecipe, Heading: "## Recipes", SubTitle: "Multi-step task sequences with concrete commands."},
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

func renderSkillsCoverageReport(catalog ir.Catalog, availableServices map[string]struct{}, helperCounts map[string]int) string {
	var builder strings.Builder
	builder.WriteString("# Skills Coverage Report\n\n")
	builder.WriteString("> Auto-generated by `dws generate-skills`. Do not edit manually.\n\n")
	builder.WriteString(fmt.Sprintf("- Catalog products: `%d`\n", len(catalog.Products)))
	builder.WriteString(fmt.Sprintf("- Generated services: `%d`\n", len(availableServices)))

	totalHelpers := 0
	for _, count := range helperCounts {
		totalHelpers += count
	}
	builder.WriteString(fmt.Sprintf("- Generated helpers: `%d`\n\n", totalHelpers))

	renderCoverageGroup(&builder, "Legacy17", legacy17CoverageTargets, availableServices, helperCounts)
	renderCoverageGroup(&builder, "Extended22", extended22CoverageTargets, availableServices, helperCounts)
	return finalizeMarkdown(builder.String())
}

func renderCoverageGroup(
	builder *strings.Builder,
	name string,
	targets []string,
	availableServices map[string]struct{},
	helperCounts map[string]int,
) {
	covered := make([]string, 0, len(targets))
	missing := make([]string, 0)
	zeroHelper := make([]string, 0)
	for _, target := range uniqueSkillProducts(targets) {
		if _, ok := availableServices[target]; !ok {
			missing = append(missing, target)
			continue
		}
		covered = append(covered, target)
		if helperCounts[target] == 0 {
			zeroHelper = append(zeroHelper, target)
		}
	}
	sort.Strings(covered)
	sort.Strings(missing)
	sort.Strings(zeroHelper)

	total := len(uniqueSkillProducts(targets))
	builder.WriteString(fmt.Sprintf("## %s\n\n", name))
	builder.WriteString(fmt.Sprintf("- Coverage: `%d/%d`\n", len(covered), total))
	builder.WriteString(fmt.Sprintf("- Coverage rate: `%.1f%%`\n", coverageRate(len(covered), total)))
	if len(missing) == 0 {
		builder.WriteString("- Missing: `none`\n")
	} else {
		builder.WriteString(fmt.Sprintf("- Missing: %s\n", strings.Join(wrapTicks(missing), ", ")))
	}
	if len(zeroHelper) == 0 {
		builder.WriteString("- Services with 0 helpers: `none`\n\n")
	} else {
		builder.WriteString(fmt.Sprintf("- Services with 0 helpers: %s\n\n", strings.Join(wrapTicks(zeroHelper), ", ")))
	}
}

func coverageRate(covered, total int) float64 {
	if total == 0 {
		return 100
	}
	return (float64(covered) / float64(total)) * 100
}

func renderProductCLI(product ir.CanonicalProduct) string {
	lines := []string{
		fmt.Sprintf("# Canonical Product: %s", product.ID),
		"",
		"Generated from shared Tool IR. Do not edit by hand.",
		"",
		fmt.Sprintf("- Display name: %s", safeValue(product.DisplayName, product.ID)),
		fmt.Sprintf("- Server key: `%s`", product.ServerKey),
		fmt.Sprintf("- Endpoint: `%s`", product.Endpoint),
		fmt.Sprintf("- Protocol: `%s`", safeValue(product.NegotiatedProtocolVersion, "unknown")),
		fmt.Sprintf("- Degraded: `%t`", product.Degraded),
		"",
		"## Tools",
		"",
	}
	for _, tool := range product.Tools {
		lines = append(lines, fmt.Sprintf("- `%s`", tool.RPCName))
		lines = append(lines, fmt.Sprintf("  - Path: `%s`", tool.CanonicalPath))
		lines = append(lines, fmt.Sprintf("  - Description: %s", safeValue(tool.Description, tool.Title)))
		lines = append(lines, fmt.Sprintf("  - Flags: %s", renderFlags(cli.BuildFlagSpecs(tool.InputSchema, tool.FlagHints))))
		lines = append(lines, fmt.Sprintf("  - Schema: `%s`", filepath.ToSlash(filepath.Join("docs/generated/schema", safeDocSegment(product.ID), safeDocSegment(tool.RPCName)+".json"))))
	}
	lines = append(lines, "")
	return finalizeMarkdown(strings.Join(lines, "\n"))
}

func renderFlags(specs []cli.FlagSpec) string {
	if len(specs) == 0 {
		return "none"
	}
	flags := make([]string, 0, len(specs))
	for _, spec := range specs {
		label := fmt.Sprintf("`--%s`", spec.FlagName)
		if spec.Shorthand != "" {
			label = label + fmt.Sprintf(" (`-%s`)", spec.Shorthand)
		}
		if spec.Alias != "" && spec.Alias != spec.FlagName {
			label = label + fmt.Sprintf(" alias `--%s`", spec.Alias)
		}
		flags = append(flags, label)
	}
	return strings.Join(flags, ", ")
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

func requiredFields(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		value, ok := entry.(string)
		if ok && value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
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
	desc := strings.TrimSpace(product.Description)
	if desc == "" {
		desc = fmt.Sprintf("DWS %s service.", product.ID)
	}
	if strings.Contains(strings.ToLower(desc), strings.ToLower(base)) {
		return truncateSkillDescription(desc)
	}
	return truncateSkillDescription(fmt.Sprintf("%s: %s", base, desc))
}

func helperDescription(product ir.CanonicalProduct, tool ir.ToolDescriptor) string {
	base := safeValue(product.DisplayName, product.ID)
	desc := safeValue(tool.Description, tool.Title)
	if desc == "" {
		desc = tool.RPCName
	}
	return truncateSkillDescription(fmt.Sprintf("%s: %s", base, desc))
}

func helperSkillName(product ir.CanonicalProduct, tool ir.ToolDescriptor, seen map[string]int) string {
	token := safeSkillSegment(tool.CLIName)
	if token == "unknown" {
		token = safeSkillSegment(tool.RPCName)
	}
	name := fmt.Sprintf("dws-%s-%s", safeSkillSegment(product.ID), token)
	if _, ok := seen[name]; !ok {
		seen[name] = 1
		return name
	}
	seen[name]++
	return fmt.Sprintf("%s-%d", name, seen[name])
}

func isBlockedTool(product ir.CanonicalProduct, tool ir.ToolDescriptor) bool {
	canonical := strings.ToLower(strings.TrimSpace(tool.CanonicalPath))
	if canonical == "" {
		canonical = strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s.%s", product.ID, tool.RPCName)))
	}
	_, blocked := blockedCanonicalTools[canonical]
	return blocked
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
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString(fmt.Sprintf("description: \"%s\"\n", sanitizeSkillDescription(spec.Description)))
	if strings.TrimSpace(spec.CLIHelp) != "" {
		builder.WriteString(fmt.Sprintf("cliHelp: \"%s\"\n", strings.TrimSpace(spec.CLIHelp)))
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
	description = truncateSkillDescription(description)
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

func workflowSkillName(value string) string {
	token := normalizeRegistryToken(value)
	if token == "" {
		return ""
	}
	token = strings.TrimPrefix(token, "recipe-")
	token = safeSkillSegment(token)
	if token == "unknown" {
		return ""
	}
	return "recipe-" + token
}

func indexRelativeLink(entryPath, linkBase string) string {
	entryPath = filepath.ToSlash(strings.TrimSpace(entryPath))
	if entryPath == "" {
		return ""
	}
	linkBase = filepath.ToSlash(strings.TrimSpace(linkBase))
	return strings.TrimPrefix(entryPath, linkBase)
}

func resolveRequiredServiceSkills(services []string, available map[string]struct{}) ([]string, []string) {
	required := make([]string, 0, len(services))
	missing := make([]string, 0)
	for _, service := range uniqueSkillProducts(services) {
		token := safeSkillSegment(service)
		if token == "unknown" {
			continue
		}
		skill := "dws-" + token
		if len(available) == 0 {
			required = append(required, skill)
			continue
		}
		if _, ok := available[token]; ok {
			required = append(required, skill)
		} else {
			missing = append(missing, skill)
		}
	}
	return required, missing
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
