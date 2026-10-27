// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"strings"
	"testing"
)

func (c *upsertByKeyCaller) SupportsMCPTool(ctx context.Context, product, tool string) (bool, error) {
	if c.capabilityFn == nil {
		return false, nil
	}
	return c.capabilityFn(ctx, product, tool)
}

func TestCrossPlatformCoverageChartsPreferConfirmedBatchWithoutReplay(t *testing.T) {
	for _, available := range []bool{true, false} {
		for _, fail := range []bool{true, false} {
			t.Run(fmt.Sprintf("available=%v/fail=%v", available, fail), func(t *testing.T) {
				discovered := 0
				caller := &upsertByKeyCaller{}
				caller.capabilityFn = func(_ context.Context, product, tool string) (bool, error) {
					discovered++
					if len(caller.calls) != 0 || product != serverMain || tool != "create_charts" {
						t.Fatal("discovery happened after dispatch")
					}
					return available, nil
				}
				writes := 0
				caller.callFn = func(_ int, _, tool string, args map[string]any) (string, error) {
					switch tool {
					case "get_dashboard":
						return `{"status":"success","data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`, nil
					case "create_charts", "create_chart":
						writes++
						if (tool == "create_charts") != available {
							t.Fatal("wrong route", tool)
						}
						if fail {
							return "", errors.New("delivery unknown")
						}
						if available {
							return `{"status":"success","data":{"baseId":"B","dashboardId":"D","results":[{"index":0,"chartId":"C","status":"accepted","success":true}]}}`, nil
						}
						return `{"status":"success","data":{"baseId":"B","dashboardId":"D","chartId":"C"}}`, nil
					case "get_chart":
						return `{"status":"success","data":{"baseId":"B","dashboardId":"D","chartId":"C","config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}}`, nil
					}
					t.Fatal("unexpected call", tool)
					return "", nil
				}
				_, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--yes")
				if (err != nil) != fail || discovered != 1 || writes != 1 {
					t.Fatalf("error=%v discoveries=%d writes=%d", err, discovered, writes)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageChartDiscoveryFailureStopsBeforeWrite(t *testing.T) {
	for _, command := range []string{"+charts-create", "+chart-templates"} {
		caller := &upsertByKeyCaller{capabilityFn: func(context.Context, string, string) (bool, error) { return false, errors.New("discovery unavailable") }}
		args := []string{"--chart-types", "LINE"}
		if command == "+charts-create" {
			args = []string{"--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--yes"}
		}
		if _, err := runAITableCompositeCLI(t, caller, command, args...); err == nil || len(caller.calls) != 0 {
			t.Fatalf("discovery failure dispatched: %v %#v", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageTemplatesPreferConfirmedProjectionAndExplicitLegacy(t *testing.T) {
	for _, available := range []bool{true, false} {
		for _, forceLegacy := range []bool{true, false} {
			caller := &upsertByKeyCaller{capabilityFn: func(context.Context, string, string) (bool, error) {
				if forceLegacy {
					t.Fatal("explicit legacy unexpectedly discovered")
				}
				return available, nil
			}}
			caller.callFn = func(_ int, _, tool string, _ map[string]any) (string, error) {
				if available && !forceLegacy {
					if tool != "get_dashboard_widget_templates" {
						t.Fatal(tool)
					}
					return `{"status":"success","data":{"LINE":{"chartType":"LINE"}}}`, nil
				}
				if tool != "get_dashboard_widgets_example" {
					t.Fatal(tool)
				}
				return `{"status":"success","data":"{\"LINE\":{\"chartType\":\"LINE\"}}"}`, nil
			}
			args := []string{"--chart-types", "LINE"}
			if forceLegacy {
				args = append(args, "--server-filter=false")
			}
			out, err := runAITableCompositeCLI(t, caller, "+chart-templates", args...)
			if err != nil || len(caller.calls) != 1 || !strings.Contains(out, `"LINE"`) {
				t.Fatalf("out=%s error=%v calls=%#v", out, err, caller.calls)
			}
		}
	}
}

func TestCrossPlatformCoverageSerialChartKeepsPreflightRefusalAndUnknownCandidate(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		cause := apperrors.NewAPI("No replay", apperrors.WithRetryable(!submitted), apperrors.WithExecutionStarted(submitted), apperrors.WithDetails(map[string]any{"mutationSubmitted": submitted, "baseId": "B", "dashboardId": "D", "chartId": "C"}))
		caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
			if tool == "get_dashboard" {
				return `{"status":"success","data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`, nil
			}
			if tool != "create_chart" {
				t.Fatal("unexpected write or readback", tool)
			}
			return "", cause
		}}
		_, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--yes")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || len(caller.calls) != 2 || typed.ExecutionStarted == nil || *typed.ExecutionStarted != submitted || typed.Retryable == submitted {
			t.Fatalf("lost refusal or replayed: %#v %v", typed, err)
		}
		if submitted {
			result := typed.Details["result"].(map[string]any)
			items := result["items"].([]map[string]any)
			if items[0]["chartId"] != "C" || items[0]["status"] != "unknown" {
				t.Fatal(items)
			}
		}
	}
}
