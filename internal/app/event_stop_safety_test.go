// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestEventStopRequiresTypedConfirmationBeforeMutation(t *testing.T) {
	root, _ := newEventStopSafetyRoot()
	root.SetArgs([]string{"event", "stop", "sub-1"})

	err := root.Execute()
	if err == nil {
		t.Fatal("event stop without --yes or --dry-run unexpectedly succeeded")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("event stop confirmation error = %T %v, want typed validation error", err, err)
	}
	if appErr.Reason != "confirmation_required" {
		t.Fatalf("event stop confirmation reason = %q, want confirmation_required", appErr.Reason)
	}
	for _, recoveryFlag := range []string{"--dry-run", "--yes"} {
		if !strings.Contains(err.Error(), recoveryFlag) {
			t.Fatalf("event stop confirmation error %q does not explain %s", err, recoveryFlag)
		}
	}
}

func TestEventStopDryRunPrecedesConfirmationAndReturnsPreview(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantAll         bool
		wantSubscribeID string
	}{
		{name: "single subscription", args: []string{"event", "stop", "sub-1", "--dry-run"}, wantSubscribeID: "sub-1"},
		{name: "all subscriptions", args: []string{"--dry-run", "event", "stop", "--all"}, wantAll: true},
		{name: "dry run wins over yes", args: []string{"event", "stop", "sub-2", "--yes", "--dry-run"}, wantSubscribeID: "sub-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, stdout := newEventStopSafetyRoot()
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("event stop dry-run error = %v", err)
			}
			var preview map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
				t.Fatalf("decode event stop dry-run preview: %v\n%s", err, stdout.String())
			}
			if preview["dry_run"] != true || preview["action"] != "event.stop" || preview["identity"] != "user" {
				t.Fatalf("event stop dry-run preview = %#v", preview)
			}
			if got, _ := preview["all"].(bool); got != test.wantAll {
				t.Fatalf("event stop dry-run all = %v, want %v", got, test.wantAll)
			}
			if got, _ := preview["subscribe_id"].(string); got != test.wantSubscribeID {
				t.Fatalf("event stop dry-run subscribe_id = %q, want %q", got, test.wantSubscribeID)
			}
		})
	}
}

func TestEventStopDryRunDoesNotBypassTargetValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing target", args: []string{"event", "stop", "--dry-run"}, want: "subscribe_id is required unless --all is set"},
		{name: "conflicting targets", args: []string{"event", "stop", "sub-1", "--all", "--dry-run"}, want: "subscribe_id and --all are mutually exclusive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, _ := newEventStopSafetyRoot()
			root.SetArgs(test.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("event stop dry-run validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func newEventStopSafetyRoot() (*cobra.Command, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	root := &cobra.Command{
		Use:           "dws",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	root.PersistentFlags().Bool("yes", false, "confirm execution")
	event := &cobra.Command{Use: "event"}
	event.AddCommand(newEventStopCommand())
	root.AddCommand(event)
	return root, stdout
}
