package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

type docsFlagCase struct {
	CommandPath string
	Flag        string
	Source      string
}

func TestDWSDocsCommandTreeCoverage(t *testing.T) {
	docPaths, _, err := parseDWSDocsCompatibility()
	if err != nil {
		t.Fatalf("parseDWSDocsCompatibility() error = %v", err)
	}
	if len(docPaths) == 0 {
		t.Skip("no command paths parsed from docs/dws (directory may not exist)")
	}

	index := buildCommandIndex(app.NewSchemaSourceRootCommand())
	missing := make([]string, 0)
	for _, path := range docPaths {
		if _, ok := index[path]; ok {
			continue
		}
		missing = append(missing, path)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("docs command paths missing in CLI (%d): %v", len(missing), headStrings(missing, 30))
	}
}

func TestDWSDocsLocalFlagsCoverage(t *testing.T) {
	docPaths, docFlags, err := parseDWSDocsCompatibility()
	if err != nil {
		t.Fatalf("parseDWSDocsCompatibility() error = %v", err)
	}
	if len(docFlags) == 0 {
		t.Skip("no local flags parsed from docs/dws (directory may not exist)")
	}
	docLeafSet := leafCommandSet(docPaths)

	index := buildCommandIndex(app.NewSchemaSourceRootCommand())
	missing := make([]string, 0)
	fallbackMatched := 0
	explicitMatched := 0

	for _, tc := range docFlags {
		if _, isLeaf := docLeafSet[tc.CommandPath]; !isLeaf {
			continue
		}
		cmd, ok := index[tc.CommandPath]
		if !ok {
			missing = append(missing, tc.CommandPath+" "+tc.Flag+" ("+tc.Source+")")
			continue
		}
		if commandHasFlag(cmd, tc.Flag) {
			explicitMatched++
			continue
		}
		if commandHasJSONFallback(cmd) {
			fallbackMatched++
			continue
		}
		missing = append(missing, tc.CommandPath+" "+tc.Flag+" ("+tc.Source+")")
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("docs local flags missing in CLI (%d): %v", len(missing), headStrings(missing, 40))
	}

	t.Logf("docs local flags checked=%d explicit=%d fallback_json_params=%d", len(docFlags), explicitMatched, fallbackMatched)
}

func parseDWSDocsCompatibility() ([]string, []docsFlagCase, error) {
	skip := map[string]struct{}{
		"README.md":          {},
		"quick-reference.md": {},
		"scenario-guide.md":  {},
		"range-audit.md":     {},
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "dws", "*.md"))
	if err != nil {
		return nil, nil, err
	}

	commandSet := make(map[string]struct{})
	flagCases := make([]docsFlagCase, 0)
	flagSet := make(map[string]struct{})

	for _, file := range files {
		name := filepath.Base(file)
		if _, ok := skip[name]; ok {
			continue
		}
		paths, flags, parseErr := parseOneDocsFile(file)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		for _, path := range paths {
			commandSet[path] = struct{}{}
		}
		for _, fc := range flags {
			key := fc.CommandPath + "|" + fc.Flag
			if _, ok := flagSet[key]; ok {
				continue
			}
			flagSet[key] = struct{}{}
			flagCases = append(flagCases, fc)
		}
	}

	paths := make([]string, 0, len(commandSet))
	for path := range commandSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	sort.Slice(flagCases, func(i, j int) bool {
		if flagCases[i].CommandPath != flagCases[j].CommandPath {
			return flagCases[i].CommandPath < flagCases[j].CommandPath
		}
		return flagCases[i].Flag < flagCases[j].Flag
	})
	return paths, flagCases, nil
}

func parseOneDocsFile(path string) ([]string, []docsFlagCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(string(data), "\n")

	paths := make([]string, 0)
	flags := make([]docsFlagCase, 0)
	pathSet := make(map[string]struct{})
	flagRegex := regexp.MustCompile(`--[a-zA-Z0-9][a-zA-Z0-9-]*`)

	inTree := false
	currentCommand := ""
	inLocalParams := false
	inLocalTable := false

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## 命令树") {
			inTree = true
			continue
		}
		if inTree && strings.HasPrefix(trimmed, "## ") {
			inTree = false
		}
		if inTree {
			if cmdPath, ok := extractInlineCode(trimmed); ok && strings.HasPrefix(cmdPath, "dws ") {
				if _, exists := pathSet[cmdPath]; !exists {
					pathSet[cmdPath] = struct{}{}
					paths = append(paths, cmdPath)
				}
			}
		}

		if strings.HasPrefix(trimmed, "### `") {
			currentCommand = ""
			inLocalParams = false
			inLocalTable = false
			if cmdPath, ok := extractHeadingCommand(trimmed); ok {
				currentCommand = cmdPath
			}
			continue
		}

		if currentCommand == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "本地参数：") {
			inLocalParams = true
			inLocalTable = false
			continue
		}
		if inLocalParams && strings.Contains(trimmed, "无本地参数") {
			inLocalParams = false
			inLocalTable = false
			continue
		}
		if !inLocalParams {
			continue
		}

		if strings.HasPrefix(trimmed, "| 参数 |") {
			inLocalTable = true
			continue
		}
		if !inLocalTable {
			continue
		}
		if strings.HasPrefix(trimmed, "| ---") {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			inLocalParams = false
			inLocalTable = false
			continue
		}

		cells := parseMarkdownRow(trimmed)
		if len(cells) == 0 {
			continue
		}
		for _, flag := range flagRegex.FindAllString(cells[0], -1) {
			flags = append(flags, docsFlagCase{
				CommandPath: currentCommand,
				Flag:        flag,
				Source:      filepath.Base(path) + ":" + strconv.Itoa(idx+1),
			})
		}
	}

	return paths, flags, nil
}

func extractInlineCode(line string) (string, bool) {
	if !strings.Contains(line, "`") {
		return "", false
	}
	start := strings.IndexByte(line, '`')
	if start < 0 {
		return "", false
	}
	end := strings.IndexByte(line[start+1:], '`')
	if end < 0 {
		return "", false
	}
	value := strings.TrimSpace(line[start+1 : start+1+end])
	if value == "" {
		return "", false
	}
	return value, true
}

func extractHeadingCommand(line string) (string, bool) {
	if !strings.HasPrefix(line, "### `") {
		return "", false
	}
	value, ok := extractInlineCode(line)
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(value, "dws ") {
		return "", false
	}
	return value, true
}

func parseMarkdownRow(row string) []string {
	raw := strings.TrimSpace(row)
	raw = strings.TrimPrefix(raw, "|")
	raw = strings.TrimSuffix(raw, "|")
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func buildCommandIndex(root *cobra.Command) map[string]*cobra.Command {
	index := make(map[string]*cobra.Command)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			path := "dws " + strings.TrimPrefix(child.CommandPath(), "dws ")
			index[path] = child
			walk(child)
		}
	}
	walk(root)
	return index
}

func commandHasFlag(cmd *cobra.Command, longFlag string) bool {
	name := strings.TrimSpace(strings.TrimPrefix(longFlag, "--"))
	if name == "" {
		return false
	}
	return cmd.LocalFlags().Lookup(name) != nil || cmd.InheritedFlags().Lookup(name) != nil
}

func commandHasJSONFallback(cmd *cobra.Command) bool {
	return commandHasFlag(cmd, "--json") && commandHasFlag(cmd, "--params")
}

func headStrings(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	return values[:max]
}

func leafCommandSet(paths []string) map[string]struct{} {
	leaf := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		leaf[path] = struct{}{}
	}
	for _, path := range paths {
		prefix := path + " "
		for _, other := range paths {
			if path == other {
				continue
			}
			if strings.HasPrefix(other, prefix) {
				delete(leaf, path)
				break
			}
		}
	}
	return leaf
}
