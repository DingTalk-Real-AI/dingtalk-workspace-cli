// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestDevAppSharedResultMapperClassifiesServiceOutcomes(t *testing.T) {
	t.Run("normalized success", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"content": map[string]any{"success": true, "result": map[string]any{"id": "a"}},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := env.Data.(map[string]any)
		if env.Outcome != output.OutcomeSuccess || data["id"] != "a" {
			t.Fatalf("success envelope=%+v data=%#v", env, env.Data)
		}
	})

	t.Run("pending approval", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"versionStatus": "AUDIT", "versionId": "v1", "unifiedAppId": "u1",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePending || env.Meta == nil || env.Meta.Operation == nil || env.Meta.Operation.NextCommand == "" {
			t.Fatalf("pending envelope=%+v", env)
		}
	})

	t.Run("approval selection uses tool normalization", func(t *testing.T) {
		result := DevAppCommandResultFromPayload(devAppVersionPublishTool, map[string]any{
			"approvalMode": "SELECT_APPROVER",
			"unifiedAppId": "u1",
			"versionId":    "v1",
			"approvalCandidates": []any{
				map[string]any{"userId": "user-1", "name": "Alice"},
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePending || env.Meta == nil || env.Meta.Operation == nil {
			t.Fatalf("approval envelope=%+v", env)
		}
		if env.Meta.Operation.State != "waiting_for_approver_selection" || env.Meta.Operation.NextCommand == "" {
			t.Fatalf("approval operation=%+v", env.Meta.Operation)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"items": []any{}, "hasMore": true, "nextCursor": "next",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Meta == nil || env.Meta.Pagination == nil || env.Meta.Pagination.EndpointExhausted || env.Meta.Pagination.NextToken != "next" {
			t.Fatalf("pagination envelope=%+v", env)
		}
	})

	t.Run("partial", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"multiProfile": true,
			"profiles": []any{
				map[string]any{"selector": "a", "ok": true, "result": map[string]any{"id": "r1"}},
				map[string]any{"selector": "b", "ok": false, "error": map[string]any{"category": "api", "reason": "rejected", "message": "bad"}},
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePartialFailure || result.ExitCode() != 7 {
			t.Fatalf("partial outcome=%s rc=%d", env.Outcome, result.ExitCode())
		}
	})

	t.Run("dry run is completed preview", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{"tool": "x"}, true)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeSuccess || !env.DryRun {
			t.Fatalf("dry-run envelope=%+v", env)
		}
	})
}
