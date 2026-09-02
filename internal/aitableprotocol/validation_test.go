// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageFieldNameUsesUTF16Length(t *testing.T) {
	if err := ValidateFieldName(strings.Repeat("字", 150)); err != nil {
		t.Fatalf("150 BMP characters: %v", err)
	}
	if err := ValidateFieldName(strings.Repeat("字", 151)); err == nil {
		t.Fatal("151 BMP characters must fail")
	}
	if err := ValidateFieldName(strings.Repeat("😀", 75)); err != nil {
		t.Fatalf("75 emoji: %v", err)
	}
	if err := ValidateFieldName(strings.Repeat("😀", 76)); err == nil {
		t.Fatal("76 emoji must fail")
	}
	if err := ValidateFieldName(" \n\t"); err == nil {
		t.Fatal("whitespace-only field name must fail")
	}
}

func TestCrossPlatformCoverageClientTokenRequiresUUIDV4(t *testing.T) {
	if err := ValidateClientToken("9e438eda-66a9-4f4a-99f5-f2f1912442f7"); err != nil {
		t.Fatalf("valid UUID v4: %v", err)
	}
	for _, value := range []string{"not-a-uuid", "9e438eda-66a9-1f4a-99f5-f2f1912442f7"} {
		if err := ValidateClientToken(value); err == nil {
			t.Fatalf("%q must fail", value)
		}
	}
}

func TestCrossPlatformCoverageDashboardRootColumnsPreserveJSONType(t *testing.T) {
	payload := func(version any, verified bool) map[string]any {
		return map[string]any{"data": map[string]any{"baseId": "base", "dashboardId": "dashboard", "meta": map[string]any{
			"schemaVersion": version, "schemaVersionTypeVerified": verified,
		}}}
	}
	for _, test := range []struct {
		name      string
		payload   map[string]any
		appMode   bool
		want      int
		wantError bool
	}{
		{name: "numeric two", payload: payload(float64(2), true), want: DashboardGridColumnsV2},
		{name: "json number two", payload: payload(json.Number("2e0"), true), want: DashboardGridColumnsV2},
		{name: "string two", payload: payload("2", true), want: DashboardGridColumnsV1},
		{name: "other verified value", payload: payload(nil, true), want: DashboardGridColumnsV1},
		{name: "application mode", payload: map[string]any{"data": map[string]any{"baseId": "base", "dashboardId": "dashboard"}}, appMode: true, want: DashboardGridColumnsV2},
		{name: "unverified", payload: payload(float64(2), false), wantError: true},
		{name: "missing schema version", payload: map[string]any{"data": map[string]any{"baseId": "base", "dashboardId": "dashboard", "meta": map[string]any{"schemaVersionTypeVerified": true}}}, want: DashboardGridColumnsV1},
		{name: "wrong base", payload: map[string]any{"data": map[string]any{"baseId": "other", "dashboardId": "dashboard"}}, appMode: true, wantError: true},
		{name: "wrong dashboard", payload: map[string]any{"data": map[string]any{"baseId": "base", "dashboardId": "other"}}, appMode: true, wantError: true},
		{name: "missing data", payload: map[string]any{}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveDashboardRootColumns(test.payload, "base", "dashboard", test.appMode)
			if test.wantError {
				if err == nil {
					t.Fatalf("ResolveDashboardRootColumns() = %d, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("ResolveDashboardRootColumns() = %d, %v; want %d", got, err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRootChartLayoutValidation(t *testing.T) {
	valid := func(parent any, x, y, w, h any) map[string]any {
		layout := map[string]any{"x": x, "y": y, "w": w, "h": h}
		if parent != nil {
			layout["parentId"] = parent
		}
		return layout
	}
	for _, test := range []struct {
		name    string
		layout  map[string]any
		columns int
		wantErr bool
	}{
		{name: "twelve columns", layout: valid(nil, float64(0), float64(0), float64(12), float64(4)), columns: 12},
		{name: "root responsive layout", layout: valid(RootResponsiveLayoutParent, float64(24), float64(0), float64(24), float64(12)), columns: 48},
		{name: "right edge overflow", layout: valid(nil, float64(6), float64(0), float64(7), float64(4)), columns: 12, wantErr: true},
		{name: "child container", layout: valid("tab-1", float64(0), float64(0), float64(6), float64(4)), columns: 12, wantErr: true},
		{name: "whitespace parent", layout: valid("   ", float64(0), float64(0), float64(6), float64(4)), columns: 12, wantErr: true},
		{name: "padded root parent", layout: valid(" root-responsive-layout ", float64(0), float64(0), float64(6), float64(4)), columns: 12, wantErr: true},
		{name: "fractional coordinate", layout: valid(nil, 0.5, float64(0), float64(6), float64(4)), columns: 12, wantErr: true},
		{name: "missing height", layout: map[string]any{"x": float64(0), "y": float64(0), "w": float64(12)}, columns: 12, wantErr: true},
		{name: "zero width", layout: valid(nil, float64(0), float64(0), float64(0), float64(4)), columns: 12, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRootChartLayout(test.layout, test.columns)
			if test.wantErr && err == nil {
				t.Fatal("ValidateRootChartLayout() returned nil, want error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateRootChartLayout() error = %v", err)
			}
		})
	}
}
