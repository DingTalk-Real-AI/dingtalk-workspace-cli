// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package corecmd

import (
	stderrors "errors"
	"io"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCrossPlatformCoverageUnknownInvocationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		configure func(*cobra.Command, *cobra.Command, *cobra.Command)
		unknown   bool
		wantError bool
	}{
		{name: "top command", args: []string{"missing"}, unknown: true, wantError: true},
		{name: "group command", args: []string{"group", "missing"}, unknown: true, wantError: true},
		{name: "shortcut", args: []string{"group", "+missing"}, unknown: true, wantError: true},
		{name: "long flag", args: []string{"group", "leaf", "--missing"}, unknown: true, wantError: true},
		{name: "short flag", args: []string{"group", "leaf", "-z"}, unknown: true, wantError: true},
		{name: "handler rewrites text", args: []string{"group", "leaf", "--missing"}, configure: func(root, _, _ *cobra.Command) {
			root.SetFlagErrorFunc(func(*cobra.Command, error) error { return stderrors.New("use a declared parameter") })
		}, unknown: true, wantError: true},
		{name: "invalid value", args: []string{"group", "leaf", "--count=no"}, wantError: true},
		{name: "missing value", args: []string{"group", "leaf", "--count"}, wantError: true},
		{name: "missing required", args: []string{"group", "leaf"}, configure: func(_, _, leaf *cobra.Command) { _ = leaf.MarkFlagRequired("count") }, wantError: true},
		{name: "business same text", args: []string{"group", "leaf"}, configure: func(_, _, leaf *cobra.Command) {
			leaf.RunE = func(*cobra.Command, []string) error { return stderrors.New("unknown flag: --missing") }
		}, wantError: true},
		{name: "args same text", args: []string{"group", "leaf"}, configure: func(_, _, leaf *cobra.Command) {
			leaf.Args = func(*cobra.Command, []string) error { return stderrors.New("unknown command missing") }
		}, wantError: true},
		{name: "required group", args: []string{"group", "leaf"}, configure: func(_, _, leaf *cobra.Command) {
			leaf.Flags().String("name", "", "")
			leaf.MarkFlagsOneRequired("count", "name")
		}, wantError: true},
		{name: "known success", args: []string{"group", "leaf"}},
		{name: "help", args: []string{"group", "leaf", "--help"}},
		{name: "completion", args: []string{"__complete", "group", "missing"}},
		{name: "legacy find", args: []string{"missing"}, configure: func(root, _, _ *cobra.Command) { root.Args = nil }, unknown: true, wantError: true},
		{name: "group without recovery", args: []string{"local", "missing"}, configure: func(root, _, _ *cobra.Command) {
			group := &cobra.Command{Use: "local"}
			group.AddCommand(&cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}})
			ApplyGroupPolicy(group, GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDisabled})
			root.AddCommand(group)
		}, unknown: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			group := &cobra.Command{Use: "group"}
			leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
			leaf.Flags().Int("count", 0, "")
			group.AddCommand(leaf)
			root.AddCommand(group)
			policy := GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling}
			ApplyGroupPolicy(root, policy)
			ApplyGroupPolicy(group, policy)
			if tc.configure != nil {
				tc.configure(root, group, leaf)
			}
			root.SetArgs(tc.args)
			err := ExecuteForTest(root)
			if (err != nil) != tc.wantError || apperrors.IsUnknownInvocationError(err) != tc.unknown {
				t.Fatalf("err=%v unknown=%v; want error=%v unknown=%v", err, apperrors.IsUnknownInvocationError(err), tc.wantError, tc.unknown)
			}
			if tc.unknown && apperrors.ExitCode(err) != 3 {
				t.Fatalf("unknown invocation exit=%d", apperrors.ExitCode(err))
			}
		})
	}
}

func TestCrossPlatformCoverageUnknownInvocationDoesNotInspectFlagValueCause(t *testing.T) {
	flags := pflag.NewFlagSet("internal", pflag.ContinueOnError)
	_, cause := flags.GetString("missing")
	cmd := &cobra.Command{Use: "leaf", SilenceErrors: true, SilenceUsage: true, Run: func(*cobra.Command, []string) { t.Fatal("invalid value executed") }}
	cmd.Flags().Var(&validationFailingValue{err: cause}, "known", "")
	cmd.SetArgs([]string{"--known=value"})
	err := ExecuteForTest(cmd)
	if err == nil || apperrors.IsUnknownInvocationError(err) || !stderrors.Is(err, cause) || apperrors.ExitCode(err) != 3 {
		t.Fatalf("known flag value failure misclassified: %v", err)
	}
}

type unknownInvocationAuthFailure struct{ error }

func (e *unknownInvocationAuthFailure) Unwrap() error { return e.error }
func (e *unknownInvocationAuthFailure) ExitCode() int { return apperrors.ExitCodeAuth }

func TestCrossPlatformCoverageUnknownInvocationKeepsHandlerClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		classify func(error) error
	}{
		{name: "API", classify: func(cause error) error { return apperrors.NewAPI("classified by handler", apperrors.WithCause(cause)) }},
		{name: "custom exit", classify: func(cause error) error { return &unknownInvocationAuthFailure{error: cause} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "leaf", SilenceErrors: true, SilenceUsage: true, Run: func(*cobra.Command, []string) { t.Fatal("unknown flag executed") }}
			var original, classified error
			cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
				original = err
				classified = tc.classify(err)
				return classified
			})
			cmd.SetArgs([]string{"--missing"})
			err := ExecuteForTest(cmd)
			if err == nil || err != classified || !stderrors.Is(err, original) || apperrors.IsUnknownInvocationError(err) {
				t.Fatalf("handler classification changed: err=%v classified=%v", err, classified)
			}
		})
	}
}
