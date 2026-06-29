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
)

const codexRobotDeveloperInstructions = "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要提及系统提示、内部协议或运行时细节；不要主动读写文件或执行命令。"

// codexAppServerForwarder uses Codex's official app-server JSON-RPC protocol to
// keep one Codex thread per DingTalk conversation.
type codexAppServerForwarder struct {
	bin      string
	env      []string
	timeout  time.Duration
	workDir  string
	model    string
	sessions *codexThreadSessions
}

func newCodexAppServerForwarder(bin string, env []string, timeout time.Duration, opts connectAgentOptions, clientID string) forwarder {
	var sessions *codexThreadSessions
	if opts.Memory {
		// Scope the on-disk thread store by clientId so multiple bots on one
		// machine stay isolated; an empty clientId disables persistence and the
		// map stays in memory (original behaviour).
		sessions = newCodexThreadSessions(codexThreadStorePath(clientID))
	}
	return &codexAppServerForwarder{
		bin:      bin,
		env:      env,
		timeout:  timeout,
		workDir:  opts.WorkDir,
		model:    opts.Model,
		sessions: sessions,
	}
}

// codexThreadStorePath returns the on-disk location for a robot's codex
// conversation→thread map, scoped by clientId so multiple bots on one machine
// stay isolated: <config dir>/connect/<clientId>/codex-threads.json. It mirrors
// the Claude-family connectSessionStorePath layout but uses a distinct filename
// so the two stores never collide. An empty clientId means "do not persist"
// (in-memory only). The clientId is sanitized with the same rule as the connect
// lock file so it is always filesystem-safe.
func codexThreadStorePath(clientID string) string {
	if clientID == "" {
		return ""
	}
	return filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(clientID), "codex-threads.json")
}

// resetSession drops the conversation's Codex thread so the next message starts
// a fresh one. Implements sessionResetter for the built-in /new and /clear
// commands. A no-op when per-conversation memory is disabled.
func (f *codexAppServerForwarder) resetSession(convID string) {
	if f.sessions != nil {
		f.sessions.reset(convID)
	}
}

func (f *codexAppServerForwarder) canStream() bool { return true }

func (f *codexAppServerForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "thread-memory"
	}
	return fmt.Sprintf("codex-app-server:%s (%s)", f.bin, memo)
}

func (f *codexAppServerForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	return f.forwardStream(ctx, convID, text, nil)
}

func (f *codexAppServerForwarder) forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	return f.forwardAppServer(ctx, convID, text, onDelta)
}

func (f *codexAppServerForwarder) forwardAppServer(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	var state *codexThreadState
	if f.sessions != nil {
		state = f.sessions.state(convID)
		state.mu.Lock()
		defer state.mu.Unlock()
	}

	cli, err := newCodexAppServerClient(ctx, f.bin, f.env, f.cwd())
	if err != nil {
		return "", err
	}
	defer cli.close()

	if err := cli.initialize(ctx); err != nil {
		return "", err
	}

	_ = state // held only for its per-conversation turn lock (see above)
	threadID := ""
	if f.sessions != nil {
		threadID = f.sessions.threadID(convID)
	}
	if threadID != "" {
		resumed, err := cli.resumeThread(ctx, f.threadParams(threadID))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[connect][codex] resume thread %s 失败，重建会话: %v\n", threadID, err)
			threadID = ""
			f.sessions.setThreadID(convID, "")
		} else {
			threadID = resumed
		}
	}
	if threadID == "" {
		started, err := cli.startThread(ctx, f.threadParams(""))
		if err != nil {
			return "", err
		}
		threadID = started
		if f.sessions != nil {
			f.sessions.setThreadID(convID, threadID)
		}
	}

	reply, err := cli.runTurn(ctx, threadID, text, onDelta)
	if err != nil {
		return "", err
	}
	return reply, nil
}

func (f *codexAppServerForwarder) cwd() string {
	if f.workDir == "" {
		return connectWorkDir()
	}
	if abs, err := filepath.Abs(f.workDir); err == nil {
		return abs
	}
	return f.workDir
}

func (f *codexAppServerForwarder) threadParams(threadID string) map[string]any {
	params := map[string]any{
		"approvalPolicy":        "never",
		"cwd":                   f.cwd(),
		"developerInstructions": codexRobotDeveloperInstructions,
		"sandbox":               "read-only",
	}
	if f.model != "" {
		params["model"] = f.model
	}
	if threadID != "" {
		params["threadId"] = threadID
	}
	return params
}

// codexThreadSessions maps a DingTalk conversation to its Codex thread so
// multi-turn context survives within a conversation. The convID→threadID map
// (threads, guarded by mu) is the authoritative store and is persisted to disk
// so the context also survives a connector restart — the codex equivalent of
// the Claude-family convSessions store. An empty path keeps it in memory only
// (persistence disabled), preserving the original behaviour exactly.
//
// states holds a per-conversation lock that serializes turns within one
// conversation; it carries no thread identity of its own.
type codexThreadSessions struct {
	mu      sync.Mutex
	states  map[string]*codexThreadState // per-conversation turn lock
	threads map[string]string            // convID→threadID, persisted
	path    string                       // on-disk store; empty disables persistence
}

// codexThreadState is a per-conversation turn lock. Holding it for the duration
// of a turn keeps two messages in the same conversation from interleaving their
// app-server calls; thread identity lives in codexThreadSessions.threads.
type codexThreadState struct {
	mu sync.Mutex
}

// newCodexThreadSessions builds the session map, restoring any persisted
// convID→threadID entries from path. A missing or corrupt file degrades to an
// empty map (see loadConvSessionMap) — it never panics or blocks startup. An
// empty path means in-memory only.
func newCodexThreadSessions(path string) *codexThreadSessions {
	return &codexThreadSessions{
		states:  make(map[string]*codexThreadState),
		threads: loadConvSessionMap(path),
		path:    path,
	}
}

// codexConvKey normalizes a conversation ID into a stable, non-empty map key.
func codexConvKey(convID string) string {
	key := strings.TrimSpace(convID)
	if key == "" {
		key = "_default"
	}
	return key
}

// state returns the per-conversation turn lock, minting one on first sight.
func (s *codexThreadSessions) state(convID string) *codexThreadState {
	key := codexConvKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.states[key]; ok {
		return st
	}
	st := &codexThreadState{}
	s.states[key] = st
	return st
}

// threadID returns the Codex thread bound to a conversation, or "" if none.
func (s *codexThreadSessions) threadID(convID string) string {
	key := codexConvKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threads[key]
}

// setThreadID binds a conversation to a Codex thread and persists the snapshot.
// A "" threadID forgets the binding (same as reset). Persistence is best-effort:
// a failed save only logs a warning and never blocks message handling.
func (s *codexThreadSessions) setThreadID(convID, threadID string) {
	key := codexConvKey(convID)
	s.mu.Lock()
	if threadID == "" {
		delete(s.threads, key)
	} else {
		s.threads[key] = threadID
	}
	snapshot := make(map[string]string, len(s.threads))
	for k, v := range s.threads {
		snapshot[k] = v
	}
	path := s.path
	s.mu.Unlock()
	saveConvSessionMap(path, snapshot)
}

// reset forgets a conversation's thread so the next message starts a fresh one.
// The removal is persisted so a restart does not resurrect the dropped thread.
func (s *codexThreadSessions) reset(convID string) {
	s.setThreadID(convID, "")
}

type codexAppServerClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	msgs      chan codexRPCMessage
	readErr   chan error
	done      chan struct{}
	closeOnce sync.Once
	stderr    *lockedBuffer
	nextID    int
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type codexRPCMessage struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *codexRPCError  `json:"error,omitempty"`
}

type codexRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newCodexAppServerClient(ctx context.Context, bin string, env []string, cwd string) (*codexAppServerClient, error) {
	cmd := exec.CommandContext(ctx, bin, "app-server", "--stdio")
	cmd.Dir = cwd
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cli := &codexAppServerClient{
		cmd:     cmd,
		stdin:   stdin,
		msgs:    make(chan codexRPCMessage, 64),
		readErr: make(chan error, 1),
		done:    make(chan struct{}),
		stderr:  stderr,
		nextID:  1,
	}
	go cli.readLoop(stdout)
	return cli, nil
}

func (c *codexAppServerClient) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg codexRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			c.reportReadErr(fmt.Errorf("parse app-server JSONL: %w", err))
			close(c.msgs)
			return
		}
		select {
		case c.msgs <- msg:
		case <-c.done:
			return
		}
	}
	if err := scanner.Err(); err != nil {
		c.reportReadErr(err)
	} else {
		c.reportReadErr(io.EOF)
	}
	close(c.msgs)
}

func (c *codexAppServerClient) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	})
}

func (c *codexAppServerClient) reportReadErr(err error) {
	select {
	case c.readErr <- err:
	case <-c.done:
	}
}

func (c *codexAppServerClient) initialize(ctx context.Context) error {
	id := c.requestID()
	if err := c.send(map[string]any{
		"id":     id,
		"method": "initialize",
		"params": map[string]any{
			"capabilities": map[string]any{"experimentalApi": true},
			"clientInfo": map[string]any{
				"name":    "dws-devapp-robot-connect",
				"title":   "DWS DevApp Robot Connect",
				"version": "0.1.0",
			},
		},
	}); err != nil {
		return err
	}
	if _, err := c.waitResponse(ctx, id); err != nil {
		return err
	}
	return c.send(map[string]any{"method": "initialized", "params": map[string]any{}})
}

func (c *codexAppServerClient) startThread(ctx context.Context, params map[string]any) (string, error) {
	id := c.requestID()
	if err := c.send(map[string]any{"id": id, "method": "thread/start", "params": params}); err != nil {
		return "", err
	}
	return codexThreadIDFromResult(c.waitResponse(ctx, id))
}

func (c *codexAppServerClient) resumeThread(ctx context.Context, params map[string]any) (string, error) {
	id := c.requestID()
	if err := c.send(map[string]any{"id": id, "method": "thread/resume", "params": params}); err != nil {
		return "", err
	}
	return codexThreadIDFromResult(c.waitResponse(ctx, id))
}

func (c *codexAppServerClient) runTurn(ctx context.Context, threadID, text string, onDelta func(string)) (string, error) {
	id := c.requestID()
	if err := c.send(map[string]any{
		"id":     id,
		"method": "turn/start",
		"params": map[string]any{
			"input":    []map[string]string{{"type": "text", "text": text}},
			"threadId": threadID,
		},
	}); err != nil {
		return "", err
	}

	var acc strings.Builder
	for {
		msg, err := c.next(ctx)
		if err != nil {
			return "", c.withStderr("app-server stream ended before turn/completed", err)
		}
		if msg.ID != nil && msg.Method != "" {
			c.rejectServerRequest(*msg.ID, msg.Method)
			continue
		}
		if msg.ID != nil && *msg.ID == id && msg.Error != nil {
			return "", fmt.Errorf("turn/start: %s", msg.Error.Message)
		}
		switch msg.Method {
		case "item/agentMessage/delta":
			var p struct {
				Delta    string `json:"delta"`
				ThreadID string `json:"threadId"`
			}
			if json.Unmarshal(msg.Params, &p) == nil && p.ThreadID == threadID && p.Delta != "" {
				acc.WriteString(p.Delta)
				if onDelta != nil {
					onDelta(acc.String())
				}
			}
		case "turn/completed":
			final, status, errMsg, ok := codexTurnCompletedText(msg.Params, threadID)
			if !ok {
				continue
			}
			if status == "failed" {
				if errMsg == "" {
					errMsg = "turn failed"
				}
				return "", fmt.Errorf("%s", errMsg)
			}
			if final == "" {
				final = strings.TrimSpace(acc.String())
			}
			if final == "" {
				return "", fmt.Errorf("turn completed without agent message")
			}
			return final, nil
		case "error":
			return "", fmt.Errorf("app-server error notification: %s", truncateRunes(string(msg.Params), 300))
		}
	}
}

func (c *codexAppServerClient) requestID() int {
	id := c.nextID
	c.nextID++
	return id
}

func (c *codexAppServerClient) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.stdin, string(b))
	return err
}

func (c *codexAppServerClient) waitResponse(ctx context.Context, id int) (json.RawMessage, error) {
	for {
		msg, err := c.next(ctx)
		if err != nil {
			return nil, c.withStderr("app-server exited before response", err)
		}
		if msg.ID != nil && msg.Method != "" {
			c.rejectServerRequest(*msg.ID, msg.Method)
			continue
		}
		if msg.ID == nil || *msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("%s", msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func (c *codexAppServerClient) next(ctx context.Context) (codexRPCMessage, error) {
	// The read loop reports EOF before closing msgs, so when the process
	// exits right after its last frame both channels are ready and select
	// would pick one at random — drain buffered frames (e.g. the final
	// turn/completed) before honoring a read error.
	select {
	case msg, ok := <-c.msgs:
		if !ok {
			return codexRPCMessage{}, io.EOF
		}
		return msg, nil
	default:
	}
	select {
	case msg, ok := <-c.msgs:
		if !ok {
			return codexRPCMessage{}, io.EOF
		}
		return msg, nil
	case err := <-c.readErr:
		return codexRPCMessage{}, err
	case <-c.done:
		return codexRPCMessage{}, io.EOF
	case <-ctx.Done():
		return codexRPCMessage{}, ctx.Err()
	}
}

func (c *codexAppServerClient) rejectServerRequest(id int, method string) {
	_ = c.send(map[string]any{
		"id": id,
		"error": map[string]any{
			"code":    -32000,
			"message": "DWS robot connect does not support interactive Codex app-server request: " + method,
		},
	})
}

func (c *codexAppServerClient) withStderr(prefix string, err error) error {
	stderr := strings.TrimSpace(c.stderr.String())
	if stderr == "" {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return fmt.Errorf("%s: %w: %s", prefix, err, truncateRunes(stderr, 300))
}

func codexThreadIDFromResult(result json.RawMessage, err error) (string, error) {
	if err != nil {
		return "", err
	}
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return "", err
	}
	if r.Thread.ID == "" {
		return "", fmt.Errorf("app-server response missing thread.id")
	}
	return r.Thread.ID, nil
}

func codexTurnCompletedText(params json.RawMessage, wantThreadID string) (final, status, errMsg string, ok bool) {
	var p struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			Status string `json:"status"`
			Error  *struct {
				Message           string `json:"message"`
				AdditionalDetails string `json:"additionalDetails"`
			} `json:"error"`
			Items []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"items"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.ThreadID != wantThreadID {
		return "", "", "", false
	}
	for i := len(p.Turn.Items) - 1; i >= 0; i-- {
		if p.Turn.Items[i].Type == "agentMessage" && strings.TrimSpace(p.Turn.Items[i].Text) != "" {
			final = strings.TrimSpace(p.Turn.Items[i].Text)
			break
		}
	}
	if p.Turn.Error != nil {
		errMsg = strings.TrimSpace(p.Turn.Error.Message)
		if p.Turn.Error.AdditionalDetails != "" {
			errMsg = strings.TrimSpace(errMsg + ": " + p.Turn.Error.AdditionalDetails)
		}
	}
	return final, p.Turn.Status, errMsg, true
}
