// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageChatCatalogSafetyMetadataIsExplicit(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(newChatCommand())

	checked := 0
	walkChatCatalogLeaves(root, func(cmd *cobra.Command) {
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Identity == nil || final.Safety == nil {
			return
		}
		if strings.TrimSpace(final.Identity.ProductID) != "chat" {
			return
		}
		checked++
		canonical := strings.TrimSpace(final.Identity.CanonicalPath)
		safety := final.Safety
		if safety.Effect == "" || safety.Risk == "" ||
			safety.Confirmation == "" || safety.Idempotency == "" {
			t.Fatalf("chat Catalog leaf %s has incomplete Safety: %+v", canonical, *safety)
		}
		if safety.Effect == "read" {
			if safety.Risk != "low" ||
				safety.Confirmation != "not_required" ||
				safety.Idempotency != "idempotent" {
				t.Fatalf("chat read Catalog leaf %s Safety = %+v, want read/low/not_required/idempotent",
					canonical, *safety)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no chat Catalog leaves checked")
	}
}

func TestCrossPlatformCoverageChatRoleAndCategoryAtomicWritesRequireConfirmation(t *testing.T) {
	root := newChatCommand()
	for _, path := range [][]string{
		{"category", "create"},
		{"group-role", "add"},
		{"group-role", "update"},
		{"group-role", "remove"},
		{"group-role", "set-user"},
		{"group-role", "remove-user"},
	} {
		cmd, remaining, err := root.Find(path)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("find %v: remaining=%v err=%v", path, remaining, err)
		}
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Safety == nil || final.Safety.Confirmation != "user_required" ||
			final.Safety.Effect != "write" || final.Safety.Risk != "medium" {
			t.Fatalf("%v safety = %#v", path, final.Safety)
		}
	}
}

func walkChatCatalogLeaves(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd == nil {
		return
	}
	if cmd.Runnable() && !cmd.HasSubCommands() {
		fn(cmd)
		return
	}
	for _, child := range cmd.Commands() {
		if child.Name() == "help" {
			continue
		}
		walkChatCatalogLeaves(child, fn)
	}
}
