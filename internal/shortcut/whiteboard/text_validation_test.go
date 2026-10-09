// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package whiteboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageWhiteboardTextAlignmentStopsDiffAndUpdate(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"r1-mon","type":"shape","text":{"verticalAlign":"center","blocks":[{"type":"paragraph","verticalAlign":"center","runs":[{"text":"课程"}]}]}}]}}`
	for _, declaration := range []shortcut.Shortcut{Diff, Update} {
		for _, embedded := range []bool{false, true} {
			t.Run(declaration.Command+map[bool]string{true: "/embedded", false: "/standalone"}[embedded], func(t *testing.T) {
				caller := &whiteboardCoverageCaller{}
				args := []string{"--node", "wb", "--source", source, "--yes"}
				if embedded {
					args = append(args, "--part-id", "part")
				} else {
					args = append(args, "--page-id", "page")
					if declaration.Command == "+update" {
						args = append(args, "--expected-revision", "1", "--request-id", "bad-text")
					}
				}
				err := runWhiteboardCoverage(t, declaration, caller, "", args...)
				var coded interface{ ExitCode() int }
				if err == nil || !errors.As(err, &coded) || coded.ExitCode() != 3 || !strings.Contains(err.Error(), "/source/nodes/0/text/blocks/0/verticalAlign") || !strings.Contains(err.Error(), "r1-mon") {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(caller.calls) != 0 {
					t.Fatalf("invalid source reached MCP: %#v", caller.calls)
				}
			})
		}
	}
}
