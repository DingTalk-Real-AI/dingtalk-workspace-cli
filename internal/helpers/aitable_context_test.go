// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableContextRetryCaller struct {
	aitableTestCaller
	contexts []context.Context
}

func (c *aitableContextRetryCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.contexts = append(c.contexts, ctx)
	return c.aitableTestCaller.CallTool(ctx, server, tool, args)
}

func TestCrossPlatformCoverageAitableRetryPreservesContext(t *testing.T) {
	testseam.Swap(t, &os.Args, []string{"dws", "aitable"})
	for _, tc := range []struct {
		name string
		tool string
		call func(context.Context, string, map[string]any) error
	}{
		{"main", "get_base", callAitableToolContext},
		{"helper", "list_workflows", callAitableHelperToolContext},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), aitableCommandContextKey{}, "invocation"))
			defer cancel()
			retryable := fmt.Errorf("timeout: retryable: true")
			caller := &aitableContextRetryCaller{aitableTestCaller: aitableTestCaller{errors: []error{retryable, retryable}}}
			InitDepsForTest(t, caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard
			testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
				ready := make(chan time.Time, 1)
				ready <- time.Time{}
				return ready
			})
			if err := tc.call(ctx, tc.tool, nil); err != nil {
				t.Fatal(err)
			}
			if len(caller.contexts) != 3 {
				t.Fatalf("attempts=%d, want 3", len(caller.contexts))
			}
			for _, got := range caller.contexts {
				if got.Value(aitableCommandContextKey{}) != "invocation" || got.Done() != ctx.Done() {
					t.Fatal("retry discarded identity or cancellation")
				}
			}
			caller.contexts, caller.calls = nil, nil
			testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
				cancel()
				return make(chan time.Time)
			})
			if err := tc.call(ctx, tc.tool, nil); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel during backoff: %v", err)
			}
			if len(caller.contexts) != 1 {
				t.Fatalf("canceled invocation continued: %d attempts", len(caller.contexts))
			}
		})
	}
}

// 记录命令的业务调用、辅助读取及兼容路由接收到的同一调用上下文。
type aitableInvocationContextCaller struct {
	aitableContextRetryCaller
	routeContexts []context.Context
}

func (c *aitableInvocationContextCaller) CallReadTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, server, tool, args)
}

func (c *aitableInvocationContextCaller) ResolveToolProduct(ctx context.Context, products []string, _ string) (string, error) {
	c.routeContexts = append(c.routeContexts, ctx)
	return products[len(products)-1], nil
}

func TestCrossPlatformCoverageAitableCommandContextChains(t *testing.T) {
	testseam.Swap(t, &aitableViewFilterReadbackSleep, func(time.Duration) {})
	testseam.Swap(t, &aitableCreateReadbackWait, func(ctx context.Context, _ time.Duration) error { return ctx.Err() })
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Time{}
		return ready
	})
	for _, tc := range []struct {
		name      string
		args      []string
		responses []string
		errors    []error
		tools     []string
		routes    int
	}{
		{
			name:   "read tool compatibility fallback",
			args:   []string{"base", "get-primary-doc-id", "--base-id=b", "--table-id=t", "--record-id=r"},
			errors: []error{apperrors.NewAPI("call failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "TOOL_NOT_FOUND"}))},
			tools:  []string{"get_cell_doc", "get_base_primary_doc_id"},
		},
		{
			name:      "compatible read receipt retry",
			args:      []string{"base", "list"},
			responses: []string{"", `{"data":{"bases":[],"nextCursor":null}}`},
			errors:    []error{errors.New("timeout: retryable: true")},
			tools:     []string{"list_bases", "list_bases"},
		},
		{
			name:      "compatible creation receipt",
			args:      []string{"record", "create", "--base-id=b", "--table-id=t", `--records=[{"cells":{"f":"value"}}]`},
			responses: []string{`{"data":{"newRecordIds":["r"]}}`},
			tools:     []string{"create_records"},
		},
		{
			name: "field creation and bounded readback context",
			args: []string{"field", "create", "--base-id=b", "--table-id=t", "--name=fixture", "--type=text", "--wait"},
			responses: []string{
				`{"results":[{"success":true,"fieldId":"f"}]}`,
				`{"fields":[{"fieldId":"f","fieldName":"fixture","type":"text"}]}`,
				`{"fields":[{"fieldId":"f","fieldName":"fixture","type":"text"}]}`,
			},
			tools: []string{"create_fields", "get_fields", "get_fields"},
		},
		{
			name: "table creation and bounded readback context",
			args: []string{"table", "create", "--base-id=b", "--name=fixture", `--fields=[{"fieldName":"fixture","type":"text"}]`, "--wait"},
			responses: []string{
				`{"data":{"tableId":"t"}}`,
				`{"fields":[{"fieldId":"f","fieldName":"fixture","type":"text"}]}`,
				`{"fields":[{"fieldId":"f","fieldName":"fixture","type":"text"}]}`,
			},
			tools: []string{"create_table", "get_fields", "get_fields"},
		},
		{
			name:      "API key metadata direct caller",
			args:      []string{"api-key", "list", "--base-id=b"},
			responses: []string{`{"status":"success","data":{"keys":[]}}`},
			tools:     []string{"list_sql_sheet_api_keys"},
		},
		{
			name: "view lookup and record pagination",
			args: []string{"record", "query", "--base-id=b", "--table-id=t", "--view-id=v", "--all"},
			responses: []string{
				`{"data":{"views":[{"viewId":"v","viewType":"Grid","filter":[],"sort":[]}]}}`,
				`{"data":{"records":[{"recordId":"r1"}],"hasMore":true,"nextCursor":"next"}}`,
				`{"data":{"records":[{"recordId":"r2"}],"hasMore":false,"nextCursor":""}}`,
			},
			tools: []string{"get_views", "query_records", "query_records"},
		},
		{
			name: "filter preflight write and delayed readback",
			args: []string{"view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=v", `--json=[{"operator":"eq","operands":["f","value"]}]`},
			responses: []string{
				`{"data":{"fields":[{"fieldId":"f","type":"text"}]}}`,
				`{"success":true}`,
				`{"data":{"views":[{"viewId":"v","viewType":"Grid","filter":[]}]}}`,
				`{"data":{"views":[{"viewId":"v","viewType":"Grid","filter":[{"operator":"eq","operands":["f","value"]}]}]}}`,
			},
			tools: []string{"get_fields", "update_view", "get_views", "get_views"},
		},
		{
			name:      "helper route and parsed response",
			args:      []string{"form", "get", "--base-id=b", "--table-id=t", "--view-id=v"},
			responses: []string{`{"data":[{"viewId":"v","title":"fixture"}]}`},
			tools:     []string{"list_form_views"},
			routes:    1,
		},
		{
			name:  "unified comment result",
			args:  []string{"comment", "list", "--base-id=b", "--table-id=t", "--record-id=r"},
			tools: []string{"list_comments"},
		},
		{
			name:   "unified helper result",
			args:   []string{"form", "share", "get", "--base-id=b", "--table-id=t", "--view-id=v"},
			tools:  []string{"get_share_form_config"},
			routes: 1,
		},
		{
			name:      "psql direct caller",
			args:      []string{"psql", "-d", "b", "-l"},
			responses: []string{`{"status":"success","data":[{"tableId":"t","tableName":"fixture"}]}`},
			tools:     []string{"otable_pg_list_tables"},
		},
		{
			name:      "entity auxiliary reader",
			args:      []string{"entity", "search", "--entity-type=DEPARTMENT", "--keyword=fixture"},
			responses: []string{`{"candidates":[{"name":"fixture","department":{"departmentId":"d"}}],"hasMore":false}`},
			tools:     []string{"search_entities"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), aitableCommandContextKey{}, tc.name), time.Minute)
			defer cancel()
			caller := &aitableInvocationContextCaller{aitableContextRetryCaller: aitableContextRetryCaller{
				aitableTestCaller: aitableTestCaller{responses: tc.responses, errors: tc.errors},
			}}
			InitDepsForTest(t, caller)
			deps.Out.w, deps.Out.errW = io.Discard, io.Discard
			testseam.Swap(t, &os.Args, append([]string{"dws", "aitable"}, tc.args...))
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			root.SilenceErrors, root.SilenceUsage = true, true
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append(append([]string{}, tc.args...), "--yes"))
			if err := corecmd.ExecuteContextForTest(root, ctx); err != nil {
				t.Fatal(err)
			}
			var gotTools []string
			for _, call := range caller.calls {
				gotTools = append(gotTools, call.tool)
			}
			if !reflect.DeepEqual(gotTools, tc.tools) {
				t.Fatalf("tools=%v, want %v", gotTools, tc.tools)
			}
			if len(caller.contexts) != len(tc.tools) || len(caller.routeContexts) != tc.routes {
				t.Fatalf("recorded business contexts=%d routes=%d, want %d/%d", len(caller.contexts), len(caller.routeContexts), len(tc.tools), tc.routes)
			}
			wantDeadline, _ := ctx.Deadline()
			cancel()
			for _, got := range append(caller.contexts, caller.routeContexts...) {
				deadline, hasDeadline := got.Deadline()
				if got.Value(aitableCommandContextKey{}) != tc.name || !hasDeadline || deadline.After(wantDeadline) {
					t.Fatal("command chain discarded invocation identity or deadline")
				}
				if !errors.Is(got.Err(), context.Canceled) {
					t.Fatal("command chain discarded parent cancellation")
				}
			}
		})
	}
}
