// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// appEventReviewKey is an event key outside the built-in catalog, i.e. an
// application event that subscribes through the server's runtime rule
// (ruleType "all", empty ruleParam and filterRule).
const appEventReviewKey = "app_business_review_finished"

func TestCrossPlatformCoveragePersonalEventAppEventBuildRuleParam(t *testing.T) {
	ruleType, ruleParam, err := personal.BuildRuleParam(appEventReviewKey, personal.RuleOptions{})
	if err != nil || ruleType != "all" || len(ruleParam) != 0 {
		t.Fatalf("BuildRuleParam(app event, empty opts) = (%q, %#v, %v), want (\"all\", {}, nil)", ruleType, ruleParam, err)
	}

	ruleType, ruleParam, err = personal.BuildRuleParam(appEventReviewKey, personal.RuleOptions{RuleType: "all"})
	if err != nil || ruleType != "all" || len(ruleParam) != 0 {
		t.Fatalf("BuildRuleParam(app event, --rule all) = (%q, %#v, %v), want (\"all\", {}, nil)", ruleType, ruleParam, err)
	}

	tests := []struct {
		name    string
		opts    personal.RuleOptions
		wantErr string
	}{
		{
			name:    "rule custom",
			opts:    personal.RuleOptions{RuleType: "at"},
			wantErr: `--rule "at" does not match application event rule "all"`,
		},
		{
			name:    "user",
			opts:    personal.RuleOptions{UserID: "test-user-001"},
			wantErr: "--user is not supported for application event " + appEventReviewKey,
		},
		{
			name:    "open dingtalk id",
			opts:    personal.RuleOptions{OpenDingTalkID: "open-test-user"},
			wantErr: "--open-dingtalk-id is not supported for application event " + appEventReviewKey,
		},
		{
			name:    "group",
			opts:    personal.RuleOptions{GroupID: "cid-test"},
			wantErr: "--group is not supported for application event " + appEventReviewKey,
		},
		{
			name:    "role types",
			opts:    personal.RuleOptions{RoleTypes: []string{"creator"}},
			wantErr: "--role-types is not supported for application event " + appEventReviewKey,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := personal.BuildRuleParam(appEventReviewKey, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("BuildRuleParam(app event, %#v) error = %v, want %q", test.opts, err, test.wantErr)
			}
		})
	}

	// Known official events keep their existing branches.
	ruleType, _, err = personal.BuildRuleParam(personal.EventMention, personal.RuleOptions{})
	if err != nil || ruleType != "at" {
		t.Fatalf("BuildRuleParam(mention, empty opts) = (%q, %v), want (\"at\", nil)", ruleType, err)
	}
	if _, _, err := personal.BuildRuleParam(personal.EventMention, personal.RuleOptions{UserID: "test-user-001"}); err == nil || !strings.Contains(err.Error(), "--user is not supported for "+personal.EventMention) {
		t.Fatalf("BuildRuleParam(mention, --user) error = %v", err)
	}
	// An empty key is "not yet resolved" (implicit reuse), never an
	// application event.
	if _, _, err := personal.BuildRuleParam("", personal.RuleOptions{}); err == nil || !strings.Contains(err.Error(), "unknown personal event key") {
		t.Fatalf("BuildRuleParam(empty key) error = %v", err)
	}

	// Wire contract: the prepared app-event subscription carries a nil
	// ruleParam so filterRule stays blank — the server rejects any non-blank
	// filterRule for application events (FILTER_RULE_NOT_SUPPORTED).
	plan, err := preparePersonalSubscription(personal.Identity{}, personalConsumeOptions{EventKey: appEventReviewKey})
	if err != nil || plan.RuleType != "all" || plan.Request.RuleType != "all" || plan.Request.RuleParam != nil {
		t.Fatalf("preparePersonalSubscription(app event) = (%#v, %v), want rule all with nil ruleParam", plan, err)
	}
	if encoded, marshalErr := json.Marshal(plan.Request.RuleParam); marshalErr != nil || string(encoded) != "null" {
		t.Fatalf("app-event ruleParam marshals to %s (%v), want null", encoded, marshalErr)
	}
}

func TestCrossPlatformCoveragePersonalEventAppEventUnfilteredOptions(t *testing.T) {
	if err := validatePersonalUnfilteredEventOptions(appEventReviewKey, personalConsumeOptions{}); err != nil {
		t.Fatalf("validatePersonalUnfilteredEventOptions(app event, empty opts) = %v, want nil", err)
	}
	// Empty key means implicit reuse (--subscribe-id without a positional
	// event key): flags are validated after the key is resolved from the
	// subscription lookup, so nothing is rejected here.
	if err := validatePersonalUnfilteredEventOptions("", personalConsumeOptions{UserID: "test-user-001"}); err != nil {
		t.Fatalf("validatePersonalUnfilteredEventOptions(empty key, --user) = %v, want nil", err)
	}
	err := validatePersonalUnfilteredEventOptions(appEventReviewKey, personalConsumeOptions{UserID: "test-user-001"})
	if err == nil || !strings.Contains(err.Error(), "application event") || !strings.Contains(err.Error(), appEventReviewKey) {
		t.Fatalf("validatePersonalUnfilteredEventOptions(app event, --user) = %v, want application event error", err)
	}
	err = validatePersonalUnfilteredEventOptions(appEventReviewKey, personalConsumeOptions{QueryCSV: "alarm"})
	if err == nil || !strings.Contains(err.Error(), "--query not supported for application event "+appEventReviewKey) {
		t.Fatalf("validatePersonalUnfilteredEventOptions(app event, --query) = %v", err)
	}
	err = validatePersonalUnfilteredEventOptions(personal.EventCardAction, personalConsumeOptions{UserID: "test-user-001"})
	if err == nil || !strings.Contains(err.Error(), "--user not supported for card event "+personal.EventCardAction) {
		t.Fatalf("validatePersonalUnfilteredEventOptions(card, --user) = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalEventAppEventMultiOptions(t *testing.T) {
	testseam.Swap(t, &personalLookupDefinition, func(eventKey string) (personal.Definition, bool) {
		return personal.Lookup(eventKey)
	})

	plans, err := preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{personal.EventSingleChat, appEventReviewKey},
		UserID:    "test-user-001",
	})
	if err != nil {
		t.Fatalf("user-scoped official event + application event: %v", err)
	}
	assertAppEventMultiPlanCleared(t, plans)

	plans, err = preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{personal.EventInChat, appEventReviewKey},
		GroupID:   "cid-test",
	})
	if err != nil {
		t.Fatalf("group-scoped official event + application event: %v", err)
	}
	assertAppEventMultiPlanCleared(t, plans)

	plans, err = preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{"app_event_alpha", "app_event_beta"},
	})
	if err != nil {
		t.Fatalf("application-event-only selection: %v", err)
	}
	assertAppEventMultiPlanCleared(t, plans)

	_, err = preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{"app_event_alpha", "app_event_beta"},
		UserID:    "test-user-001",
	})
	if err == nil || !strings.Contains(err.Error(), "do not use --user") {
		t.Fatalf("application-event-only selection with --user error = %v", err)
	}

	_, err = preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{personal.EventMention, appEventReviewKey},
		QueryCSV:  "alarm",
	})
	if err == nil || !strings.Contains(err.Error(), "require all selected events to be message receive events") {
		t.Fatalf("application event with --query error = %v", err)
	}

	// Existing constraint regression: user-scoped and group-scoped official
	// events still cannot be consumed in one command.
	_, err = preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{personal.EventSingleChat, personal.EventInChat},
		UserID:    "test-user-001",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be consumed in one command") {
		t.Fatalf("user-scoped + group-scoped mixing error = %v", err)
	}
}

func assertAppEventMultiPlanCleared(t *testing.T, plans []personalConsumeOptions) {
	t.Helper()
	for _, plan := range plans {
		if !strings.HasPrefix(plan.EventKey, "app_") {
			continue
		}
		if plan.UserID != "" || plan.OpenDingTalkID != "" || plan.GroupID != "" || plan.Rule != "" {
			t.Fatalf("application event plan retained single-event fields: %#v", plan)
		}
	}
}
