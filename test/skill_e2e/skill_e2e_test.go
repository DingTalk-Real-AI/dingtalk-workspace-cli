//go:build skill_verify
// +build skill_verify

// Package skill_e2e_test runs every backtick `dws ...` command from skill
// docs through a real Cobra binary in --mock mode and rejects unknown-command
// and unknown-flag dispatch failures. Mock JSON envelopes and help-like
// non-JSON output are both accepted because some documented entries
// intentionally name a command group rather than a leaf operation.
//
// Build tag rationale: this test depends on a real `dws` binary built from the
// current checkout and invokes hundreds of subprocesses. Default `go test
// ./...` skips it. Build with `go build -o dws ./cmd`, then run explicitly
// with: `go test -tags skill_verify ./test/skill_e2e/...`
//
// This is Layer B of the skill verification matrix — pure open-source,
// no real tenant required. Layer A (test/skill_static) checks the current
// source command tree and multi-skill flags; this layer actually invokes each
// supported command end-to-end.
//
// Why --mock instead of real calls: open-source CI has no real DingTalk
// tenant, and depending on one would either (a) leak credentials or
// (b) make the test flaky on tenant drift. --mock returns a deterministic
// shape (`{"_mock": true, "_tool": "<canonical>", "result": [], "success": true}`)
// for Cobra commands that resolve through the MCP path. Unknown command/flag
// failures surface here exactly as they would on a real tenant.
package skill_e2e_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	backtickRe = regexp.MustCompile("`(dws\\s+[^`]+?)`")
	quotedRe   = regexp.MustCompile(`"[^"]*"`)

	// Sub-paths intentionally referenced as anti-pattern guidance.
	antiPatternAllowlist = map[string]bool{
		"dws calendar list":         true,
		"dws minutes detail":        true,
		"dws minutes info":          true,
		"dws minutes summary":       true,
		"dws minutes transcription": true,
		"dws report inbox":          true,
		"dws skill add":             true,
		"dws skill find":            true,
	}

	// Line markers indicating an anti-pattern context (whole line skipped).
	antiPatternLineMarkers = []string{
		"[禁止]", "【禁止】", "禁止使用", "禁止编造", "❌",
		"错误写法", "高频错误", "反例", "不存在", "不应该",
		"不要写", "不要用", "反模式", "错例",
		"该命令不存在", "不是合法", "unknown flag", "LLM 高频幻觉",
		"臆造", "虚构", "编造", "缺少子命令", "缺少 scope",
		"不识别", "不认识", "不存在此", "不是顶层", "应该是", "应为",
		"参数名是", "参数错", "应该用", "废弃", "已下线",
	}

	// Verbs that imply writes; auto-append --dry-run.
	writeVerbs = map[string]bool{
		"create": true, "update": true, "delete": true, "send": true,
		"recall": true, "approve": true, "reject": true, "revoke": true,
		"commit": true, "mkdir": true, "add": true, "remove": true,
		"set": true, "invite": true, "transfer": true, "quit": true,
		"rename": true, "move": true, "copy": true, "append": true,
		"insert": true, "merge": true, "unmerge": true, "write": true,
		"replace": true, "mute": true, "cancel": true, "install": true,
		"publish": true, "forward": true, "reply": true,
	}

	// Utility commands do not participate in the MCP --mock envelope path;
	// Layer A still validates their Cobra registration. Executing them here can
	// trigger local setup, authentication, upgrades, or long-running consumers.
	mockUnsupportedRoots = map[string]bool{
		"api": true, "auth": true, "cache": true, "catalog": true,
		"completion": true, "config": true, "doctor": true, "event": true,
		"mcp": true, "plugin": true, "profile": true, "recovery": true,
		"schema": true, "skill": true, "upgrade": true, "version": true,
	}
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
}

func dwsBinary(t *testing.T, root string) string {
	t.Helper()
	if configured := os.Getenv("DWS_TEST_BINARY"); configured != "" {
		p, err := filepath.Abs(configured)
		if err != nil {
			t.Fatalf("resolve DWS_TEST_BINARY: %v", err)
		}
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			t.Fatalf("DWS_TEST_BINARY must name an executable file: %s", p)
		}
		return p
	}
	name := "dws"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(root, name)
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p
	}
	t.Skipf("current-checkout dws binary not found; run `go build -o %s ./cmd` or set DWS_TEST_BINARY", name)
	return ""
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

func extract(root string) ([]cmdRef, error) {
	dir := filepath.Join(root, "skills")
	var files []string
	for _, pat := range []string{
		dir + "/mono/SKILL.md",
		dir + "/mono/references/products/*.md",
		dir + "/multi/*/SKILL.md",
	} {
		m, err := filepath.Glob(pat)
		if err != nil {
			return nil, err
		}
		files = append(files, m...)
	}
	var err error
	files, err = appendMultiReferenceMarkdown(files, filepath.Join(dir, "multi"))
	if err != nil {
		return nil, err
	}

	var refs []cmdRef
	for _, f := range files {
		out, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		text := string(out)
		lineNo := 0
		scanner := bufio.NewScanner(strings.NewReader(text))
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024*4)
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if isAntiLine(line) {
				continue
			}
			for _, m := range backtickRe.FindAllStringSubmatch(line, -1) {
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

func isAntiLine(line string) bool {
	for _, marker := range antiPatternLineMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func shouldSkip(cmd string) bool {
	tokens := strings.Fields(cmd)
	if len(tokens) >= 2 && mockUnsupportedRoots[tokens[1]] {
		return true
	}
	if cmd == "dws sheet export" || strings.HasPrefix(cmd, "dws sheet export ") {
		// The export helper performs its asynchronous polling loop even under
		// --mock; Layer A validates its command path without making this audit
		// wait for the production export timeout.
		return true
	}
	if strings.HasPrefix(cmd, "dws dev ") || cmd == "dws dev" {
		// dev (product id: devapp) is a target MCP-discovered command tree. Until the MCP product
		// is published into the open-source registry, validate its Agent routing
		// through test/skill_tests.md rather than mock cobra dispatch.
		return true
	}
	if strings.Contains(cmd, "[flags]") || strings.Contains(cmd, "[command]") || strings.Contains(cmd, "[--") {
		return true
	}
	if strings.Contains(cmd, "|") || strings.Contains(cmd, "$(") || strings.Contains(cmd, " & ") {
		return true
	}
	if strings.Contains(cmd, "...") || strings.Contains(cmd, " > ") {
		return true
	}
	// Sub-path must not contain placeholder syntax.
	subPart := cmd
	if idx := strings.Index(cmd, " --"); idx >= 0 {
		subPart = cmd[:idx]
	}
	// Drop quoted strings before scanning
	subPart = quotedRe.ReplaceAllString(subPart, "__Q__")
	for _, t := range strings.Fields(subPart) {
		if strings.HasPrefix(t, "<") || strings.Contains(t, "/") || strings.Contains(t, "*") {
			return true
		}
	}
	// Placeholders with spaces inside angle brackets confuse word-based parsing
	for _, m := range regexp.MustCompile(`<[^>]*>`).FindAllString(cmd, -1) {
		if strings.Contains(m, " ") {
			return true
		}
	}
	return false
}

func subPath(cmd string) string {
	cmd2 := quotedRe.ReplaceAllString(cmd, "__Q__")
	var tokens []string
	for _, t := range strings.Fields(cmd2) {
		if strings.HasPrefix(t, "-") {
			break
		}
		tokens = append(tokens, t)
	}
	return strings.Join(tokens, " ")
}

func isWrite(sub string) bool {
	tokens := strings.Fields(sub)
	for i, t := range tokens {
		if i == 0 {
			continue
		}
		if writeVerbs[t] {
			return true
		}
		// Compound names like "send-by-bot", "recall-by-bot" etc.
		for w := range writeVerbs {
			if strings.HasPrefix(t, w+"-") {
				return true
			}
		}
	}
	return false
}

// substitute replaces well-known placeholders with safe dummy values.
func substitute(cmd string) string {
	subst := map[string]string{
		"<keyword>": "test", "<query>": "test", "<群名>": "test",
		"<姓名>": "wukong01", "<标题>": "_test", "<内容>": "_test",
		"<date>": "2026-05-21", "<today>": "2026-05-21",
		"<start>":  "2026-05-14T00:00:00+08:00",
		"<end>":    "2026-05-21T23:59:59+08:00",
		"<ISO>":    "2026-05-21T10:00:00+08:00",
		"<userId>": "userDummy", "<groupId>": "cidDummy==",
		"<openConversationId>": "cidDummy==",
		"<openMessageId>":      "msgDummy==", "<openDingTalkId>": "Dummy",
		"<robotCode>": "rcDummy", "<robot-code>": "rcDummy",
		"<token>": "tk", "<reportId>": "1", "<templateId>": "1",
		"<processCode>": "PROC_d", "<instanceId>": "INST_d",
		"<taskUuid>": "0123456789abcdef0123456789abcdef",
		"<id>":       "d", "<spaceId>": "sp", "<workspaceId>": "ws",
		"<nodeId>": "n", "<sheetId>": "Sheet1", "<baseId>": "b",
		"<tableId>": "t", "<fieldId>": "f", "<deptId>": "1",
		"<labelId>": "1", "<key>": "k", "<email>": "x@y.z",
		"<from>": "x@y.z", "<to>": "x@y.z",
		"<calendarId>": "primary", "<eventId>": "e",
		"<dimension>": "rows", "<position>": "0", "<length>": "1",
		"<role>": "OWNER", "<scope>": "mine", "<type>": "myWikiSpace",
		"<format>": "xlsx", "<range>": "A1:B2",
		"<NODE_ID>": "n", "<SHEET_ID>": "Sheet1", "<BASE_ID>": "b",
		"<TABLE_ID>": "t", "<FIELD_ID>": "f", "<DOC_ID>": "n",
		"<TPL_ID>": "1", "<RES_ID>": "r", "<TASK_ID>": "TASK_d",
		"<file>": "/tmp/test.txt", "<path>": "/tmp/test.txt",
	}
	for k, v := range subst {
		cmd = strings.ReplaceAll(cmd, k, v)
	}
	// Catch-all for any remaining <foo>
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(cmd, "dummy")
}

func TestSkillCommandsMockDispatch(t *testing.T) {
	root := repoRoot(t)
	dws := dwsBinary(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_DISABLE_KEYCHAIN", "1")
	t.Setenv("DWS_KEYCHAIN_DIR", t.TempDir())

	refs, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Dedupe by exact command text.
	seen := map[string]bool{}
	var unique []cmdRef
	for _, r := range refs {
		if antiPatternAllowlist[subPath(r.Cmd)] || seen[r.Cmd] {
			continue
		}
		seen[r.Cmd] = true
		unique = append(unique, r)
	}
	t.Logf("Extracted %d refs, %d unique runnable commands", len(refs), len(unique))

	type failure struct {
		ref    cmdRef
		actual string
		reason string
	}
	var fails []failure
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 8) // parallelism cap

	for _, r := range unique {
		r := r
		wg.Add(1)
		semaphore <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()
			actual := substitute(r.Cmd)
			if !strings.Contains(actual, " --mock") {
				actual += " --mock"
			}
			if !strings.Contains(actual, " --format") {
				actual += " --format json"
			}
			if isWrite(subPath(r.Cmd)) && !strings.Contains(actual, " --dry-run") {
				actual += " --dry-run"
			}
			argv := rejoinQuoted(actual)
			out, _ := exec.Command(dws, argv[1:]...).CombinedOutput()
			s := string(out)
			if strings.Contains(s, "unknown flag") || strings.Contains(s, "unknown command") || strings.Contains(s, "unknown subcommand") {
				mu.Lock()
				fails = append(fails, failure{ref: r, actual: actual, reason: firstLine(s, 200)})
				mu.Unlock()
				return
			}
			// Parse JSON; must have either _mock=true or success-marker.
			var j map[string]any
			if err := json.Unmarshal(out, &j); err != nil {
				// Non-JSON output is fine for help-like commands; only flag a fail if
				// stderr/exit-code indicates dispatch error (we already caught unknown above).
				return
			}
			if mock, ok := j["_mock"].(bool); ok && mock {
				return // PASS: mocked dispatch worked
			}
			// Some commands (helper-overrides) don't proxy through mock; accept success markers too.
			if v, ok := j["success"]; ok && (v == true || v == "true") {
				return
			}
			if errBlock, ok := j["error"].(map[string]any); ok {
				msg, _ := errBlock["message"].(string)
				if strings.Contains(msg, "unknown flag") || strings.Contains(msg, "unknown command") || strings.Contains(msg, "unknown subcommand") {
					mu.Lock()
					fails = append(fails, failure{ref: r, actual: actual, reason: msg})
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if len(fails) == 0 {
		t.Logf("✓ All %d unique skill commands dispatch cleanly under --mock", len(unique))
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skill commands that fail at cobra/envelope dispatch under --mock:\n\n", len(fails)))
	for _, f := range fails {
		rel, _ := filepath.Rel(root, f.ref.File)
		sb.WriteString(fmt.Sprintf("  ❌ %s:%d\n", rel, f.ref.Line))
		sb.WriteString(fmt.Sprintf("     cmd:    %s\n", f.ref.Cmd))
		sb.WriteString(fmt.Sprintf("     actual: %s\n", f.actual))
		sb.WriteString(fmt.Sprintf("     reason: %s\n\n", f.reason))
	}
	t.Fatal(sb.String())
}

func firstLine(s string, max int) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// rejoinQuoted is a tiny shell-tokenizer that keeps "double-quoted" runs as one arg.
func rejoinQuoted(s string) []string {
	var argv []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			inQ = !inQ
		case ' ', '\t':
			if inQ {
				cur.WriteByte(c)
				continue
			}
			if cur.Len() > 0 {
				argv = append(argv, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		argv = append(argv, cur.String())
	}
	return argv
}
