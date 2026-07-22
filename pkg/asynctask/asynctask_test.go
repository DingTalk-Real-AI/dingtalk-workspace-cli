// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package asynctask

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 用极小 backoff 加速测试
var testBackoff = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond}

func TestSubmit_HappyPath(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "job-1", nil }
	calls := 0
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		calls++
		if calls < 3 {
			return QueryResult{Status: StatusProcessing}, nil
		}
		return QueryResult{Status: StatusSuccess, DownloadURL: "https://example/x.docx"}, nil
	}
	res, err := Submit(context.Background(), submitFn, queryFn, Options{
		Backoff: testBackoff,
		Timeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Status != StatusSuccess || res.DownloadURL != "https://example/x.docx" {
		t.Fatalf("got: %+v", res)
	}
	if res.Attempts != 3 {
		t.Fatalf("attempts: %d", res.Attempts)
	}
}

func TestSubmit_Failed(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "job-2", nil }
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		return QueryResult{Status: StatusFailed, Message: "INVALID_NODE"}, nil
	}
	res, _ := Submit(context.Background(), submitFn, queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if res.Status != StatusFailed || res.Message != "INVALID_NODE" {
		t.Fatalf("got: %+v", res)
	}
}

func TestSubmit_Timeout(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "job-3", nil }
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		return QueryResult{Status: StatusProcessing}, nil
	}
	res, err := Submit(context.Background(), submitFn, queryFn, Options{
		Backoff: testBackoff,
		Timeout: 10 * time.Millisecond, // 故意很短
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Status != StatusTimeout {
		t.Fatalf("expect TIMEOUT, got: %+v", res)
	}
	if res.JobID != "job-3" {
		t.Fatalf("job id lost: %s", res.JobID)
	}
}

func TestSubmit_QueryRetry(t *testing.T) {
	// 模拟单次 query 失败但下次成功的场景：本包应该忽略单次失败继续轮询
	submitFn := func(ctx context.Context) (string, error) { return "job-4", nil }
	calls := 0
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		calls++
		if calls == 1 {
			return QueryResult{}, errors.New("transient 502")
		}
		return QueryResult{Status: StatusSuccess, DownloadURL: "ok"}, nil
	}
	res, err := Submit(context.Background(), submitFn, queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if err != nil || res.Status != StatusSuccess {
		t.Fatalf("transient retry: %+v err=%v", res, err)
	}
}

func TestSubmit_SubmitError(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "", errors.New("auth fail") }
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) { return QueryResult{}, nil }
	_, err := Submit(context.Background(), submitFn, queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if err == nil {
		t.Fatal("expected submit error")
	}
}

func TestResume_AlreadyHasJobID(t *testing.T) {
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		if jobID != "preexisting-job" {
			t.Fatalf("jobID lost: %s", jobID)
		}
		return QueryResult{Status: StatusSuccess, DownloadURL: "ok"}, nil
	}
	res, err := Resume(context.Background(), "preexisting-job", queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if err != nil || res.Status != StatusSuccess {
		t.Fatalf("resume: %+v err=%v", res, err)
	}
}

func TestSubmit_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	submitFn := func(ctx context.Context) (string, error) { return "job-x", nil }
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		return QueryResult{Status: StatusProcessing}, nil
	}
	_, err := Submit(ctx, submitFn, queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expect ctx.Canceled, got %v", err)
	}
}

func TestSubmit_ProgressCallback(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "job-5", nil }
	calls := 0
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		calls++
		if calls < 2 {
			return QueryResult{Status: StatusProcessing}, nil
		}
		return QueryResult{Status: StatusSuccess}, nil
	}
	var seenStatuses []Status
	progress := func(attempt int, status Status, elapsed time.Duration) {
		seenStatuses = append(seenStatuses, status)
	}
	_, _ = Submit(context.Background(), submitFn, queryFn, Options{
		Backoff:    testBackoff,
		Timeout:    1 * time.Second,
		ProgressFn: progress,
	})
	if len(seenStatuses) < 2 {
		t.Fatalf("progress not called enough: %v", seenStatuses)
	}
	if seenStatuses[len(seenStatuses)-1] != StatusSuccess {
		t.Fatalf("final status not seen: %v", seenStatuses)
	}
}

func TestSubmit_UnknownStatusKeepsPolling(t *testing.T) {
	submitFn := func(ctx context.Context) (string, error) { return "job-6", nil }
	calls := 0
	queryFn := func(ctx context.Context, jobID string) (QueryResult, error) {
		calls++
		if calls < 3 {
			return QueryResult{Status: "WEIRD_STATUS"}, nil
		}
		return QueryResult{Status: StatusSuccess}, nil
	}
	res, _ := Submit(context.Background(), submitFn, queryFn, Options{Backoff: testBackoff, Timeout: 1 * time.Second})
	if res.Status != StatusSuccess {
		t.Fatalf("unknown status not handled: %+v", res)
	}
}
