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

func newFlagsTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "send", Example: "  dws send --content hi"}
	cmd.Flags().String("content", "", "内容")
	cmd.Flags().String("title", "", "标题")
	return cmd
}

func TestValidateRequiredFlagsReportsAllMissing(t *testing.T) {
	cmd := newFlagsTestCommand()
	err := ValidateRequiredFlags(cmd, "content", "title")
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content, --title") {
		t.Fatalf("err = %v, want both flags reported", err)
	}
	if !strings.Contains(err.Error(), cmd.Example) {
		t.Fatalf("err = %v, want example hint", err)
	}
}

func TestValidateRequiredFlagsPassesWhenSet(t *testing.T) {
	cmd := newFlagsTestCommand()
	if err := cmd.Flags().Set("content", "hi"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequiredFlags(cmd, "content"); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestMissingRequiredFlagsErrorNilForEmpty(t *testing.T) {
	if err := MissingRequiredFlagsError(newFlagsTestCommand()); err != nil {
		t.Fatalf("err = %v, want nil for no missing flags", err)
	}
}

func TestMissingRequiredFlagsErrorFormatsNames(t *testing.T) {
	err := MissingRequiredFlagsError(newFlagsTestCommand(), "content")
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content") {
		t.Fatalf("err = %v, want formatted missing flag", err)
	}
}

func TestValidateRequiredFlagWithAliases(t *testing.T) {
	cmd := newFlagsTestCommand()
	cmd.Flags().String("body", "", "别名")
	err := ValidateRequiredFlagWithAliases(cmd, "content", "body")
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --content (or --body)") {
		t.Fatalf("err = %v, want alias-aware missing error", err)
	}
	if err := cmd.Flags().Set("body", "hi"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequiredFlagWithAliases(cmd, "content", "body"); err != nil {
		t.Fatalf("err = %v, want nil when alias satisfies requirement", err)
	}
}
