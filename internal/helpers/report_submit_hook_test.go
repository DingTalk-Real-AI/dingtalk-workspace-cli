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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

type recordingReportSenderSubmitter struct {
	calls      int
	submission ReportSenderSubmission
}

func (s *recordingReportSenderSubmitter) Submit(_ context.Context, _ *cobra.Command, submission ReportSenderSubmission) error {
	s.calls++
	s.submission = submission
	return nil
}

func TestAttachReportSenderSubmissionAddsFlagToCanonicalAndDeprecatedPaths(t *testing.T) {
	t.Parallel()

	report, _, _ := newReportSubmitTestTree()
	attachReportSenderSubmission([]*cobra.Command{report}, &recordingReportSenderSubmitter{})

	for _, path := range [][]string{{"entry", "submit"}, {"create"}} {
		cmd := findReportTestCommand(report, path...)
		if cmd == nil {
			t.Fatalf("command %v not found", path)
		}
		if cmd.Flags().Lookup("sender-user-id") == nil {
			t.Fatalf("command %v missing --sender-user-id", path)
		}
	}
}

func TestAttachReportSenderSubmissionDelegatesWithoutSender(t *testing.T) {
	t.Parallel()

	report, submit, originalCalls := newReportSubmitTestTree()
	submitter := &recordingReportSenderSubmitter{}
	attachReportSenderSubmission([]*cobra.Command{report}, submitter)

	if err := submit.RunE(submit, nil); err != nil {
		t.Fatalf("submit RunE error = %v", err)
	}
	if *originalCalls != 1 {
		t.Fatalf("original calls = %d, want 1", *originalCalls)
	}
	if submitter.calls != 0 {
		t.Fatalf("OAPI submitter calls = %d, want 0", submitter.calls)
	}
}

func TestAttachReportSenderSubmissionRoutesSenderToOAPI(t *testing.T) {
	t.Parallel()

	report, submit, originalCalls := newReportSubmitTestTree()
	submitter := &recordingReportSenderSubmitter{}
	attachReportSenderSubmission([]*cobra.Command{report}, submitter)

	contentsFile := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(contentsFile, []byte(`[{"key":"今日完成工作","sort":"0","type":"1","contentType":"markdown","content":"done"}]`), 0o600); err != nil {
		t.Fatalf("write contents file: %v", err)
	}
	mustSetReportTestFlag(t, submit, "sender-user-id", "sender-1")
	mustSetReportTestFlag(t, submit, "template-id", "template-1")
	mustSetReportTestFlag(t, submit, "contents-file", contentsFile)
	mustSetReportTestFlag(t, submit, "dd-from", "agent")
	mustSetReportTestFlag(t, submit, "to-chat", "true")
	mustSetReportTestFlag(t, submit, "to-user-ids", "receiver-1,receiver-2")

	if err := submit.RunE(submit, nil); err != nil {
		t.Fatalf("submit RunE error = %v", err)
	}
	if *originalCalls != 0 {
		t.Fatalf("original calls = %d, want 0", *originalCalls)
	}
	if submitter.calls != 1 {
		t.Fatalf("OAPI submitter calls = %d, want 1", submitter.calls)
	}
	if got := submitter.submission.SenderUserID; got != "sender-1" {
		t.Fatalf("sender = %q, want sender-1", got)
	}
	if got := len(submitter.submission.ToUserIDs); got != 2 {
		t.Fatalf("recipient count = %d, want 2", got)
	}
}

func newReportSubmitTestTree() (*cobra.Command, *cobra.Command, *int) {
	originalCalls := 0
	newLeaf := func(use string) *cobra.Command {
		cmd := &cobra.Command{
			Use: use,
			RunE: func(*cobra.Command, []string) error {
				originalCalls++
				return nil
			},
		}
		cmd.Flags().String("template-id", "", "")
		cmd.Flags().String("contents", "", "")
		cmd.Flags().String("contents-file", "", "")
		cmd.Flags().String("dd-from", "dws", "")
		cmd.Flags().Bool("to-chat", false, "")
		cmd.Flags().String("to-user-ids", "", "")
		return cmd
	}

	submit := newLeaf("submit")
	entry := &cobra.Command{Use: "entry"}
	entry.AddCommand(submit)
	create := newLeaf("create")
	report := &cobra.Command{Use: "report"}
	report.AddCommand(entry, create)
	return report, submit, &originalCalls
}

func findReportTestCommand(root *cobra.Command, path ...string) *cobra.Command {
	current := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func mustSetReportTestFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}
