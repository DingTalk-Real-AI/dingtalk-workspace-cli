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
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

type validationExitCoderError struct{}

func (*validationExitCoderError) Error() string { return "explicit exit code" }
func (*validationExitCoderError) ExitCode() int { return 42 }

type validationFailingValue struct{ err error }

func (*validationFailingValue) String() string     { return "" }
func (v *validationFailingValue) Set(string) error { return v.err }
func (*validationFailingValue) Type() string       { return "validation-failing" }

func requireValidationError(t *testing.T, err error, reason string) {
	t.Helper()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("error = %T %v, want *errors.Error", err, err)
	}
	if typed.Category != apperrors.CategoryValidation || typed.Reason != reason || typed.ExitCode() != apperrors.ExitCodeValidation {
		t.Fatalf("error = category %q reason %q exit %d", typed.Category, typed.Reason, typed.ExitCode())
	}
}

func TestCrossPlatformCoverageValidationAdapters(t *testing.T) {
	if err := PrepareCommandTree(nil); err == nil {
		t.Fatal("nil root must fail preparation")
	}

	t.Run("positionals", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		if err := PrepareCommandTree(root); err == nil {
			t.Fatal("duplicate preparation must fail")
		}
		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_positionals")
	})

	t.Run("flag parse", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().Int("count", 0, "count")
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
	})

	t.Run("standalone corecmd flag parse", func(t *testing.T) {
		cmd := New(Spec{
			Use:    "leaf",
			Flags:  []FlagSpec{{Name: "count", Kind: KindInt}},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		prepareValidationTree(t, cmd)
		cmd.SetArgs([]string{"--count", "not-an-int"})
		_, err := cmd.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
	})

	t.Run("mounted corecmd inherits parent flag handler", func(t *testing.T) {
		rootHandlerErr := stderrors.New("root flag handler")
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		root.SetFlagErrorFunc(func(*cobra.Command, error) error { return rootHandlerErr })
		leaf := New(Spec{
			Use:    "leaf",
			Flags:  []FlagSpec{{Name: "count", Kind: KindInt}},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
		if !stderrors.Is(err, rootHandlerErr) {
			t.Fatalf("mounted flag error did not use the parent handler: %v", err)
		}
	})

	t.Run("nested corecmd inherits the nearest handler without recursion", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
			return fmt.Errorf("root handler: %w", err)
		})
		parent := New(Spec{
			Use:    "parent",
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		leaf := New(Spec{
			Use:    "leaf",
			Flags:  []FlagSpec{{Name: "count", Kind: KindInt}},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		parent.AddCommand(leaf)
		root.AddCommand(parent)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"parent", "leaf", "--count", "not-an-int"})

		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
		if !strings.Contains(err.Error(), "root handler") {
			t.Fatalf("nested flag error did not use the root handler: %v", err)
		}
	})

	t.Run("final install wraps a replaced handler", func(t *testing.T) {
		replacementErr := stderrors.New("replacement flag handler")
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := New(Spec{
			Use:    "leaf",
			Flags:  []FlagSpec{{Name: "count", Kind: KindInt}},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		leaf.SetFlagErrorFunc(func(*cobra.Command, error) error { return replacementErr })
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})

		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
		if !stderrors.Is(err, replacementErr) {
			t.Fatalf("flag error did not use replacement handler: %v", err)
		}
	})

	t.Run("final install wraps replaced args and pre-run hooks", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := New(Spec{
			Use:   "leaf",
			Flags: []FlagSpec{{Name: "name", Kind: KindString, Required: true}},
			PostMount: func(cmd *cobra.Command) {
				cmd.Args = cobra.ExactArgs(1)
			},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		rejectArgs := true
		leaf.Args = func(*cobra.Command, []string) error {
			if rejectArgs {
				return stderrors.New("replacement args error")
			}
			return nil
		}
		leaf.PreRunE = func(*cobra.Command, []string) error { return nil }
		root.AddCommand(leaf)
		prepareValidationTree(t, root)

		root.SetArgs([]string{"leaf", "value"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_positionals")

		rejectArgs = false
		root.SetArgs([]string{"leaf"})
		_, err = root.ExecuteC()
		requireValidationError(t, err, "missing_required_flags")
	})

	t.Run("preparation marks a custom pre-run hook", func(t *testing.T) {
		cmd := New(Spec{
			Use: "leaf",
			PostMount: func(cmd *cobra.Command) {
				cmd.PreRun = func(*cobra.Command, []string) {}
			},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		prepareValidationTree(t, cmd)
		if cmd.Annotations[preparedCommandAnnotation] != "true" {
			t.Fatal("command was not marked prepared")
		}
	})

	t.Run("nil flag handlers fall back to parser errors", func(t *testing.T) {
		rootHandlerCalls := 0
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
			rootHandlerCalls++
			if err == nil {
				t.Fatal("parent handler received nil")
			}
			return err
		})
		leaf := New(Spec{
			Use:   "leaf",
			Flags: []FlagSpec{{Name: "count", Kind: KindInt}},
			PostMount: func(cmd *cobra.Command) {
				cmd.SetFlagErrorFunc(func(*cobra.Command, error) error { return nil })
			},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})

		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
		if rootHandlerCalls != 0 {
			t.Fatalf("root handler calls = %d, want 0 for an explicit local handler", rootHandlerCalls)
		}

		bare := &cobra.Command{Use: "bare", SilenceErrors: true, SilenceUsage: true, RunE: func(*cobra.Command, []string) error { return nil }}
		bare.Flags().Int("count", 0, "count")
		bare.SetFlagErrorFunc(func(*cobra.Command, error) error { return nil })
		prepareValidationTree(t, bare)
		bare.SetArgs([]string{"--count", "not-an-int"})
		_, err = bare.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
	})

	t.Run("standalone preparation preserves authoritative parser causes", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{
			{name: "typed", err: apperrors.NewAuth("auth failed")},
			{name: "exit coder", err: &validationExitCoderError{}},
			{name: "canceled", err: context.Canceled},
			{name: "deadline", err: context.DeadlineExceeded},
		} {
			t.Run(tc.name, func(t *testing.T) {
				handlerCalls := 0
				cmd := New(Spec{
					Use: "leaf",
					PostMount: func(cmd *cobra.Command) {
						cmd.Flags().Var(&validationFailingValue{err: tc.err}, "value", "value")
						cmd.SetFlagErrorFunc(func(*cobra.Command, error) error {
							handlerCalls++
							return stderrors.New("handler destroyed parser cause")
						})
					},
					Invoke: func(*Ctx, map[string]any) error { return nil },
				})
				prepareValidationTree(t, cmd)
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				cmd.SetArgs([]string{"--value", "invalid"})

				_, err := cmd.ExecuteC()
				if !stderrors.Is(err, tc.err) {
					t.Fatalf("Execute error = %v, want parser cause %v", err, tc.err)
				}
				if handlerCalls != 0 {
					t.Fatalf("local handler calls = %d, want 0", handlerCalls)
				}
			})
		}
	})

	t.Run("final adapter preserves authoritative parser cause", func(t *testing.T) {
		parserCause := apperrors.NewAPI("API failed")
		replacementCalls := 0
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := New(Spec{
			Use: "leaf",
			PostMount: func(cmd *cobra.Command) {
				cmd.Flags().Var(&validationFailingValue{err: parserCause}, "value", "value")
			},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		leaf.SetFlagErrorFunc(func(*cobra.Command, error) error {
			replacementCalls++
			return stderrors.New("replacement destroyed parser cause")
		})
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf", "--value", "invalid"})

		_, err := root.ExecuteC()
		if !stderrors.Is(err, parserCause) {
			t.Fatalf("Execute error = %v, want parser cause %v", err, parserCause)
		}
		if replacementCalls != 0 {
			t.Fatalf("replacement handler calls = %d, want 0", replacementCalls)
		}
	})

	t.Run("mounted corecmd preserves post-mount handler", func(t *testing.T) {
		rootHandlerCalls := 0
		localHandlerCalls := 0
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
			rootHandlerCalls++
			return fmt.Errorf("root handler: %w", err)
		})
		leaf := New(Spec{
			Use:   "leaf",
			Flags: []FlagSpec{{Name: "count", Kind: KindInt}},
			PostMount: func(cmd *cobra.Command) {
				cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
					localHandlerCalls++
					return fmt.Errorf("local handler: %w", err)
				})
			},
			Invoke: func(*Ctx, map[string]any) error { return nil },
		})
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		if err := PrepareCommandTree(root); err == nil {
			t.Fatal("duplicate preparation must fail")
		}
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})

		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
		if !strings.HasPrefix(err.Error(), "local handler:") || strings.Contains(err.Error(), "root handler:") {
			t.Fatalf("flag error did not select the nearest handler: %v", err)
		}
		if localHandlerCalls != 1 || rootHandlerCalls != 0 {
			t.Fatalf("handler calls = local %d root %d, want local 1 root 0", localHandlerCalls, rootHandlerCalls)
		}
	})

	t.Run("mounted corecmd preserves authoritative local errors", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{
			{name: "typed", err: apperrors.NewAuth("auth failed")},
			{name: "exit coder", err: &validationExitCoderError{}},
			{name: "canceled", err: context.Canceled},
			{name: "deadline", err: context.DeadlineExceeded},
		} {
			t.Run(tc.name, func(t *testing.T) {
				parentHandlerCalls := 0
				root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
				root.SetFlagErrorFunc(func(*cobra.Command, error) error {
					parentHandlerCalls++
					return stderrors.New("parent replaced authoritative error")
				})
				leaf := New(Spec{
					Use:   "leaf",
					Flags: []FlagSpec{{Name: "count", Kind: KindInt}},
					PostMount: func(cmd *cobra.Command) {
						cmd.SetFlagErrorFunc(func(*cobra.Command, error) error { return tc.err })
					},
					Invoke: func(*Ctx, map[string]any) error { return nil },
				})
				root.AddCommand(leaf)
				prepareValidationTree(t, root)
				root.SetArgs([]string{"leaf", "--count", "not-an-int"})

				_, err := root.ExecuteC()
				if !stderrors.Is(err, tc.err) {
					t.Fatalf("Execute error = %v, want authoritative error %v", err, tc.err)
				}
				if parentHandlerCalls != 0 {
					t.Fatalf("parent handler calls = %d, want 0", parentHandlerCalls)
				}
			})
		}
	})

	t.Run("required after pre-run", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		preRunCalled := false
		leaf := &cobra.Command{
			Use: "leaf",
			PreRun: func(cmd *cobra.Command, _ []string) {
				preRunCalled = true
				if alias, _ := cmd.Flags().GetString("alias"); alias != "" {
					_ = cmd.Flags().Set("name", alias)
				}
			},
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		leaf.Flags().String("name", "", "name")
		leaf.Flags().String("alias", "", "alias")
		_ = leaf.MarkFlagRequired("name")
		root.AddCommand(leaf)
		prepareValidationTree(t, root)

		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		if !preRunCalled {
			t.Fatal("original PreRun was not called")
		}
		requireValidationError(t, err, "missing_required_flags")
		if err.Error() != "missing required flag(s): --name" {
			t.Fatalf("required message = %q", err)
		}
	})

	t.Run("pre-run-e preserves ordinary errors", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		preRunCalled := false
		boom := stderrors.New("pre-run I/O failed")
		leaf := &cobra.Command{
			Use: "leaf",
			PreRunE: func(*cobra.Command, []string) error {
				preRunCalled = true
				return boom
			},
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		if !preRunCalled {
			t.Fatal("original PreRunE was not called")
		}
		if err != boom {
			t.Fatalf("PreRunE error = %T %v, want original error", err, err)
		}
	})

	t.Run("flag group", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().String("left", "", "left")
		leaf.Flags().String("right", "", "right")
		leaf.MarkFlagsOneRequired("left", "right")
		root.AddCommand(leaf)
		prepareValidationTree(t, root)
		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag_group")
	})

	t.Run("required fallback", func(t *testing.T) {
		err := normalizeRequiredFlagError(&cobra.Command{Use: "leaf"}, stderrors.New("required"))
		requireValidationError(t, err, "missing_required_flags")
	})

	t.Run("required annotation false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "leaf"}
		cmd.Flags().String("required", "", "required")
		cmd.Flags().String("optional", "", "optional")
		_ = cmd.MarkFlagRequired("required")
		_ = cmd.Flags().SetAnnotation("optional", cobra.BashCompOneRequiredFlag, []string{"false"})
		if got := missingRequiredFlagMessage(cmd); got != "missing required flag(s): --required" {
			t.Fatalf("missingRequiredFlagMessage() = %q", got)
		}
	})
}

func TestCrossPlatformCoverageFrameworkValidationHooksAreTyped(t *testing.T) {
	boom := stderrors.New("invalid local value")
	cmd := New(Spec{
		Use: "leaf",
		Validate: func(*cobra.Command, []string) error {
			return boom
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	cmd.SetArgs(nil)
	prepareValidationTree(t, cmd)
	_, err := cmd.ExecuteC()
	requireValidationError(t, err, "invalid_parameters")
	if !stderrors.Is(err, boom) {
		t.Fatal("Validate error lost its cause")
	}

	cmd = New(Spec{
		Use: "leaf",
		Flags: []FlagSpec{{
			Name: "value", Usage: "value", Default: "bad",
			Transform: func(string) (any, error) { return nil, boom },
		}},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	prepareValidationTree(t, cmd)
	_, err = cmd.ExecuteC()
	requireValidationError(t, err, "invalid_flag_value")
	if !stderrors.Is(err, boom) {
		t.Fatal("Transform error lost its cause")
	}

	t.Setenv("DWS_VALIDATION_BAD_INT", "not-an-int")
	cmd = New(Spec{
		Use: "leaf",
		Flags: []FlagSpec{{
			Name: "count", Usage: "count", Kind: KindInt, EnvVar: "DWS_VALIDATION_BAD_INT",
		}},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	prepareValidationTree(t, cmd)
	_, err = cmd.ExecuteC()
	requireValidationError(t, err, "invalid_flag_value")

	cmd = New(Spec{
		Use:    "leaf",
		Flags:  []FlagSpec{{Name: "name", Usage: "name", MarkRequired: true}},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	prepareValidationTree(t, cmd)
	_, err = cmd.ExecuteC()
	requireValidationError(t, err, "missing_required_flags")
}

func prepareValidationTree(t *testing.T, root *cobra.Command) {
	t.Helper()
	if err := PrepareCommandTree(root); err != nil {
		t.Fatal(err)
	}
}
