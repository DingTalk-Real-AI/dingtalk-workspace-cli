// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProjectedChatUnifiedResultBoundaries(t *testing.T) {
	t.Run("malformed response becomes typed failure", func(t *testing.T) {
		cmd := &cobra.Command{Use: "project"}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		err := writeProjectedChatPayload(cmd, "im/project", "{", func(value map[string]any) map[string]any { return value })
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "malformed_tool_response" {
			t.Fatalf("error = %#v, want malformed_tool_response", err)
		}
	})

	t.Run("valid response stores projected result", func(t *testing.T) {
		cmd := &cobra.Command{Use: "project"}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		if err := writeProjectedChatPayload(cmd, "im/project", `{"id":"m1"}`, func(value map[string]any) map[string]any {
			return map[string]any{"messageId": value["id"]}
		}); err != nil {
			t.Fatal(err)
		}
		if err := output.StoreResult(ctx, output.Success(map[string]any{"second": true})); err == nil {
			t.Fatal("projected result was not stored before the duplicate write")
		}
	})
}

func TestCrossPlatformCoverageChatMessageRangeFallbackRolloutBoundaries(t *testing.T) {
	setup := func(t *testing.T, response string, rollout output.RolloutState) (*cobra.Command, PagedMCPCommandConfig) {
		t.Helper()
		oldDeps := deps
		t.Cleanup(func() { deps = oldDeps })
		InitDeps(&pagedCommandCaller{steps: []scriptedToolStep{{text: response}}})
		cmd := &cobra.Command{Use: "range"}
		cmd.SetContext(context.Background())
		output.SetCommandRollout(cmd, rollout)
		return cmd, pagedChatMessageRangeAllConfig(cmd)
	}

	t.Run("unified malformed response", func(t *testing.T) {
		_, cfg := setup(t, "{", output.RolloutUnifiedActive)
		err := cfg.Fallback(map[string]any{})
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "malformed_tool_response" {
			t.Fatalf("error = %#v, want malformed_tool_response", err)
		}
	})

	t.Run("unified stores valid response", func(t *testing.T) {
		cmd, cfg := setup(t, `{"result":{"conversationMessagesList":[],"hasMore":false}}`, output.RolloutUnifiedActive)
		ctx, _ := output.WithResultStore(cmd.Context())
		cmd.SetContext(ctx)
		if err := cfg.Fallback(map[string]any{}); err != nil {
			t.Fatal(err)
		}
		if err := output.StoreResult(ctx, output.Success(map[string]any{"second": true})); err == nil {
			t.Fatal("range fallback did not store its unified result")
		}
	})

	t.Run("dual validation failure", func(t *testing.T) {
		_, cfg := setup(t, `{"result":{"conversationMessagesList":[],"hasMore":false}}`, output.RolloutDualValidate)
		validationErr := errors.New("invalid range shadow")
		testseam.Swap(t, &validateRuntimeResult, func(output.CommandResult) error { return validationErr })
		if err := cfg.Fallback(map[string]any{}); !errors.Is(err, validationErr) {
			t.Fatalf("error = %v, want validation error", err)
		}
	})
}

func TestCrossPlatformCoveragePagedResultPrimitiveBoundaries(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  int
	}{
		{value: int32(2), want: 2},
		{value: int64(3), want: 3},
		{value: float64(4), want: 4},
		{value: json.Number("5"), want: 5},
		{value: json.Number("invalid"), want: 0},
		{value: struct{}{}, want: 0},
	} {
		if got := chatResultInt(tc.value); got != tc.want {
			t.Fatalf("chatResultInt(%#v) = %d, want %d", tc.value, got, tc.want)
		}
	}

	meta, err := chatMessageRangeAllFrameworkMeta(map[string]any{
		"count": 2, "pagesFetched": 1, "hasMore": true, "paginationKnown": true,
		"result": map[string]any{"nextCursor": "cursor-2"},
	})
	if err != nil || meta.Pagination == nil || meta.Pagination.NextToken != "cursor-2" {
		t.Fatalf("range metadata = %#v, err = %v", meta, err)
	}
	if _, err := chatMessageRangeAllFrameworkMeta(map[string]any{"paginationKnown": false}); err == nil {
		t.Fatal("unknown range pagination unexpectedly succeeded")
	}
	if _, err := chatMessageRangeAllFrameworkMeta(map[string]any{
		"hasMore": true, "paginationKnown": true,
	}); err == nil {
		t.Fatal("missing range continuation unexpectedly succeeded")
	}
	if _, err := pagedCommandFrameworkMeta(pagedCommandMessagesConfig(nil), pagingMetadata{HasMore: true}); err == nil {
		t.Fatal("missing generic continuation unexpectedly succeeded")
	}

	for _, tc := range []struct {
		name      string
		args      map[string]any
		remaining int
		want      any
	}{
		{name: "nil", args: nil, remaining: 1},
		{name: "non-positive budget", args: map[string]any{"limit": 9}, remaining: 0, want: 9},
		{name: "missing", args: map[string]any{}, remaining: 2},
		{name: "int", args: map[string]any{"limit": 9}, remaining: 2, want: 2},
		{name: "int64", args: map[string]any{"limit": int64(9)}, remaining: 3, want: int64(3)},
		{name: "float64", args: map[string]any{"limit": float64(9)}, remaining: 4, want: float64(4)},
	} {
		t.Run("clamp "+tc.name, func(t *testing.T) {
			clampPagedCommandPageSize(tc.args, "limit", tc.remaining)
			if tc.args != nil && tc.want != nil && tc.args["limit"] != tc.want {
				t.Fatalf("args = %#v, want limit %#v", tc.args, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageIncompleteResultCategoryAndPagedWriterBoundaries(t *testing.T) {
	for _, tc := range []struct {
		category      apperrors.Category
		cause         error
		wantRetryable bool
	}{
		{category: apperrors.CategoryAuth, cause: apperrors.NewAuth("cause")},
		{category: apperrors.CategoryValidation, cause: apperrors.NewValidation("cause")},
		{category: apperrors.CategoryDiscovery, cause: apperrors.NewDiscovery("cause")},
		{category: apperrors.CategoryInternal, cause: apperrors.NewInternal("cause"), wantRetryable: true},
		{category: apperrors.CategoryPartial, cause: &apperrors.Error{Category: apperrors.CategoryPartial, Message: "cause"}, wantRetryable: true},
	} {
		category := tc.category
		cause := tc.cause
		err := NewIncompleteResultError("partial", cause, true)
		var typed *apperrors.Error
		if !errors.As(err, &typed) {
			t.Fatalf("category %s did not produce typed error: %v", category, err)
		}
		want := category
		if category == apperrors.CategoryPartial {
			want = apperrors.CategoryInternal
		}
		if typed.Category != want || typed.Retryable != tc.wantRetryable {
			t.Fatalf("category %s => %#v, want category=%s retryable=%t", category, typed, want, tc.wantRetryable)
		}
	}

	oldDeps := deps
	t.Cleanup(func() { deps = oldDeps })
	InitDeps(&pagedCommandCaller{})
	collection := newPagedCollection(pagedCommandMessagesConfig(nil))
	if err := collection.Add([]any{map[string]any{"id": "m1"}}); err != nil {
		t.Fatal(err)
	}
	cfg := pagedCommandMessagesConfig(nil)

	t.Run("unified stores successful result", func(t *testing.T) {
		cmd := &cobra.Command{Use: "paged"}
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		if err := writePagedCommandResult(cmd, map[string]any{"result": map[string]any{}}, cfg, collection, pagingMetadata{Pages: 1, Total: 1}); err != nil {
			t.Fatal(err)
		}
		if err := output.StoreResult(ctx, output.Success(map[string]any{"second": true})); err == nil {
			t.Fatal("paged result was not stored before the duplicate write")
		}
	})

	t.Run("dual validation failure stops legacy write", func(t *testing.T) {
		validationErr := errors.New("invalid shadow")
		testseam.Swap(t, &validateRuntimeResult, func(output.CommandResult) error { return validationErr })
		cmd := &cobra.Command{Use: "paged"}
		output.SetCommandRollout(cmd, output.RolloutDualValidate)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		err := writePagedCommandResult(cmd, map[string]any{"result": map[string]any{}}, cfg, collection, pagingMetadata{Pages: 1, Total: 1})
		if !errors.Is(err, validationErr) || stdout.Len() != 0 {
			t.Fatalf("error = %v, stdout = %q", err, stdout.String())
		}
	})

	t.Run("truncated page preserves legacy partial payload", func(t *testing.T) {
		cmd := &cobra.Command{Use: "paged"}
		output.SetCommandRollout(cmd, output.RolloutLegacyOnly)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		err := writePagedCommandResult(cmd, map[string]any{"result": map[string]any{}}, cfg, collection, pagingMetadata{
			TruncatedWithinPage: true, HasMore: true, Pages: 1, Total: 1,
		})
		if err != nil || stdout.Len() == 0 {
			t.Fatalf("error = %v, stdout = %q", err, stdout.String())
		}
	})
}

func TestCrossPlatformCoveragePagedDryRunValidationBoundaries(t *testing.T) {
	t.Run("invalid config", func(t *testing.T) {
		caller := &pagedCommandCaller{dry: true}
		cfg := pagedCommandMessagesConfig(nil)
		cfg.ServerID = ""
		if _, _, err := runPagedCommandTest(t, caller, cfg); err == nil {
			t.Fatal("invalid dry-run config unexpectedly succeeded")
		}
	})

	t.Run("dual shadow validation", func(t *testing.T) {
		oldDeps := deps
		t.Cleanup(func() { deps = oldDeps })
		InitDeps(&pagedCommandCaller{dry: true})
		validationErr := errors.New("invalid preview")
		testseam.Swap(t, &validateRuntimeResult, func(output.CommandResult) error { return validationErr })
		cmd := &cobra.Command{Use: "paged"}
		output.SetCommandRollout(cmd, output.RolloutDualValidate)
		cmd.Flags().String("cursor", "0", "")
		AddPagedMCPFlags(cmd)
		if err := RunPagedMCPCommand(cmd, pagedCommandMessagesConfig(nil)); !errors.Is(err, validationErr) {
			t.Fatalf("error = %v, want validation error", err)
		}
	})
}
