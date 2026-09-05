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
	"context"
	stderrors "errors"
	"io"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageValidationTraversal(t *testing.T) {
	for _, traverse := range []bool{false, true} {
		name := "find"
		if traverse {
			name = "traverse"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			root := &cobra.Command{Use: "root", TraverseChildren: traverse, SilenceErrors: true, SilenceUsage: true}
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.PersistentFlags().Int("count", 0, "")
			root.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { calls++; return nil }})
			if err := PrepareCommandTree(root); err != nil {
				t.Fatal(err)
			}
			root.SetArgs([]string{"--count", "bad", "leaf"})
			_, err := root.ExecuteC()
			requireValidationError(t, err, "invalid_flag")
			if calls != 0 {
				t.Fatal("invalid invocation reached business code")
			}
		})
	}

	for _, scope := range []string{"root-local", "root-persistent", "nested-local", "nested-persistent", "inherited-persistent"} {
		for _, invalid := range []string{"--count=bad", "--unknown=value"} {
			for _, outcome := range []string{"original", "nil", "api", "canceled", "deadline", "exit-code"} {
				t.Run(scope+"/"+invalid+"/"+outcome, func(t *testing.T) {
					root := &cobra.Command{Use: "root", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
					root.SetOut(io.Discard)
					root.SetErr(io.Discard)
					group := &cobra.Command{Use: "group"}
					business, hooks, calls, ancestorCalls := 0, 0, 0, 0
					leaf := &cobra.Command{Use: "leaf", PreRun: func(*cobra.Command, []string) { hooks++ }, Run: func(*cobra.Command, []string) { business++ }}
					root.AddCommand(group)
					group.AddCommand(leaf)
					parser := group
					args := []string{"group", invalid, "leaf"}
					switch scope {
					case "root-local":
						root.Flags().Int("count", 0, "")
						parser, args = root, []string{invalid, "group", "leaf"}
					case "root-persistent":
						root.PersistentFlags().Int("count", 0, "")
						parser, args = root, []string{invalid, "group", "leaf"}
					case "nested-local":
						group.Flags().Int("count", 0, "")
					case "nested-persistent":
						group.PersistentFlags().Int("count", 0, "")
					case "inherited-persistent":
						root.PersistentFlags().Int("count", 0, "")
					}
					var want, original error
					switch outcome {
					case "api":
						want = apperrors.NewAPI("remote failure")
					case "canceled":
						want = context.Canceled
					case "deadline":
						want = context.DeadlineExceeded
					case "exit-code":
						want = &validationExitCoderError{}
					}
					root.SetFlagErrorFunc(func(*cobra.Command, error) error { ancestorCalls++; return stderrors.New("wrong ancestor handler") })
					parser.SetFlagErrorFunc(func(current *cobra.Command, err error) error {
						calls++
						if current != parser {
							t.Fatalf("handler command = %v, want %v", current, parser)
						}
						original = err
						if outcome == "original" {
							return err
						}
						return want
					})
					prepareValidationTree(t, root)
					root.SetArgs(args)
					_, err := root.ExecuteC()
					if want != nil {
						if err != want {
							t.Fatalf("error identity = %v, want %v", err, want)
						}
					} else {
						requireValidationError(t, err, "invalid_flag")
						if original == nil || !stderrors.Is(err, original) {
							t.Fatalf("parser cause lost: %v", err)
						}
					}
					if calls != 1 || ancestorCalls != 0 || hooks != 0 || business != 0 {
						t.Fatalf("handler=%d ancestor=%d hooks=%d business=%d", calls, ancestorCalls, hooks, business)
					}
				})
			}
		}
	}

	t.Run("authoritative parser causes bypass replacement handlers", func(t *testing.T) {
		for _, cause := range []error{apperrors.NewAPI("remote failure"), context.Canceled, context.DeadlineExceeded, &validationExitCoderError{}} {
			t.Run(cause.Error(), func(t *testing.T) {
				root := &cobra.Command{Use: "root", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
				group := &cobra.Command{Use: "group"}
				group.Flags().Var(&validationFailingValue{err: cause}, "value", "")
				replacements, boundaryCalls, business := 0, 0, 0
				group.SetFlagErrorFunc(func(*cobra.Command, error) error { replacements++; return stderrors.New("replacement") })
				group.AddCommand(&cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) { business++ }})
				root.AddCommand(group)
				prepareValidationTree(t, root)
				// Observe the input to the installed adapter without replacing its
				// policy, just as the app's transparent cleanup decorator does.
				adapter := group.FlagErrorFunc()
				var parserErr error
				group.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
					boundaryCalls++
					parserErr = err
					return adapter(cmd, err)
				})
				root.SetArgs([]string{"group", "--value=bad", "leaf"})
				_, err := root.ExecuteC()
				if err == nil || err != parserErr || !stderrors.Is(err, cause) || replacements != 0 || boundaryCalls != 1 || business != 0 {
					t.Fatalf("err=%v parser=%v cause=%v replacements=%d boundary=%d business=%d", err, parserErr, cause, replacements, boundaryCalls, business)
				}
			})
		}
	})

	t.Run("successful traversal preserves selection flags and hook order", func(t *testing.T) {
		var order []string
		root := &cobra.Command{Use: "root", TraverseChildren: true}
		root.Flags().String("root-local", "", "")
		root.PersistentFlags().String("shared", "", "")
		group := &cobra.Command{Use: "group", Aliases: []string{"g"}, PersistentPreRun: func(*cobra.Command, []string) { order = append(order, "persistent") }}
		group.Flags().String("group-local", "", "")
		leaf := &cobra.Command{Use: "leaf", Args: func(*cobra.Command, []string) error { order = append(order, "args"); return nil }, PreRun: func(*cobra.Command, []string) { order = append(order, "pre") }, Run: func(*cobra.Command, []string) { order = append(order, "run") }, PostRun: func(*cobra.Command, []string) { order = append(order, "post") }}
		root.AddCommand(group)
		group.AddCommand(leaf)
		root.SetFlagErrorFunc(func(*cobra.Command, error) error { t.Fatal("successful parse invoked error handler"); return nil })
		prepareValidationTree(t, root)
		root.SetArgs([]string{"--root-local=one", "g", "--group-local=two", "--shared=three", "leaf", "position"})
		selected, err := root.ExecuteC()
		if err != nil || selected != leaf {
			t.Fatalf("selected=%v err=%v", selected, err)
		}
		if !reflect.DeepEqual(order, []string{"args", "persistent", "pre", "run", "post"}) {
			t.Fatalf("hook order = %v", order)
		}
		for _, check := range []struct {
			cmd        *cobra.Command
			flag, want string
		}{{root, "root-local", "one"}, {group, "group-local", "two"}, {leaf, "shared", "three"}} {
			value, err := check.cmd.Flags().GetString(check.flag)
			if err != nil || value != check.want {
				t.Fatalf("%s = %q, %v", check.flag, value, err)
			}
		}
	})
}
