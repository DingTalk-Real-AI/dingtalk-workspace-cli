// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageFramework2HelperOutputEdges(t *testing.T) {
	if err := writeCommandPayload(nil, map[string]any{"ok": true}); err != nil {
		t.Fatalf("nil command payload: %v", err)
	}

	cmd := &cobra.Command{Use: "output"}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := writeEnvelope(cmd, output.NewSuccessEnvelope(map[string]any{"id": "x"})); err != nil {
		t.Fatalf("success envelope: %v", err)
	}
	if !strings.Contains(stdout.String(), `"outcome": "success"`) {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if err := writeEnvelope(nil, nil); err != nil {
		t.Fatalf("nil failure envelope: %v", err)
	}

	f := NewFormatterWithWriters(nil, nil)
	if f.w != os.Stdout || f.errW != os.Stderr {
		t.Fatalf("nil writers did not use process defaults")
	}
	var nextOut, nextErr bytes.Buffer
	f.SetWriters(&nextOut, nil)
	f.SetWriters(nil, &nextErr)
	if f.w != &nextOut || f.errW != &nextErr {
		t.Fatalf("SetWriters did not replace each non-nil side")
	}
}

func TestCrossPlatformCoverageFramework2MCPDataEdges(t *testing.T) {
	original := deps
	t.Cleanup(func() { deps = original })

	InitDeps(&coverageErrorCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"id":"x"}`}}}})
	data, err := CallMCPToolDataOnServer(nil, "dev", "get_thing", nil)
	if err != nil || data.(map[string]any)["id"] != "x" {
		t.Fatalf("decoded data=%#v err=%v", data, err)
	}

	InitDeps(&coverageErrorCaller{err: errors.New("transport failed")})
	if _, err := CallMCPToolDataOnServer(context.Background(), "dev", "get_thing", nil); err == nil {
		t.Fatal("expected transport error")
	}

	InitDeps(&coverageErrorCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "   "}}}})
	data, err = CallMCPToolDataOnServer(context.Background(), "dev", "get_thing", nil)
	if err != nil || len(data.(map[string]any)) != 0 {
		t.Fatalf("empty response data=%#v err=%v", data, err)
	}

	InitDeps(&coverageErrorCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "{"}}}})
	if _, err := CallMCPToolDataOnServer(context.Background(), "dev", "get_thing", nil); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestCrossPlatformCoverageFramework2LegacyTextAdapter(t *testing.T) {
	original := deps
	t.Cleanup(func() { deps = original })
	caller := &coverageErrorCaller{}
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out = NewFormatterWithWriters(&stdout, &bytes.Buffer{})
	if err := RenderLegacyMCPText("get_thing", `{"id":"x"}`); err != nil {
		t.Fatalf("RenderLegacyMCPText: %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "x"`) {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestCrossPlatformCoverageFramework2LeafResultEdges(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: expected panic", name)
			}
		}()
		fn()
	}
	resultCall := func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
		return output.Success(map[string]any{"ok": true}), nil
	}
	mustPanic("metadata result call", func() {
		DeclareLeafMetadata(&cobra.Command{Use: "meta"}, LeafSpec{ResultCall: resultCall})
	})
	mustPanic("two call styles", func() {
		FromLeafSpec(LeafSpec{Use: "both", Call: func(*cobra.Command, string, map[string]any) error { return nil }, ResultCall: resultCall})
	})

	cmd := prepareUnifiedTestCommand(NewLeafCommand(LeafSpec{
		Use: "result", Tool: "get_thing", OutputRollout: output.RolloutUnifiedActive, ResultCall: resultCall,
	}))
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ResultCall command: %v", err)
	}
}

func TestCrossPlatformCoverageFramework2DevDocRemainsLegacy(t *testing.T) {
	cmd := newDevDocSearchCommand()
	if got := output.CommandRollout(cmd); got != output.RolloutLegacyOnly {
		t.Fatalf("dev doc rollout=%s, want legacy_only", got)
	}
}

func TestCrossPlatformCoverageFramework2ConnectLegacyEdges(t *testing.T) {
	original := connectDaemonDirOverride
	t.Cleanup(func() { connectDaemonDirOverride = original })

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	connectDaemonDirOverride = blocked
	stop := newDevAppRobotConnectStopCommand()
	stop.Flags().Bool("yes", true, "")
	if err := stop.Flags().Set("robot-client-id", "ding-test"); err != nil {
		t.Fatal(err)
	}
	if err := stop.RunE(stop, nil); err == nil {
		t.Fatal("expected daemon directory error")
	}

	connectDaemonDirOverride = t.TempDir()
	list := newDevAppRobotConnectListCommand(nil)
	output.SetCommandRollout(list, output.RolloutLegacyOnly)
	if list.Flags().Lookup("format") == nil {
		list.Flags().String("format", "table", "")
	} else if err := list.Flags().Set("format", "table"); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	list.SetOut(&stdout)
	if err := list.RunE(list, nil); err != nil {
		t.Fatalf("legacy list: %v", err)
	}
	if !strings.Contains(stdout.String(), "no connectors found") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
