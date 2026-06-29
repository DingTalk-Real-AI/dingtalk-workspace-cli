// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package paging

import (
	"context"
	"errors"
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

func TestFetchAll_SinglePage(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{"a", "b", "c"}, NextCursor: ""},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if got.HasMore || got.Pages != 1 || len(got.Records) != 3 {
		t.Fatalf("single page: got=%+v", got)
	}
}

func TestFetchAll_MultiPage(t *testing.T) {
	s := &stubFetcher{
		pages: []Page{
			{Records: []any{1, 2}, NextCursor: "c1"},
			{Records: []any{3, 4}, NextCursor: "c2"},
			{Records: []any{5}, NextCursor: ""},
		},
	}
	got := FetchAll(context.Background(), s.Fetch, Options{InterPageDelay: 1 * time.Millisecond})
	if got.HasMore || got.Pages != 3 || len(got.Records) != 5 {
		t.Fatalf("multi page: got=%+v", got)
	}
}

func TestFetchAll_PageLimit(t *testing.T) {
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
}

func TestFetchAll_MidErrorPartial(t *testing.T) {
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
}

func TestFetchAll_InitialCursorResume(t *testing.T) {
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

func TestFetchAll_ContextCancel(t *testing.T) {
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

func TestFetchAll_UnlimitedPageLimit(t *testing.T) {
	// PageLimit 显式置 -1 时（约定为"不限"），翻完为止
	// （当前实现 PageLimit==0 用默认值 50；要真正无限制需另设特殊值，
	// 这里测试默认 50 上限内能完成全量）
	pages := make([]Page, 10)
	for i := range pages {
		next := ""
		if i < len(pages)-1 {
			next = "c" + string(rune('0'+i))
		}
		pages[i] = Page{Records: []any{i}, NextCursor: next}
	}
	s := &stubFetcher{pages: pages}
	got := FetchAll(context.Background(), s.Fetch, Options{
		PageLimit:      100, // 高于实际页数
		InterPageDelay: 1 * time.Millisecond,
	})
	if got.HasMore || got.Pages != 10 || len(got.Records) != 10 {
		t.Fatalf("unlimited: got=%+v", got)
	}
}
