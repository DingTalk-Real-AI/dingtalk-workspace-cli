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
	"bytes"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRecoveryDeprecatedUnsupportedShim(t *testing.T) {
	root := NewRootCommand()
	group := mustFindCommand(t, root, "recovery")
	if group.Hidden || group.Deprecated == "" || !group.Runnable() {
		t.Fatalf("recovery group contract: hidden=%v deprecated=%q runnable=%v", group.Hidden, group.Deprecated, group.Runnable())
	}

	for _, leaf := range []string{"plan", "execute", "finalize"} {
		cmd := mustFindCommand(t, root, "recovery", leaf)
		if cmd.Hidden || cmd.Deprecated == "" || !cmd.Runnable() {
			t.Fatalf("recovery %s contract: hidden=%v deprecated=%q runnable=%v", leaf, cmd.Hidden, cmd.Deprecated, cmd.Runnable())
		}
		wantFlags := []string{"event-id"}
		switch leaf {
		case "plan", "execute":
			wantFlags = append(wantFlags, "last")
		case "finalize":
			wantFlags = append(wantFlags, "outcome", "execution-file")
		}
		for _, flag := range wantFlags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Fatalf("recovery %s missing --%s", leaf, flag)
			}
		}
		for _, child := range newRecoveryCommand().Commands() {
			if child.Name() != leaf {
				continue
			}
			if err := child.RunE(child, nil); err == nil || !strings.Contains(err.Error(), "不再支持") {
				t.Fatalf("recovery %s RunE = %v, want 不再支持", leaf, err)
			}
		}
	}

	for _, format := range []string{"", "json", "pretty", "table"} {
		var out bytes.Buffer
		cmd := &cobra.Command{Use: "dws"}
		cmd.PersistentFlags().String("format", format, "")
		cmd.SetOut(&out)
		sub := &cobra.Command{Use: "recovery"}
		cmd.AddCommand(sub)
		err := printRecoveryUnsupported(sub, "dws recovery plan")
		if err == nil {
			t.Fatalf("format=%q returned nil error", format)
		}
		typed, ok := err.(*apperrors.Error)
		if !ok || typed.Category != apperrors.CategoryValidation {
			t.Fatalf("format=%q error = %T/%v, want validation Error", format, err, err)
		}
		got := out.String() + err.Error()
		if !strings.Contains(got, "不再支持") {
			t.Fatalf("format=%q missing 不再支持:\n%s", format, got)
		}
		if format == "" || format == "json" || format == "pretty" {
			if !strings.Contains(got, `"status":"unsupported"`) && !strings.Contains(got, `"status": "unsupported"`) {
				t.Fatalf("format=%q missing unsupported JSON status:\n%s", format, got)
			}
		}
	}

	for _, format := range []string{"json", "pretty", "table"} {
		cmd := &cobra.Command{Use: "dws"}
		cmd.PersistentFlags().String("format", format, "")
		cmd.SetOut(failWriter{})
		sub := &cobra.Command{Use: "recovery"}
		cmd.AddCommand(sub)
		if err := printRecoveryUnsupported(sub, "dws recovery plan"); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("format=%q write failure = %v, want write failed", format, err)
		}
	}

	if err := newRecoveryCommand().RunE(newRecoveryCommand(), nil); err == nil || !strings.Contains(err.Error(), "不再支持") {
		t.Fatalf("recovery parent RunE = %v, want 不再支持", err)
	}
	captureRuntimeFailure(executor.Invocation{}, nil, nil)
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

var errWriteFailed = errors.New("write failed")
