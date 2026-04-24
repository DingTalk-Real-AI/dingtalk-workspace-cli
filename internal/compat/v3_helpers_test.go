// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package compat

import (
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestKindFromTypeName covers the schema v3 explicit Type field → ValueKind map.
func TestKindFromTypeName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want ValueKind
	}{
		{"", ValueString},
		{"string", ValueString},
		{"STRING", ValueString},
		{"int", ValueInt},
		{"integer", ValueInt},
		{"number", ValueInt},
		{"bool", ValueBool},
		{"boolean", ValueBool},
		{"stringSlice", ValueStringSlice},
		{"string_slice", ValueStringSlice},
		{"[]string", ValueStringSlice},
		{"weird-unknown-type", ValueString},
	}
	for _, tc := range cases {
		if got := kindFromTypeName(tc.in); got != tc.want {
			t.Errorf("kindFromTypeName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestApplyOmitWhen verifies all three modes plus zero-detection.
func TestApplyOmitWhen(t *testing.T) {
	t.Parallel()

	t.Run("mode_zero_drops_zero_values", func(t *testing.T) {
		params := map[string]any{
			"a": 0,
			"b": "",
			"c": false,
			"d": []any{},
			"e": map[string]any{},
			"f": "non-empty",
			"g": 42,
		}
		for k := range params {
			applyOmitWhen(params, k, "zero")
		}
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			if _, ok := params[k]; ok {
				t.Errorf("expected key %q to be dropped by omitWhen=zero", k)
			}
		}
		for _, k := range []string{"f", "g"} {
			if _, ok := params[k]; !ok {
				t.Errorf("expected key %q to be preserved", k)
			}
		}
	})

	t.Run("mode_never_preserves_zero_values", func(t *testing.T) {
		params := map[string]any{"explicitFalse": false, "explicitZero": 0}
		applyOmitWhen(params, "explicitFalse", "never")
		applyOmitWhen(params, "explicitZero", "never")
		if len(params) != 2 {
			t.Errorf("expected both keys preserved under 'never', got %v", params)
		}
	})

	t.Run("mode_empty_is_noop", func(t *testing.T) {
		params := map[string]any{"a": ""}
		applyOmitWhen(params, "a", "empty")
		if _, ok := params["a"]; !ok {
			t.Errorf("empty mode should not drop keys here (upstream CollectBindings does)")
		}
	})

	t.Run("missing_key_safe", func(t *testing.T) {
		params := map[string]any{}
		applyOmitWhen(params, "nope", "zero")
		if len(params) != 0 {
			t.Errorf("expected no-op on missing key, got %v", params)
		}
	})
}

// TestIsZeroValue covers every branch of the helper.
func TestIsZeroValue(t *testing.T) {
	t.Parallel()
	zeros := []any{
		nil,
		"",
		"   ",
		false,
		0,
		int64(0),
		float64(0),
		[]any{},
		[]string{},
		map[string]any{},
	}
	for i, z := range zeros {
		if !isZeroValue(z) {
			t.Errorf("case %d: expected zero value for %#v", i, z)
		}
	}
	nonZeros := []any{"x", true, 1, int64(1), float64(1.5), []any{1}, []string{"a"}, map[string]any{"k": 1}}
	for i, nz := range nonZeros {
		if isZeroValue(nz) {
			t.Errorf("case %d: expected non-zero for %#v", i, nz)
		}
	}
}

// TestRuntimeDefaultResolvers_BuiltIns asserts $now and $today always resolve.
func TestRuntimeDefaultResolvers_BuiltIns(t *testing.T) {
	// NOTE: not t.Parallel — edition.Get() global state is shared.
	resolvers := runtimeDefaultResolvers()
	now := resolvers["$now"]
	if now == nil {
		t.Fatal("$now resolver missing")
	}
	if v, ok := now(context.Background()); !ok || v == "" {
		t.Errorf("$now returned empty value: %q ok=%v", v, ok)
	}
	today := resolvers["$today"]
	if today == nil {
		t.Fatal("$today resolver missing")
	}
	if v, ok := today(context.Background()); !ok || !strings.Contains(v, "-") {
		t.Errorf("$today returned unexpected value: %q ok=%v", v, ok)
	}
}

// TestRuntimeDefaultResolvers_OverlayMerge covers the edition overlay hook.
func TestRuntimeDefaultResolvers_OverlayMerge(t *testing.T) {
	prev := edition.Get()
	defer edition.Override(prev)

	edition.Override(&edition.Hooks{
		RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
			return map[string]edition.RuntimeDefaultFn{
				"$currentUserId": func(ctx context.Context) (string, bool) {
					return "test-user-001", true
				},
			}
		},
	})

	resolvers := runtimeDefaultResolvers()
	fn := resolvers["$currentUserId"]
	if fn == nil {
		t.Fatal("$currentUserId missing after overlay install")
	}
	if v, ok := fn(context.Background()); !ok || v != "test-user-001" {
		t.Errorf("$currentUserId=%q ok=%v", v, ok)
	}
}
