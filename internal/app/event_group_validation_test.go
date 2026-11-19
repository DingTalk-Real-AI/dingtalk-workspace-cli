// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEventConsumeRejectsMultipleGroups(t *testing.T) {
	for _, multiEvent := range []bool{false, true} {
		for _, dryRun := range []bool{false, true} {
			name := "single_event"
			if multiEvent {
				name = "multiple_events"
			}
			if dryRun {
				name += "_dry_run"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string) (personal.Identity, error) {
					return personal.Identity{ClientID: "fixture-client", CorpID: "fixture-corp", UserID: "fixture-user", SourceID: "open"}, nil
				})
				createCalls, consumeCalls := 0, 0
				testseam.Swap(t, &personalCreateSubscription, func(*personal.Client, context.Context, personal.CreateSubscriptionRequest) (*personal.Subscription, error) {
					createCalls++
					return nil, errors.New("unexpected subscription creation")
				})
				testseam.Swap(t, &personalConsumeRun, func(context.Context, consume.Config) error {
					consumeCalls++
					return nil
				})
				testseam.Swap(t, &personalConsumeRunMany, func(context.Context, consume.Config, []consume.ConsumerSpec) error {
					consumeCalls++
					return nil
				})
				cmd := newEventConsumeCommand()
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				args := []string{personal.EventInChat}
				if multiEvent {
					args = append(args, personal.EventReadGroup)
				}
				args = append(args, "--group", "cid-fixture-a,cid-fixture-b", "--duration", "1s")
				if dryRun {
					args = append(args, "--dry-run")
				}
				cmd.SetArgs(args)
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "--group accepts exactly one openConversationId") {
					t.Errorf("Execute() error = %v, want single-group validation error", err)
				}
				if createCalls != 0 || consumeCalls != 0 {
					t.Fatalf("invalid group reached subscription/consumer: create=%d consume=%d", createCalls, consumeCalls)
				}
			})
		}
	}
}
