// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageFromLeafSpecCallDispatch(t *testing.T) {
	called := false
	cmd := NewLeafCommand(LeafSpec{
		Use:  "call-dispatch",
		Tool: "call_tool",
		Call: func(*cobra.Command, string, map[string]any) error {
			called = true
			return nil
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("Call dispatch was not invoked")
	}
}

func TestCrossPlatformCoverageReportCommandRegistersProductDecl(t *testing.T) {
	t.Cleanup(func() { contract.ClearProductDeclForTest("report") })
	contract.ClearProductDeclForTest("report")
	cmd := newReportCommand()
	if cmd == nil || cmd.Use != "report" {
		t.Fatalf("report command = %#v", cmd)
	}
	if !contract.HasProductDecl("report") {
		t.Fatal("newReportCommand must register ProductDecl for report")
	}
	decl, ok := contract.LookupProductDecl("report")
	if !ok || decl.Selection.AgentSummary == "" || len(decl.Selection.UseWhen) == 0 {
		t.Fatalf("report ProductDecl = %#v, ok=%v", decl, ok)
	}
	templateCmd, _, err := cmd.Find([]string{"template", "list"})
	if err != nil || templateCmd == nil {
		t.Fatalf("report template list command missing: %v", err)
	}
	templateCmd.SetOut(io.Discard)
	templateCmd.SetErr(io.Discard)

	createCmd, _, err := cmd.Find([]string{"create"})
	if err != nil || createCmd == nil {
		t.Fatalf("report create command missing: %v", err)
	}
	if createCmd.Flags().Lookup("to-chat") == nil {
		t.Fatal("deprecated create alias must expose --to-chat from ApplyParamDecls")
	}
}
