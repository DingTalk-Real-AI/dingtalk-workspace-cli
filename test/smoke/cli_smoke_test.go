package smoke_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

const cliSmokeHelperEnv = "DWS_CLI_SMOKE_HELPER"

// TestCLIHelperProcess executes the real app entrypoint in a subprocess. Product
// routing inspects os.Args, so using Cobra.SetArgs would not exercise the same
// path as the dws binary.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv(cliSmokeHelperEnv) != "1" {
		return
	}

	marker := -1
	for i, arg := range os.Args {
		if arg == "--" {
			marker = i
			break
		}
	}
	if marker < 0 {
		fmt.Fprintln(os.Stderr, "CLI smoke helper: missing -- argument marker")
		os.Exit(2)
	}
	os.Args = append([]string{"dws"}, os.Args[marker+1:]...)
	os.Exit(app.Execute())
}

func TestCLISmoke_AllPublicCommandsSupportHelp(t *testing.T) {
	_ = isolatedCLIEnv(t)

	root := app.NewRootCommand()
	root.InitDefaultHelpCmd()
	paths := publicCommandPaths(root)
	if len(paths) == 0 {
		t.Fatal("public command traversal returned no commands")
	}

	for _, path := range paths {
		path := path
		name := "root"
		if len(path) > 0 {
			name = strings.Join(path, "/")
		}
		t.Run(name, func(t *testing.T) {
			args := append(append([]string(nil), path...), "--help")
			// Help never executes a product handler, so SetArgs is sufficient here.
			// Real product dispatch is covered below through app.Execute in a
			// subprocess because that path intentionally inspects os.Args.
			// Reuse one root: --help does not mutate command state, and rebuilding
			// the tree per path balloons memory ~845x under the race detector.
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			err := root.Execute()
			if err != nil {
				t.Fatalf("dws %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, output.String())
			}
			if strings.TrimSpace(output.String()) == "" {
				t.Fatalf("dws %s returned empty help", strings.Join(args, " "))
			}
		})
	}

	t.Logf("validated --help for %d public Cobra command paths", len(paths))
}

func TestCLISmoke_RepresentativeStaticCommandsReturnMockJSON(t *testing.T) {
	env := isolatedCLIEnv(t)

	tests := []struct {
		name     string
		args     []string
		wantTool string
	}{
		{
			name:     "contact search",
			args:     []string{"--mock", "--format", "json", "contact", "user", "search", "--query", "Ada"},
			wantTool: "search_contact_by_key_word",
		},
		{
			name: "calendar list",
			args: []string{
				"--mock", "--format", "json", "calendar", "event", "list",
				"--start", "2026-07-10T09:00:00+08:00",
				"--end", "2026-07-10T10:00:00+08:00",
			},
			wantTool: "list_calendar_events",
		},
		{
			name:     "ding list",
			args:     []string{"--mock", "--format", "json", "ding", "message", "list", "--type", "UNREAD"},
			wantTool: "list_ding_messages",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCLI(t, env, tc.args...)
			if err != nil {
				t.Fatalf("dws %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(tc.args, " "), err, stdout, stderr)
			}

			var payload map[string]any
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("dws %s returned non-JSON stdout: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(tc.args, " "), err, stdout, stderr)
			}
			if payload["_mock"] != true {
				t.Fatalf("dws %s _mock = %#v, want true; payload=%#v", strings.Join(tc.args, " "), payload["_mock"], payload)
			}
			if payload["_tool"] != tc.wantTool {
				t.Fatalf("dws %s _tool = %#v, want %q; payload=%#v", strings.Join(tc.args, " "), payload["_tool"], tc.wantTool, payload)
			}
		})
	}
}

func TestCrossPlatformCoverageCLISmokeCommandTypoGuidance(t *testing.T) {
	env := isolatedCLIEnv(t)
	tests := []struct {
		name           string
		args           []string
		wantInput      string
		wantHelp       string
		wantHint       string
		wantSuggestion string
		wantReason     string
	}{
		{
			name:           "legacy group typo",
			args:           []string{"chat", "searchh", "--query", "ignored", "--format", "json"},
			wantInput:      "searchh",
			wantHelp:       "dws chat --help",
			wantSuggestion: "search",
		},
		{
			name:           "nested legacy group typo with child flag",
			args:           []string{"chat", "message", "liss", "--conversation-id", "cid", "--format", "json"},
			wantInput:      "liss",
			wantHelp:       "dws chat message --help",
			wantSuggestion: "list",
		},
		{
			name:      "legacy group typo without suggestion",
			args:      []string{"chat", "zzzzz", "--format", "json"},
			wantInput: "zzzzz",
			wantHelp:  "dws chat --help",
		},
		{
			name:           "top level typo",
			args:           []string{"chatt", "--format", "json"},
			wantInput:      "chatt",
			wantHelp:       "dws --help",
			wantSuggestion: "chat",
		},
		{
			name:           "auth group typo",
			args:           []string{"auth", "sttus", "--format", "json"},
			wantInput:      "sttus",
			wantHelp:       "dws auth --help",
			wantSuggestion: "status",
		},
		{
			name:           "event group typo",
			args:           []string{"event", "lisst", "--format", "json"},
			wantInput:      "lisst",
			wantHelp:       "dws event --help",
			wantSuggestion: "list",
		},
		{
			name:           "shortcut group typo",
			args:           []string{"shortcut", "lst", "--format", "json"},
			wantInput:      "lst",
			wantHelp:       "dws shortcut --help",
			wantSuggestion: "list",
		},
		{
			name:           "plus shortcut typo",
			args:           []string{"chat", "+chat-mesages", "--format", "json"},
			wantInput:      "+chat-mesages",
			wantHelp:       "dws chat --help",
			wantSuggestion: "+chat-messages",
			wantReason:     "unknown_shortcut",
		},
		{
			name:           "audit group typo",
			args:           []string{"audit", "tal", "--format", "json"},
			wantInput:      "tal",
			wantHelp:       "dws audit --help",
			wantSuggestion: "tail",
		},
		{
			name:           "hybrid aisearch typo",
			args:           []string{"aisearch", "persno", "--query", "ignored", "--format", "json"},
			wantInput:      "persno",
			wantHelp:       "dws aisearch --help",
			wantSuggestion: "person",
		},
		{
			name:           "hybrid dev connect typo",
			args:           []string{"dev", "connect", "statsu", "--format", "json"},
			wantInput:      "statsu",
			wantHelp:       "dws dev connect --help",
			wantSuggestion: "status",
		},
		{
			name:           "hybrid aitable view typo",
			args:           []string{"aitable", "view", "get", "crad", "--format", "json"},
			wantInput:      "crad",
			wantHelp:       "dws aitable view get --help",
			wantSuggestion: "card",
		},
		{
			name:           "hybrid chat members typo",
			args:           []string{"chat", "group", "members", "ad", "--format", "json"},
			wantInput:      "ad",
			wantHelp:       "dws chat group members --help",
			wantSuggestion: "add",
		},
		{
			name:           "hybrid doc export typo",
			args:           []string{"doc", "export", "gte", "--format", "json"},
			wantInput:      "gte",
			wantHelp:       "dws doc export --help",
			wantSuggestion: "get",
		},
		{
			name:           "hybrid report inbox typo",
			args:           []string{"report", "inbox", "lsit", "--format", "json"},
			wantInput:      "lsit",
			wantHelp:       "dws report inbox --help",
			wantSuggestion: "list",
		},
		{
			name:           "nested app group typo",
			args:           []string{"mcp", "url", "gt", "--format", "json"},
			wantInput:      "gt",
			wantHelp:       "dws mcp url --help",
			wantSuggestion: "get",
		},
		{
			name:           "calendar group typo",
			args:           []string{"calendar", "room", "serach", "--format", "json"},
			wantInput:      "serach",
			wantHelp:       "dws calendar room --help",
			wantSuggestion: "search",
		},
		{
			name:           "sheet group typo",
			args:           []string{"sheet", "range", "reead", "--format", "json"},
			wantInput:      "reead",
			wantHelp:       "dws sheet range --help",
			wantSuggestion: "read",
		},
		{
			name:           "sheet deep path guidance",
			args:           []string{"sheet", "read", "--format", "json"},
			wantInput:      "read",
			wantHelp:       "dws sheet --help",
			wantSuggestion: "range read",
		},
		{
			name:           "hybrid doc import typo",
			args:           []string{"doc", "import", "gte", "--format", "json"},
			wantInput:      "gte",
			wantHelp:       "dws doc import --help",
			wantSuggestion: "get",
		},
		{
			name:           "hybrid sheet import typo",
			args:           []string{"sheet", "import", "gte", "--format", "json"},
			wantInput:      "gte",
			wantHelp:       "dws sheet import --help",
			wantSuggestion: "get",
		},
		{
			name:      "shared compatibility hint",
			args:      []string{"todo", "create", "--format", "json"},
			wantInput: "create",
			wantHelp:  "dws todo --help",
			wantHint:  "dws todo task create",
		},
		{
			name:      "chat compatibility hint",
			args:      []string{"chat", "send", "--group", "cid", "--text", "hello", "--format", "json"},
			wantInput: "send",
			wantHelp:  "dws chat --help",
			wantHint:  "dws chat message send",
		},
		{
			name:      "contact compatibility hint",
			args:      []string{"contact", "user", "info", "--ids", "uid", "--format", "json"},
			wantInput: "info",
			wantHelp:  "dws contact user --help",
			wantHint:  "dws contact user get",
		},
		{
			name:      "calendar compatibility hint",
			args:      []string{"calendar", "get", "--format", "json"},
			wantInput: "get",
			wantHelp:  "dws calendar --help",
			wantHint:  "dws calendar event get",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := runCLI(t, env, test.args...)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
				t.Fatalf("dws %s exit = %v, want 3\nstdout:\n%s\nstderr:\n%s", strings.Join(test.args, " "), err, stdout, stderr)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("dws %s wrote stdout:\n%s", strings.Join(test.args, " "), stdout)
			}
			if strings.Contains(stderr, "available:") {
				t.Fatalf("dws %s leaked an unbounded command list:\n%s", strings.Join(test.args, " "), stderr)
			}

			var payload struct {
				Error struct {
					Category string `json:"category"`
					Reason   string `json:"reason"`
					Hint     string `json:"hint"`
					Details  struct {
						Input       string   `json:"input"`
						Suggestions []string `json:"suggestions"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
				t.Fatalf("dws %s returned non-JSON stderr: %v\nstderr:\n%s", strings.Join(test.args, " "), err, stderr)
			}
			wantReason := test.wantReason
			if wantReason == "" {
				wantReason = "unknown_subcommand"
			}
			if payload.Error.Category != "validation" || payload.Error.Reason != wantReason {
				t.Fatalf("dws %s error = %#v", strings.Join(test.args, " "), payload.Error)
			}
			if !strings.Contains(payload.Error.Hint, test.wantHelp) {
				t.Fatalf("dws %s hint = %q, want parent help %q", strings.Join(test.args, " "), payload.Error.Hint, test.wantHelp)
			}
			if test.wantHint != "" && !strings.Contains(payload.Error.Hint, test.wantHint) {
				t.Fatalf("dws %s hint = %q, want authored guidance %q", strings.Join(test.args, " "), payload.Error.Hint, test.wantHint)
			}
			if payload.Error.Details.Input != test.wantInput {
				t.Fatalf("dws %s details.input = %q, want %q", strings.Join(test.args, " "), payload.Error.Details.Input, test.wantInput)
			}
			if len(payload.Error.Details.Suggestions) > 3 {
				t.Fatalf("dws %s suggestions = %#v, want at most 3", strings.Join(test.args, " "), payload.Error.Details.Suggestions)
			}
			if test.wantSuggestion != "" && !slices.Contains(payload.Error.Details.Suggestions, test.wantSuggestion) {
				t.Fatalf("dws %s suggestions = %#v, want %q", strings.Join(test.args, " "), payload.Error.Details.Suggestions, test.wantSuggestion)
			}
		})
	}

	t.Run("legitimate positional commands remain executable", func(t *testing.T) {
		const positionalCommandTimeout = 30 * time.Second

		stdout, stderr, err := runCLIWithTimeout(t, env, positionalCommandTimeout, "schema", "chat message send", "--compact", "--format", "json")
		if err != nil || strings.TrimSpace(stdout) == "" {
			t.Fatalf("dws schema positional path failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		stdout, stderr, err = runCLI(t, env, "completion", "zsh")
		if err != nil || strings.TrimSpace(stdout) == "" {
			t.Fatalf("dws completion positional shell failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		stdout, stderr, err = runCLIWithTimeout(t, env, positionalCommandTimeout, "help", "auth")
		if err != nil || !strings.Contains(stdout, "dws auth") {
			t.Fatalf("dws help auth failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
	})
}

// publicCommandPaths follows only canonical, user-visible Cobra commands.
// Hidden commands and commands marked Deprecated by Cobra are intentionally
// excluded; aliases are not separate command nodes. Parents and leaves are both
// checked, while positional placeholders in Use are never synthesized.
func publicCommandPaths(root *cobra.Command) [][]string {
	var paths [][]string
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd != root && (cmd.Hidden || cmd.Deprecated != "") {
			return
		}

		paths = append(paths, append([]string(nil), path...))
		for _, child := range cmd.Commands() {
			walk(child, append(path, child.Name()))
		}
	}
	walk(root, nil)

	sort.Slice(paths, func(i, j int) bool {
		return strings.Join(paths[i], " ") < strings.Join(paths[j], " ")
	})
	return paths
}

func isolatedCLIEnv(t *testing.T) []string {
	t.Helper()

	root := t.TempDir()
	controlled := map[string]string{
		"HOME":                     root,
		"USERPROFILE":              root,
		"DWS_CONFIG_DIR":           filepath.Join(root, "config"),
		"DWS_KEYCHAIN_DIR":         filepath.Join(root, "keychain"),
		"DWS_DISABLE_KEYCHAIN":     "1",
		"HTTP_PROXY":               "http://127.0.0.1:1",
		"HTTPS_PROXY":              "http://127.0.0.1:1",
		"http_proxy":               "http://127.0.0.1:1",
		"https_proxy":              "http://127.0.0.1:1",
		"NO_PROXY":                 "127.0.0.1,localhost,::1",
		"no_proxy":                 "127.0.0.1,localhost,::1",
		cliSmokeHelperEnv:          "1",
		"DWS_ALLOW_HTTP_ENDPOINTS": "1",
		"DWS_TRUSTED_DOMAINS":      "127.0.0.1,localhost,::1",
	}
	for key, value := range controlled {
		t.Setenv(key, value)
	}

	env := make([]string, 0, len(controlled)+8)
	for _, key := range []string{"PATH", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "TZ", "SYSTEMROOT"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	for key, value := range controlled {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	return env
}

func runCLI(t *testing.T, env []string, args ...string) (string, string, error) {
	t.Helper()
	return runCLIWithTimeout(t, env, 10*time.Second, args...)
}

func runCLIWithTimeout(t *testing.T, env []string, timeout time.Duration, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	processArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], processArgs...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("dws %s timed out: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), ctx.Err(), stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String(), err
}
