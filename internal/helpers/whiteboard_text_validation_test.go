// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageWhiteboardTextAlignmentStopsBeforeEffects(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"r1-mon","type":"shape","text":{"verticalAlign":"center","blocks":[{"type":"paragraph","verticalAlign":"center","runs":[{"text":"课程"}]}]}}]}}`
	for _, operation := range []string{"render", "create-with-content", "update"} {
		t.Run(operation, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json"}
			buf := installWhiteboardTestCaller(t, caller)
			cmd := newWhiteboardCommand()
			cmd.PersistentFlags().Bool("yes", false, "")
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			artifact := filepath.Join(t.TempDir(), "preview.svg")
			args := []string{operation, "--source", source, "--yes"}
			switch operation {
			case "render":
				args = append(args, "--output", artifact)
			case "create-with-content":
				args = append(args, "--name", "课表", "--request-id", "bad-text")
			case "update":
				args[2] = writeWhiteboardFixture(t, source)
				args = append(args, "--node", "doc", "--part-id", "part")
			}
			cmd.SetArgs(args)
			err := corecmd.ExecuteForTest(cmd)
			var coded interface{ ExitCode() int }
			if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 || !strings.Contains(err.Error(), "/source/nodes/0/text/blocks/0/verticalAlign") || !strings.Contains(err.Error(), "r1-mon") {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid source reached MCP: %#v", caller.calls)
			}
			if _, err := os.Stat(artifact); !os.IsNotExist(err) {
				t.Fatalf("unexpected artifact: %v", err)
			}
		})
	}
}

// The execution boundary also protects callers that bypass flag preparation.
func TestCrossPlatformCoverageWhiteboardTextAlignmentCreateExecution(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json"}
	installWhiteboardTestCaller(t, caller)
	_, err := callStandaloneWhiteboardCreateResult(&cobra.Command{}, "", map[string]any{
		"source": `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","text":{"blocks":[{"verticalAlign":"center"}]}}]}`,
	})
	var coded interface{ ExitCode() int }
	if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 || !strings.Contains(err.Error(), "/source/nodes/0/text/blocks/0/verticalAlign") || len(caller.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardTextRunValidationUsesInputExitCode(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"shape","text":{"blocks":[{"type":"paragraph","runs":[{"text":"line one\nline two"}]}]}}]}}`
	for _, operation := range []string{"render", "update"} {
		t.Run(operation, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json"}
			buf := installWhiteboardTestCaller(t, caller)
			cmd := newWhiteboardCommand()
			cmd.PersistentFlags().Bool("yes", false, "")
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			artifact := filepath.Join(t.TempDir(), "preview.svg")
			args := []string{operation, "--source", source, "--yes"}
			if operation == "render" {
				args = append(args, "--output", artifact)
			} else {
				args[2] = writeWhiteboardFixture(t, source)
				args = append(args, "--node", "doc", "--part-id", "part")
			}
			cmd.SetArgs(args)
			err := corecmd.ExecuteForTest(cmd)
			var coded interface{ ExitCode() int }
			if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 ||
				!strings.Contains(err.Error(), "/source/nodes/0/text/blocks/0/runs/0/text") {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid source reached MCP: %#v", caller.calls)
			}
			if _, err := os.Stat(artifact); !os.IsNotExist(err) {
				t.Fatalf("unexpected artifact: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardPresentationOrderStopsBeforeEffects(t *testing.T) {
	source := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"shape","type":"shape","presentationOrder":0}]}`

	t.Run("render", func(t *testing.T) {
		caller := &whiteboardTestCaller{format: "json"}
		buf := installWhiteboardTestCaller(t, caller)
		cmd := newWhiteboardCommand()
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		artifact := filepath.Join(t.TempDir(), "preview.svg")
		cmd.SetArgs([]string{"render", "--source", source, "--output", artifact, "--yes"})

		err := corecmd.ExecuteForTest(cmd)
		var coded interface{ ExitCode() int }
		if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 ||
			!strings.Contains(err.Error(), "/source/nodes/0/presentationOrder") ||
			!strings.Contains(err.Error(), "shape") {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid source reached MCP: %#v", caller.calls)
		}
		if _, err := os.Stat(artifact); !os.IsNotExist(err) {
			t.Fatalf("unexpected artifact: %v", err)
		}
	})

	t.Run("create execution boundary", func(t *testing.T) {
		caller := &whiteboardTestCaller{format: "json"}
		installWhiteboardTestCaller(t, caller)
		_, err := callStandaloneWhiteboardCreateResult(&cobra.Command{}, "", map[string]any{"source": source})
		var coded interface{ ExitCode() int }
		if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 ||
			!strings.Contains(err.Error(), "/source/nodes/0/presentationOrder") ||
			!strings.Contains(err.Error(), "shape") || len(caller.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, caller.calls)
		}
	})
}
