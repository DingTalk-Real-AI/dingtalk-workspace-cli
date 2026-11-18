// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type outboundCallCapture struct {
	count         int
	product, tool string
	args          map[string]any
	ctx           context.Context
	text          string
	err           error
}

func (c *outboundCallCapture) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.count++
	c.product, c.tool, c.args, c.ctx = product, tool, args, ctx
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, c.err
}
func (*outboundCallCapture) Format() string { return "json" }
func (*outboundCallCapture) DryRun() bool   { return false }
func (*outboundCallCapture) Fields() string { return "" }
func (*outboundCallCapture) JQ() string     { return "" }

func outboundCallTestRoot(t *testing.T, caller edition.ToolCaller, args ...string) *cobra.Command {
	t.Helper()
	InitDepsForTest(t, caller)
	root := newOutboundCallCommand()
	root.PersistentFlags().Bool("yes", false, "confirm")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.PersistentFlags().String("format", "json", "format")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	return root
}

func TestCrossPlatformCoverageOutboundCallSingleInput(t *testing.T) {
	caller := &outboundCallCapture{text: `{"taskId":"t"}`}
	root := outboundCallTestRoot(t, caller, "create", "--callee", "fixture-user-id", "--task", " p ", "--opening", " g ", "--yes")
	ctx := context.WithValue(context.Background(), outboundCallContextKey{}, "invocation")
	leaf, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(caller.args)
	if caller.count != 1 || caller.product != "outbound-call" || caller.tool != outboundCallCreateTool || string(raw) != `{"calleeInfos":[{"callee":"fixture-user-id","greeting":" g ","prompt":" p "}]}` || caller.ctx.Value(outboundCallContextKey{}) != "invocation" {
		t.Fatalf("wrong dispatch: %#v payload=%s", caller, raw)
	}
	var stream bytes.Buffer
	leaf.SetOut(&stream)
	code, emitted, err := output.EmitStoredResult(leaf)
	if err != nil || code != 0 || !emitted || !strings.Contains(stream.String(), `"outcome": "pending"`) || !strings.Contains(stream.String(), "outbound-call detail") {
		t.Fatalf("pending result: code=%d emitted=%v err=%v output=%s", code, emitted, err, &stream)
	}
}

type outboundCallContextKey struct{}

func TestCrossPlatformCoverageOutboundCallValidationAndConfirmation(t *testing.T) {
	for _, args := range [][]string{
		{"create"},
		{"create", "--callee", "u", "--task", "p", "--yes"},
		{"create", "--callee", "u", "--opening", "g", "--yes"},
		{"create", "--task", "p", "--opening", "g", "--yes"},
		{"create", "--callee", " ", "--task", "p", "--opening", "g", "--yes"},
		{"create", "--callee", "u", "--task", " ", "--opening", "g", "--yes"},
		{"create", "--callee", "u", "--task", "p", "--opening", " "},
		{"create", "--callee", "u", "--task", "p", "--opening", "g"},
		{"detail"}, {"detail", "--task-id", " "},
		{"detail", "--task-id", "t", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			caller := &outboundCallCapture{}
			root := outboundCallTestRoot(t, caller, args...)
			if err := corecmd.ExecuteForTest(root); err == nil || apperrors.ExitCode(err) != 3 {
				t.Fatalf("expected validation/3, got %v", err)
			}
			if caller.count != 0 {
				t.Fatal("invalid/unconfirmed input reached remote")
			}
		})
	}
}

func TestCrossPlatformCoverageOutboundCallDryRun(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--callee", "u", "--task", "p", "--opening", "g", "--dry-run"},
		{"detail", "--task-id", "t", "--dry-run"},
	} {
		caller := &outboundCallCapture{}
		root := outboundCallTestRoot(t, caller, args...)
		leaf, err := corecmd.ExecuteCForTest(root)
		if err != nil || caller.count != 0 {
			t.Fatalf("dry-run err=%v calls=%d", err, caller.count)
		}
		var stream bytes.Buffer
		leaf.SetOut(&stream)
		_, _, err = output.EmitStoredResult(leaf)
		if err != nil || !strings.Contains(stream.String(), `"dry_run": true`) || !strings.Contains(stream.String(), `"executed": false`) {
			t.Fatalf("preview: %v %s", err, &stream)
		}
	}
}

func TestCrossPlatformCoverageOutboundCallResultOutcomes(t *testing.T) {
	for _, tc := range []struct {
		command, text, outcome, subtype string
		err                             error
		code                            int
	}{
		{"create", `{"taskId":"t"}`, "pending", "", nil, 0},
		{"create", "accepted", "failure", "submission_unknown", nil, 1},
		{"create", `{"success":false,"message":"denied"}`, "failure", "upstream_failure", nil, 1},
		{"create", "", "failure", "submission_unknown", nil, 1},
		{"create", "", "failure", "submission_unknown", context.DeadlineExceeded, 1},
		{"create", "", "failure", "submission_unknown", context.Canceled, 1},
		{"create", "", "failure", "submission_unknown", &net.OpError{Op: "read", Net: "tcp", Err: io.EOF}, 1},
		{"create", "", "failure", "submission_unknown", apperrors.NewAPI("fixture request failure", apperrors.WithCause(&transport.CallError{Stage: transport.CallStageRequest, Cause: io.ErrUnexpectedEOF})), 1},
		{"create", "", "failure", "submission_unknown", apperrors.NewDiscovery("fixture body failure", apperrors.WithOperation("tools/call"), apperrors.WithReason("tools_call_response_read_failed")), 1},
		{"detail", `{"taskId":"t","callStatus":4,"errMsg":10}`, "success", "", nil, 0},
		{"detail", `{"success":false}`, "failure", "upstream_failure", nil, 1},
		{"detail", "", "failure", "invalid_response", nil, 1},
		{"detail", "accepted", "failure", "invalid_response", nil, 1},
	} {
		caller := &outboundCallCapture{text: tc.text, err: tc.err}
		args := []string{"detail", "--task-id", "t"}
		if tc.command == "create" {
			args = []string{"create", "--callee", "u", "--task", "p", "--opening", "g", "--yes"}
		}
		root := outboundCallTestRoot(t, caller, args...)
		leaf, err := corecmd.ExecuteCForTest(root)
		if err != nil {
			t.Fatal(err)
		}
		var stream bytes.Buffer
		leaf.SetOut(&stream)
		code, _, err := output.EmitStoredResult(leaf)
		if err != nil || code != tc.code {
			t.Fatalf("%s: code=%d err=%v output=%s", tc.command, code, err, &stream)
		}
		var env output.Envelope
		if err := json.Unmarshal(stream.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if string(env.Outcome) != tc.outcome || (tc.subtype != "" && (env.Error == nil || env.Error.Subtype != tc.subtype)) || caller.count != 1 {
			t.Fatalf("outcome: %#v calls=%d", env, caller.count)
		}
	}
	caller := &outboundCallCapture{err: apperrors.NewAuth("login required")}
	root := outboundCallTestRoot(t, caller, "detail", "--task-id", "t")
	if err := corecmd.ExecuteForTest(root); !errors.Is(err, caller.err) {
		t.Fatalf("classification lost: %v", err)
	}
	caller = &outboundCallCapture{err: apperrors.NewAuth("auth timed out", apperrors.WithCause(context.DeadlineExceeded))}
	root = outboundCallTestRoot(t, caller, "create", "--callee", "u", "--task", "p", "--opening", "g", "--yes")
	if err := corecmd.ExecuteForTest(root); err != caller.err {
		t.Fatalf("auth failure reclassified: %v", err)
	}
	for _, original := range []error{
		apperrors.NewAPI("fixture unsent failure", apperrors.WithExecutionStarted(false), apperrors.WithCause(&transport.CallError{Stage: transport.CallStageRequest, Cause: context.DeadlineExceeded})),
		apperrors.NewDiscovery("fixture discovery timeout", apperrors.WithOperation("tools/list"), apperrors.WithCause(context.DeadlineExceeded)),
		apperrors.NewAPI("HTTP 503 timeout invalid_response without transport evidence"),
	} {
		caller := &outboundCallCapture{err: original}
		root := outboundCallTestRoot(t, caller, "create", "--callee", "u", "--task", "p", "--opening", "g", "--yes")
		if err := corecmd.ExecuteForTest(root); err != original {
			t.Fatalf("unrelated or explicitly unsent error reclassified: %v", err)
		}
	}
}

func TestCrossPlatformCoverageOutboundCallSubmissionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cause     error
		uncertain bool
	}{
		{"connection_lost", &transport.CallError{Stage: transport.CallStageRequest, Cause: io.EOF}, true},
		{"http_rejected", &transport.CallError{Stage: transport.CallStageHTTP, HTTPStatus: 400}, false},
		{"http_timeout", &transport.CallError{Stage: transport.CallStageHTTP, HTTPStatus: 408}, true},
		{"server_failed", &transport.CallError{Stage: transport.CallStageHTTP, HTTPStatus: 503}, true},
		{"invalid_parameters", &transport.CallError{Stage: transport.CallStageJSONRPC, RPCCode: -32602}, false},
		{"unknown_tool", &transport.CallError{Stage: transport.CallStageJSONRPC, RPCCode: -32601}, false},
		{"rpc_internal", &transport.CallError{Stage: transport.CallStageJSONRPC, RPCCode: -32603}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := apperrors.NewAPI("fixture failure", apperrors.WithCause(tc.cause))
			if got := outboundCallSubmissionUncertain(err); got != tc.uncertain {
				t.Fatalf("submission uncertainty = %v, want %v", got, tc.uncertain)
			}
		})
	}
}
