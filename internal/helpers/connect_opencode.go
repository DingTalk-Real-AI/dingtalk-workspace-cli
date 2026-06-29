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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	opencodeServerUsername     = "opencode"
	opencodeServerHealthPath   = "/global/health"
	opencodeNoTextReply        = "（本地 agent 无文本输出）"
	opencodeServerStartupWait  = 20 * time.Second
	opencodeServerPollInterval = 150 * time.Millisecond
	// opencodeHealthProbeTimeout bounds a single /global/health probe so startup
	// detection stays snappy even though the shared http.Client has no overall
	// deadline (message turns rely on the per-turn ctx instead).
	opencodeHealthProbeTimeout = 10 * time.Second
)

var errOpencodeSessionMissing = errors.New("opencode session missing")

// opencodeForwarder forwards DingTalk messages to a local `opencode serve`
// instance. DWS owns the local server process and keeps a per-conversation
// mapping to opencode's native session id; opencode owns the actual message
// history in its own storage.
type opencodeForwarder struct {
	bin      string
	env      []string
	timeout  time.Duration
	workDir  string
	model    string
	sessions *opencodeSessions // convID→opencode sessionID, persisted; nil = stateless
	server   *opencodeServer
}

func newOpencodeForwarder(bin string, env []string, timeout time.Duration, opts connectAgentOptions, clientID string) forwarder {
	var sessions *opencodeSessions
	if opts.Memory {
		sessions = newOpencodeSessions(opencodeSessionStorePath(clientID))
	}
	f := &opencodeForwarder{
		bin:      bin,
		env:      env,
		timeout:  timeout,
		workDir:  opts.WorkDir,
		model:    opts.Model,
		sessions: sessions,
	}
	f.server = newOpencodeServer(bin, env, f.cwd())
	return f
}

// Group-chat bots answer once per message. opencode server still supports
// events, but DWS deliberately uses the synchronous message API here.
func (f *opencodeForwarder) canStream() bool { return false }

func (f *opencodeForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "server-session-memory"
	}
	return fmt.Sprintf("opencode-server:%s (%s)", f.bin, memo)
}

func (f *opencodeForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	return f.forwardStream(ctx, convID, text, nil)
}

func (f *opencodeForwarder) forwardStream(ctx context.Context, convID, text string, _ func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	client, err := f.server.ensure(ctx)
	if err != nil {
		return "", err
	}
	reply, err := f.forwardWithClient(ctx, client, convID, text)
	if errors.Is(err, errOpencodeSessionMissing) && f.sessions != nil {
		f.sessions.reset(convID)
		reply, err = f.forwardWithClient(ctx, client, convID, text)
	}
	return reply, err
}

func (f *opencodeForwarder) forwardWithClient(ctx context.Context, client *opencodeHTTPClient, convID, text string) (string, error) {
	sessionID := ""
	if f.sessions != nil {
		sessionID = f.sessions.id(convID)
	}
	if sessionID == "" {
		created, err := client.createSession(ctx)
		if err != nil {
			return "", err
		}
		sessionID = created
		if f.sessions != nil {
			f.sessions.set(convID, sessionID)
		}
	}
	reply, err := client.sendMessage(ctx, sessionID, text, f.model)
	if err != nil {
		return "", err
	}
	if agentReplyIsError(reply) {
		return agentBackendErrorReply(reply), nil
	}
	if strings.TrimSpace(reply) == "" {
		return opencodeNoTextReply, nil
	}
	return brandReply(f.name(), reply), nil
}

func (f *opencodeForwarder) name() string { return "opencode" }

func (f *opencodeForwarder) cwd() string {
	if f.workDir == "" {
		return connectWorkDir()
	}
	if abs, err := filepath.Abs(f.workDir); err == nil {
		return abs
	}
	return f.workDir
}

func (f *opencodeForwarder) close() error {
	if f.server != nil {
		return f.server.close()
	}
	return nil
}

// resetSession drops the conversation's opencode session so the next message
// starts a fresh one, leaving the old session on the opencode server (it stays
// resumable by id). Backs the /new command. A no-op when memory is disabled.
func (f *opencodeForwarder) resetSession(convID string) {
	if f.sessions != nil {
		f.sessions.reset(convID)
	}
}

// clearSession backs the /clear command: it actively deletes the conversation's
// session on the opencode server (DELETE /session/:id) and then drops the local
// mapping, so the context is truly wiped rather than just forgotten. A missing
// server session is treated as already cleared. Implements sessionClearer.
func (f *opencodeForwarder) clearSession(ctx context.Context, convID string) error {
	if f.sessions == nil {
		return nil
	}
	sessionID := f.sessions.id(convID)
	f.sessions.reset(convID)
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	client, err := f.server.ensure(ctx)
	if err != nil {
		return err
	}
	return client.deleteSession(ctx, sessionID)
}

type opencodeServer struct {
	bin        string
	env        []string
	workDir    string
	mu         sync.Mutex
	baseURL    string
	password   string
	cmd        *exec.Cmd
	done       chan error
	httpClient *http.Client
}

func newOpencodeServer(bin string, env []string, workDir string) *opencodeServer {
	return &opencodeServer{
		bin:     bin,
		env:     env,
		workDir: workDir,
		// No client-level Timeout: a turn can legitimately run for minutes, so the
		// per-request ctx (forwardStream's f.timeout, default 300s) governs instead.
		// A hardcoded 30s here used to abort long agent replies mid-flight with
		// "Client.Timeout exceeded while awaiting headers".
		httpClient: &http.Client{},
	}
}

func (s *opencodeServer) ensure(ctx context.Context) (*opencodeHTTPClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpClient == nil {
		s.httpClient = &http.Client{}
	}
	client := &opencodeHTTPClient{baseURL: s.baseURL, username: opencodeServerUsername, password: s.password, httpClient: s.httpClient}
	if s.baseURL != "" && client.health(ctx) == nil {
		return client, nil
	}
	if s.cmd != nil {
		_ = s.closeLocked()
	}

	port, err := freeLocalPort()
	if err != nil {
		return nil, err
	}
	password := randomHex(24)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(s.bin, "serve", "--pure", "--hostname", "127.0.0.1", "--port", fmt.Sprint(port))
	if s.workDir != "" {
		cmd.Dir = s.workDir
	}
	cmd.Env = append(os.Environ(), s.env...)
	cmd.Env = append(cmd.Env,
		"OPENCODE_SERVER_USERNAME="+opencodeServerUsername,
		"OPENCODE_SERVER_PASSWORD="+password,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 opencode serve 失败：%w", err)
	}

	s.baseURL = baseURL
	s.password = password
	s.cmd = cmd
	s.done = make(chan error, 1)
	go func() {
		s.done <- cmd.Wait()
	}()
	client = &opencodeHTTPClient{baseURL: s.baseURL, username: opencodeServerUsername, password: s.password, httpClient: s.httpClient}
	if err := s.waitHealthy(ctx, client); err != nil {
		_ = s.closeLocked()
		return nil, err
	}
	return client, nil
}

func (s *opencodeServer) waitHealthy(ctx context.Context, client *opencodeHTTPClient) error {
	deadline := time.Now().Add(opencodeServerStartupWait)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.done != nil {
			select {
			case err := <-s.done:
				s.done = nil
				if err != nil {
					return fmt.Errorf("opencode serve 已退出：%w", err)
				}
				return fmt.Errorf("opencode serve 已退出")
			default:
			}
		}
		if err := client.health(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(opencodeServerPollInterval)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 opencode serve 就绪超时：%w", lastErr)
	}
	return fmt.Errorf("等待 opencode serve 就绪超时")
}

func (s *opencodeServer) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *opencodeServer) closeLocked() error {
	if s.cmd == nil || s.cmd.Process == nil {
		s.cmd = nil
		s.done = nil
		return nil
	}
	err := s.cmd.Process.Kill()
	if s.done != nil {
		<-s.done
	}
	s.cmd = nil
	s.done = nil
	s.baseURL = ""
	s.password = ""
	return err
}

type opencodeHTTPClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func (c *opencodeHTTPClient) health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, opencodeHealthProbeTimeout)
	defer cancel()
	var out struct {
		Healthy bool `json:"healthy"`
	}
	if err := c.doJSON(ctx, http.MethodGet, opencodeServerHealthPath, nil, &out); err != nil {
		return err
	}
	if !out.Healthy {
		return fmt.Errorf("opencode serve health=false")
	}
	return nil
}

func (c *opencodeHTTPClient) createSession(ctx context.Context) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/session", map[string]any{}, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.ID) == "" {
		return "", fmt.Errorf("opencode server response missing session id")
	}
	return out.ID, nil
}

// deleteSession disposes a session on the opencode server (DELETE /session/:id).
// A 404 (session already gone) is treated as success so /clear stays idempotent.
func (c *opencodeHTTPClient) deleteSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if err := c.doJSON(ctx, http.MethodDelete, "/session/"+sessionID, nil, nil); err != nil {
		if errors.Is(err, errOpencodeSessionMissing) {
			return nil
		}
		return err
	}
	return nil
}

func (c *opencodeHTTPClient) sendMessage(ctx context.Context, sessionID, text, model string) (string, error) {
	body := map[string]any{
		"parts": []map[string]any{{"type": "text", "text": text}},
	}
	if m := opencodeModelRef(model); m != nil {
		body["model"] = m
	}
	var out struct {
		Parts []opencodePart `json:"parts"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/session/"+sessionID+"/message", body, &out); err != nil {
		if errors.Is(err, errOpencodeSessionMissing) {
			return "", err
		}
		return "", err
	}
	return strings.TrimSpace(opencodePartsText(out.Parts)), nil
}

func (c *opencodeHTTPClient) doJSON(ctx context.Context, method, path string, in any, out any) error {
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("opencode server URL is empty")
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && strings.HasPrefix(path, "/session/") {
		return errOpencodeSessionMissing
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("opencode server %s %s 失败：%s", method, path, truncateRunes(msg, 300))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

type opencodePart struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Delta string          `json:"delta"`
	Parts []opencodePart  `json:"parts"`
	Data  json.RawMessage `json:"data"`
}

func opencodePartsText(parts []opencodePart) string {
	var b strings.Builder
	var walk func([]opencodePart)
	walk = func(items []opencodePart) {
		for _, p := range items {
			if p.Type == "text" {
				if p.Text != "" {
					b.WriteString(p.Text)
				}
				if p.Delta != "" {
					b.WriteString(p.Delta)
				}
				if len(p.Data) > 0 {
					var nested struct {
						Text string `json:"text"`
					}
					if json.Unmarshal(p.Data, &nested) == nil && nested.Text != "" {
						b.WriteString(nested.Text)
					}
				}
			}
			if len(p.Parts) > 0 {
				walk(p.Parts)
			}
		}
	}
	walk(parts)
	return b.String()
}

func opencodeModelRef(model string) map[string]string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	provider, modelID, ok := strings.Cut(model, "/")
	if !ok || strings.TrimSpace(provider) == "" || strings.TrimSpace(modelID) == "" {
		return map[string]string{"modelID": model}
	}
	return map[string]string{"providerID": provider, "modelID": modelID}
}

func freeLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// opencodeSessionStorePath returns the on-disk location for a robot's opencode
// conversation→session map, scoped by clientId so multiple bots on one machine
// stay isolated: <config dir>/connect/<clientId>/opencode-sessions.json. An
// empty clientId means "do not persist" (in-memory only).
func opencodeSessionStorePath(clientID string) string {
	if clientID == "" {
		return ""
	}
	return filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(clientID), "opencode-sessions.json")
}

// opencodeSessions maps a DingTalk conversation to the opencode session id
// returned by the server API. The map is the authoritative store (guarded by mu)
// and is persisted to disk so context can survive a connector restart when
// --agent-memory is enabled.
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
