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

package cmdutil

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSuggestFlagFix_falseGlue_starttime1(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("start", "", "begin time")
	_ = cmd.Flags().SetAnnotation("start", "x-cli-format", []string{"date-time"})

	err := errUnknownFlag("starttime1")
	fix := SuggestFlagFix(cmd, err)
	if strings.Contains(fix.Suggestion, "Space required") {
		t.Fatalf("should not treat as glued value, got %q", fix.Suggestion)
	}
	if fix.AutoFixFlag != "" {
		t.Fatalf("AutoFixFlag = %q, want empty", fix.AutoFixFlag)
	}
	if !strings.Contains(fix.Suggestion, "--help") {
		t.Fatalf("expected help fallback, got %q", fix.Suggestion)
	}
}

func TestSuggestFlagFix_trueGlue_isoDateSuffix(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("start", "", "begin time")
	_ = cmd.Flags().SetAnnotation("start", "x-cli-format", []string{"date-time"})

	err := errUnknownFlag("start2026-02-07")
	fix := SuggestFlagFix(cmd, err)
	wantSub := "Space required between flag and value: --start 2026-02-07"
	if fix.Suggestion != wantSub {
		t.Fatalf("Suggestion = %q, want %q", fix.Suggestion, wantSub)
	}
	if fix.AutoFixFlag != "start" || fix.AutoFixValue != "2026-02-07" {
		t.Fatalf("AutoFix = %q/%q, want start/2026-02-07", fix.AutoFixFlag, fix.AutoFixValue)
	}
}

func TestSuggestFlagFix_levenshteinAddsUsage(t *testing.T) {
	cmd := &cobra.Command{Use: "send"}
	cmd.Flags().String("conversation-id", "", "Conversation id")

	err := errUnknownFlag("conversaton-id")
	fix := SuggestFlagFix(cmd, err)
	if !strings.HasPrefix(fix.Suggestion, "Did you mean --conversation-id?") {
		t.Fatalf("unexpected: %q", fix.Suggestion)
	}
	if !strings.Contains(fix.Suggestion, "Conversation id") {
		t.Fatalf("expected usage in hint, got %q", fix.Suggestion)
	}
}

func TestSuggestFlagFix_skipsHiddenAlias(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("start", "", "begin time")
	_ = cmd.Flags().SetAnnotation("start", "x-cli-format", []string{"date-time"})
	cmd.Flags().String("start-time", "", "")
	_ = cmd.Flags().MarkHidden("start-time")

	fix := SuggestFlagFix(cmd, errUnknownFlag("starttime1"))
	if strings.Contains(fix.Suggestion, "start-time") {
		t.Fatalf("must not recommend hidden alias, got %q", fix.Suggestion)
	}
	if !strings.Contains(fix.Suggestion, "--help") {
		t.Fatalf("expected help fallback, got %q", fix.Suggestion)
	}
}

func TestVisibleFlagNames_skipsHiddenAndInternal(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("alpha", "", "")
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("beta", "", "")
	_ = cmd.Flags().MarkHidden("beta")

	names := VisibleFlagNames(cmd)
	if len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("got %v, want [alpha]", names)
	}
}

func errUnknownFlag(body string) error {
	return &stubFlagErr{msg: "unknown flag: --" + body}
}

type stubFlagErr struct{ msg string }

func (e *stubFlagErr) Error() string { return e.msg }
