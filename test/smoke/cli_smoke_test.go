package smoke_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
			cmd := app.NewRootCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(args)
			err := cmd.Execute()
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
