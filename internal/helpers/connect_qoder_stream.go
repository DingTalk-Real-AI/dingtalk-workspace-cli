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
	"bufio"
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

	"github.com/google/uuid"
)

const qoderInitializeTimeout = 60 * time.Second

// qoderStreamForwarder keeps one qodercli stream-json subprocess alive for the
// lifetime of `dws dev connect`. DWS sends each DingTalk turn as a JSON user
// message with a per-conversation session_id, so Qoder keeps context without a
// fresh CLI startup on every chat message.
type qoderStreamForwarder struct {
	name     string
	bin      string
	env      []string
	timeout  time.Duration
	workDir  string
	model    string
	yolo     bool
	sessions *convSessions

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	done   chan error
	stderr *lockedStringBuffer
}

func newQoderStreamForwarder(name, bin string, env []string, timeout time.Duration, opts connectAgentOptions, sessions *convSessions) forwarder {
	return &qoderStreamForwarder{
		name:     name,
		bin:      bin,
		env:      env,
		timeout:  timeout,
		workDir:  opts.WorkDir,
		model:    opts.Model,
		yolo:     opts.Yolo,
		sessions: sessions,
	}
}

func (f *qoderStreamForwarder) canStream() bool { return true }

func (f *qoderStreamForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "stream-session-memory"
	}
	return fmt.Sprintf("qoder-stream:%s (%s)", f.bin, memo)
}

func (f *qoderStreamForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	return f.forwardStream(ctx, convID, text, nil)
}

func (f *qoderStreamForwarder) forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	ctx, cancel := applyTimeout(ctx, f.timeout)
	defer cancel()

	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.ensureLocked(ctx); err != nil {
		return "", err
	}
	sessionID := uuid.NewString()
	if f.sessions != nil {
		sessionID = f.sessions.id(convID)
	}
	msg := map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": text},
		"parent_tool_use_id": nil,
		"session_id":         sessionID,
	}
	if err := f.writeJSONLocked(msg); err != nil {
		f.closeLocked()
		return "", fmt.Errorf("写入 Qoder 请求失败：%w", err)
	}

	var acc strings.Builder
	for {
		line, err := f.readLineLocked(ctx)
		if err != nil {
			f.closeLocked()
			if f.sessions != nil {
				f.sessions.reset(convID)
			}
			return "", err
		}
		if f.handleControlRequestLocked(line) {
			continue
		}
		delta, final, done := parseQoderPersistentLine(line)
		if delta != "" {
			acc.WriteString(delta)
			if onDelta != nil {
				onDelta(brandReply(f.name, acc.String()))
			}
		}
		if !done {
			continue
		}
		final = strings.TrimSpace(final)
		if final == "" {
			final = strings.TrimSpace(acc.String())
		}
		if agentReplyIsError(final) {
			if f.sessions != nil {
				f.sessions.reset(convID)
			}
			return agentBackendErrorReply(final), nil
		}
		if final == "" {
			return "（本地 agent 无文本输出）", nil
		}
		return brandReply(f.name, final), nil
	}
}

func (f *qoderStreamForwarder) resetSession(convID string) {
	if f.sessions != nil {
		f.sessions.reset(convID)
	}
}

func (f *qoderStreamForwarder) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeLocked()
}

func (f *qoderStreamForwarder) commandArgs() []string {
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
	}
	if f.yolo {
		args = append(args, "--permission-mode", "bypass_permissions", "--dangerously-skip-permissions")
	} else {
		args = append(args,
			"--system-prompt", "",
			"--setting-sources", "",
			"--tools", "",
		)
	}
	if f.model != "" {
		args = append(args, "--model", f.model)
	}
	return args
}

func (f *qoderStreamForwarder) cwd() string {
	if f.workDir == "" {
		return connectWorkDir()
	}
	if abs, err := filepath.Abs(f.workDir); err == nil {
		return abs
	}
	return f.workDir
}

func (f *qoderStreamForwarder) ensureLocked(ctx context.Context) error {
	if f.cmd != nil {
		select {
		case err := <-f.done:
			msg := ""
			if f.stderr != nil {
				msg = strings.TrimSpace(f.stderr.String())
			}
			f.clearProcessLocked()
			if err != nil {
				if msg != "" {
					fmt.Fprintf(os.Stderr, "[connect][qoder] stream-json 进程已退出，尝试重启：%s\n", truncateRunes(msg, 300))
				} else {
					fmt.Fprintf(os.Stderr, "[connect][qoder] stream-json 进程已退出，尝试重启：%v\n", err)
				}
			} else {
				fmt.Fprintln(os.Stderr, "[connect][qoder] stream-json 进程已退出，尝试重启")
			}
		default:
			return nil
		}
	}

	cmd := exec.Command(f.bin, f.commandArgs()...)
	cmd.Dir = f.cwd()
	cmd.Env = append(os.Environ(), f.env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &lockedStringBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Qoder stream-json 失败：%w", err)
	}

	f.cmd = cmd
	f.stdin = stdin
	f.lines = make(chan string, 1024)
	f.done = make(chan error, 1)
	f.stderr = stderr

	go scanQoderLines(stdout, f.lines)
	go func() {
		f.done <- cmd.Wait()
	}()
	return f.initializeLocked(ctx)
}

func (f *qoderStreamForwarder) initializeLocked(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, qoderInitializeTimeout)
	defer cancel()
	requestID := "dws_init_" + uuid.NewString()
	request := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request": map[string]any{
			"type":                "initialize",
			"subtype":             "initialize",
			"modelPolicyProvider": false,
			"initializeTimeoutMs": int(qoderInitializeTimeout / time.Millisecond),
		},
	}
	if err := f.writeJSONLocked(request); err != nil {
		f.closeLocked()
		return fmt.Errorf("初始化 Qoder stream-json 失败：%w", err)
	}
	for {
		line, err := f.readLineLocked(ctx)
		if err != nil {
			f.closeLocked()
			return err
		}
		if f.handleControlRequestLocked(line) {
			continue
		}
		matched, err := qoderControlResponse(line, requestID)
		if err != nil {
			f.closeLocked()
			return err
		}
		if matched {
			return nil
		}
	}
}

func (f *qoderStreamForwarder) writeJSONLocked(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = f.stdin.Write(append(data, '\n'))
	return err
}

func (f *qoderStreamForwarder) readLineLocked(ctx context.Context) (string, error) {
	for {
		select {
		case line, ok := <-f.lines:
			if !ok {
				msg := "Qoder stream-json stdout 已关闭"
				if f.stderr != nil {
					if s := strings.TrimSpace(f.stderr.String()); s != "" {
						msg = s
					}
				}
				return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "{") {
				continue
			}
			return line, nil
		case err := <-f.done:
			msg := ""
			if f.stderr != nil {
				msg = strings.TrimSpace(f.stderr.String())
			}
			f.clearProcessLocked()
			if err != nil {
				if msg == "" {
					msg = err.Error()
				}
				return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
			}
			return "", fmt.Errorf("本地 %s agent 已退出", f.name)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func (f *qoderStreamForwarder) handleControlRequestLocked(line string) bool {
	var ev struct {
		Type      string         `json:"type"`
		RequestID string         `json:"request_id"`
		Request   map[string]any `json:"request"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "control_request" || ev.RequestID == "" {
		return false
	}
	subtype, _ := ev.Request["subtype"].(string)
	if subtype == "" {
		subtype, _ = ev.Request["type"].(string)
	}
	fmt.Fprintf(os.Stderr, "[connect][qoder] 自动拒绝/跳过未桥接的 control_request subtype=%q requestId=%s\n", subtype, ev.RequestID)
	response := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "error",
			"request_id": ev.RequestID,
			"error":      "DWS qoder connector does not provide host callbacks",
			"code":       "unsupported_control_request",
		},
	}
	_ = f.writeJSONLocked(response)
	return true
}

func (f *qoderStreamForwarder) closeLocked() error {
	var killErr error
	if f.stdin != nil {
		_ = f.stdin.Close()
	}
	if f.cmd != nil && f.cmd.Process != nil {
		killErr = f.cmd.Process.Kill()
	}
	if f.done != nil {
		select {
		case <-f.done:
		case <-time.After(2 * time.Second):
		}
	}
	f.clearProcessLocked()
	return killErr
}

func (f *qoderStreamForwarder) clearProcessLocked() {
	f.cmd = nil
	f.stdin = nil
	f.lines = nil
	f.done = nil
	f.stderr = nil
}

func scanQoderLines(stdout io.Reader, lines chan<- string) {
	defer close(lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
}

func qoderControlResponse(line, requestID string) (bool, error) {
	var ev struct {
		Type     string `json:"type"`
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"response"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "control_response" {
		return false, nil
	}
	if ev.Response.RequestID != requestID {
		return false, nil
	}
	if ev.Response.Subtype == "error" {
		if ev.Response.Error == "" {
			ev.Response.Error = "unknown initialize error"
		}
		return true, fmt.Errorf("初始化 Qoder stream-json 失败：%s", truncateRunes(ev.Response.Error, 300))
	}
	return true, nil
}

func parseQoderPersistentLine(line string) (delta, final string, done bool) {
	var ev struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Error   string `json:"error"`
		Result  string `json:"result"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return "", "", false
	}
	text := func() string {
		var b strings.Builder
		for _, c := range ev.Message.Content {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
		return strings.TrimSpace(b.String())
	}
	switch {
	case ev.Type == "assistant":
		if t := text(); t != "" {
			return t + "\n\n", "", false
		}
	case ev.Type == "result":
		if ev.Subtype != "" && ev.Subtype != "success" {
			if ev.Error != "" {
				return "", ev.Error, true
			}
		}
		if t := text(); t != "" {
			return "", t, true
		}
		return "", strings.TrimSpace(ev.Result), true
	case ev.Type == "error":
		if ev.Error != "" {
			return "", ev.Error, true
		}
	}
	return "", "", false
}

type lockedStringBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *lockedStringBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedStringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
