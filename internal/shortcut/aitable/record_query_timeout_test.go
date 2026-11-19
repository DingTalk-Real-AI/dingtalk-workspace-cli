// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func queryTransportTimeout(reason string) error {
	return apperrors.NewAPI("transport deadline", apperrors.WithOperation("tools/call"),
		apperrors.WithFailureStage("request"), apperrors.WithReason(reason), apperrors.WithCause(context.DeadlineExceeded))
}

type queryContextCaller struct {
	*upsertByKeyCaller
	observe func(context.Context)
}

func (c *queryContextCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.observe(ctx)
	return c.upsertByKeyCaller.CallTool(ctx, product, tool, args)
}

func (c *queryContextCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.observe(ctx)
	return c.upsertByKeyCaller.CallReadTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageRecordQueryTimeoutFallbackRestartsWithOneBudget(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, reason := range []string{"request_timeout", "http_client_timeout", "body_timeout"} {
			t.Run(fmt.Sprintf("all_%t/%s", all, reason), func(t *testing.T) {
				rows := updateFixtureRecords(0, 120, "new")
				base := &upsertByKeyCaller{}
				base.callFn = func(index int, _, _ string, args map[string]any) (string, error) {
					if index == 0 {
						if reason == "body_timeout" {
							return "", apperrors.NewDiscovery("body read", apperrors.WithOperation("tools/call"),
								apperrors.WithReason("tools_call_response_read_failed"), apperrors.WithCause(context.DeadlineExceeded))
						}
						return "", queryTransportTimeout(reason)
					}
					return scanRecordQueryResponse(t, rows, args, 100), nil
				}
				var fallbackCtx context.Context
				var deadline time.Time
				caller := &queryContextCaller{upsertByKeyCaller: base}
				caller.observe = func(ctx context.Context) {
					if len(base.calls) == 0 {
						return
					}
					d, ok := ctx.Deadline()
					if !ok || time.Until(d) > recordQueryTimeoutFallbackBudget || time.Until(d) <= 0 {
						t.Fatal("fallback has no bounded deadline", d)
					}
					if deadline.IsZero() {
						deadline, fallbackCtx = d, ctx
					} else if !deadline.Equal(d) {
						t.Fatal("each fallback page received a new budget")
					}
				}
				extra := []string{"--cursor", "offset-10"}
				wantRows, start := 100, 10
				if all {
					extra, wantRows, start = []string{"--all", "--max-records", "120"}, 120, 0
				}
				parent, release := context.WithTimeout(context.Background(), 2*recordQueryTimeoutFallbackBudget)
				defer release()
				payload, err := runRecordQueryShortcutCLIContext(t, parent, caller, 100, extra...)
				if err != nil {
					t.Fatal(err)
				}
				if parent.Err() != nil {
					t.Fatal("fallback cancelled the command's parent context")
				}
				got := payload["data"].(map[string]any)["records"].([]any)
				if len(got) != wantRows || got[0].(map[string]any)["recordId"] != rows[start]["recordId"] {
					t.Fatalf("fallback result = %#v", payload)
				}
				for i, call := range base.calls {
					wantLimit := 20
					if i == 0 {
						wantLimit = 100
					}
					if call.args["limit"] != wantLimit {
						t.Fatalf("call %d args=%#v", i, call.args)
					}
				}
				if fallbackCtx == nil || fallbackCtx.Err() != context.Canceled {
					t.Fatal("fallback context was not released")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutDoesNotRetryUnsafeOrSmallCases(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ctx   func() (context.Context, context.CancelFunc)
		limit int
		extra []string
	}{
		{"parent_cancelled", func() (context.Context, context.CancelFunc) {
			c, cancel := context.WithCancel(context.Background())
			cancel()
			return c, cancel
		}, 100, nil},
		{"parent_expired", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, 100, nil},
		{"budget_insufficient", func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), time.Second)
		}, 100, nil},
		{"small_window", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }, 20, nil},
		{"small_all", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }, 20, []string{"--all"}},
		{"exact_ids", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }, 100, []string{"--record-ids", "r1,r2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: queryTransportTimeout("request_timeout")}}}
			result, err := runRecordQueryShortcutCLIContext(t, ctx, caller, tc.limit, tc.extra...)
			if err == nil || result != nil || len(caller.calls) != 1 {
				t.Fatalf("unsafe retry: %#v %v calls=%d", result, err, len(caller.calls))
			}
		})
	}
	for name, err := range map[string]error{
		"bare_deadline":    context.DeadlineExceeded,
		"cancelled":        context.Canceled,
		"bare_message":     errors.New("request_timeout"),
		"tls_timeout":      queryTransportTimeout("tls_timeout"),
		"other_operation":  apperrors.NewAPI("timeout", apperrors.WithOperation("tools/list"), apperrors.WithReason("request_timeout"), apperrors.WithFailureStage("request")),
		"wrong_stage":      apperrors.NewAPI("timeout", apperrors.WithOperation("tools/call"), apperrors.WithReason("request_timeout")),
		"no_timeout_cause": apperrors.NewAPI("timeout", apperrors.WithOperation("tools/call"), apperrors.WithReason("request_timeout"), apperrors.WithFailureStage("request")),
		"server_timeout":   apperrors.NewAPI("timeout", apperrors.WithOperation("tools/call"), apperrors.WithReason("request_timeout"), apperrors.WithFailureStage("request"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "QUERY_TIMEOUT"})),
		"body_broken":      apperrors.NewDiscovery("broken", apperrors.WithOperation("tools/call"), apperrors.WithReason("tools_call_response_read_failed"), apperrors.WithCause(errors.New("EOF"))),
	} {
		t.Run(name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: err}}}
			result, got := runRecordQueryShortcutCLI(t, caller, 100)
			if got == nil || result != nil || len(caller.calls) != 1 {
				t.Fatal("unclassified/unrelated error retried", got, result, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutAfterPartialPageDiscardsAttempt(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprint(all), func(t *testing.T) {
			oldRows, newRows := updateFixtureRecords(0, 120, "old"), updateFixtureRecords(0, 120, "new")
			caller := &upsertByKeyCaller{callFn: func(index int, _, _ string, args map[string]any) (string, error) {
				if index == 0 {
					return scanRecordQueryResponse(t, oldRows, args, 50), nil
				}
				if index == 1 {
					return "", queryTransportTimeout("request_timeout")
				}
				return scanRecordQueryResponse(t, newRows, args, 100), nil
			}}
			extra, count := []string{}, 100
			if all {
				extra, count = []string{"--all", "--max-records", "120"}, 120
			}
			payload, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
			if err != nil {
				t.Fatal(err)
			}
			got := payload["data"].(map[string]any)["records"].([]any)
			if len(got) != count || caller.calls[2].args["cursor"] != nil {
				t.Fatal("fallback did not restart from original cursor", caller.calls)
			}
			for i, row := range got {
				if !reflect.DeepEqual(row, newRows[i]) {
					t.Fatalf("old/new attempts mixed at %d", i)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutFallbackStopsOnSecondFailure(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: queryTransportTimeout("request_timeout")}, {err: queryTransportTimeout("request_timeout")}}}
	result, err := runRecordQueryShortcutCLI(t, caller, 100)
	if err == nil || result != nil || len(caller.calls) != 2 || caller.calls[1].args["limit"] != 20 {
		t.Fatal("fallback was unbounded", result, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryTransportTimeoutRecoversThroughCommand(t *testing.T) {
	rows := updateFixtureRecords(0, 100, "new")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if requests.Add(1) == 1 {
			<-r.Context().Done() // Exercise the transport's real HTTP deadline classification.
			return
		}
		args := request.Params.Arguments
		args["limit"] = int(args["limit"].(float64))
		text := scanRecordQueryResponse(t, rows, args, 100)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID,
			"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
	}))
	defer server.Close()
	httpClient := server.Client()
	httpClient.Timeout = 200 * time.Millisecond
	client := transport.NewClient(httpClient)
	client.MaxRetries = 0
	var activeCtx context.Context
	base := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		result, err := client.CallTool(activeCtx, server.URL, tool, args)
		if err != nil {
			return "", err
		}
		return result.Blocks[0].Text, nil
	}}
	caller := &queryContextCaller{upsertByKeyCaller: base, observe: func(ctx context.Context) { activeCtx = ctx }}
	result, err := runRecordQueryShortcutCLI(t, caller, 100)
	if err != nil || len(result["data"].(map[string]any)["records"].([]any)) != 100 || requests.Load() != 6 {
		t.Fatal("real transport timeout did not recover with five small pages", result, err, requests.Load())
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutCannotResetBudgetOrRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &upsertByKeyCaller{callFn: func(index int, _, _ string, _ map[string]any) (string, error) {
		if index == 0 {
			cancel()
		}
		return "", queryTransportTimeout("request_timeout")
	}}
	result, err := runRecordQueryShortcutCLIContext(t, ctx, caller, 100)
	if err == nil || result != nil || len(caller.calls) != 1 {
		t.Fatal("cancelled read retried", result, err, caller.calls)
	}
	// A private wrapper must preserve diagnostics even when no retry budget remains.
	if !errors.Is(err, context.DeadlineExceeded) || err.Error() == "" {
		t.Fatal("transport cause lost", err)
	}

	rows := updateFixtureRecords(0, 100, "old")
	caller = &upsertByKeyCaller{callFn: func(index int, _, _ string, args map[string]any) (string, error) {
		if index == 0 {
			return scanRecordQueryResponse(t, rows, args, 80), nil
		}
		return "", queryTransportTimeout("request_timeout")
	}}
	result, err = runRecordQueryShortcutCLI(t, caller, 100)
	if err == nil || result != nil || len(caller.calls) != 2 || caller.calls[1].args["limit"] != 20 {
		t.Fatal("a timeout at an already-small page was retried", result, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutDoesNotReplayWriteVerification(t *testing.T) {
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		if tool == "query_records" {
			if args["limit"] != 20 {
				t.Fatal("verification page changed", args)
			}
			return "", queryTransportTimeout("request_timeout")
		}
		return `{"success":true}`, nil
	}}
	_, err := runRecordBatchCLI(t, caller, "+record-update", updateFixtureRecords(0, 100, "new"))
	if err == nil || len(caller.calls) != 2 || caller.calls[0].tool != "update_records" || caller.calls[1].tool != "query_records" {
		t.Fatal("write or verification was replayed", err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryTimeoutFallbackRejectsMalformedPayloadAndMidReadCancellation(t *testing.T) {
	for _, text := range []string{"", "{"} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: queryTransportTimeout("request_timeout")}, {text: text}}}
		result, err := runRecordQueryShortcutCLI(t, caller, 100)
		if err == nil || result != nil || len(caller.calls) != 2 {
			t.Fatal("malformed fallback accepted or retried", result, err, caller.calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := &upsertByKeyCaller{callFn: func(index int, _, _ string, args map[string]any) (string, error) {
		if index == 0 {
			return "", queryTransportTimeout("request_timeout")
		}
		cancel()
		return scanRecordQueryResponse(t, updateFixtureRecords(0, 100, "new"), args, 100), nil
	}}
	result, err := runRecordQueryShortcutCLIContext(t, ctx, base, 100)
	var classified *apperrors.Error
	if !errors.Is(err, context.Canceled) || !errors.As(err, &classified) || classified.ExitCode() != apperrors.ExitCodeAPI ||
		classified.Retryable || classified.Details["discard_previous_results"] != true || result != nil || len(base.calls) != 2 {
		t.Fatal("cancelled fallback continued to another page", result, err, base.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryFallbackExhaustedContextIsClassified(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, release := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer release()
	for _, ctx := range []context.Context{cancelled, expired} {
		// An already-stopped fallback must return before touching the RPC runtime.
		data, err := (recordQueryReadPolicy{ctx: ctx}).callPage(nil, nil)
		var classified *apperrors.Error
		if data != nil || !errors.Is(err, ctx.Err()) || !errors.As(err, &classified) ||
			classified.Reason != "record_query_fallback_stopped" || classified.ExitCode() != apperrors.ExitCodeAPI ||
			classified.Retryable || classified.Details["discard_previous_results"] != true {
			t.Fatal("fallback context lost its classification or cause", data, err)
		}
	}
}

func TestCrossPlatformCoverageRecordQueryIncompleteErrorDiscardsRead(t *testing.T) {
	for _, all := range []bool{false, true} {
		caller := &upsertByKeyCaller{callFn: func(index int, _, _ string, args map[string]any) (string, error) {
			if index == 0 {
				return scanRecordQueryResponse(t, updateFixtureRecords(0, 120, "old"), args, 50), nil
			}
			return `{"status":"error","error":{"code":"QUERY_RECORDS_INCOMPLETE","type":"SYSTEM_ERROR","retryable":false},"data":null}`, nil
		}}
		extra := []string{}
		if all {
			extra = []string{"--all", "--max-records", "120"}
		}
		result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
		var classified *apperrors.Error
		if !errors.As(err, &classified) || classified.Reason != "pagination_incomplete_read" ||
			classified.Retryable || classified.Details["discard_previous_results"] != true || result != nil || len(caller.calls) != 2 {
			t.Fatal("incomplete read was accepted or retried", result, err, caller.calls)
		}
	}
}
