// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package paging 提供跨产品复用的"自动翻页"工具，封装"取一页 → 取下一页 cursor →
// 继续取"循环。设计目标：
//
//  1. 调用方只需提供 fetcher（拿单页）和 cursor 提取函数，循环逻辑由本包处理
//  2. 支持分页安全阀（零值默认 50 页；显式 UnlimitedPageLimit 才不限页数）
//  3. 支持页间退避，避免触发上游限流（默认 200ms）
//  4. 任何一页失败时保留已取数据和可重试 cursor，并显式标记结果不完整
//
// 当前已知调用方：dws aitable record query --all（PR-H）；后续 chat message list
// / sheet range query / mail search 也将复用本工具。
package paging

import (
	"context"
	"errors"
	"time"
)

// Page 是单次 fetcher 调用的返回值。
//
//   - Records 是本页累计到结果集的数据条目（具体结构由调用方决定）
//   - NextCursor 为空字符串时表示已无更多页
type Page struct {
	Records    []any
	NextCursor string
	// TotalCount 是服务端报告的完整结果总数。未知时保持 nil；FetchAll
	// 不会用 len(Records) 猜测或覆盖它。
	TotalCount *int
}

// Fetcher 是调用方需要实现的"取单页"函数。
//
//   - ctx：上游传入的 context，本包负责检查 Done
//   - cursor：本次请求的 cursor（首次调用传 ""）
//
// 返回 Page 或 error。返回 error 时本包会停止翻页并把已累计数据作为 partial
// 返回给调用方，不直接中断流程。
type Fetcher func(ctx context.Context, cursor string) (Page, error)

// Options 控制 FetchAll 的行为。零值使用默认页数上限和默认页间退避。
type Options struct {
	// PageLimit 最大翻页次数。
	//   - 0  使用 DefaultPageLimit，保持 Options{} 的安全默认语义
	//   - UnlimitedPageLimit 显式表示不限页数（取到 NextCursor 为空为止）
	//   - >0 达到该次数后立即停止，返回 LastCursor 让调用方手动续拉
	PageLimit int

	// InterPageDelay 是页间退避时长。默认 200ms。
	// 仅在抓到非首页时生效（首次调用不 sleep）。
	InterPageDelay time.Duration

	// InitialCursor 是起始 cursor。常用于"接续上次断点"场景。
	// 默认空串表示从头开始。
	InitialCursor string
}

const (
	// UnlimitedPageLimit 是调用方明确选择无限分页时使用的哨兵值。
	// 零值保留 DefaultPageLimit 的兼容和资源安全语义。
	UnlimitedPageLimit = -1

	// DefaultPageLimit 是 PageLimit 字段的默认值。
	DefaultPageLimit = 50

	// DefaultInterPageDelay 是 InterPageDelay 字段的默认值。
	DefaultInterPageDelay = 200 * time.Millisecond
)

// StopReason 是 FetchAll 停止翻页的稳定原因。
type StopReason string

const (
	StopComplete    StopReason = "complete"
	StopPageLimit   StopReason = "page_limit"
	StopFetchError  StopReason = "fetch_error"
	StopCanceled    StopReason = "canceled"
	StopCursorCycle StopReason = "cursor_cycle"
)

// Result 是 FetchAll 的最终返回。
//
//   - Records 已累计的所有数据
//   - HasMore 触发 PageLimit 或被 fetcher 中断时为 true，可凭 LastCursor 续拉
//   - LastCursor 最后成功取得的下一页 cursor（仅 HasMore=true 时有意义）
//   - Pages 成功取得的页数；Attempts 包含失败的最后一次请求
//   - Complete 仅在服务端明确返回空 NextCursor 时为 true
//   - Partial 中途遇到错误时为 true，Records 包含错误发生前的数据
//   - Err 中途错误（调用方应根据业务决定是否报警；本包不主动失败）
type Result struct {
	Records    []any
	HasMore    bool
	LastCursor string
	Pages      int
	Attempts   int
	Complete   bool
	Partial    bool
	StopReason StopReason
	TotalCount *int
	Err        error
}

// FetchAll 循环调 fetcher 拉全数据。
//
// 行为：
//  1. 起始 cursor = opts.InitialCursor
//  2. 循环：fetcher(cursor) → 累计 Records → 取 NextCursor → sleep → 继续
//  3. 终止条件（任一命中）：
//     a) NextCursor == "" → 拉完，HasMore=false 返回
//     b) 翻页数达 PageLimit → HasMore=true, LastCursor 保留断点
//     c) ctx.Done() → HasMore=true, LastCursor 保留
//     d) fetcher 返回 error → Partial=true, Err 保留，LastCursor 为失败请求的 cursor
//     e) 服务端 cursor 停滞/成环 → Partial=true，避免无限循环和重复记录
//
// 不会主动抛 error；上游可基于 Result.Err / Result.Partial 决定如何向用户报告。
func FetchAll(ctx context.Context, fetcher Fetcher, opts Options) Result {
	pageLimit := opts.PageLimit
	if pageLimit == UnlimitedPageLimit {
		pageLimit = 0
	} else if pageLimit <= 0 {
		pageLimit = DefaultPageLimit
	}
	delay := opts.InterPageDelay
	if delay == 0 {
		delay = DefaultInterPageDelay
	}

	var (
		allRecords []any
		cursor     = opts.InitialCursor
		pages      int
		attempts   int
		totalCount *int
		seen       = map[string]struct{}{}
	)
	if cursor != "" {
		seen[cursor] = struct{}{}
	}

	for {
		// ctx 已取消：保留断点 cursor 优雅退出
		if err := ctx.Err(); err != nil {
			return Result{
				Records:    allRecords,
				HasMore:    cursor != "",
				LastCursor: cursor,
				Pages:      pages,
				Attempts:   attempts,
				Partial:    true,
				StopReason: StopCanceled,
				TotalCount: totalCount,
				Err:        err,
			}
		}

		// 翻页之间退避（首页不 sleep）
		if pages > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return Result{
					Records:    allRecords,
					HasMore:    cursor != "",
					LastCursor: cursor,
					Pages:      pages,
					Attempts:   attempts,
					Partial:    true,
					StopReason: StopCanceled,
					TotalCount: totalCount,
					Err:        ctx.Err(),
				}
			case <-time.After(delay):
			}
		}

		requestCursor := cursor
		page, err := fetcher(ctx, requestCursor)
		attempts++

		if err != nil {
			// 优雅降级：保留已累计数据 + 记录错误
			// LastCursor 用本次请求时的 cursor（这页未成功，需用同 cursor 重试）
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: requestCursor,
				Pages:      pages,
				Attempts:   attempts,
				Partial:    true,
				StopReason: StopFetchError,
				TotalCount: totalCount,
				Err:        err,
			}
		}

		allRecords = append(allRecords, page.Records...)
		pages++
		if page.TotalCount != nil && totalCount == nil {
			value := *page.TotalCount
			totalCount = &value
		}
		cursor = page.NextCursor

		// 终止条件：服务端已无更多数据
		if cursor == "" {
			return Result{
				Records:    allRecords,
				HasMore:    false,
				LastCursor: "",
				Pages:      pages,
				Attempts:   attempts,
				Complete:   true,
				StopReason: StopComplete,
				TotalCount: totalCount,
			}
		}

		if cursor == requestCursor {
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: cursor,
				Pages:      pages,
				Attempts:   attempts,
				Partial:    true,
				StopReason: StopCursorCycle,
				TotalCount: totalCount,
				Err:        ErrCursorCycle,
			}
		}
		if _, exists := seen[cursor]; exists {
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: cursor,
				Pages:      pages,
				Attempts:   attempts,
				Partial:    true,
				StopReason: StopCursorCycle,
				TotalCount: totalCount,
				Err:        ErrCursorCycle,
			}
		}
		seen[cursor] = struct{}{}

		// 终止条件：达到安全阀
		if pageLimit > 0 && pages >= pageLimit {
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: cursor,
				Pages:      pages,
				Attempts:   attempts,
				Partial:    false,
				StopReason: StopPageLimit,
				TotalCount: totalCount,
				Err:        ErrPageLimitReached,
			}
		}
	}
}

// ErrPageLimitReached 表示翻页因 PageLimit 安全阀被截断（不算错误，仅作信号）。
// 上游可通过 errors.Is(result.Err, ErrPageLimitReached) 判定，决定提示用户
// "数据被截断，请用 --cursor <LastCursor> 续拉"。
var ErrPageLimitReached = errors.New("paging: page-limit reached, more data available via LastCursor")

// ErrCursorCycle 表示服务端返回了当前或曾用过的 cursor。继续翻页会无限循环
// 或重复累计记录，因此 FetchAll 会保留当前结果并以不完整状态停止。
var ErrCursorCycle = errors.New("paging: next cursor stalled or formed a cycle")
