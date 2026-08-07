// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cmdutil

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageHintSubCmdCarriesHintOnlyIdentity(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	hint := HintSubCmd("search", "use parent find")
	parent.AddCommand(hint)

	if !hint.Hidden || !IsHintOnlyCommand(hint) {
		t.Fatalf("hint command = %#v", hint)
	}
	if IsHintOnlyCommand(nil) || IsHintOnlyCommand(&cobra.Command{Use: "real"}) {
		t.Fatal("ordinary command was classified as hint-only")
	}
	if err := hint.RunE(hint, nil); err == nil || !strings.Contains(err.Error(), "use parent find") {
		t.Fatalf("hint error = %v", err)
	}
}
