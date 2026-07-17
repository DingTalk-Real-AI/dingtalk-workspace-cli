//go:build skill_verify
// +build skill_verify

// Package skill_static_test verifies that every `dws ...` command example
// referenced in skill documentation can actually be dispatched by the
// open-source dws CLI command tree.
//
// Build tag rationale: this is a repository-wide documentation audit and is
// intentionally opt-in rather than part of every package's unit-test loop.
// Run explicitly with: `go test -tags skill_verify ./test/skill_static/...`
//
// Layer A of the skill verification matrix:
//  1. Every backtick `dws ...` command in skills/**/*.md is parsed.
//  2. For each, the sub-command path (tokens before first --flag) is
//     checked against the Cobra tree constructed from the current source.
//  3. Registered aliases are accepted; unknown descendants are rejected.
//  4. Flags in recursively discovered multi-skill docs are checked against
//     the registered leaf command. Mono docs retain historical bad-case
//     examples and are path-checked only.
//
// The test deliberately tolerates a small whitelist of intentional
// anti-pattern references (e.g. "dws minutes detail" appearing in the
// 「高频错误命令对照表」 column of a 错例 → 正解 table). Anything else
// must dispatch cleanly.
package skill_static_test

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

var (
	backtickCmd = regexp.MustCompile("`(dws\\s+[^`]+?)`")

	// Sub-paths intentionally referenced as anti-pattern examples
	// (kept in docs to guide LLMs away from common hallucinations).
	antiPatternAllowlist = map[string]bool{
		"dws calendar list":         true, // calendar.md: "CLI doesn't have this; do not invent it"
		"dws minutes detail":        true, // minutes.md anti-pattern table
		"dws minutes info":          true, // (top-level, not under `get`)
		"dws minutes summary":       true,
		"dws minutes transcribe":    true, // minutes.md anti-pattern table: LLM 凭印象编造的子命令，正解是 `get transcription`
		"dws minutes transcription": true,
		"dws report inbox":          true, // explicit "禁止编造" warnings
		"dws skill add":             true, // backward-compat stub, exists but not in Available Commands
		"dws skill find":            true, // backward-compat stub
	}

	// Argument-like tokens the parser should not mistake for sub-commands.
	dateLikeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	// test/skill_static/skill_static_test.go → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
}

func appendMultiReferenceMarkdown(files []string, multiRoot string) ([]string, error) {
	err := filepath.WalkDir(multiRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(multiRoot, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) >= 3 && parts[1] == "references" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

type cmdRef struct {
	File string
	Line int
	Cmd  string
}

// extractCommands scans skills/**/*.md for every backtick `dws ...` command.
func extractCommands(root string) ([]cmdRef, error) {
	skillsDir := filepath.Join(root, "skills")
	var refs []cmdRef
	files, err := filepath.Glob(skillsDir + "/mono/SKILL.md")
	if err != nil {
		return nil, err
	}
	var more []string
	if more, err = filepath.Glob(skillsDir + "/mono/references/products/*.md"); err == nil {
		files = append(files, more...)
	}
	if more, err = filepath.Glob(skillsDir + "/mono/references/*.md"); err == nil {
		files = append(files, more...)
	}
	if more, err = filepath.Glob(skillsDir + "/multi/*/SKILL.md"); err == nil {
		files = append(files, more...)
	}
	if files, err = appendMultiReferenceMarkdown(files, filepath.Join(skillsDir, "multi")); err != nil {
		return nil, err
	}
	for _, f := range files {
		data, err := readFile(f)
		if err != nil {
			return nil, err
		}
		lineNo := 0
		scanner := bufio.NewScanner(strings.NewReader(data))
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024*4)
		for scanner.Scan() {
			lineNo++
			matches := backtickCmd.FindAllStringSubmatch(scanner.Text(), -1)
			for _, m := range matches {
				cmd := strings.TrimSpace(m[1])
				if shouldSkip(cmd) {
					continue
				}
				refs = append(refs, cmdRef{File: f, Line: lineNo, Cmd: cmd})
			}
		}
	}
	return refs, nil
}

func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// anglePlaceholderRe matches any <...> placeholder. We check raw cmd because
// placeholders with spaces (e.g. <url 或 dentryUuid>) confuse word-based
// parsing — easier to reject upfront.
var anglePlaceholderRe = regexp.MustCompile(`<[^>]*>`)

func shouldSkip(cmd string) bool {
	if strings.HasPrefix(cmd, "dws dev ") || cmd == "dws dev" {
		// dev (product id: devapp) is a target MCP-discovered command tree. Until the MCP product
		// is published into the open-source registry, validate its Agent routing
		// through test/skill_tests.md rather than static cobra dispatch.
		return true
	}
	if strings.Contains(cmd, "[flags]") || strings.Contains(cmd, "[command]") {
		return true
	}
	if strings.Contains(cmd, "|") || strings.Contains(cmd, "$(") || strings.Contains(cmd, " & ") {
		return true
	}
	if strings.Contains(cmd, "...") || strings.Contains(cmd, " > ") {
		return true
	}
	// Markdown optional-flag notation like `[--limit <1-20>]` or `[--yes]`
	// confuses word-based parsing. Treat any "[--" as a templated example.
	if strings.Contains(cmd, "[--") {
		return true
	}
	// Replace placeholders with a safe token, then check sub-path portion
	// for any remaining noise. If the placeholder contains spaces it would
	// already have polluted word-based parsing — easier to skip entirely.
	if anglePlaceholderRe.MatchString(cmd) {
		// Allow only commands where placeholder is entirely inside flag values
		// (after first " --"). Sub-path tokens (before first --flag) must be clean.
		idx := strings.Index(cmd, " --")
		if idx < 0 {
			return true // no --flag, but has <>, so whole cmd is templated
		}
		if anglePlaceholderRe.MatchString(cmd[:idx]) {
			return true
		}
		// Placeholder inside flag values is fine — but if it spans spaces,
		// our word-based parser would break. Detect spaces inside any <...>.
		for _, m := range anglePlaceholderRe.FindAllString(cmd, -1) {
			if strings.Contains(m, " ") {
				return true
			}
		}
	}
	// Reject "/" or "*" within sub-path tokens.
	subPart := cmd
	if idx := strings.Index(cmd, " --"); idx >= 0 {
		subPart = cmd[:idx]
	}
	for _, t := range strings.Fields(subPart) {
		if strings.Contains(t, "/") || strings.Contains(t, "*") {
			return true
		}
	}
	return false
}

// quotedRe matches double-quoted substrings (possibly with spaces inside).
var quotedRe = regexp.MustCompile(`"[^"]*"`)

// parseSubPath returns (sub-path, leaf, flags). sub-path is tokens up to but
// not including the first --flag and any clearly-non-subcommand token.
func parseSubPath(cmd string) (subPath, leaf string, flags []string) {
	// Collapse "quoted strings" to a single placeholder so word-split works.
	cmd = quotedRe.ReplaceAllString(cmd, "__QUOTED__")
	tokens := strings.Fields(cmd)
	var subTokens []string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if strings.HasPrefix(t, "--") || (strings.HasPrefix(t, "-") && len(t) > 1 && !strings.ContainsAny(string(t[1]), "0123456789")) {
			// flag, take name before '='
			flags = append(flags, strings.SplitN(t, "=", 2)[0])
			// skip its value if present
			if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
				i++
			}
			continue
		}
		if len(subTokens) == 0 && t != "dws" {
			break // doesn't start with dws → not a real cmd
		}
		// Stop sub-path on noise tokens
		if dateLikeRe.MatchString(t) || strings.Contains(t, "/") || strings.Contains(t, "*") {
			break
		}
		subTokens = append(subTokens, t)
	}
	if len(subTokens) < 2 {
		return "", "", flags
	}
	subPath = strings.Join(subTokens, " ")
	leaf = subTokens[len(subTokens)-1]
	return subPath, leaf, flags
}

// exactCommand walks the current source tree's Cobra command graph without
// executing a subprocess. Aliases are accepted because they dispatch to the
// same registered command at runtime.
func exactCommand(root *cobra.Command, sub string) *cobra.Command {
	tokens := strings.Fields(strings.TrimPrefix(sub, "dws "))
	current := root
	for i, token := range tokens {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == token || child.HasAlias(token) {
				next = child
				break
			}
		}
		if next == nil {
			// Once a runnable command is reached, remaining non-flag tokens are
			// positional arguments rather than descendants. Validate them with
			// the command's Cobra Args contract when one is present.
			if current.Runnable() {
				args := tokens[i:]
				if current.Args == nil || current.Args(current, args) == nil {
					return current
				}
			}
			return nil
		}
		current = next
	}
	return current
}

func commandHasFlag(command *cobra.Command, flag string) bool {
	flag = strings.TrimLeft(flag, "-")
	if flag == "" {
		return false
	}
	command.InitDefaultHelpFlag()
	if command.Flags().Lookup(flag) != nil || command.PersistentFlags().Lookup(flag) != nil || command.InheritedFlags().Lookup(flag) != nil {
		return true
	}
	if len(flag) == 1 {
		return command.Flags().ShorthandLookup(flag) != nil || command.PersistentFlags().ShorthandLookup(flag) != nil || command.InheritedFlags().ShorthandLookup(flag) != nil
	}
	return false
}

func availableCommands(root *cobra.Command, sub string) []string {
	tokens := strings.Fields(strings.TrimPrefix(sub, "dws "))
	if len(tokens) > 0 {
		tokens = tokens[:len(tokens)-1]
	}
	parent := root
	for _, token := range tokens {
		next := exactCommand(parent, "dws "+token)
		if next == nil {
			return nil
		}
		parent = next
	}
	var names []string
	for _, child := range parent.Commands() {
		names = append(names, child.Name())
	}
	return names
}

func TestSkillCommandsDispatch(t *testing.T) {
	repo := repoRoot(t)
	// Keep command-tree construction isolated from the developer's plugins and
	// macOS Keychain while validating the open-source surface.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_DISABLE_KEYCHAIN", "1")
	t.Setenv("DWS_KEYCHAIN_DIR", t.TempDir())
	commandRoot := app.NewRootCommand()

	refs, err := extractCommands(repo)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	t.Logf("Extracted %d command references from skill docs", len(refs))

	type failure struct {
		ref    cmdRef
		reason string
	}
	var fails []failure
	seen := map[string]bool{} // dedupe identical sub-path and flag sets

	for _, r := range refs {
		sub, _, flags := parseSubPath(r.Cmd)
		if sub == "" {
			continue
		}
		if antiPatternAllowlist[sub] {
			continue
		}
		key := sub + "|" + strings.Join(flags, ",") + "|" + r.File
		if seen[key] {
			continue
		}
		seen[key] = true
		if command := exactCommand(commandRoot, sub); command != nil {
			var unknownFlags []string
			if strings.Contains(filepath.ToSlash(r.File), "/skills/multi/") {
				for _, flag := range flags {
					if !commandHasFlag(command, flag) {
						unknownFlags = append(unknownFlags, flag)
					}
				}
			}
			if len(unknownFlags) > 0 {
				fails = append(fails, failure{ref: r, reason: fmt.Sprintf("unknown flags for `%s`: %v", sub, unknownFlags)})
			}
			continue
		}
		tokens := strings.Fields(sub)
		if len(tokens) < 2 {
			continue
		}
		parent := strings.Join(tokens[:len(tokens)-1], " ")
		leaf := tokens[len(tokens)-1]
		available := availableCommands(commandRoot, sub)
		fails = append(fails, failure{ref: r, reason: fmt.Sprintf("`%s` not in Available Commands of `%s`: %v", leaf, parent, available)})
	}

	if len(fails) == 0 {
		t.Logf("✓ All extracted commands dispatch cleanly (sub-paths registered, anti-patterns whitelisted)")
		return
	}

	// Format failure report
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skill-doc commands that don't dispatch in the current open-source dws command tree:\n\n", len(fails)))
	for _, f := range fails {
		rel, _ := filepath.Rel(repo, f.ref.File)
		sb.WriteString(fmt.Sprintf("  ❌ %s:%d\n     cmd: %s\n     reason: %s\n\n", rel, f.ref.Line, f.ref.Cmd, f.reason))
	}
	t.Fatal(sb.String())
}
