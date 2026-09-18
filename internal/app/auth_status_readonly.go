// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"
	"io"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func runAuthStatusReadOnly(cmd *cobra.Command, configDir, selector string) error {
	if strings.TrimSpace(selector) == "" {
		selector = authpkg.RuntimeProfile()
	}
	data, err := authpkg.ReadTokenDataForProfile(configDir, selector)
	var diagnostic *authStatusDiagnostic
	switch {
	case errors.Is(err, authpkg.ErrTokenMigrationRequired):
		diagnostic = &authStatusDiagnostic{
			Reason:  "local_state_requires_repair",
			Message: "本地登录态需要迁移或修复，无法只读判断登录状态",
			Hint:    "请使用相同 --profile 运行 dws auth status（不加 --readonly）。",
		}
	case errors.Is(err, authpkg.ErrTokenDataNotFound):
		// A missing selected credential is a confirmed local logout.
	case err != nil || data == nil:
		diagnostic = authStatusDiagnosticFromError(err)
		if diagnostic == nil {
			diagnostic = &authStatusDiagnostic{
				Reason:  "local_state_unreadable",
				Message: "无法安全读取所选身份的本地登录态，无法判断登录状态",
				Hint:    "检查 --profile 和本地凭证存储；可运行 dws auth status（不加 --readonly）进一步诊断。",
			}
		}
	}
	// Reuse status's fields and expiry/login semantics, never raw errors or
	// credential values. A diagnostic is inconclusive, not a confirmed logout.
	authenticated := err == nil && data != nil && authStatusAuthenticated(data)
	return writeAuthStatusResult(cmd, authenticated, false, data, diagnostic)
}

func isAuthStatusReadOnlyCommand(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Name() != "status" || cmd.Parent() == nil || cmd.Parent().Name() != "auth" {
		return false
	}
	readonly, err := cmd.Flags().GetBool("readonly")
	return err == nil && readonly
}

// Observe argv without mutating the executable tree's flag values or Changed
// state. Other flags only need their declared argument-consumption behavior;
// normal execution still performs their real type validation exactly once.
func authStatusReadOnlyRequested(cmd *cobra.Command, args []string) bool {
	flags := pflag.NewFlagSet("auth-status-startup", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Bool("readonly", false, "")
	copyFlag := func(flag *pflag.Flag) {
		if flags.Lookup(flag.Name) != nil {
			return
		}
		flags.StringP(flag.Name, flag.Shorthand, "", "")
		flags.Lookup(flag.Name).NoOptDefVal = flag.NoOptDefVal
	}
	cmd.Flags().VisitAll(copyFlag)
	cmd.InheritedFlags().VisitAll(copyFlag)
	if err := flags.Parse(args); err != nil {
		return false
	}
	readonly, _ := flags.GetBool("readonly")
	return readonly
}

// Only the process entry point can specialize startup from os.Args. Public
// reusable roots must keep their complete runtime tree for later SetArgs calls.
type authStatusProcessStartupKey struct{}
