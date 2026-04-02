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

// Package cmdutil provides reusable CLI helper functions for Cobra commands.
// Both the open-source CLI and private overlays import this package to avoid
// duplicating flag validation, time parsing, and UX helpers.
package cmdutil

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// GroupRunE is a reusable RunE for parent (group) commands that have no
// business logic of their own. With args it returns an error listing available
// subcommands; without args it shows help.
func GroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		var names []string
		for _, sub := range cmd.Commands() {
			if !sub.Hidden {
				names = append(names, sub.Name())
			}
		}
		return fmt.Errorf("unknown subcommand %q for %q\n  available: %s\n  hint: %s --help",
			args[0], cmd.CommandPath(), strings.Join(names, ", "), cmd.CommandPath())
	}
	return cmd.Help()
}

// HintSubCmd creates a hidden subcommand that only prints a disambiguation hint.
func HintSubCmd(use, hint string) *cobra.Command {
	return &cobra.Command{
		Use:    use,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ambiguous command %q for %q\n  hint: %s",
				use, cmd.Parent().CommandPath(), hint)
		},
	}
}

// MustGetFlag retrieves a string flag value, checking both local and inherited flags.
func MustGetFlag(cmd *cobra.Command, name string) string {
	val, _ := cmd.Flags().GetString(name)
	if val == "" {
		val, _ = cmd.InheritedFlags().GetString(name)
	}
	return val
}

// FlagOrFallback reads the primary flag; if empty, falls back through alias
// flags in order, returning the first non-empty value.
func FlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	if v, _ := cmd.Flags().GetString(primary); v != "" {
		return v
	}
	for _, alias := range aliases {
		if v, _ := cmd.Flags().GetString(alias); v != "" {
			return v
		}
	}
	return ""
}

// MustFlagOrFallback works like FlagOrFallback but returns an error when all
// flags are empty.
func MustFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	v := FlagOrFallback(cmd, primary, aliases...)
	if v == "" {
		return "", fmt.Errorf("flag --%s is required\n  hint: %s", primary, cmd.Example)
	}
	return v, nil
}

// MustFlagWithHint returns an error with an explicit usage example when the
// flag is empty.
func MustFlagWithHint(cmd *cobra.Command, name, example string) (string, error) {
	v := MustGetFlag(cmd, name)
	if v == "" {
		return "", fmt.Errorf("flag --%s is required\n  hint: %s", name, example)
	}
	return v, nil
}

// ValidateRequiredFlags checks that all named string flags are non-empty.
// Returns a formatted error listing all missing flags, or nil.
func ValidateRequiredFlags(cmd *cobra.Command, names ...string) error {
	var missing []string
	for _, name := range names {
		v, _ := cmd.Flags().GetString(name)
		if v == "" {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required flag(s): %s\n  usage: %s\n  example:\n%s",
		strings.Join(missing, ", "), cmd.UseLine(), cmd.Example)
}

// ValidateRequiredFlagWithAliases checks that at least one of the primary flag
// or its aliases is non-empty.
func ValidateRequiredFlagWithAliases(cmd *cobra.Command, primary string, aliases ...string) error {
	if FlagOrFallback(cmd, primary, aliases...) != "" {
		return nil
	}
	all := append([]string{primary}, aliases...)
	return fmt.Errorf("missing required flag: --%s (or %s)\n  usage: %s\n  example:\n%s",
		primary, "--"+strings.Join(all[1:], " / --"), cmd.UseLine(), cmd.Example)
}

// ConfirmDelete asks for interactive confirmation before destructive operations.
// Returns true if --yes/-y flag is set or the user types "yes"/"y".
func ConfirmDelete(cmd *cobra.Command, resourceType, resourceName string) bool {
	if yes, _ := cmd.Flags().GetBool("yes"); yes {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == "--yes" || arg == "-y" {
			return true
		}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "About to delete %s: %s\n", resourceType, resourceName)
	fmt.Fprint(cmd.ErrOrStderr(), "Confirm deletion? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "yes" || answer == "y" {
		return true
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Operation cancelled")
	return false
}
