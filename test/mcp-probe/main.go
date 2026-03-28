// dws-cli-parity-checker is a standalone black-box acceptance tool that verifies
// the dws CLI surface against the schema emitted by `dws schema`.
//
// How it works:
//  1. Run `dws schema` to discover the catalog.
//  2. For each product/tool, fetch focused schema metadata from `dws schema`.
//  3. Start a transparent HTTP proxy and rewrite catalog endpoints to the proxy.
//  4. Generate synthetic arguments from each tool's input_schema.
//  5. Invoke the CLI through multiple parameter surfaces and compare captured
//     MCP `tools/call` payloads against the schema-derived arguments.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/checker"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/proxy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/runner"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/schema"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/surface"
)

type checkResult struct {
	label  string
	passed bool
	diff   string
}

type invocationVariant struct {
	label    string
	cliArgs  []string
	expected map[string]any
}

func main() {
	dwsBinary := flag.String("dws", "dws", "path to dws binary")
	truthDWSBinary := flag.String("truth-dws", "", "path to truth-source dws binary used for help-surface parity checks")
	filterFlag := flag.String("filter", "", "only run tools whose schema path or cli path contains this string")
	verbose := flag.Bool("verbose", false, "print detailed output including command stderr/stdout")
	flag.Parse()

	ctx := context.Background()

	catalogJSON, err := fetchSchema(ctx, *dwsBinary, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to fetch catalog schema: %v\n", err)
		os.Exit(1)
	}

	catalog, err := schema.ParseCatalog(catalogJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to parse catalog schema: %v\n", err)
		os.Exit(1)
	}

	realEndpoint := firstCatalogEndpoint(catalog)
	if realEndpoint == "" {
		fmt.Fprintf(os.Stderr, "ERROR: catalog has no products with endpoints\n")
		os.Exit(1)
	}

	mcpProxy, err := proxy.New(realEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to start proxy: %v\n", err)
		os.Exit(1)
	}
	defer mcpProxy.Close()

	tools, err := discoverTools(ctx, *dwsBinary, catalog, *filterFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to discover tool schemas: %v\n", err)
		os.Exit(1)
	}

	fixtureCatalog := schema.BuildFixtureCatalog(catalog, tools).WithProxyEndpoint(mcpProxy.URL())
	fixtureFile, err := writeCatalogFixture(fixtureCatalog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to write catalog fixture: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(fixtureFile)

	dwsRunner := &runner.Runner{
		DWSBinary:          *dwsBinary,
		CatalogFixturePath: fixtureFile,
		ExtraEnv:           inheritAuthEnv(),
		Timeout:            30 * time.Second,
	}

	fmt.Printf("=== dws cli parity checker ===\n")
	fmt.Printf("Binary:      %s\n", *dwsBinary)
	if strings.TrimSpace(*truthDWSBinary) != "" {
		fmt.Printf("Truth:       %s\n", *truthDWSBinary)
	}
	fmt.Printf("Proxy:       %s → %s\n", mcpProxy.URL(), realEndpoint)
	if *filterFlag != "" {
		fmt.Printf("Filter:      %s\n", *filterFlag)
	}
	fmt.Printf("Tools:       %d discovered\n", len(tools))
	if strings.TrimSpace(*truthDWSBinary) != "" {
		fmt.Printf("Surface:     enabled\n")
	}
	fmt.Printf("\n")

	passed, failed := 0, 0
	for _, tool := range tools {
		result := runTool(ctx, tool, dwsRunner, mcpProxy, *verbose)
		if result.passed {
			fmt.Printf("[PASS] %s\n", result.label)
			passed++
			continue
		}
		fmt.Printf("[FAIL] %s\n", result.label)
		if result.diff != "" {
			indented := strings.ReplaceAll(result.diff, "\n", "\n       ")
			fmt.Printf("       %s\n", indented)
		}
		failed++
	}

	if strings.TrimSpace(*truthDWSBinary) != "" {
		supportedRoots := supportedSurfaceRoots(catalog)
		results, err := runTruthSurfaceParity(
			ctx,
			&runner.Runner{
				DWSBinary: *truthDWSBinary,
				ExtraEnv:  helpEnv(),
				Timeout:   30 * time.Second,
			},
			&runner.Runner{
				DWSBinary: *dwsBinary,
				ExtraEnv:  helpEnv(),
				Timeout:   30 * time.Second,
			},
			supportedRoots,
			*filterFlag,
			*verbose,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to run truth-source help surface parity: %v\n", err)
			os.Exit(1)
		}
		for _, result := range results {
			if result.passed {
				fmt.Printf("[PASS] %s\n", result.label)
				passed++
				continue
			}
			fmt.Printf("[FAIL] %s\n", result.label)
			if result.diff != "" {
				indented := strings.ReplaceAll(result.diff, "\n", "\n       ")
				fmt.Printf("       %s\n", indented)
			}
			failed++
		}
	}

	fmt.Printf("\nResults: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func runTruthSurfaceParity(
	ctx context.Context,
	truthRunner *runner.Runner,
	candidateRunner *runner.Runner,
	supportedRoots map[string]struct{},
	filter string,
	verbose bool,
) ([]checkResult, error) {
	pages, err := surface.Crawl(ctx, truthRunner)
	if err != nil {
		return nil, err
	}

	results := make([]checkResult, 0, len(pages))
	for _, expected := range pages {
		if !isSupportedSurfacePath(expected.Path, supportedRoots) {
			continue
		}
		if !matchesSurfaceFilter(filter, expected.Path) {
			continue
		}

		args := helpArgs(expected.Path)
		result, err := candidateRunner.Run(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("run candidate help for %q: %w", strings.Join(expected.Path, " "), err)
		}

		label := surfaceLabel(expected.Path)
		if verbose {
			fmt.Printf("  [surface] %s\n", label)
			fmt.Printf("  [args] %s\n", strings.Join(args, " "))
			if result.Stdout != "" {
				fmt.Printf("  [stdout] %s\n", result.Stdout)
			}
			if result.Stderr != "" {
				fmt.Printf("  [stderr] %s\n", result.Stderr)
			}
		}

		if result.ExitCode != 0 {
			results = append(results, checkResult{
				label: label,
				diff:  fmt.Sprintf("candidate command help failed\nstderr: %s", result.Stderr),
			})
			continue
		}

		actual := surface.ParsePage(expected.Path, result.Stdout)
		diff := surface.ComparePage(expected, actual)
		if diff.Equal {
			results = append(results, checkResult{label: label, passed: true})
			continue
		}

		results = append(results, checkResult{
			label: label,
			diff:  formatSurfaceDiff(expected, actual, diff),
		})
	}

	return results, nil
}

func runTool(
	ctx context.Context,
	tool schema.ToolSchema,
	dwsRunner *runner.Runner,
	mcpProxy *proxy.Proxy,
	verbose bool,
) checkResult {
	variants, err := buildVariants(tool)
	if err != nil {
		return checkResult{
			label: toolLabel(tool),
			diff:  fmt.Sprintf("build variants: %v", err),
		}
	}

	for _, variant := range variants {
		mcpProxy.DrainCalls()

		result, err := dwsRunner.Run(ctx, append(append([]string{}, variant.cliArgs...), "--yes"))
		if err != nil {
			return checkResult{
				label: variant.label,
				diff:  fmt.Sprintf("exec error: %v", err),
			}
		}
		if verbose {
			fmt.Printf("  [tool] %s\n", variant.label)
			fmt.Printf("  [args] %s\n", strings.Join(variant.cliArgs, " "))
			if result.Stdout != "" {
				fmt.Printf("  [stdout] %s\n", result.Stdout)
			}
			if result.Stderr != "" {
				fmt.Printf("  [stderr] %s\n", result.Stderr)
			}
		}
		if result.ExitCode != 0 {
			return checkResult{
				label: variant.label,
				diff:  fmt.Sprintf("dws exited with %d (stderr=%q)", result.ExitCode, result.Stderr),
			}
		}

		capturedCalls := mcpProxy.DrainCalls()
		if len(capturedCalls) != 1 {
			return checkResult{
				label: variant.label,
				diff:  fmt.Sprintf("expected 1 tools/call, captured %d", len(capturedCalls)),
			}
		}

		captured := capturedCalls[0]
		if captured.ToolName != tool.Tool.RPCName {
			return checkResult{
				label: variant.label,
				diff:  fmt.Sprintf("tool mismatch: expected %q, got %q", tool.Tool.RPCName, captured.ToolName),
			}
		}

		compare := checker.CompareArguments(variant.expected, captured.Arguments)
		if !compare.Equal {
			return checkResult{
				label: variant.label,
				diff: fmt.Sprintf(
					"arguments mismatch\nexpected: %s\nactual:   %s\n%s",
					marshalCompact(variant.expected),
					marshalCompact(captured.Arguments),
					compare.Diff,
				),
			}
		}
	}

	return checkResult{label: toolLabel(tool), passed: true}
}

func buildVariants(tool schema.ToolSchema) ([]invocationVariant, error) {
	rawArgs, err := tool.GenerateArguments()
	if err != nil {
		return nil, err
	}
	expected, err := tool.NormalizeArguments(rawArgs)
	if err != nil {
		return nil, err
	}

	primaryArgs, err := schema.BuildFlagArgs(tool, rawArgs, false)
	if err != nil {
		return nil, err
	}

	var variants []invocationVariant
	variants = append(variants, invocationVariant{
		label:    toolLabel(tool) + " [flags]",
		cliArgs:  primaryArgs,
		expected: expected,
	})

	if schema.HasUsableAliases(tool.Flags) {
		aliasArgs, err := schema.BuildFlagArgs(tool, rawArgs, true)
		if err != nil {
			return nil, err
		}
		variants = append(variants, invocationVariant{
			label:    toolLabel(tool) + " [aliases]",
			cliArgs:  aliasArgs,
			expected: expected,
		})
	}

	if schema.HasFlattenedNestedFlags(tool.Flags) {
		jsonFlatArgs, err := schema.BuildPublicJSONArgs(tool, rawArgs, "--json", false)
		if err != nil {
			return nil, err
		}
		variants = append(variants, invocationVariant{
			label:    toolLabel(tool) + " [json-flat]",
			cliArgs:  jsonFlatArgs,
			expected: expected,
		})
	}

	jsonArgs, err := schema.BuildJSONArgs(tool, rawArgs, "--json")
	if err != nil {
		return nil, err
	}
	variants = append(variants, invocationVariant{
		label:    toolLabel(tool) + " [json]",
		cliArgs:  jsonArgs,
		expected: expected,
	})

	return variants, nil
}

func formatSurfaceDiff(expected, actual surface.Page, diff surface.Diff) string {
	lines := append([]string{}, diff.Details...)
	if containsDiff(diff.Details, "available commands mismatch") {
		lines = append(lines,
			fmt.Sprintf("expected commands: %s", marshalCompact(surfaceCommandNames(expected.Available))),
			fmt.Sprintf("actual commands:   %s", marshalCompact(surfaceCommandNames(actual.Available))),
		)
	}
	if containsDiff(diff.Details, "local flags mismatch") {
		lines = append(lines,
			fmt.Sprintf("expected flags: %s", marshalCompact(surfaceFlagNames(expected.LocalFlags))),
			fmt.Sprintf("actual flags:   %s", marshalCompact(surfaceFlagNames(actual.LocalFlags))),
		)
	}
	return strings.Join(lines, "\n")
}

func discoverTools(ctx context.Context, dwsBinary string, catalog schema.Catalog, filter string) ([]schema.ToolSchema, error) {
	discovered := make([]schema.ToolSchema, 0)
	for _, product := range catalog.Products {
		productQuery := strings.TrimSpace(product.Command)
		if productQuery == "" {
			productQuery = product.ID
		}

		productJSON, err := fetchSchema(ctx, dwsBinary, productQuery)
		if err != nil {
			return nil, fmt.Errorf("fetch product schema %q: %w", productQuery, err)
		}

		productSchema, err := schema.ParseProduct(productJSON)
		if err != nil {
			return nil, fmt.Errorf("parse product schema %q: %w", productQuery, err)
		}

		for _, toolSummary := range productSchema.Tools {
			if filter != "" && !matchesFilter(filter, productSchema.Product, toolSummary) {
				continue
			}

			toolJSON, err := fetchSchema(ctx, dwsBinary, toolSummary.CanonicalPath)
			if err != nil {
				return nil, fmt.Errorf("fetch tool schema %q: %w", toolSummary.CanonicalPath, err)
			}

			toolSchema, err := schema.ParseTool(toolJSON)
			if err != nil {
				return nil, fmt.Errorf("parse tool schema %q: %w", toolSummary.CanonicalPath, err)
			}
			if toolSchema.Tool.RPCName == "" {
				toolSchema.Tool = toolSummary
			}
			if len(toolSchema.CLIPath) == 0 {
				toolSchema.CLIPath = append(toolSchema.CLIPath, toolSummary.CLIPath...)
			}
			if len(toolSchema.Flags) == 0 {
				toolSchema.Flags = append(toolSchema.Flags, toolSummary.Flags...)
			}
			if len(toolSchema.Required) == 0 {
				toolSchema.Required = append(toolSchema.Required, toolSummary.Required...)
			}
			discovered = append(discovered, toolSchema)
		}
	}

	sort.Slice(discovered, func(i, j int) bool {
		return toolLabel(discovered[i]) < toolLabel(discovered[j])
	})
	return discovered, nil
}

func fetchSchema(ctx context.Context, dwsBinary string, query string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := []string{"schema"}
	if strings.TrimSpace(query) != "" {
		args = append(args, query)
	}

	cmd := exec.CommandContext(ctx, dwsBinary, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("dws %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func writeCatalogFixture(catalog schema.FixtureCatalog) (string, error) {
	data, err := json.Marshal(catalog)
	if err != nil {
		return "", fmt.Errorf("marshal catalog: %w", err)
	}
	file, err := os.CreateTemp("", "dws-mcp-probe-fixture-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("write fixture: %w", err)
	}
	return file.Name(), nil
}

func inheritAuthEnv() []string {
	authPrefixes := []string{
		"DWS_",
		"DINGTALK_",
		"HOME",
		"PATH",
		"USER",
		"TMPDIR",
		"XDG_",
	}
	var env []string
	for _, entry := range os.Environ() {
		for _, prefix := range authPrefixes {
			if strings.HasPrefix(entry, prefix) {
				env = append(env, entry)
				break
			}
		}
	}
	return env
}

func firstCatalogEndpoint(catalog schema.Catalog) string {
	for _, product := range catalog.Products {
		if strings.TrimSpace(product.Endpoint) != "" {
			return product.Endpoint
		}
	}
	return ""
}

func matchesFilter(filter string, product schema.Product, tool schema.Tool) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}

	candidates := []string{
		product.ID,
		product.Command,
		tool.CanonicalPath,
		tool.RPCName,
		tool.CLIName,
		strings.Join(tool.CLIPath, " "),
		strings.Join(tool.CLIPath, "."),
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), filter) {
			return true
		}
	}
	return false
}

func matchesSurfaceFilter(filter string, path []string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}

	candidates := []string{
		strings.Join(path, " "),
		strings.Join(path, "."),
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), filter) {
			return true
		}
	}
	return false
}

func toolLabel(tool schema.ToolSchema) string {
	if strings.TrimSpace(tool.Path) != "" {
		return tool.Path
	}
	if strings.TrimSpace(tool.Tool.CanonicalPath) != "" {
		return tool.Tool.CanonicalPath
	}
	return strings.Join(tool.CLIPath, " ")
}

func surfaceLabel(path []string) string {
	if len(path) == 0 {
		return "root [truth-surface]"
	}
	return strings.Join(path, " ") + " [truth-surface]"
}

func marshalCompact(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func containsDiff(values []string, needle string) bool {
	return slicesContains(values, needle)
}

func supportedSurfaceRoots(catalog schema.Catalog) map[string]struct{} {
	out := make(map[string]struct{}, len(catalog.Products))
	for _, product := range catalog.Products {
		command := strings.TrimSpace(product.Command)
		if command == "" {
			command = strings.TrimSpace(product.ID)
		}
		if command == "" {
			continue
		}
		out[command] = struct{}{}
	}
	return out
}

func helpEnv() []string {
	env := inheritAuthEnv()
	env = append(env,
		"COLUMNS=200",
		"NO_COLOR=1",
		"TERM=dumb",
	)
	return env
}

func helpArgs(path []string) []string {
	args := append([]string{}, path...)
	args = append(args, "--help")
	return args
}

func slicesContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func isSupportedSurfacePath(path []string, supportedRoots map[string]struct{}) bool {
	if len(path) == 0 {
		return false
	}
	if len(supportedRoots) == 0 {
		return true
	}
	_, ok := supportedRoots[path[0]]
	return ok
}

func surfaceCommandNames(entries []surface.CommandEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

func surfaceFlagNames(entries []surface.FlagEntry) [][]string {
	out := make([][]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, append([]string{}, entry.Names...))
	}
	return out
}
