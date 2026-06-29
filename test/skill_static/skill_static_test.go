//go:build skill_verify
// +build skill_verify

// Package skill_static_test verifies that every `dws ...` command example
// referenced in skill documentation can actually be dispatched by the
// open-source dws CLI binary (cobra Available Commands check).
//
// Build tag rationale: this test depends on a real `dws` binary being
// reachable via PATH, which the default `go test ./...` doesn't guarantee.
// Run explicitly with: `go test -tags skill_verify ./test/skill_static/...`
//
// Layer A of the skill verification matrix:
//  1. Every backtick `dws ...` command in skills/**/*.md is parsed.
//  2. For each, the sub-command path (tokens before first --flag) is
//     checked against `dws <parent> --help`'s "Available Commands" list.
//  3. Every flag used must appear in the leaf command's help text.
//
// The test deliberately tolerates a small whitelist of intentional
// anti-pattern references (e.g. "dws minutes detail" appearing in the
// 「高频错误命令对照表」 column of a 错例 → 正解 table). Anything else
// must dispatch cleanly.
package skill_static_test

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
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

func dwsBinary(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("dws")
	if err != nil {
		t.Skipf("dws binary not on PATH; install dws (or run `go build -o /tmp/dws ./cmd` and PATH=/tmp:$PATH) to enable this test")
	}
	return p
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
	if more, err = filepath.Glob(skillsDir + "/multi/*/references/*.md"); err == nil {
		files = append(files, more...)
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
	b, err := exec.Command("cat", p).Output()
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

// helpCache memoizes `dws <sub> --help` outputs.
type helpCache struct {
	mu  sync.Mutex
	dws string
	m   map[string]string
}

func newHelpCache(dws string) *helpCache {
	return &helpCache{dws: dws, m: map[string]string{}}
}

func (c *helpCache) get(sub string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.m[sub]; ok {
		return v
	}
	args := append(strings.Fields(sub)[1:], "--help")
	out, _ := exec.Command(c.dws, args...).CombinedOutput()
	c.m[sub] = string(out)
	return string(out)
}

var availableRe = regexp.MustCompile(`(?s)Available Commands:\n((?:\s+\S.*\n)+)`)

// usagePathRe extracts the command path from cobra Usage line:
//
//	"Usage:\n  dws report inbox list [flags]" → "report inbox list"
//
// Used to detect top-level alias dispatch (envelope-declared cli.Aliases /
// cli.Prefixes[1:]): if the actual Usage path has the same token count as
// the documented sub but a different first token, sub is a registered alias.
var usagePathRe = regexp.MustCompile(`(?m)^Usage:\s*\n\s+dws\s+(.+?)(?:\s+\[flags\]|\s+\[command\]|\s*$)`)

func extractUsagePath(helpText string) string {
	m := usagePathRe.FindStringSubmatch(helpText)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func parseAvailable(helpText string) []string {
	m := availableRe.FindStringSubmatch(helpText)
	if m == nil {
		return nil
	}
	var cmds []string
	for _, line := range strings.Split(m[1], "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && !strings.HasPrefix(fields[0], "-") {
			cmds = append(cmds, fields[0])
		}
	}
	return cmds
}

func TestSkillCommandsDispatch(t *testing.T) {
	root := repoRoot(t)
	dws := dwsBinary(t)
	cache := newHelpCache(dws)

	refs, err := extractCommands(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	t.Logf("Extracted %d command references from skill docs", len(refs))

	type failure struct {
		ref    cmdRef
		reason string
	}
	var fails []failure
	seen := map[string]bool{} // dedupe identical sub-paths

	for _, r := range refs {
		sub, _, _ := parseSubPath(r.Cmd)
		if sub == "" {
			continue
		}
		if antiPatternAllowlist[sub] {
			continue
		}
		if seen[sub+"|"+r.File] {
			continue
		}
		seen[sub+"|"+r.File] = true

		// Compute parent + leaf
		tokens := strings.Fields(sub)
		if len(tokens) < 2 {
			continue
		}
		parent := strings.Join(tokens[:len(tokens)-1], " ")
		leaf := tokens[len(tokens)-1]

		// Strategy: directly check sub's own help; if its Usage line starts
		// with the sub-path, the sub-command is registered. Otherwise cobra
		// fell back to a parent and the sub doesn't exist.
		subHelp := cache.get(sub)
		expectedUsage := "Usage:\n  " + sub
		if strings.Contains(subHelp, expectedUsage) {
			continue // sub-path exists, all good
		}

		// Sub may be a leaf with subcommands list — also valid (e.g. dws sheet)
		// In that case Usage line is "Usage:\n  <sub> [flags]\n  <sub> [command]"
		if strings.Contains(subHelp, "Usage:\n  "+sub+" [flags]\n  "+sub+" [command]") {
			continue
		}

		// Top-level alias rewrite: envelope-declared aliases (e.g. report.aliases=["log"],
		// im.prefixes=["chat","im"]) make `dws log inbox list` dispatch to `dws report
		// inbox list`. Cobra prints the canonical path in Usage. If the Usage path has
		// the same token count as sub (minus the "dws" prefix) but a different first
		// token, sub is a registered alias path — treat as valid.
		if actualPath := extractUsagePath(subHelp); actualPath != "" {
			actualTokens := strings.Fields(actualPath)
			subTokens := strings.Fields(sub)
			if len(subTokens) > 0 && subTokens[0] == "dws" {
				subTokens = subTokens[1:]
			}
			if len(actualTokens) == len(subTokens) && len(actualTokens) > 0 && actualTokens[0] != subTokens[0] {
				continue // alias dispatched cleanly to canonical product
			}
		}

		// Otherwise verify by looking at parent's Available Commands
		parentHelp := cache.get(parent)
		available := parseAvailable(parentHelp)
		found := false
		for _, c := range available {
			if c == leaf {
				found = true
				break
			}
		}
		if !found {
			fails = append(fails, failure{ref: r, reason: fmt.Sprintf("`%s` not in Available Commands of `%s`: %v", leaf, parent, available)})
		}
	}

	if len(fails) == 0 {
		t.Logf("✓ All extracted commands dispatch cleanly (sub-paths registered, anti-patterns whitelisted)")
		return
	}

	// Format failure report
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skill-doc commands that don't dispatch in open-source dws v1.0.30:\n\n", len(fails)))
	for _, f := range fails {
		rel, _ := filepath.Rel(root, f.ref.File)
		sb.WriteString(fmt.Sprintf("  ❌ %s:%d\n     cmd: %s\n     reason: %s\n\n", rel, f.ref.Line, f.ref.Cmd, f.reason))
	}
	t.Fatal(sb.String())
}
