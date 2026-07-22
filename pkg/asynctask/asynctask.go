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

// Package asynctask 封装"提交 → 渐进式退避轮询 → (可选) 下载"模式，复用给
// doc export / sheet export / aitable export 三个产品。
//
// 设计要点：
//
//  1. 调用方只需提供"提交"和"查询状态"两个回调，循环逻辑由本包处理
//  2. 渐进式退避（1s/2s/5s/10s/15s/30s/30s...，可自定义）
//  3. 默认 5 分钟超时，超时返回 jobId 让用户用 `... get --job-id <ID>` 续等
//  4. 状态机统一：PROCESSING/PENDING → SUCCESS → 拿 downloadUrl
//     ↓
//     FAILED → 返回 error
//  5. 可选 PUT 下载：传入 OutputPath 时本包负责 HTTP GET 落盘
package asynctask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Status 表示异步任务的状态。
type Status string

const (
	StatusProcessing Status = "PROCESSING"
	StatusPending    Status = "PENDING"
	StatusSuccess    Status = "SUCCESS"
	StatusFailed     Status = "FAILED"
	StatusTimeout    Status = "TIMEOUT" // 本地超时（非服务端状态）
)

// SubmitFunc 是"提交任务"回调，返回 jobId 或 error。
type SubmitFunc func(ctx context.Context) (jobID string, err error)

// QueryResult 是 QueryFunc 的返回。
//
//   - Status 状态枚举
//   - DownloadURL 服务端返回的下载链接（仅 SUCCESS 时有效）
//   - Message 失败原因 / 进度描述
//   - Raw 原始响应（用于调用方在最终输出里附带）
type QueryResult struct {
	Status      Status
	DownloadURL string
	Message     string
	Raw         map[string]any
}

// QueryFunc 是"查询任务状态"回调。
type QueryFunc func(ctx context.Context, jobID string) (QueryResult, error)

// Options 控制 WaitWithBackoff 的行为。
type Options struct {
	// Backoff 是页间退避序列（毫秒）。零值或空使用 DefaultBackoff。
	// 序列耗尽后保持最后一个间隔继续轮询。
	Backoff []time.Duration

	// Timeout 是整体轮询上限。零值使用 DefaultTimeout（5 分钟）。
	Timeout time.Duration

	// ProgressFn 是每轮查询后的进度回调（可选）。用于打印 "[2/30] processing..."
	// 之类的 stderr 提示。
	ProgressFn func(attempt int, status Status, elapsed time.Duration)
}

var (
	// DefaultBackoff 是渐进式退避序列：1s/2s/5s/10s/15s/30s，之后稳定 30s。
	// 与悟空 wukong/products/aitable.go / doc.go 对齐。
	DefaultBackoff = []time.Duration{
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		15 * time.Second,
		30 * time.Second,
	}

	// DefaultTimeout 是默认超时上限。
	DefaultTimeout = 5 * time.Minute
)

// FinalResult 是 WaitWithBackoff 的最终返回。
type FinalResult struct {
	JobID       string
	Status      Status
	DownloadURL string
	Message     string
	Raw         map[string]any
	Attempts    int
	Elapsed     time.Duration
}

// Submit 提交任务并轮询直到完成 / 失败 / 超时。
//
// 行为：
//  1. 调用 submitFn 拿到 jobID
//  2. 进入轮询循环：按 backoff 序列等待 → 调 queryFn → 检查状态
//  3. SUCCESS → 返回 FinalResult.Status = SUCCESS + downloadURL
//  4. FAILED  → 返回 FinalResult.Status = FAILED + Message（不抛 error，让调用方决定）
//  5. PROCESSING/PENDING → 继续轮询
//  6. 超过 Timeout → 返回 FinalResult.Status = TIMEOUT + JobID（调用方可提示用户用 ... get --job-id <ID>）
//  7. ctx 取消 → 返回带 ctx.Err() 的 error
func Submit(ctx context.Context, submitFn SubmitFunc, queryFn QueryFunc, opts Options) (FinalResult, error) {
	jobID, err := submitFn(ctx)
	if err != nil {
		return FinalResult{}, fmt.Errorf("submit failed: %w", err)
	}
	if jobID == "" {
		return FinalResult{}, errors.New("submit returned empty jobID")
	}
	return Resume(ctx, jobID, queryFn, opts)
}

// Resume 接续轮询已知 jobID 的任务（提交步骤已完成）。
// 用于 `... export get --job-id <ID>` 兜底命令。
func Resume(ctx context.Context, jobID string, queryFn QueryFunc, opts Options) (FinalResult, error) {
	backoff := opts.Backoff
	if len(backoff) == 0 {
		backoff = DefaultBackoff
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	deadline := time.Now().Add(timeout)
	start := time.Now()
	attempt := 0

	var lastErr error

	for {
		attempt++

		// 超时检查前置：保证 Timeout < backoff[0] 也能正常返回 TIMEOUT，不会卡在 sleep
		if time.Now().After(deadline) {
			msg := fmt.Sprintf("task did not complete within %v; resume with the original jobID %s", timeout, jobID)
			if lastErr != nil {
				msg += fmt.Sprintf(" (last query error: %v)", lastErr)
			}
			return FinalResult{
				JobID:    jobID,
				Status:   StatusTimeout,
				Message:  msg,
				Attempts: attempt - 1,
				Elapsed:  time.Since(start),
			}, nil
		}

		// 等待间隔（首次也等，避免提交后立即查询，给服务端处理时间）
		// sleep 长度上限取 deadline 剩余时间，避免 sleep 后再发现已超时
		interval := backoff[len(backoff)-1] // 序列耗尽后用最后一个
		if attempt-1 < len(backoff) {
			interval = backoff[attempt-1]
		}
		if remain := time.Until(deadline); remain > 0 && interval > remain {
			interval = remain
		}
		select {
		case <-ctx.Done():
			return FinalResult{JobID: jobID, Attempts: attempt, Elapsed: time.Since(start)}, ctx.Err()
		case <-time.After(interval):
		}

		qr, err := queryFn(ctx, jobID)
		if err != nil {
			// 单次查询失败不立即放弃，下一轮重试；保存 error 给最后兜底（superseded
			// by 下次成功；timeout 时会把它附到 Message 帮助排查）
			lastErr = err
			if opts.ProgressFn != nil {
				opts.ProgressFn(attempt, "", time.Since(start))
			}
			continue
		}
		lastErr = nil

		if opts.ProgressFn != nil {
			opts.ProgressFn(attempt, qr.Status, time.Since(start))
		}

		switch qr.Status {
		case StatusSuccess:
			return FinalResult{
				JobID:       jobID,
				Status:      StatusSuccess,
				DownloadURL: qr.DownloadURL,
				Message:     qr.Message,
				Raw:         qr.Raw,
				Attempts:    attempt,
				Elapsed:     time.Since(start),
			}, nil
		case StatusFailed:
			return FinalResult{
				JobID:    jobID,
				Status:   StatusFailed,
				Message:  qr.Message,
				Raw:      qr.Raw,
				Attempts: attempt,
				Elapsed:  time.Since(start),
			}, nil
		case StatusProcessing, StatusPending, "":
			// 继续轮询（空字符串容错：部分上游可能 status 缺失）
			continue
		default:
			// 未知状态：当作 processing 继续等
			continue
		}
	}
}

// Download 把 downloadURL 的内容 GET 到本地 outputPath。
// 单独抽出来便于调用方在不调 Submit 时也能复用（例如 sheet export 已有 jobId 的场景）。
func Download(ctx context.Context, downloadURL, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download HTTP %d: %s", resp.StatusCode, string(body))
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
