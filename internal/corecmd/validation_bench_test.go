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
	"fmt"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

// BenchmarkPreparedCommandExecution isolates framework/Cobra dispatch from
// command-tree construction and transport. Reusing flags is deliberate here;
// each sub-benchmark repeats identical argv on its own prepared tree.
func BenchmarkPreparedCommandExecution(b *testing.B) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{"success", []string{"--name", "Alice", "--count", "2"}, false},
		{"invalid_flag", []string{"--count", "invalid"}, true},
		{"missing_required", []string{}, true},
		{"invalid_positionals", []string{"unexpected"}, true},
		{"invalid_parameters", []string{"--name", "invalid"}, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			cmd := New(Spec{
				Use:       "leaf",
				Flags:     []FlagSpec{{Name: "name", Kind: KindString, MarkRequired: true}, {Name: "count", Kind: KindInt}},
				PostMount: func(cmd *cobra.Command) { cmd.Args = cobra.NoArgs },
				Validate: func(cmd *cobra.Command, _ []string) error {
					name, _ := cmd.Flags().GetString("name")
					if name == "invalid" {
						return fmt.Errorf("name is invalid")
					}
					return nil
				},
				Invoke: func(*Ctx, map[string]any) error { return nil },
			})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			if err := PrepareCommandTree(cmd); err != nil {
				b.Fatal(err)
			}
			cmd.SetArgs(tc.args)
			// Warm Cobra's automatically generated help/completion state.
			if _, err := cmd.ExecuteC(); (err != nil) != tc.wantError {
				b.Fatalf("unexpected warm-up error: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := cmd.ExecuteC(); (err != nil) != tc.wantError {
					b.Fatalf("unexpected execution error: %v", err)
				}
			}
		})
	}
}
