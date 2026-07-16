package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEventStopPreviewConfirmationAndStdinCoverage(t *testing.T) {
	originalNormalize := eventNormalizeAs
	t.Cleanup(func() { eventNormalizeAs = originalNormalize })
	eventNormalizeAs = func(string) (string, error) { return "app", nil }
	newStopRoot := func(dryRun, yes bool) (*cobra.Command, *bytes.Buffer) {
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", dryRun, "")
		root.PersistentFlags().Bool("yes", yes, "")
		root.AddCommand(newEventStopCommand())
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		return root, &output
	}
	root, output := newStopRoot(true, false)
	root.SetArgs([]string{"stop"})
	if err := root.Execute(); err != nil || !strings.Contains(output.String(), "dry_run") {
		t.Fatalf("event stop dry-run = %v, %q", err, output.String())
	}
	root, _ = newStopRoot(false, false)
	root.SetArgs([]string{"stop"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("event stop confirmation error = %v", err)
	}

	originalStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = originalStdin })
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = closed
	if shouldWatchStdinEOF(0, 0) {
		t.Fatal("shouldWatchStdinEOF(closed stdin) = true")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	os.Stdin = read
	if !shouldWatchStdinEOF(0, 0) {
		t.Fatal("shouldWatchStdinEOF(pipe) = false")
	}
	var cfg consume.Config
	applyEventConsumeStdin(nil, 0, 0, strings.NewReader("ignored"))
	applyEventConsumeStdin(&cfg, 1, 0, strings.NewReader("bounded"))
	if cfg.Stdin != nil {
		t.Fatal("bounded consume received stdin watcher")
	}
	wantReader := strings.NewReader("pipe")
	applyEventConsumeStdin(&cfg, 0, 0, wantReader)
	if cfg.Stdin != wantReader {
		t.Fatal("unbounded pipe consume did not receive stdin")
	}
}

func TestCrossPlatformCoverageRunnerPluginStdioAndDryRunErrorCoverage(t *testing.T) {
	if _, err := (*runtimeRunner)(nil).Run(context.Background(), executor.Invocation{}); err == nil {
		t.Fatal("nil runtime runner error = nil")
	}
	t.Setenv(authpkg.AgentCodeEnv, "codex")
	t.Setenv("HOME", t.TempDir())
	t.Setenv(envDingtalkAgent, "agent")
	t.Setenv(envDingtalkTraceID, "trace")
	t.Setenv(envDingtalkMessageID, "message")
	headers := resolveIdentityHeaders()
	for _, key := range []string{"x-dws-agent-instance-id", "x-dingtalk-agent", "x-dingtalk-trace-id", "x-dingtalk-message-id"} {
		if headers[key] == "" {
			t.Fatalf("identity header %q missing: %#v", key, headers)
		}
	}

	stdioMu.Lock()
	previous := stdioClients
	stdioClients = map[string]*transport.StdioClient{"plugin/server": transport.NewStdioClient("missing", nil, nil)}
	stdioMu.Unlock()
	t.Cleanup(func() {
		stdioMu.Lock()
		stdioClients = previous
		stdioMu.Unlock()
	})
	if _, ok := LookupStdioClient("server"); !ok {
		t.Fatal("LookupStdioClient(server suffix) failed")
	}
	if StopStdioClient("missing") {
		t.Fatal("StopStdioClient(missing) = true")
	}

	registerPluginHTTPServer(mcptypes.ServerDescriptor{Key: "coverage", Endpoint: "https://example.test", AuthHeaders: map[string]string{"Authorization": "Bearer token"}})

	originalDryRun := toolCallerDryRun
	t.Cleanup(func() { toolCallerDryRun = originalDryRun })
	toolCallerDryRun = func(context.Context, executor.Invocation) (executor.Result, error) {
		return executor.Result{}, errors.New("dry-run")
	}
	adapter := newToolCallerAdapter(nil, &GlobalFlags{DryRun: true})
	if _, err := adapter.CallTool(context.Background(), "product", "tool", nil); err == nil {
		t.Fatal("tool caller dry-run error = nil")
	}
}
