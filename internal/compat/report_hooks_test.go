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

package compat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newReportSubmitStub mirrors the leaf command shape emitted by
// BuildDynamicCommands for `report entry submit` (envelope: create_report).
// Only the flags the hook touches are registered. --contents is marked
// required to reproduce the envelope's MarkFlagRequired so the relaxation
// behaviour can be asserted.
func newReportSubmitStub() *cobra.Command {
	cmd := &cobra.Command{Use: "submit", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("contents", "", "contents JSON array")
	cmd.Flags().String("contents-file", "", "contents JSON file")
	cmd.Flags().String("template-id", "", "template id")
	_ = cmd.MarkFlagRequired("contents")
	return cmd
}

const reportContentsPayload = `[{"key":"今日完成工作","sort":"0","content":"done","contentType":"markdown","type":"1"}]`

func TestResolveReportContents_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contents.json")
	if err := os.WriteFile(path, []byte(reportContentsPayload), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	cmd := newReportSubmitStub()
	if err := cmd.Flags().Set("contents-file", path); err != nil {
		t.Fatal(err)
	}
	if err := resolveReportContents(cmd); err != nil {
		t.Fatalf("resolveReportContents(file): %v", err)
	}
	got, _ := cmd.Flags().GetString("contents")
	if got != reportContentsPayload {
		t.Fatalf("--contents not populated from file: %q", got)
	}
	// contents-file must be cleared so omitWhen:empty drops the dead param.
	if cf, _ := cmd.Flags().GetString("contents-file"); cf != "" {
		t.Fatalf("--contents-file should be cleared after resolution, got %q", cf)
	}
}

func TestResolveReportContents_FromStdin(t *testing.T) {
	cmd := newReportSubmitStub()
	if err := cmd.Flags().Set("contents", "-"); err != nil {
		t.Fatal(err)
	}
	cmd.SetIn(strings.NewReader(reportContentsPayload))
	if err := resolveReportContents(cmd); err != nil {
		t.Fatalf("resolveReportContents(stdin): %v", err)
	}
	got, _ := cmd.Flags().GetString("contents")
	if got != reportContentsPayload {
		t.Fatalf("--contents not populated from stdin: %q", got)
	}
}

func TestResolveReportContents_InlineUntouched(t *testing.T) {
	cmd := newReportSubmitStub()
	if err := cmd.Flags().Set("contents", reportContentsPayload); err != nil {
		t.Fatal(err)
	}
	if err := resolveReportContents(cmd); err != nil {
		t.Fatalf("resolveReportContents(inline): %v", err)
	}
	got, _ := cmd.Flags().GetString("contents")
	if got != reportContentsPayload {
		t.Fatalf("inline --contents must be left untouched, got %q", got)
	}
}

func TestResolveReportContents_FilePriorityOverInline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contents.json")
	if err := os.WriteFile(path, []byte(reportContentsPayload), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newReportSubmitStub()
	if err := cmd.Flags().Set("contents", `[{"stale":"inline"}]`); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("contents-file", path); err != nil {
		t.Fatal(err)
	}
	if err := resolveReportContents(cmd); err != nil {
		t.Fatalf("resolveReportContents: %v", err)
	}
	got, _ := cmd.Flags().GetString("contents")
	if got != reportContentsPayload {
		t.Fatalf("--contents-file must win over inline --contents, got %q", got)
	}
}

func TestResolveReportContents_MissingFileErrors(t *testing.T) {
	cmd := newReportSubmitStub()
	if err := cmd.Flags().Set("contents-file", filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatal(err)
	}
	err := resolveReportContents(cmd)
	if err == nil {
		t.Fatal("expected error for missing --contents-file path")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── installReportHook composition ──────────────────────────────

func TestInstallReportHook_RelaxesRequiredToOneOf(t *testing.T) {
	cmd := newReportSubmitStub()
	// Before the hook, --contents carries the cobra required annotation.
	if cmd.Flags().Lookup("contents").Annotations[cobra.BashCompOneRequiredFlag] == nil {
		t.Fatal("precondition: --contents should start out required")
	}
	installReportHook(cmd, "report", "create_report")
	// After the hook, the individual required annotation must be cleared so a
	// --contents-file-only invocation is not rejected at parse time.
	if cmd.Flags().Lookup("contents").Annotations[cobra.BashCompOneRequiredFlag] != nil {
		t.Fatal("installReportHook should clear the individual required on --contents")
	}
	// And a PreRunE must now be installed to resolve the source.
	if cmd.PreRunE == nil {
		t.Fatal("installReportHook should install a PreRunE")
	}
}

func TestInstallReportHook_NoOpForOtherProduct(t *testing.T) {
	cmd := newReportSubmitStub()
	installReportHook(cmd, "chat", "create_report")
	if cmd.Flags().Lookup("contents").Annotations[cobra.BashCompOneRequiredFlag] == nil {
		t.Fatal("non-report product must not touch required annotation")
	}
	if cmd.PreRunE != nil {
		t.Fatal("non-report product must not install a PreRunE")
	}
}

func TestInstallReportHook_NoOpForOtherReportTool(t *testing.T) {
	cmd := newReportSubmitStub()
	installReportHook(cmd, "report", "get_received_report_list")
	if cmd.PreRunE != nil {
		t.Fatal("non-target report tool must not install a PreRunE")
	}
}

func TestInstallReportHook_ChainsExistingPreRunE(t *testing.T) {
	cmd := newReportSubmitStub()
	originalCalled := false
	cmd.PreRunE = func(*cobra.Command, []string) error { originalCalled = true; return nil }
	installReportHook(cmd, "report", "create_report")
	if err := cmd.Flags().Set("contents", reportContentsPayload); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !originalCalled {
		t.Fatal("original PreRunE was dropped")
	}
}

func TestInstallReportHook_BailsIfChainedPreRunEFails(t *testing.T) {
	cmd := newReportSubmitStub()
	cmd.PreRunE = func(*cobra.Command, []string) error { return errors.New("original boom") }
	installReportHook(cmd, "report", "create_report")
	err := cmd.PreRunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "original boom") {
		t.Fatalf("expected original PreRunE error to bubble, got %v", err)
	}
}

func TestInstallReportHook_NilCmdSafe(t *testing.T) {
	installReportHook(nil, "report", "create_report")
}
