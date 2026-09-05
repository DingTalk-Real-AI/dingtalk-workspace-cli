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
	"errors"
	"fmt"
	"io"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageWithValidation(t *testing.T) {
	raw := errors.New("invalid value")
	for _, failure := range []error{raw, apperrors.NewAPI("api"), &validationExitCoderError{}, context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			input := fmt.Errorf("wrapped: %w", failure)
			validations, continuations := 0, 0
			run := WithValidation(func(*cobra.Command, []string) error {
				validations++
				return input
			}, func(*cobra.Command, []string) error {
				continuations++
				return nil
			})
			for i := 0; i < 2; i++ {
				err := run(&cobra.Command{}, nil)
				if failure == raw {
					requireValidationError(t, err, "invalid_parameters")
					if !errors.Is(err, input) {
						t.Fatal("lost validation cause")
					}
				} else if err != input {
					t.Fatalf("error identity changed: %v", err)
				}
			}
			if validations != 2 || continuations != 0 {
				t.Fatalf("calls: validation=%d continuation=%d", validations, continuations)
			}
		})
	}
	for _, hasValidate := range []bool{false, true} {
		var validate func(*cobra.Command, []string) error
		var order []string
		if hasValidate {
			validate = func(*cobra.Command, []string) error { order = append(order, "validate"); return nil }
		}
		business := errors.New("business I/O failure")
		run := WithValidation(validate, func(*cobra.Command, []string) error { order = append(order, "business"); return business })
		if got := run(&cobra.Command{}, nil); got != business {
			t.Fatal("business error was reclassified")
		}
		if hasValidate && fmt.Sprint(order) != "[validate business]" || !hasValidate && fmt.Sprint(order) != "[business]" {
			t.Fatalf("order=%v", order)
		}
	}
	t.Run("nil continuation", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("nil continuation must panic during construction")
			}
		}()
		WithValidation(nil, nil)
	})
}

func TestCrossPlatformCoveragePrepareCommandTree(t *testing.T) {
	t.Run("prepared subtree rejects entire assembly without mutation", func(t *testing.T) {
		child := &cobra.Command{Use: "child"}
		prepareValidationTree(t, child)
		raw := errors.New("raw args")
		root := &cobra.Command{Use: "root", Args: func(*cobra.Command, []string) error { return raw }}
		root.AddCommand(child)
		if err := PrepareCommandTree(root); err == nil {
			t.Fatal("prepared subtree accepted")
		}
		if root.Args(root, nil) != raw || root.Annotations[preparedCommandAnnotation] != "" {
			t.Fatal("failed preparation mutated the root")
		}
		if err := PrepareCommandTree(child); err == nil {
			t.Fatal("non-root accepted")
		}
	})
	t.Run("handler snapshots do not recursively capture ancestors", func(t *testing.T) {
		calls := 0
		root := &cobra.Command{Use: "root"}
		root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { calls++; return fmt.Errorf("hint: %w", err) })
		group := &cobra.Command{Use: "group"}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		group.AddCommand(leaf)
		root.AddCommand(group)
		prepareValidationTree(t, root)
		for _, node := range []*cobra.Command{root, group, leaf} {
			calls = 0
			requireValidationError(t, node.FlagErrorFunc()(node, errors.New("bad flag")), "invalid_flag")
			if calls != 1 {
				t.Fatalf("%s handler called %d times", node.CommandPath(), calls)
			}
		}
	})
	t.Run("Cobra reuse and fresh invocation are distinct", func(t *testing.T) {
		calls := 0
		newCommand := func() *cobra.Command {
			cmd := New(Spec{Use: "leaf", Flags: []FlagSpec{{Name: "name", Kind: KindString, MarkRequired: true}}, Invoke: func(*Ctx, map[string]any) error { calls++; return nil }})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			prepareValidationTree(t, cmd)
			return cmd
		}
		cmd := newCommand()
		cmd.SetArgs([]string{"--name", "Alice"})
		if _, err := cmd.ExecuteC(); err != nil {
			t.Fatal(err)
		}
		cmd.SetArgs([]string{})
		if _, err := cmd.ExecuteC(); err != nil {
			t.Fatalf("Cobra retained-value compatibility changed: %v", err)
		}
		if !cmd.Flags().Changed("name") || calls != 2 {
			t.Fatal("Cobra flag state unexpectedly reset")
		}
		fresh := newCommand()
		fresh.SetArgs([]string{})
		_, err := fresh.ExecuteC()
		requireValidationError(t, err, "missing_required_flags")
		if calls != 2 {
			t.Fatal("fresh invocation ran with missing parameter")
		}
	})
	t.Run("alias normalization before required and group checks", func(t *testing.T) {
		cmd := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		cmd.Flags().String("canonical", "", "")
		cmd.Flags().String("alias", "", "")
		_ = cmd.MarkFlagRequired("canonical")
		cmd.MarkFlagsOneRequired("canonical", "alias")
		cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
			value, _ := cmd.Flags().GetString("alias")
			return cmd.Flags().Set("canonical", value)
		}
		prepareValidationTree(t, cmd)
		cmd.SetArgs([]string{"--alias", "value"})
		if _, err := cmd.ExecuteC(); err != nil {
			t.Fatal(err)
		}
	})
}
