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
	stderrors "errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

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
	InstallValidationAdapters(nil)

	t.Run("positionals", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
		root.AddCommand(leaf)
		InstallValidationAdapters(root)
		InstallValidationAdapters(root)
		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_positionals")
	})

	t.Run("flag parse", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().Int("count", 0, "count")
		root.AddCommand(leaf)
		InstallValidationAdapters(root)
		root.SetArgs([]string{"leaf", "--count", "not-an-int"})
		_, err := root.ExecuteC()
		requireValidationError(t, err, "invalid_flag")
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
		InstallValidationAdapters(root)

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

	t.Run("pre-run-e", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		preRunCalled := false
		boom := stderrors.New("invalid pre-run value")
		leaf := &cobra.Command{
			Use: "leaf",
			PreRunE: func(*cobra.Command, []string) error {
				preRunCalled = true
				return boom
			},
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		root.AddCommand(leaf)
		InstallValidationAdapters(root)
		root.SetArgs([]string{"leaf"})
		_, err := root.ExecuteC()
		if !preRunCalled {
			t.Fatal("original PreRunE was not called")
		}
		requireValidationError(t, err, "invalid_parameters")
		if !stderrors.Is(err, boom) {
			t.Fatal("PreRunE error lost its cause")
		}
	})

	t.Run("flag group", func(t *testing.T) {
		root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().String("left", "", "left")
		leaf.Flags().String("right", "", "right")
		leaf.MarkFlagsOneRequired("left", "right")
		root.AddCommand(leaf)
		InstallValidationAdapters(root)
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
	_, err = cmd.ExecuteC()
	requireValidationError(t, err, "invalid_flag_value")

	cmd = New(Spec{
		Use:    "leaf",
		Flags:  []FlagSpec{{Name: "name", Usage: "name", MarkRequired: true}},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	_, err = cmd.ExecuteC()
	requireValidationError(t, err, "missing_required_flags")
}
