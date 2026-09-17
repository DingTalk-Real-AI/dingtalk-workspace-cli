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

package corecmd

import (
	"bytes"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageLazyCobraValidation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		constraint string
		reason     string
	}{
		{"completion args", []string{"completion", "bash", "unexpected"}, "", "invalid_positionals"},
		{"hidden args", []string{"__complete"}, "", "invalid_positionals"},
		{"hidden alias args", []string{"__completeNoDesc"}, "", "invalid_positionals"},
		{"completion required", []string{"completion", "bash"}, "required", "missing_required_flags"},
		{"completion group", []string{"completion", "bash"}, "group", "invalid_flag_group"},
		{"custom help args", []string{"help"}, "help", "invalid_positionals"},
		{"completion success", []string{"completion", "bash"}, "", ""},
		{"hidden success", []string{"__complete", "leaf", ""}, "", ""},
		{"help success", []string{"help", "leaf"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
			root.SetOut(&output)
			root.SetErr(&output)
			root.AddCommand(&cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) { t.Fatal("completion/help invoked business") }})
			switch tc.constraint {
			case "required":
				root.PersistentFlags().String("name", "", "")
				if err := root.MarkPersistentFlagRequired("name"); err != nil {
					t.Fatal(err)
				}
			case "group":
				root.PersistentFlags().String("left", "", "")
				root.PersistentFlags().String("right", "", "")
				root.MarkFlagsOneRequired("left", "right")
			case "help":
				root.SetHelpCommand(&cobra.Command{Use: "help", Args: cobra.ExactArgs(1), Run: func(*cobra.Command, []string) { t.Fatal("invalid help executed") }})
			}
			prepareValidationTree(t, root)
			root.SetArgs(tc.args)
			cmd, err := root.ExecuteC()
			if cmd == nil || cmd == root {
				t.Fatalf("selected command=%v err=%v", cmd, err)
			}
			if cmd.Annotations[preparedCommandAnnotation] != "" {
				t.Fatal("fixture must exercise a lazily generated node")
			}
			if tc.reason != "" {
				requireValidationError(t, err, tc.reason)
			} else {
				if err != nil || output.Len() == 0 {
					t.Fatalf("success err=%v output=%q", err, output.String())
				}
				if tc.name == "hidden success" && !strings.Contains(output.String(), ":0") {
					t.Fatalf("completion protocol lost: %q", output.String())
				}
			}
		})
	}
}
