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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// opencode keeps per-conversation context, but unlike the Claude family it does
// not let the caller mint the session id up front: `opencode run` creates a
// session on the first turn and reports its id in the `--format json` event
// stream, and a later turn continues it with `--session <id>`. So the connector
// CAPTURES the id from the output (like codex's thread id) instead of minting a
// UUID, persists the convID→sessionID map, and replays it on the next message.
// This is what lets opencode behave as an always-on digital employee whose
// context survives both follow-up messages and a connector restart.

// opencodeForwarder forwards one DingTalk message to a local `opencode run`,
// parsing the JSON event stream for the reply text and the assigned session id.
type opencodeForwarder struct {
	bin      string
	env      []string
	timeout  time.Duration
	workDir  string
	model    string
	sessions *opencodeSessions // convID→sessionID, persisted; nil = stateless
}

func newOpencodeForwarder(bin string, env []string, timeout time.Duration, opts connectAgentOptions, clientID string) forwarder {
	var sessions *opencodeSessions
	if opts.Memory {
		sessions = newOpencodeSessions(opencodeSessionStorePath(clientID))
	}
	return &opencodeForwarder{
		bin:      bin,
		env:      env,
		timeout:  timeout,
		workDir:  opts.WorkDir,
		model:    opts.Model,
		sessions: sessions,
	}
}

func (f *opencodeForwarder) canStream() bool { return true }

func (f *opencodeForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "session-memory"
	}
	return fmt.Sprintf("opencode:%s (%s)", f.bin, memo)
}

func (f *opencodeForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	return f.forwardStream(ctx, convID, text, nil)
}

// forwardStream runs `opencode run --format json`, accumulating the visible text
// from `text` events (calling onDelta with the text-so-far when non-nil) and
// capturing the session id the stream reports. On the first turn of a
// conversation no `--session` is passed; the captured id is then persisted so
// the next message — and a restart — continues the same session.
func (f *opencodeForwarder) forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	priorSession := ""
	if f.sessions != nil {
		priorSession = f.sessions.id(convID)
	}

	args := []string{"run", "--pure", "--format", "json"}
	if f.model != "" {
		args = append(args, "--model", f.model)
	}
	if priorSession != "" {
		args = append(args, "--session", priorSession)
	}
	args = append(args, text)

	cmd := exec.CommandContext(ctx, f.bin, args...)
	if f.workDir != "" {
		cmd.Dir = f.workDir
	} else {
		cmd.Dir = connectWorkDir()
	}
	if len(f.env) > 0 {
		cmd.Env = append(os.Environ(), f.env...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var acc strings.Builder
	capturedSession := ""
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		sid, delta := parseOpencodeLine(line)
		if sid != "" {
			capturedSession = sid
		}
		if delta != "" {
			acc.WriteString(delta)
			if onDelta != nil {
				onDelta(brandReply(f.name(), acc.String()))
			}
		}
	}
	waitErr := cmd.Wait()

	// Persist the captured session as soon as we have one for a conversation
	// that did not have it yet (first turn), so the next message continues it.
	if f.sessions != nil && capturedSession != "" && capturedSession != priorSession {
		f.sessions.set(convID, capturedSession)
	}

	finalText := strings.TrimSpace(acc.String())
	// A bare backend error must not be forwarded as the answer; drop the session
	// so the next message starts clean, and return an actionable hint.
	if agentReplyIsError(finalText) {
		if f.sessions != nil {
			f.sessions.reset(convID)
		}
		return agentBackendErrorReply(finalText), nil
	}
	if finalText != "" {
		return brandReply(f.name(), finalText), nil
	}
	if waitErr != nil {
		msg := waitErr.Error()
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			msg = s
		}
		return "", fmt.Errorf("本地 opencode agent 调用失败：%s", truncateRunes(msg, 300))
	}
	return "（本地 agent 无文本输出）", nil
}

func (f *opencodeForwarder) name() string { return "opencode" }

// resetSession drops the conversation's opencode session so the next message
// starts a fresh one. Implements sessionResetter for the /new and /clear
// commands. A no-op when per-conversation memory is disabled.
func (f *opencodeForwarder) resetSession(convID string) {
	if f.sessions != nil {
		f.sessions.reset(convID)
	}
}

// parseOpencodeLine extracts (sessionID, visible-text delta) from one
// `opencode run --format json` event line. Every event carries the top-level
// sessionID; only `text` events carry reply text in part.text. Either return
// value may be empty.
func parseOpencodeLine(line string) (sessionID, delta string) {
	var ev struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionID"`
		Part      struct {
			Text string `json:"text"`
		} `json:"part"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return "", ""
	}
	if ev.Type == "text" {
		return ev.SessionID, ev.Part.Text
	}
	return ev.SessionID, ""
}

// opencodeSessionStorePath returns the on-disk location for a robot's opencode
// conversation→session map, scoped by clientId so multiple bots on one machine
// stay isolated: <config dir>/connect/<clientId>/opencode-sessions.json. It
// mirrors the Claude-family connectSessionStorePath layout with a distinct
// filename so the stores never collide. An empty clientId means "do not persist"
// (in-memory only). The clientId is sanitized like the connect lock file so it
// is always filesystem-safe.
func opencodeSessionStorePath(clientID string) string {
	if clientID == "" {
		return ""
	}
	return filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(clientID), "opencode-sessions.json")
}

// opencodeSessions maps a DingTalk conversation to the opencode session id
// captured from the agent's output. The map is the authoritative store (guarded
// by mu) and is persisted to disk so context survives a connector restart. An
// empty path keeps it in memory only (persistence disabled).
type opencodeSessions struct {
	mu   sync.Mutex
	m    map[string]string
	path string
}

func newOpencodeSessions(path string) *opencodeSessions {
	return &opencodeSessions{m: loadConvSessionMap(path), path: path}
}

func opencodeConvKey(convID string) string {
	key := strings.TrimSpace(convID)
	if key == "" {
		key = "_default"
	}
	return key
}

// id returns the opencode session bound to a conversation, or "" if none.
func (s *opencodeSessions) id(convID string) string {
	key := opencodeConvKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key]
}

// set binds a conversation to an opencode session and persists the snapshot.
// Best-effort: a failed save only logs a warning and never blocks message
// handling.
func (s *opencodeSessions) set(convID, sessionID string) {
	key := opencodeConvKey(convID)
	s.mu.Lock()
	if sessionID == "" {
		delete(s.m, key)
	} else {
		s.m[key] = sessionID
	}
	snapshot := make(map[string]string, len(s.m))
	for k, v := range s.m {
		snapshot[k] = v
	}
	path := s.path
	s.mu.Unlock()
	saveConvSessionMap(path, snapshot)
}

// reset forgets a conversation's session so the next message starts a fresh one.
// The removal is persisted so a restart does not resurrect the dropped session.
func (s *opencodeSessions) reset(convID string) {
	s.set(convID, "")
}
