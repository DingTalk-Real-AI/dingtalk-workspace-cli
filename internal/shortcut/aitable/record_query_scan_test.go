// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const recordQueryLegacyEmptyFixture = `{"success":true,"status":"success","data":{}}`

func TestCrossPlatformCoverageRecordQueryRejectsContradictoryEmptyPages(t *testing.T) {
	for _, all := range []bool{false, true} {
		for name, raw := range map[string]string{
			"legacy_more":                   `{"success":true,"status":"success","data":{},"hasMore":true}`,
			"legacy_cursor":                 `{"success":true,"status":"success","data":{},"nextCursor":"next"}`,
			"legacy_positive_total":         `{"success":true,"status":"success","data":{},"totalCount":120}`,
			"original_probe":                `{"success":true,"status":"success","data":{},"hasMore":true,"nextCursor":"next","totalCount":120}`,
			"explicit_empty_positive_total": `{"data":{"records":[],"totalCount":120}}`,
			"negative_total":                `{"data":{"records":[],"totalCount":-1}}`,
			"fractional_total":              `{"data":{"records":[],"totalCount":0.5}}`,
			"invalid_total":                 `{"data":{"records":[],"totalCount":"unknown"}}`,
			"huge_total":                    `{"data":{"records":[],"totalCount":1e30}}`,
			"string_more":                   `{"success":true,"status":"success","data":{},"hasMore":"true","totalCount":0}`,
			"invalid_cursor_type":           `{"data":{"records":[],"nextCursor":12}}`,
			"nested_total":                  `{"success":true,"status":"success","data":{},"pagination":{"totalCount":120}}`,
			"conflicting_totals":            `{"data":{"records":[],"totalCount":0},"totalCount":120}`,
			"legacy_snake_cursor":           `{"success":true,"status":"success","data":{},"next_cursor":"next"}`,
			"unsupported_empty_cursor":      `{"data":{"records":[],"next_cursor":"next"}}`,
		} {
			t.Run(fmt.Sprintf("all_%t/%s", all, name), func(t *testing.T) {
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: raw}}}
				extra := []string{}
				if all {
					extra = append(extra, "--all")
				}
				result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "record_query_inconsistent_empty_response" || typed.Retryable || result != nil || len(caller.calls) != 1 {
					t.Fatalf("contradictory empty accepted/retried: result=%#v err=%v calls=%d", result, err, len(caller.calls))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordQueryEmptyPageRetainsEarlierCount(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, tc := range []struct {
			name, total, terminal string
			fail                  bool
		}{
			{"dropped_count", "120", `{"data":{"records":[]}}`, true},
			{"legacy_dropped_count", "120", recordQueryLegacyEmptyFixture, true},
			{"invalid_earlier_count", `"bad"`, `{"data":{"records":[]}}`, true},
			{"valid_terminal_probe", "1", `{"data":{"records":[]}}`, false},
			{"earlier_nullable_total", "null", `{"data":{"records":[]}}`, false},
		} {
			t.Run(fmt.Sprintf("all_%t/%s", all, tc.name), func(t *testing.T) {
				first := fmt.Sprintf(`{"data":{"records":[{"recordId":"first"}],"nextCursor":"next","totalCount":%s}}`, tc.total)
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: first}, {text: tc.terminal}}}
				extra := []string{}
				if all {
					extra = append(extra, "--all")
				}
				result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				if (err != nil) != tc.fail || len(caller.calls) != 2 {
					t.Fatal(result, err, len(caller.calls))
				}
				if tc.fail && result != nil {
					t.Fatal("incomplete result emitted as success", result)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordQueryValidEmptyAndEmptyContinuation(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			steps  []upsertByKeyStep
			limits []int
		}{
			{"explicit_zero", []upsertByKeyStep{{text: `{"data":{"records":[],"totalCount":0}}`}}, []int{100}},
			{"nullable_total", []upsertByKeyStep{{text: `{"data":{"records":[],"totalCount":null}}`}}, []int{100}},
			{"legacy_proven_zero", []upsertByKeyStep{{text: `{"success":true,"status":"success","data":{},"totalCount":"0"}`}}, []int{100}},
			{"nested_zero", []upsertByKeyStep{{text: `{"data":{"records":[],"totalCount":0},"totalCount":0,"pagination":{"hasMore":false,"nextCursor":null}}`}}, []int{100}},
			{"legacy_unproven_zero", []upsertByKeyStep{{text: recordQueryLegacyEmptyFixture}, {text: recordQueryLegacyEmptyFixture}}, []int{100, 20}},
			{"legacy_nullable_total", []upsertByKeyStep{{text: `{"success":true,"status":"success","data":{},"totalCount":null}`}, {text: `{"success":true,"status":"success","data":{},"totalCount":null}`}}, []int{100, 20}},
			{"empty_with_cursor", []upsertByKeyStep{{text: `{"data":{"records":[],"nextCursor":"next","totalCount":0}}`}, {text: `{"data":{"records":[],"totalCount":0}}`}}, []int{100, 100}},
		} {
			t.Run(fmt.Sprintf("all_%t/%s", all, tc.name), func(t *testing.T) {
				caller := &upsertByKeyCaller{steps: tc.steps}
				extra := []string{}
				if all {
					extra = append(extra, "--all")
				}
				result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				rows, found := findRecords(result)
				if err != nil || !found || len(rows) != 0 || responseHasMore(result) || len(caller.calls) != len(tc.limits) {
					t.Fatalf("empty result: %#v err=%v calls=%d", result, err, len(caller.calls))
				}
				for i, call := range caller.calls {
					if call.args["limit"] != tc.limits[i] {
						t.Fatalf("request %d: %#v", i, call.args)
					}
				}
			})
		}
	}
	// A caller resuming after an already-read full page can legitimately receive
	// an empty terminal page with a positive total for the whole query.
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"records":[],"totalCount":100}}`}}}
	result, err := runRecordQueryShortcutCLI(t, caller, 100, "--cursor", "already-read")
	if err != nil || responseHasMore(result) || len(caller.calls) != 1 {
		t.Fatal(result, err, caller.calls)
	}
	caller = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: recordQueryLegacyEmptyFixture}}}
	result, err = runRecordQueryShortcutCLI(t, caller, 5)
	if err != nil || responseHasMore(result) || len(caller.calls) != 1 {
		t.Fatal("small legacy empty page was retried", result, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryFallbackDiscardsEarlierAttempt(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, cursor := range []string{"", "offset-5"} {
			if all && cursor != "" {
				continue
			}
			t.Run(fmt.Sprintf("all_%t/cursor_%s", all, cursor), func(t *testing.T) {
				old := updateFixtureRecords(0, 20, "old attempt")
				fresh := updateFixtureRecords(1000, 120, "fresh attempt")
				initial, _ := json.Marshal(map[string]any{"data": map[string]any{"records": old, "nextCursor": "old-attempt"}})
				caller := &upsertByKeyCaller{}
				caller.callFn = func(index int, _, _ string, args map[string]any) (string, error) {
					switch index {
					case 0:
						return string(initial), nil
					case 1:
						return recordQueryLegacyEmptyFixture, nil
					case 2:
						got, _ := args["cursor"].(string)
						if got != cursor {
							t.Fatalf("fallback reused failed-attempt cursor: %#v", args)
						}
					}
					if args["limit"] != 20 {
						t.Fatalf("fallback exceeded small page: %#v", args)
					}
					return scanRecordQueryResponse(t, fresh, args, 20), nil
				}
				extra := []string{}
				if all {
					extra = append(extra, "--all", "--max-records", "120")
				} else if cursor != "" {
					extra = append(extra, "--cursor", cursor)
				}
				result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				if err != nil {
					t.Fatal(err)
				}
				start, end := 0, 100
				if cursor != "" {
					start, end = 5, 105
				}
				if all {
					end = 120
				}
				assertRecordQueryRows(t, result, fresh[start:end])
			})
		}
	}
}

func TestCrossPlatformCoverageRecordQueryOversizedResponseFallbackIsBounded(t *testing.T) {
	tooLarge := apperrors.NewDiscovery("oversized", apperrors.WithOperation("tools/call"), apperrors.WithReason("tools_call_response_too_large"))
	for _, all := range []bool{false, true} {
		for _, failAgain := range []bool{false, true} {
			t.Run(fmt.Sprintf("all_%t/failAgain_%t", all, failAgain), func(t *testing.T) {
				records := updateFixtureRecords(0, 100, "完成")
				caller := &upsertByKeyCaller{}
				caller.callFn = func(index int, _, _ string, args map[string]any) (string, error) {
					if index == 0 || failAgain {
						return "", tooLarge
					}
					if args["limit"] != 20 {
						t.Fatalf("fallback limit: %#v", args)
					}
					return scanRecordQueryResponse(t, records, args, 20), nil
				}
				extra := []string{}
				if all {
					extra = append(extra, "--all")
				}
				result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				if failAgain {
					if !errors.Is(err, tooLarge) || result != nil || len(caller.calls) != 2 {
						t.Fatalf("unbounded fallback: %#v %v calls=%d", result, err, len(caller.calls))
					}
				} else {
					if err != nil || len(caller.calls) != 6 {
						t.Fatal(result, err, len(caller.calls))
					}
					assertRecordQueryRows(t, result, records)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordQueryDoesNotDowngradeOtherErrors(t *testing.T) {
	for _, all := range []bool{false, true} {
		for name, err := range map[string]error{
			"timeout":         context.DeadlineExceeded,
			"cancelled":       context.Canceled,
			"permission":      apperrors.NewAuth("denied"),
			"message_only":    errors.New("response too large"),
			"wrong_operation": apperrors.NewDiscovery("oversized", apperrors.WithOperation("tools/list"), apperrors.WithReason("tools_call_response_too_large")),
			"wrong_category":  apperrors.NewAPI("oversized", apperrors.WithOperation("tools/call"), apperrors.WithReason("tools_call_response_too_large")),
		} {
			t.Run(fmt.Sprintf("all_%t/%s", all, name), func(t *testing.T) {
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: err}}}
				extra := []string{}
				if all {
					extra = append(extra, "--all")
				}
				result, got := runRecordQueryShortcutCLI(t, caller, 100, extra...)
				if got == nil || result != nil || len(caller.calls) != 1 {
					t.Fatalf("unexpected retry: %#v %v calls=%d", result, got, len(caller.calls))
				}
			})
		}
	}
}

// BenchmarkRecordQueryScanPageValidation measures only the new validation
// overhead for representative nonempty pages, excluding transport and decoding.
func BenchmarkRecordQueryScanPageValidation(b *testing.B) {
	for _, size := range []int{1, 100} {
		b.Run(fmt.Sprintf("rows_%d", size), func(b *testing.B) {
			records := updateFixtureRecords(0, size, "benchmark")
			data := map[string]any{"records": records}
			policy := recordQueryReadPolicy{pageSize: 100, scan: true}
			state := recordQueryReadState{query: map[string]any{}, pageSize: 100}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := policy.validateEmptyPage(data, records, true, state); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
