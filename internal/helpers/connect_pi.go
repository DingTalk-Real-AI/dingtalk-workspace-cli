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

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/google/uuid"
)

// piForwarder 每轮运行官方 JSON 事件流，以进程退出确认重试和工具调用全部完成。
// 会话文件独立于子进程存活，可跨连接重启续聊；不使用全局 --continue。
type piForwarder struct {
	bin        string
	env        []string
	opts       connectAgentOptions
	timeout    time.Duration
	sessions   *convSessions
	sessionDir string
	turnMu     sync.Mutex
	mu         sync.Mutex
	cancel     context.CancelFunc
	closed     bool
}

func newPiForwarder(bin string, env []string, timeout time.Duration, opts connectAgentOptions, scopeID string) forwarder {
	f := &piForwarder{bin: bin, env: env, opts: opts, timeout: timeout}
	if opts.Memory && scopeID != "" {
		f.sessionDir = filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(scopeID), "pi")
		store := filepath.Join(f.sessionDir, "sessions.json")
		f.sessions = newConvSessions(store)
	}
	return f
}

func (f *piForwarder) label() string   { return "pi-json:" + f.bin }
func (f *piForwarder) canStream() bool { return true }
func (f *piForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	return f.forwardStream(ctx, convID, text, nil)
}

func (f *piForwarder) commandArgs(convID string) ([]string, string, error) {
	// 扩展可以注册自动执行工具或交互 UI；通道只使用官方内置工具。
	args := []string{"--print", "--mode", "json", "--no-extensions"}
	if !f.opts.Yolo {
		// Pi 没有可桥接的逐工具批准协议，ask 模式禁用工具，不能静默放行。
		args = append(args, "--no-tools")
	}
	if f.opts.Model != "" {
		args = append(args, "--model", f.opts.Model)
	}
	if f.sessions == nil {
		return append(args, "--no-session"), "", nil
	}
	id := f.sessions.id(convID)
	if _, err := uuid.Parse(id); err != nil {
		return nil, "", fmt.Errorf("Pi 会话映射中的 ID 无效，请使用 /new 重建会话")
	}
	if err := os.MkdirAll(f.sessionDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("创建 Pi 会话目录失败")
	}
	path := filepath.Join(f.sessionDir, id+".jsonl")
	return append(args, "--session", path), path, nil
}

func (f *piForwarder) forwardWithAttachments(ctx context.Context, convID, text string, attachments []connectMediaAttachment) (string, error) {
	return f.forwardStreamWithAttachments(ctx, convID, text, attachments, nil)
}

func (f *piForwarder) forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	return f.forwardStreamWithAttachments(ctx, convID, text, nil, onDelta)
}

func (f *piForwarder) forwardStreamWithAttachments(parent context.Context, convID, text string, attachments []connectMediaAttachment, onDelta func(string)) (string, error) {
	// 默认 timeout=0 时也必须能由 close 取消本轮，不能依赖调用方取消。
	turnCtx, cancelTurn := context.WithCancel(parent)
	defer cancelTurn()
	ctx, cancel := applyTimeout(turnCtx, f.timeout)
	defer cancel()
	f.turnMu.Lock()
	defer f.turnMu.Unlock()
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return "", fmt.Errorf("Pi 连接已关闭")
	}
	f.cancel = cancelTurn
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.cancel = nil; f.mu.Unlock() }()
	args, sessionFile, err := f.commandArgs(convID)
	if err != nil {
		return "", err
	}
	if sessionFile != "" {
		defer func() { _ = os.Chmod(sessionFile, 0o600) }()
	}
	for _, attachment := range attachments {
		if attachment.MediaType == "audio" || attachment.MediaType == "video" {
			return "", fmt.Errorf("Pi 当前通道支持图片和文本文件，不支持直接传入音频或视频")
		}
		path, err := filepath.Abs(attachment.LocalPath)
		if err != nil || strings.TrimSpace(attachment.LocalPath) == "" {
			return "", fmt.Errorf("Pi 附件路径无效")
		}
		args = append(args, "@"+path)
	}
	cmd := exec.CommandContext(ctx, f.bin, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = f.opts.WorkDir
	if cmd.Dir == "" {
		cmd.Dir = connectWorkDir()
	}
	cmd.Env = append(os.Environ(), f.env...)
	// 通过 stdin 传问题，避免 @file、斜杠和以 - 开头的文本被当成 CLI 参数。
	cmd.Stdin = strings.NewReader(text)
	stream := &piJSONOutput{onDelta: onDelta}
	cmd.Stdout = stream
	// 不将 stderr / 提供商错误正文转发到群或数字员工日志。
	cmd.Stderr = io.Discard
	completed := false
	defer func() {
		// 失败或取消可能留下不完整会话；解除映射，下一轮使用新的 UUID。
		if !completed && f.sessions != nil {
			f.sessions.reset(convID)
		}
	}()
	err = cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("Pi 调用失败，请检查 pi 登录、模型配置和工作目录")
	}
	if err := stream.finish(); err != nil {
		return "", err
	}
	completed = true
	return strings.TrimSpace(stream.final), nil
}

func (f *piForwarder) resetSession(convID string) {
	f.turnMu.Lock()
	defer f.turnMu.Unlock()
	if f.sessions != nil {
		f.sessions.reset(convID)
	}
}

func (f *piForwarder) close() error {
	f.mu.Lock()
	f.closed = true
	if f.cancel != nil {
		f.cancel()
	}
	f.mu.Unlock()
	f.turnMu.Lock()
	f.turnMu.Unlock()
	return nil
}

type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stopReason"`
}

// piJSONOutput 按 LF 分帧，忽略思考/工具输出，仅投影助手文本。
// 限制单帧大小；不保存完整事件流，防止工具结果无限堆积。
type piJSONOutput struct {
	pending   []byte
	partial   strings.Builder
	final     string
	failed    bool
	completed bool
	onDelta   func(string)
}

func (s *piJSONOutput) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			i = len(p)
		}
		if len(s.pending)+i > 16*1024*1024 {
			return 0, fmt.Errorf("Pi JSON 事件超过大小限制")
		}
		s.pending = append(s.pending, p[:i]...)
		p = p[i:]
		if len(p) == 0 {
			break
		}
		p = p[1:]
		if err := s.record(s.pending); err != nil {
			return 0, err
		}
		s.pending = s.pending[:0]
	}
	return n, nil
}

func (s *piJSONOutput) record(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var ev struct {
		Type                  string    `json:"type"`
		Message               piMessage `json:"message"`
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
		Aborted bool `json:"aborted"`
	}
	if json.Unmarshal(raw, &ev) != nil {
		return fmt.Errorf("Pi 返回了无效的 JSON 事件")
	}
	switch ev.Type {
	case "agent_start":
		s.completed = false
		s.final = ""
		s.failed = false
	case "message_start":
		if ev.Message.Role == "assistant" {
			s.partial.Reset()
		}
	case "message_update":
		if ev.AssistantMessageEvent.Type == "text_delta" {
			s.partial.WriteString(ev.AssistantMessageEvent.Delta)
			if s.onDelta != nil {
				s.onDelta(s.partial.String())
			}
		}
	case "message_end":
		if ev.Message.Role != "assistant" {
			break
		}
		s.failed = ev.Message.StopReason == "error" || ev.Message.StopReason == "aborted"
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(ev.Message.Content, &parts) != nil {
			return fmt.Errorf("Pi 助手消息结构无效")
		}
		var text strings.Builder
		for _, part := range parts {
			if part.Type == "text" {
				text.WriteString(part.Text)
			}
		}
		s.final = text.String()
	case "agent_end":
		s.completed = true
	case "agent_settled":
		if ev.Aborted {
			s.failed = true
		}
	}
	return nil
}

func (s *piJSONOutput) finish() error {
	if len(s.pending) > 0 {
		if err := s.record(s.pending); err != nil {
			return err
		}
	}
	if s.failed {
		return fmt.Errorf("Pi 模型调用失败或已中止，请检查模型供应商配置")
	}
	if !s.completed || strings.TrimSpace(s.final) == "" {
		return fmt.Errorf("Pi 未返回完整的助手文本")
	}
	return nil
}
