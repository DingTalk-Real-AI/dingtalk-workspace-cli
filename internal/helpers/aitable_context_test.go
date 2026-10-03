// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

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
