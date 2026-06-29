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
//  2. 支持 --page-limit 安全阀（默认 50 页 ≈ 5000 条，0 = 无限制）
//  3. 支持页间退避，避免触发上游限流（默认 200ms）
//  4. 任何一页失败时优雅降级：已取数据保留 + 返回最后一页 cursor，不抛错
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
}

// Fetcher 是调用方需要实现的"取单页"函数。
//
//   - ctx：上游传入的 context，本包负责检查 Done
//   - cursor：本次请求的 cursor（首次调用传 ""）
//
// 返回 Page 或 error。返回 error 时本包会停止翻页并把已累计数据作为 partial
// 返回给调用方，不直接中断流程。
type Fetcher func(ctx context.Context, cursor string) (Page, error)

// Options 控制 FetchAll 的行为。零值即为合理默认。
type Options struct {
	// PageLimit 最大翻页次数。
	//   - 0  表示无限制（取到 NextCursor 为空为止）
	//   - >0 达到该次数后立即停止，返回 LastCursor 让调用方手动续拉
	// 默认（DefaultPageLimit）= 50 页。
	PageLimit int

	// InterPageDelay 是页间退避时长。默认 200ms。
	// 仅在抓到非首页时生效（首次调用不 sleep）。
	InterPageDelay time.Duration

	// InitialCursor 是起始 cursor。常用于"接续上次断点"场景。
	// 默认空串表示从头开始。
	InitialCursor string
}

const (
	// DefaultPageLimit 是 PageLimit 字段的默认值。
	DefaultPageLimit = 50

	// DefaultInterPageDelay 是 InterPageDelay 字段的默认值。
	DefaultInterPageDelay = 200 * time.Millisecond
)

// Result 是 FetchAll 的最终返回。
//
//   - Records 已累计的所有数据
//   - HasMore 触发 PageLimit 或被 fetcher 中断时为 true，可凭 LastCursor 续拉
//   - LastCursor 最后成功取得的下一页 cursor（仅 HasMore=true 时有意义）
//   - Pages 本次实际翻了多少页
//   - Partial 中途遇到错误时为 true，Records 包含错误发生前的数据
//   - Err 中途错误（调用方应根据业务决定是否报警；本包不主动失败）
type Result struct {
	Records    []any
	HasMore    bool
	LastCursor string
	Pages      int
	Partial    bool
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
//     d) fetcher 返回 error → Partial=true, Err 保留，HasMore 依赖最后一次成功的 cursor
//
// 不会主动抛 error；上游可基于 Result.Err / Result.Partial 决定如何向用户报告。
func FetchAll(ctx context.Context, fetcher Fetcher, opts Options) Result {
	pageLimit := opts.PageLimit
	if pageLimit == 0 {
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
	)

	for {
		// ctx 已取消：保留断点 cursor 优雅退出
		if err := ctx.Err(); err != nil {
			return Result{
				Records:    allRecords,
				HasMore:    cursor != "",
				LastCursor: cursor,
				Pages:      pages,
				Partial:    true,
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
					Partial:    true,
					Err:        ctx.Err(),
				}
			case <-time.After(delay):
			}
		}

		page, err := fetcher(ctx, cursor)
		pages++

		if err != nil {
			// 优雅降级：保留已累计数据 + 记录错误
			// LastCursor 用本次请求时的 cursor（这页未成功，需用同 cursor 重试）
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: cursor,
				Pages:      pages,
				Partial:    true,
				Err:        err,
			}
		}

		allRecords = append(allRecords, page.Records...)
		cursor = page.NextCursor

		// 终止条件：服务端已无更多数据
		if cursor == "" {
			return Result{
				Records:    allRecords,
				HasMore:    false,
				LastCursor: "",
				Pages:      pages,
			}
		}

		// 终止条件：达到安全阀
		if pages >= pageLimit {
			return Result{
				Records:    allRecords,
				HasMore:    true,
				LastCursor: cursor,
				Pages:      pages,
				Partial:    false,
				Err:        ErrPageLimitReached,
			}
		}
	}
}

// ErrPageLimitReached 表示翻页因 PageLimit 安全阀被截断（不算错误，仅作信号）。
// 上游可通过 errors.Is(result.Err, ErrPageLimitReached) 判定，决定提示用户
// "数据被截断，请用 --cursor <LastCursor> 续拉"。
var ErrPageLimitReached = errors.New("paging: page-limit reached, more data available via LastCursor")
