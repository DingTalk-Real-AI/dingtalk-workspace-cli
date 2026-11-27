// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWhiteboardLineHeightReadback(t *testing.T) {
	for _, embedded := range []bool{true, false} {
		for _, tc := range []struct{ name, requested, actual, reason string }{
			{"preserved", `,"lineHeight":2`, `,"lineHeight":2`, ""},
			{"lost", `,"lineHeight":2`, "", "readback_field_missing"},
			{"changed", `,"lineHeight":2`, `,"lineHeight":1.5`, "readback_field_mismatch"},
			{"default", "", `,"lineHeight":1.15`, ""},
			{"old-service-default", "", `,"lineHeight":1.5`, "readback_field_mismatch"},
		} {
			t.Run(tc.name+map[bool]string{true: "/embedded", false: "/standalone"}[embedded], func(t *testing.T) {
				text := func(height string) string {
					return `{"blocks":[{"type":"paragraph"` + height + `,"runs":[{"text":"检验流程配置"}]}]}`
				}
				source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text","text":` + text(tc.requested) + `}]}}`
				readback := `[{"id":"real-1","type":"text","text":` + text(tc.actual) + `}]`
				caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
				args := []string{"--source", source, "--yes"}
				updateTool, queryTool := toolUpdate, toolQuery
				if embedded {
					args = append(args, "--node", "doc", "--part-id", "part")
					caller.responses[toolUpdate] = []string{validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)}
					caller.responses[toolQuery] = []string{validWhiteboardQueryResponse(readback)}
				} else {
					updateTool, queryTool = toolUpdateStandalone, toolQueryStandalone
					args = append(args, "--node", "wb", "--page-id", "page-1", "--expected-revision", "12", "--request-id", "line-height-test")
					caller.responses[toolUpdateStandalone] = []string{validStandaloneWhiteboardUpdateResponse("wb", "append", 12, 13, []string{"n1"}, []string{"real-1"}, 0)}
					caller.responses[toolQueryStandalone] = []string{validStandaloneWhiteboardQueryResponse("wb", "page", 13, readback)}
				}
				err := runWhiteboardCoverage(t, Update, caller, "", args...)
				if tc.reason == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var failure *apperrors.Error
					if !errors.As(err, &failure) || failure.Reason != tc.reason || failure.Details["commitState"] != "committed" || failure.Details["verified"] != false || failure.Retryable {
						t.Fatalf("lost committed mismatch evidence: %v", err)
					}
				}
				if len(caller.calls) != 2 || caller.calls[0].tool != updateTool || caller.calls[1].tool != queryTool {
					t.Fatalf("must write once and read once without replay: %#v", caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageWhiteboardInvalidLineHeightStopsBeforeMCP(t *testing.T) {
	caller := &whiteboardCoverageCaller{}
	err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--yes", "--source", `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text","text":{"blocks":[{"type":"paragraph","lineHeight":0.9,"runs":[{"text":""}]}]}}]}}`)
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 3 || !strings.Contains(err.Error(), "/text/blocks/0/lineHeight") || len(caller.calls) != 0 {
		t.Fatalf("invalid lineHeight reached MCP: %v %#v", err, caller.calls)
	}
}
