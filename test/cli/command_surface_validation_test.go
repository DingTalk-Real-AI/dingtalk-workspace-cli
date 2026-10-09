package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var skillCommandRefRe = regexp.MustCompile("`(dws\\s+[^`]+?)`")

func TestCommandIndexMatchesVisibleCommandTree(t *testing.T) {
	t.Parallel()

	indexPaths, err := commandIndexPaths(filepath.Join("..", "..", "docs", "command-index.md"))
	if err != nil {
		t.Fatalf("commandIndexPaths() error = %v", err)
	}
	treePaths := visibleCommandPaths(app.NewRootCommand())

	missingFromTree, missingFromIndex := diffStringSets(indexPaths, treePaths)
	if len(missingFromTree) > 0 || len(missingFromIndex) > 0 {
		t.Fatalf(
			"command index drift: missing_from_tree=%v missing_from_index=%v",
			headStrings(missingFromTree, 30),
			headStrings(missingFromIndex, 30),
		)
	}
	t.Logf("command index matches visible command tree: commands=%d", len(indexPaths))
}

func TestSkillMonoAndMultiCommandReferencesDoNotDrift(t *testing.T) {
	t.Parallel()

	index := buildCommandAliasIndex(app.NewRootCommand())
	monoRefs, err := skillCommandRefs(filepath.Join("..", "..", "skills", "mono"))
	if err != nil {
		t.Fatalf("mono skillCommandRefs() error = %v", err)
	}
	multiRefs, err := skillCommandRefs(filepath.Join("..", "..", "skills", "multi"))
	if err != nil {
		t.Fatalf("multi skillCommandRefs() error = %v", err)
	}

	monoFailures := unresolvedSkillRefs(monoRefs, index)
	multiFailures := unresolvedSkillRefs(multiRefs, index)
	if len(monoFailures) > 0 || len(multiFailures) > 0 {
		t.Fatalf(
			"skill command reference drift: mono=%v multi=%v",
			headStrings(monoFailures, 30),
			headStrings(multiFailures, 30),
		)
	}
	t.Logf("skill command references resolved: mono=%d multi=%d", len(monoRefs), len(multiRefs))
}

func TestVisibleCommandHelpRenders(t *testing.T) {
	t.Parallel()

	paths := visibleCommandPaths(app.NewRootCommand())
	if len(paths) == 0 {
		t.Fatal("no visible commands found")
	}

	var failures []string
	for _, path := range paths {
		root := app.NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append(commandArgs(path), "--help"))
		if err := root.Execute(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if strings.TrimSpace(out.String()) == "" {
			failures = append(failures, path+": empty help output")
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("visible command help failures (%d/%d): %v", len(failures), len(paths), headStrings(failures, 30))
	}
	t.Logf("visible command help rendered for %d commands", len(paths))
}

type skillCommandRef struct {
	Path string
	Line int
	Cmd  string
}

func skillCommandRefs(root string) ([]skillCommandRef, error) {
	var refs []skillCommandRef
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if isIntentionalInvalidSkillLine(line) {
				continue
			}
			for _, match := range skillCommandRefRe.FindAllStringSubmatch(line, -1) {
				cmd := strings.TrimSpace(match[1])
				if shouldSkipSkillCommandRef(cmd) {
					continue
				}
				refs = append(refs, skillCommandRef{Path: path, Line: i + 1, Cmd: cmd})
			}
		}
		return nil
	})
	return refs, err
}

func unresolvedSkillRefs(refs []skillCommandRef, index map[string]*cobra.Command) []string {
	var failures []string
	seen := make(map[string]struct{})
	for _, ref := range refs {
		path := skillCommandPath(ref.Cmd, index)
		if path == "" {
			key := ref.Path + ":" + fmt.Sprint(ref.Line) + " " + ref.Cmd
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			failures = append(failures, key)
		}
	}
	sort.Strings(failures)
	return failures
}

func skillCommandPath(cmd string, index map[string]*cobra.Command) string {
	cmd = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(cmd, "value")
	tokens := strings.Fields(cmd)
	if len(tokens) < 2 || tokens[0] != "dws" {
		return ""
	}
	var path []string
	for i, token := range tokens {
		if i > 0 && strings.HasPrefix(token, "-") {
			if len(path) <= 1 {
				// Global flag before the subcommand path, e.g. --profile.
				continue
			}
			break
		}
		if strings.ContainsAny(token, "*/|") || strings.HasPrefix(token, "$") {
			break
		}
		if strings.HasPrefix(token, "<") || strings.HasPrefix(token, "[") {
			if len(path) <= 1 && i > 0 {
				// Placeholder value of a preceding global flag, e.g. <x>.
				continue
			}
			break
		}
		path = append(path, token)
	}
	for len(path) >= 2 {
		candidate := strings.Join(path, " ")
		if _, ok := index[candidate]; ok {
			return candidate
		}
		path = path[:len(path)-1]
	}
	return ""
}

func shouldSkipSkillCommandRef(cmd string) bool {
	if strings.Contains(cmd, "<cmd>") {
		return true
	}
	if fields := strings.Fields(cmd); len(fields) >= 2 && (strings.HasPrefix(fields[1], "<") || strings.HasPrefix(fields[1], "$")) {
		// Generic usage illustration such as `dws <cli_path> --help`; there is
		// no concrete subcommand path to validate.
		return true
	}
	if strings.Contains(cmd, "[flags]") || strings.Contains(cmd, "[command]") || strings.Contains(cmd, "[--") {
		return true
	}
	if strings.Contains(cmd, "...") || strings.Contains(cmd, " > ") || strings.Contains(cmd, "$(") || strings.Contains(cmd, " & ") {
		return true
	}
	if strings.HasPrefix(cmd, "dws dev ") || cmd == "dws dev" {
		return true
	}
	return false
}

func buildCommandAliasIndex(root *cobra.Command) map[string]*cobra.Command {
	index := buildCommandIndex(root)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			baseParts := strings.Fields(child.CommandPath())
			if len(baseParts) > 0 {
				for _, alias := range child.Aliases {
					aliasParts := append([]string(nil), baseParts...)
					aliasParts[len(aliasParts)-1] = alias
					index[strings.Join(aliasParts, " ")] = child
				}
			}
			walk(child)
		}
	}
	walk(root)
	return index
}

func isIntentionalInvalidSkillLine(line string) bool {
	markers := []string{
		"[禁止]", "【禁止】", "禁止使用", "禁止编造", "错误写法", "高频错误", "反例",
		"不存在", "不要写", "不要用", "反模式", "错例", "该命令不存在",
		"不是合法", "unknown flag", "LLM 高频幻觉", "臆造", "虚构", "编造",
		"不识别", "不认识", "应该用", "应为", "废弃", "已下线",
	}
	for _, marker := range markers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func commandIndexPaths(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| `dws ") {
			continue
		}
		start := strings.IndexByte(line, '`')
		if start < 0 {
			continue
		}
		end := strings.IndexByte(line[start+1:], '`')
		if end < 0 {
			continue
		}
		path := line[start+1 : start+1+end]
		path = strings.TrimSuffix(path, " [command]")
		set[path] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func diffStringSets(left, right []string) ([]string, []string) {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	var missingFromRight []string
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			missingFromRight = append(missingFromRight, value)
		}
	}
	var missingFromLeft []string
	for _, value := range right {
		if _, ok := leftSet[value]; !ok {
			missingFromLeft = append(missingFromLeft, value)
		}
	}
	return missingFromRight, missingFromLeft
}

func TestVisibleCommandFlagsAcceptRepresentativeValues(t *testing.T) {
	t.Parallel()

	commands := visibleCommands(app.NewRootCommand())
	if len(commands) == 0 {
		t.Fatal("no visible commands found")
	}

	checked := 0
	var failures []string
	for _, cmd := range commands {
		for _, flags := range []*pflag.FlagSet{cmd.LocalFlags(), cmd.InheritedFlags()} {
			flags.VisitAll(func(flag *pflag.Flag) {
				if flag.Hidden {
					return
				}
				value, ok := representativeFlagValue(flag)
				if !ok {
					failures = append(failures, fmt.Sprintf("%s --%s: unsupported flag type %q", cmd.CommandPath(), flag.Name, flag.Value.Type()))
					return
				}
				checked++
				if err := flag.Value.Set(value); err != nil {
					failures = append(failures, fmt.Sprintf("%s --%s=%q: %v", cmd.CommandPath(), flag.Name, value, err))
				}
			})
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("visible flag value failures (%d/%d): %v", len(failures), checked, headStrings(failures, 50))
	}
	t.Logf("visible command flags accepted representative values: commands=%d flags=%d", len(commands), checked)
}

func visibleCommandPaths(root *cobra.Command) []string {
	commands := visibleCommands(root)
	paths := make([]string, 0, len(commands))
	for _, cmd := range commands {
		paths = append(paths, cmd.CommandPath())
	}
	paths = append(paths, "dws help")
	sort.Strings(paths)
	return paths
}

func visibleCommands(root *cobra.Command) []*cobra.Command {
	var commands []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := append([]*cobra.Command(nil), cmd.Commands()...)
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if child.Hidden {
				continue
			}
			commands = append(commands, child)
			walk(child)
		}
	}
	walk(root)
	return commands
}

func commandArgs(path string) []string {
	path = strings.TrimSpace(strings.TrimPrefix(path, "dws"))
	if path == "" {
		return nil
	}
	return strings.Fields(path)
}

func representativeFlagValue(flag *pflag.Flag) (string, bool) {
	if flag == nil || flag.Value == nil {
		return "", false
	}
	switch flag.Value.Type() {
	case "bool":
		return "true", true
	case "count":
		return "1", true
	case "duration":
		return (time.Second).String(), true
	case "float32", "float64":
		return "1.5", true
	case "int", "int8", "int16", "int32", "int64":
		return "1", true
	case "intSlice":
		return "1,2", true
	case "ip":
		return "127.0.0.1", true
	case "ipMask":
		return "255.255.255.0", true
	case "ipNet":
		return "127.0.0.0/24", true
	case "string":
		return representativeStringFlagValue(flag), true
	case "stringArray", "stringSlice":
		return representativeStringFlagValue(flag), true
	case "stringToString":
		return "key=value", true
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "1", true
	default:
		return "", false
	}
}

func representativeStringFlagValue(flag *pflag.Flag) string {
	switch flag.Name {
	case "format":
		return "json"
	case "output":
		return "out.json"
	case "timeout":
		return "30"
	default:
		if flag.DefValue != "" && flag.DefValue != "[]" {
			return flag.DefValue
		}
		return "value"
	}
}
