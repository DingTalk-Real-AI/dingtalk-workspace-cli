// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cmdutil

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	var structured *apperrors.Error
	if err := hint.RunE(hint, nil); !errors.As(err, &structured) ||
		structured.Hint != "use parent find (Run 'parent --help' for the full list)" {
		t.Fatalf("hint error = %#v", structured)
	}
	assertResolutionDetails(t, structured, "search", []string{})
}
