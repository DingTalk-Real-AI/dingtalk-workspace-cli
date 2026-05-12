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

package app

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestFlagErrorWithSuggestions_authStructured(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "login", Run: func(*cobra.Command, []string) {}}
	orig := fmt.Errorf("unknown flag: --json")
	err := flagErrorWithSuggestions(cmd, orig)
	var ae *apperrors.Error
	if !stderrors.As(err, &ae) {
		t.Fatalf("want *apperrors.Error, got %T", err)
	}
	if ae.Message != orig.Error() {
		t.Fatalf("Message = %q, want %q", ae.Message, orig.Error())
	}
	if ae.Reason != "unknown_flag" {
		t.Fatalf("Reason = %q, want unknown_flag", ae.Reason)
	}
	if ae.Hint == "" || !strings.Contains(ae.Hint, "format json") {
		t.Fatalf("Hint = %q", ae.Hint)
	}
	if ae.Cause != orig {
		t.Fatalf("Cause = %v, want orig", ae.Cause)
	}
	if !stderrors.Is(err, orig) {
		t.Fatal("errors.Is(err, orig) should hold via unwrap")
	}
}

func TestFlagErrorWithSuggestions_unknownFlagHintAndFlags(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().String("start", "", "begin time")
	_ = cmd.Flags().SetAnnotation("start", "x-cli-format", []string{"date-time"})
	orig := fmt.Errorf("unknown flag: --starttime1")
	err := flagErrorWithSuggestions(cmd, orig)
	var ae *apperrors.Error
	if !stderrors.As(err, &ae) {
		t.Fatalf("want *apperrors.Error, got %T", err)
	}
	if ae.Reason != "unknown_flag" {
		t.Fatalf("Reason = %q", ae.Reason)
	}
	if strings.Contains(ae.Hint, "Space required") {
		t.Fatalf("false glue must not suggest space: %q", ae.Hint)
	}
	if !strings.Contains(ae.Hint, "help") {
		t.Fatalf("expected help fallback in hint, got %q", ae.Hint)
	}
	if len(ae.AvailableFlags) != 1 || ae.AvailableFlags[0] != "start" {
		t.Fatalf("AvailableFlags = %v, want [start]", ae.AvailableFlags)
	}
}
