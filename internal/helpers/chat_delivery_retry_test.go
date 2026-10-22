// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func receipt1001Error() error {
	return apperrors.NewAPI("private-diagnostics", apperrors.WithServerDiag(apperrors.ServerDiagnostics{
		ServerErrorCode: "1001",
	}))
}

type timedEmployeeReceiptCaller struct {
	digitalEmployeeProtocolCaller
	t           *testing.T
	started     time.Time
	delay       time.Duration
	successAt   int
	queryTimes  []time.Duration
	deadlines   []time.Time
	cancelQuery context.CancelFunc
}

func (c *timedEmployeeReceiptCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.t.Helper()
	if product != "im" || tool != "query_message_send_status" || args["openTaskId"] != "task-1" {
		c.t.Fatalf("回执重试改变了接口或任务: %s/%s %#v", product, tool, args)
	}
	c.queryTimes = append(c.queryTimes, time.Since(c.started))
	deadline, _ := ctx.Deadline()
	c.deadlines = append(c.deadlines, deadline)
	if c.cancelQuery != nil {
		c.cancelQuery()
	}
	if err := waitForDigitalEmployeeReceipt(ctx, c.delay); err != nil {
		return nil, err
	}
	if c.successAt == len(c.queryTimes) {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"openMessageId":"reply-1","sendStatus":"SUCCESS"}`}}}, nil
	}
	return nil, receipt1001Error()
}

func TestCrossPlatformCoverageChatDeliveryReceipt1001Window(t *testing.T) {
	for _, tc := range []struct {
		name          string
		delay         time.Duration
		successAt     int
		parentTimeout time.Duration
		cancelQuery   bool
		wantElapsed   time.Duration
		wantQueries   int
	}{
		{"eventual success", 100 * time.Millisecond, 3, 0, false, 800 * time.Millisecond, 3},
		{"slow errors exhaust window", 400 * time.Millisecond, 0, 0, false, 2 * time.Second, 4},
		{"wait reaches deadline", 1900 * time.Millisecond, 0, 0, false, 2 * time.Second, 1},
		{"request reaches deadline", 3 * time.Second, 0, 0, false, 2 * time.Second, 1},
		{"parent deadline wins", 100 * time.Millisecond, 0, 300 * time.Millisecond, false, 300 * time.Millisecond, 1},
		{"parent cancellation wins", time.Second, 0, 0, true, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 虚拟时钟同时推进网络等待和轮询间隔，避免真实睡眠和时间边界抖动。
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.parentTimeout > 0 {
					var stop context.CancelFunc
					ctx, stop = context.WithTimeout(ctx, tc.parentTimeout)
					defer stop()
				}
				caller := &timedEmployeeReceiptCaller{t: t, started: started, delay: tc.delay, successAt: tc.successAt}
				if tc.cancelQuery {
					caller.cancelQuery = cancel
				}
				InitDepsForTest(t, caller)
				result, err := ResolveChatDelivery(ctx, map[string]any{"openTaskId": "task-1"}, "conversation-1", "idem-1")
				switch {
				case tc.successAt > 0:
					if err != nil || result["openMessageId"] != "reply-1" || result["deliveryStatus"] != "delivered" {
						t.Fatalf("1001 后未恢复回执: result=%v err=%v", result, err)
					}
				case tc.parentTimeout > 0:
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("未保留调用方 deadline: %v", err)
					}
				case tc.cancelQuery:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("未保留调用方取消: %v", err)
					}
				default:
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "delivery_unknown" || !typed.RetryableSet || typed.Retryable {
						t.Fatalf("超时必须保留送达未知且禁止重发: %v", err)
					}
				}
				if elapsed := time.Since(started); elapsed != tc.wantElapsed || len(caller.queryTimes) != tc.wantQueries {
					t.Fatalf("elapsed=%v queries=%v, want elapsed=%v queries=%d", elapsed, caller.queryTimes, tc.wantElapsed, tc.wantQueries)
				}
				wantDeadline := started.Add(2 * time.Second)
				if tc.parentTimeout > 0 {
					wantDeadline = started.Add(tc.parentTimeout)
				}
				for i, deadline := range caller.deadlines {
					if !deadline.Equal(wantDeadline) || caller.queryTimes[i] >= 2*time.Second {
						t.Fatalf("第 %d 次查询重新计时或超过窗口: at=%v deadline=%v want=%v", i+1, caller.queryTimes[i], deadline, wantDeadline)
					}
				}
			})
		})
	}
}
