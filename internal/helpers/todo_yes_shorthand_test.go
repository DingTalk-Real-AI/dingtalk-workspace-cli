// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import "testing"

// TestTodoTaskDeleteHasYesShorthand is a regression test for #370: the local
// --yes flag on `todo task delete` shadows the global persistent --yes/-y, so
// the -y shorthand must be registered on the local flag too, otherwise
// `dws todo task delete -y` fails with "unknown shorthand flag: 'y'".
func TestTodoTaskDeleteHasYesShorthand(t *testing.T) {
	cmd := newTodoTaskDeleteCommand(nil)

	yesFlag := cmd.Flags().Lookup("yes")
	if yesFlag == nil {
		t.Fatal("delete command is missing the --yes flag")
	}
	if yesFlag.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q, want %q (regression #370)", yesFlag.Shorthand, "y")
	}
	if cmd.Flags().ShorthandLookup("y") == nil {
		t.Error("-y shorthand is not resolvable on `todo task delete` (regression #370)")
	}
}
