// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfirmDangerousActionDoesNotDescribePublicationAsDelete(t *testing.T) {
	cmd := &cobra.Command{Use: "set"}
	cmd.Flags().Bool("yes", false, "skip confirmation")
	cmd.SetIn(strings.NewReader("no\n"))
	var output bytes.Buffer
	cmd.SetErr(&output)

	confirmed := confirmDangerousAction(cmd, "enable internet publishing", "file-1")

	if confirmed {
		t.Fatal("negative answer unexpectedly confirmed the action")
	}
	text := output.String()
	if !strings.Contains(text, "About to enable internet publishing: file-1") {
		t.Fatalf("confirmation output does not name the real action: %q", text)
	}
	if strings.Contains(strings.ToLower(text), "delete") {
		t.Fatalf("non-delete confirmation still describes a delete: %q", text)
	}
}

func TestConfirmDangerousActionUsesParsedCobraYesFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "set"}
	cmd.Flags().Bool("yes", false, "skip confirmation")
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	cmd.SetIn(strings.NewReader("no\n"))
	var output bytes.Buffer
	cmd.SetErr(&output)

	if !confirmDangerousAction(cmd, "enable internet publishing", "file-1") {
		t.Fatal("parsed --yes=true did not bypass the interactive prompt")
	}
	if output.Len() != 0 {
		t.Fatalf("--yes=true unexpectedly produced a prompt: %q", output.String())
	}
}
