// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// recordingRunner captures invocations so tests can assert that a wrapper
// did or did not forward the call to the underlying handler.
type recordingRunner struct {
	calls []executor.Invocation
	err   error
}

func (r *recordingRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, inv)
	if r.err != nil {
		return executor.Result{}, r.err
	}
	return executor.Result{Invocation: inv, Response: map[string]any{"success": true}}, nil
}

// withReportDeprecationWarning is the contract every deprecated alias wraps
// its RunE with. The wrapper must:
//   - emit a stderr line starting with "[deprecated]"
//   - name both the old path the user invoked and the new canonical path
//   - call the wrapped handler exactly once and propagate its error
func TestWithReportDeprecationWarning_PrintsStderrAndForwards(t *testing.T) {
	t.Parallel()

	called := 0
	innerErr := errors.New("inner sentinel")
	wrapped := withReportDeprecationWarning("sent", "outbox list", func(*cobra.Command, []string) error {
		called++
		return innerErr
	})

	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "sent"}
	cmd.SetErr(&stderr)

	gotErr := wrapped(cmd, nil)
	if !errors.Is(gotErr, innerErr) {
		t.Fatalf("expected inner error to propagate, got %v", gotErr)
	}
	if called != 1 {
		t.Fatalf("inner handler called %d times, want 1", called)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "[deprecated]") {
		t.Fatalf("stderr missing [deprecated] marker: %q", msg)
	}
	if !strings.Contains(msg, "dws report sent") {
		t.Fatalf("stderr missing old path: %q", msg)
	}
	if !strings.Contains(msg, "dws report outbox list") {
		t.Fatalf("stderr missing new canonical path: %q", msg)
	}
}

// newReportCreatedCommand must be a leaf attached to the helpers report root,
// with the deprecation wrapper already on RunE pointing at outbox list. The
// shape — leaf, not group — matters because MergeHardcodedLeaves only grafts
// helper leaves that have no same-named envelope counterpart.
func TestNewReportCreatedCommand_IsLeafWithDeprecationWrapper(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	cmd := newReportCreatedCommand(runner)
	if cmd == nil {
		t.Fatal("nil command returned")
	}
	if cmd.Use != "created" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "created")
	}
	if cmd.HasSubCommands() {
		t.Fatalf("created should be a leaf, not a group")
	}
	if !strings.Contains(cmd.Short, "[deprecated]") {
		t.Fatalf("short missing [deprecated]: %q", cmd.Short)
	}
	if cmd.RunE == nil {
		t.Fatal("RunE not wired")
	}

	// Exercise RunE end-to-end with --dry-run via the root --dry-run flag
	// so the runner records the invocation without making any network call.
	var stderr bytes.Buffer
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", true, "")
	root.AddCommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"created", "--cursor", "0", "--size", "5", "--start", "2026-03-01T00:00:00+08:00", "--end", "2026-03-08T00:00:00+08:00"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stderr.String(), "[deprecated] `dws report created`") {
		t.Fatalf("stderr missing old path marker: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "dws report outbox list") {
		t.Fatalf("stderr missing new path: %q", stderr.String())
	}
}

// AttachReportLegacyInboxAlias targets the dynamic-built `report inbox` group:
// it must register the legacy flags (start/end/cursor/size/limit/...) and
// wire a deprecation-wrapped RunE. Subcommands (e.g. inbox list) must stay
// reachable and unmodified.
func TestAttachReportLegacyInboxAlias_EnrichesGroupInPlace(t *testing.T) {
	t.Parallel()

	// Build a stand-in dynamic tree mirroring the envelope shape: a `report`
	// root with an `inbox` group whose only child is `list`. The list leaf
	// already has the canonical flags from envelope-side registration.
	listLeaf := &cobra.Command{
		Use:  "list",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	listLeaf.Flags().String("start", "", "")
	listLeaf.Flags().String("end", "", "")

	inboxGroup := &cobra.Command{
		Use: "inbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	inboxGroup.AddCommand(listLeaf)

	reportRoot := &cobra.Command{Use: "report"}
	reportRoot.AddCommand(inboxGroup)

	runner := &recordingRunner{}
	AttachReportLegacyInboxAlias([]*cobra.Command{reportRoot}, runner)

	// Flags must have been added to the GROUP (not the list child).
	for _, name := range []string{"start", "end", "cursor", "size", "limit", "sender-user-ids"} {
		if inboxGroup.Flags().Lookup(name) == nil {
			t.Errorf("inbox group missing --%s after hook", name)
		}
	}
	// list child stays intact — hook must NOT have replaced it.
	if listLeaf.Parent() != inboxGroup {
		t.Errorf("list child detached from inbox group")
	}
	// Group RunE must now be the deprecation-wrapped handler. Verify by
	// invoking it directly with the required flags set on the group.
	var stderr bytes.Buffer
	inboxGroup.SetErr(&stderr)
	inboxGroup.SetOut(&bytes.Buffer{})

	if err := inboxGroup.Flags().Set("start", "2026-03-01T00:00:00+08:00"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := inboxGroup.Flags().Set("end", "2026-03-08T00:00:00+08:00"); err != nil {
		t.Fatalf("set end: %v", err)
	}

	// Wire a root with --dry-run so the runner records without network IO.
	dwsRoot := &cobra.Command{Use: "dws"}
	dwsRoot.PersistentFlags().Bool("dry-run", true, "")
	dwsRoot.AddCommand(reportRoot)

	if err := inboxGroup.RunE(inboxGroup, nil); err != nil {
		t.Fatalf("inbox RunE failed: %v", err)
	}
	if !strings.Contains(stderr.String(), "[deprecated] `dws report inbox`") {
		t.Fatalf("stderr missing deprecation marker: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "dws report inbox list") {
		t.Fatalf("stderr missing new path: %q", stderr.String())
	}
}

// AttachReportLegacyInboxAlias on a tree without `report inbox` must be a
// no-op — callers should be able to invoke the hook unconditionally.
func TestAttachReportLegacyInboxAlias_NoopWhenInboxAbsent(t *testing.T) {
	t.Parallel()

	other := &cobra.Command{Use: "other"}
	report := &cobra.Command{Use: "report"}
	// No inbox child.

	runner := &recordingRunner{}
	// Should not panic / not error.
	AttachReportLegacyInboxAlias([]*cobra.Command{nil, other, report}, runner)

	if report.HasSubCommands() {
		t.Errorf("expected no children synthesised; got %d", len(report.Commands()))
	}
}

// reportSentRunE / reportListRunE are extracted so that the legacy aliases
// (sent / created and list / inbox) can each wrap them. Sanity-check that
// reusing the extracted handler with a dry-run runner produces a
// recordingRunner invocation for the right MCP tool name.
func TestReportSentRunE_InvokesCanonicalTool(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	cmd := newReportSentCommand(runner)
	// Disable the wrapper to test the inner body directly.
	cmd.RunE = reportSentRunE(runner)

	var out bytes.Buffer
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", true, "")
	root.AddCommand(cmd)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"sent", "--cursor", "0", "--size", "5", "--start", "2026-03-01T00:00:00+08:00", "--end", "2026-03-08T00:00:00+08:00"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// EchoRunner-style: dry-run path short-circuits before runner.Run, so
	// recordingRunner.calls stays empty. We instead confirm the dry-run
	// payload is written to stdout.
	if !strings.Contains(out.String(), "get_send_report_list") {
		t.Fatalf("stdout missing canonical tool name: %q", out.String())
	}
}

func TestReportListRunE_InvokesCanonicalTool(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	cmd := newReportListCommand(runner)
	cmd.RunE = reportListRunE(runner)

	var out bytes.Buffer
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", true, "")
	root.AddCommand(cmd)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"list", "--cursor", "0", "--size", "5", "--start", "2026-03-01T00:00:00+08:00", "--end", "2026-03-08T00:00:00+08:00"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "get_received_report_list") {
		t.Fatalf("stdout missing canonical tool name: %q", out.String())
	}
}
