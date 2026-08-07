// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package paging

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// stubFetcher 用一个预设的 page 序列模拟翻页响应。
type stubFetcher struct {
	pages     []Page
	pageErr   map[int]error // 第几次调用要返回 err（从 0 开始）
	calls     int
	lastInput string
}

func (s *stubFetcher) Fetch(ctx context.Context, cursor string) (Page, error) {
	s.lastInput = cursor
	idx := s.calls
	s.calls++
	if err, ok := s.pageErr[idx]; ok {
		return Page{}, err
	}
	if idx >= len(s.pages) {
		// 模拟服务端没数据：空 page + 空 cursor
		return Page{}, nil
	}
	return s.pages[idx], nil
}

func TestCrossPlatformCoverageFetchAllSinglePage(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{"a", "b", "c"}, NextCursor: ""},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if got.HasMore || !got.Complete || got.StopReason != StopComplete || got.Pages != 1 || got.Attempts != 1 || len(got.Records) != 3 {
		t.Fatalf("single page: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllMultiPage(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{1, 2}, NextCursor: "c1"},
			{Records: []any{3, 4}, NextCursor: "c2"},
			{Records: []any{5}, NextCursor: ""},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if got.HasMore || !got.Complete || got.Pages != 3 || got.Attempts != 3 || len(got.Records) != 5 {
		t.Fatalf("multi page: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllPageLimit(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{1}, NextCursor: "c1"},
			{Records: []any{2}, NextCursor: "c2"},
			{Records: []any{3}, NextCursor: "c3"},
			{Records: []any{4}, NextCursor: "c4"},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{
		PageLimit:      2,
		InterPageDelay: 1 * time.Millisecond,
	})
	if !got.HasMore || got.Pages != 2 || got.LastCursor != "c2" {
		t.Fatalf("page limit: got=%+v", got)
	}
	if !errors.Is(got.Err, ErrPageLimitReached) {
		t.Fatalf("expected ErrPageLimitReached, got %v", got.Err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("page limit records: got=%v", got.Records)
	}
	if got.Complete || got.Partial || got.StopReason != StopPageLimit {
		t.Fatalf("page limit completeness: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllMidErrorPartial(t *testing.T) {
	upstreamErr := errors.New("502 bad gateway")
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{1}, NextCursor: "c1"},
		},
		pageErr: map[int]error{1: upstreamErr}, // 第二次调用挂
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if !got.Partial || !errors.Is(got.Err, upstreamErr) {
		t.Fatalf("partial: got=%+v", got)
	}
	if len(got.Records) != 1 || got.LastCursor != "c1" {
		t.Fatalf("partial preserves data + cursor: got=%+v", got)
	}
	if !got.HasMore {
		t.Fatalf("partial should signal HasMore=true so caller can retry")
	}
	if got.Pages != 1 || got.Attempts != 2 || got.StopReason != StopFetchError {
		t.Fatalf("partial counters: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllInitialCursorResume(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{"resumed"}, NextCursor: ""},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InitialCursor: "saved-from-last-time"})
	if s.lastInput != "saved-from-last-time" {
		t.Fatalf("InitialCursor not propagated: got %q", s.lastInput)
	}
	if len(got.Records) != 1 || got.HasMore {
		t.Fatalf("resume: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	s := &stubFetcher{pages: []Page{{Records: []any{1}, NextCursor: "c1"}}}
	got := FetchAll(ctx, s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", got.Err)
	}
	if !got.Partial {
		t.Fatalf("partial on cancel: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllUnlimitedPageLimit(t *testing.T) {
	// 无限分页必须由哨兵值显式选择；用超过默认安全阀的页数证明它生效。
	pages := make([]Page, DefaultPageLimit+1)
	for i := range pages {
		next := ""
		if i < len(pages)-1 {
			next = "c" + string(rune('0'+i))
		}
		pages[i] = Page{Records: []any{i}, NextCursor: next}
	}
	s := &stubFetcher{pages: pages}
	got := FetchAll(context.Background(), s.Fetch, Options{
		PageLimit:      UnlimitedPageLimit,
		InterPageDelay: 1 * time.Millisecond,
	})
	if got.HasMore || !got.Complete || got.Pages != len(pages) || len(got.Records) != len(pages) {
		t.Fatalf("unlimited: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllZeroValueUsesDefaultPageLimit(t *testing.T) {
	pages := make([]Page, DefaultPageLimit+1)
	for i := range pages {
		pages[i] = Page{Records: []any{i}, NextCursor: fmt.Sprintf("cursor-%d", i+1)}
	}
	s := &stubFetcher{pages: pages}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: time.Nanosecond})
	if got.Pages != DefaultPageLimit || got.Attempts != DefaultPageLimit || len(got.Records) != DefaultPageLimit {
		t.Fatalf("zero-value page limit = %+v", got)
	}
	if !got.HasMore || got.Complete || got.Partial || got.StopReason != StopPageLimit || !errors.Is(got.Err, ErrPageLimitReached) {
		t.Fatalf("zero-value safety result = %+v", got)
	}
	if s.calls != DefaultPageLimit {
		t.Fatalf("zero-value fetch calls = %d, want %d", s.calls, DefaultPageLimit)
	}
}

func TestCrossPlatformCoverageFetchAllUnsupportedNegativeUsesDefaultPageLimit(t *testing.T) {
	calls := 0
	got := FetchAll(context.Background(), func(context.Context, string) (Page, error) {
		calls++
		return Page{NextCursor: fmt.Sprintf("cursor-%d", calls)}, nil
	}, Options{PageLimit: -2, InterPageDelay: time.Nanosecond})
	if calls != DefaultPageLimit || got.StopReason != StopPageLimit {
		t.Fatalf("unsupported negative page limit = %+v calls:%d", got, calls)
	}
}

func TestCrossPlatformCoverageFetchAllPreservesServerTotalCount(t *testing.T) {
	total := 123
	s := &stubFetcher{pages: []Page{{Records: []any{1, 2}, TotalCount: &total}}}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: time.Millisecond})
	if got.TotalCount == nil || *got.TotalCount != total {
		t.Fatalf("server total count lost: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllStopsOnCursorCycle(t *testing.T) {
	s := &stubFetcher{pages: []Page{
		{Records: []any{1}, NextCursor: "c1"},
		{Records: []any{2}, NextCursor: "c1"},
	}}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: time.Millisecond})
	if !got.Partial || got.Complete || got.StopReason != StopCursorCycle || !errors.Is(got.Err, ErrCursorCycle) {
		t.Fatalf("cursor cycle: got=%+v", got)
	}
	if got.Pages != 2 || got.Attempts != 2 || got.LastCursor != "c1" || len(got.Records) != 2 {
		t.Fatalf("cursor cycle progress: got=%+v", got)
	}
}

func TestCrossPlatformCoverageFetchAllCancellationDuringDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	got := FetchAll(ctx, func(context.Context, string) (Page, error) {
		calls++
		time.AfterFunc(10*time.Millisecond, cancel)
		return Page{Records: []any{"first"}, NextCursor: "next"}, nil
	}, Options{InterPageDelay: time.Second})
	if calls != 1 || got.StopReason != StopCanceled || !errors.Is(got.Err, context.Canceled) || got.LastCursor != "next" {
		t.Fatalf("delay cancellation = %+v calls:%d", got, calls)
	}
}

func TestCrossPlatformCoverageFetchAllStopsOnPriorCursorCycle(t *testing.T) {
	s := &stubFetcher{pages: []Page{
		{Records: []any{1}, NextCursor: "c1"},
		{Records: []any{2}, NextCursor: "c2"},
		{Records: []any{3}, NextCursor: "c1"},
	}}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: time.Millisecond})
	if got.StopReason != StopCursorCycle || got.Pages != 3 || got.LastCursor != "c1" || !errors.Is(got.Err, ErrCursorCycle) {
		t.Fatalf("prior cursor cycle = %+v", got)
	}
}
