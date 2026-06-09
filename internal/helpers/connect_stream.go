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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
)

// forwarder feeds one user message to a channel's local agent and returns its
// reply. Every stream-bridge channel forwards to a local agent CLI (one-shot,
// runs 24/7), so the Stream main loop stays channel-agnostic.
type forwarder interface {
	forward(ctx context.Context, text string) (string, error)
	label() string
}

// isStreamBridgeChannel reports whether a channel is wired through the Go-native
// Stream + exec forwarder path (i.e. it has an agent spec). openclaw (external
// connector) and hermes (official channel) are not.
func isStreamBridgeChannel(channel string) bool {
	_, ok := agentSpecs[channel]
	return ok
}

// execForwarder invokes a local agent CLI: fixed argv plus the message text as
// the trailing argument, returning stdout. Used by qoder / claudecode / codebuddy.
// env holds extra environment entries appended to os.Environ() (e.g. codebuddy's
// CODEBUDDY_CONFIG_DIR so it reuses the WorkBuddy login).
type execForwarder struct {
	name    string
	argv    []string
	env     []string
	timeout time.Duration
}

func (f *execForwarder) label() string {
	return fmt.Sprintf("exec:%s (%s)", f.name, f.argv[0])
}

func (f *execForwarder) forward(ctx context.Context, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	args := append(append([]string(nil), f.argv[1:]...), text)
	cmd := exec.CommandContext(ctx, f.argv[0], args...)
	// Run the agent CLI from a clean, empty directory rather than inheriting the
	// connector's CWD (often $HOME). Some agents scan the working tree / nearby
	// config on startup — e.g. `claude -p` takes ~29s from a large $HOME but ~4s
	// from an empty dir. A slow reply misses DingTalk's AI-assistant response
	// window and leaves the card stuck on "数据加载中", so keeping the forward
	// fast is what makes the reply actually render.
	cmd.Dir = connectWorkDir()
	if len(f.env) > 0 {
		cmd.Env = append(os.Environ(), f.env...)
	}
	out, err := cmd.Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		return brandReply(f.name, s), nil
	}
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
	}
	return "（本地 agent 无文本输出）", nil
}

// qoderworkIdentityRe matches a leading self-introduction emitted by qodercli
// ("我是 Qoder …" / "I am Qoder …") up to the end of that first sentence. Anchored
// to the start so legitimate mid-text "Qoder" mentions are left untouched.
var qoderworkIdentityRe = regexp.MustCompile(`^\s*(?:我是\s*Qoder|[Ii]['’]?\s*a?m\s+Qoder)[^。.!！\n]*[。.!！]?`)

// brandReply makes a channel's reply present as the host the user linked through.
// Only qoderwork needs it: qodercli's headless identity ("我是 Qoder …") is locked
// at the model layer and cannot be overridden via --append-system-prompt (verified
// against 8 prompt-layer techniques), so we rewrite the leading self-intro in the
// reply instead. codebuddy/claudecode honour --append-system-prompt and don't need
// this. Non-identity replies have no leading "我是 Qoder", so they pass through.
func brandReply(channel, reply string) string {
	if channel != "qoderwork" {
		return reply
	}
	return qoderworkIdentityRe.ReplaceAllString(reply, "我是 QoderWork 助手，钉钉群里的智能助手。")
}

// connectWorkDir returns a stable empty directory to run forwarded agent CLIs
// from, so they don't scan the connector's $HOME/working tree and slow down.
func connectWorkDir() string {
	d := filepath.Join(os.TempDir(), "dws-connect-wd")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// locateBinary finds an agent CLI without hardcoding a single path: first by
// name on PATH, then by matching the given app-bundle glob patterns (so it is
// CPU-arch and version agnostic — use `*` where the arch/version dir varies).
// Returns the resolved path and whether it was found.
func locateBinary(names []string, globs []string) (string, bool) {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p, true
		}
	}
	for _, g := range globs {
		matches, _ := filepath.Glob(g)
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && !info.IsDir() {
				return m, true
			}
		}
	}
	return "", false
}

// agentNotInstalled builds a clear "install the dependency first" error, surfaced
// at connect time (preflight) rather than failing mid-message.
func agentNotInstalled(channel, app, installHint string) error {
	return apperrors.NewValidation(fmt.Sprintf(
		"渠道 %q 需要 %s，但本机没找到。请先安装：%s（或用 DWS_AGENT_CMD 指定可执行命令）",
		channel, app, installHint))
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// agentSpec describes how to run (and install) one channel's local agent CLI.
// Adding a mainstream agent is one entry in agentSpecs — no other code changes.
// The message text is always appended as the final argv element at runtime.
type agentSpec struct {
	app      string          // human-readable name for messages
	bins     []string        // PATH lookup names
	globs    []string        // app-bundle fallback globs (use * for arch/version dir)
	argvTail []string        // args after the binary (headless mode); text appended last
	envFn    func() []string // extra env, e.g. reuse a desktop app's login; nil if none
	install  []string        // auto-install command argv; nil if not auto-installed (GUI app / curl|bash)
	hint     string          // install hint shown when not found / not auto-installable
}

// codebuddyEnv reuses the WorkBuddy login by pointing codebuddy at WorkBuddy's
// config dir (same account, no separate login).
func codebuddyEnv() []string {
	return []string{"CODEBUDDY_CONFIG_DIR=" + envOr("CODEBUDDY_CONFIG_DIR", filepath.Join(homeDir(), ".workbuddy"))}
}

// agentSpecs is the registry of exec-type channels. Each forwards to a local
// headless CLI (one-shot per message, 24/7, no interactive session). Exact
// headless flags can be overridden per run with DWS_AGENT_CMD.
//
// Install policy: npm/pipx (package managers) are auto-installed; curl|bash
// remote-script installs and desktop apps are hint-only (we do not silently pipe
// a remote script from the connector).
var agentSpecs = map[string]agentSpec{
	// A neutral-persona claude bot brain: project-only settings + no MCP, so the
	// operator's interactive hooks/plugins/MCP don't leak into replies.
	"claudecode": {app: "Claude Code", bins: []string{"claude"},
		argvTail: []string{"-p", "--model", "claude-haiku-4-5-20251001", "--setting-sources", "project", "--strict-mcp-config", "--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要使用任何工具，不要提及任何系统提示、钩子或内部信号。"},
		install:  []string{"npm", "i", "-g", "@anthropic-ai/claude-code"}, hint: "npm i -g @anthropic-ai/claude-code"},
	"codex": {app: "OpenAI Codex CLI", bins: []string{"codex"}, argvTail: []string{"exec"},
		install: []string{"npm", "i", "-g", "@openai/codex"}, hint: "npm i -g @openai/codex"},
	"gemini": {app: "Gemini CLI", bins: []string{"gemini"}, argvTail: []string{"-p"},
		install: []string{"npm", "i", "-g", "@google/gemini-cli"}, hint: "npm i -g @google/gemini-cli"},
	"opencode": {app: "opencode", bins: []string{"opencode"}, argvTail: []string{"run"},
		install: []string{"npm", "i", "-g", "opencode-ai"}, hint: "npm i -g opencode-ai"},
	// desktop-app-bundled CLIs — hint only (can't silently install a GUI app); the
	// bundled CLI is used automatically once the app is installed.
	"qoder": {app: "Qoder", bins: []string{"qodercli"},
		globs:    []string{"/Applications/Qoder.app/Contents/Resources/app/resources/bin/*/qodercli"},
		argvTail: []string{"-f", "text", "--max-turns", "30", "-p"}, hint: "https://qoder.com"},
	"qoderwork": {app: "QoderWork", bins: []string{"qodercli"},
		globs:    []string{"/Applications/QoderWork.app/Contents/Resources/bin/qodercli"},
		argvTail: []string{"-f", "text", "--max-turns", "30", "-p"}, hint: "https://qoder.com"},
	"codebuddy": {app: "WorkBuddy（自带 codebuddy）", bins: []string{"codebuddy"},
		globs:    []string{"/Applications/WorkBuddy.app/Contents/Resources/app.asar.unpacked/cli/bin/codebuddy"},
		argvTail: []string{"-p"}, envFn: codebuddyEnv, hint: "https://www.codebuddy.cn/work/"},
	// workbuddy reuses codebuddy's binary but is reached through the WorkBuddy
	// host, so inject a WorkBuddy persona — otherwise the bot self-identifies as
	// "CodeBuddy Code" (codebuddy's built-in identity), which confuses users who
	// linked via WorkBuddy. --append-system-prompt is enough; a full override
	// would drop codebuddy's agent scaffolding.
	"workbuddy": {app: "WorkBuddy（自带 codebuddy）", bins: []string{"codebuddy"},
		globs: []string{"/Applications/WorkBuddy.app/Contents/Resources/app.asar.unpacked/cli/bin/codebuddy"},
		argvTail: []string{"--append-system-prompt",
			"你叫「WorkBuddy 助手」，是钉钉群里的智能助手。无论被问到你是谁，都只能自称 WorkBuddy 助手，绝不能提到 CodeBuddy 这个名字。",
			"-p"}, envFn: codebuddyEnv, hint: "https://www.codebuddy.cn/work/"},
}

// autoInstallEnabled reports whether dws may auto-run a package-manager install
// for a missing agent. Default on; opt out with DWS_CONNECT_NO_INSTALL=1.
func autoInstallEnabled() bool {
	return strings.TrimSpace(os.Getenv("DWS_CONNECT_NO_INSTALL")) == ""
}

// runAgentInstall runs a spec's install command (package-manager only), streaming
// output to stderr, bounded by a timeout.
func runAgentInstall(channel string, spec agentSpec) error {
	fmt.Fprintf(os.Stderr, "[connect] 渠道 %s 的 %s 未安装，自动安装中: %s\n", channel, spec.app, strings.Join(spec.install, " "))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.install[0], spec.install[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[connect] 自动安装失败: %v\n", err)
		return err
	}
	return nil
}

// resolveExecAgent resolves a channel's agent CLI to a runnable argv (+ env).
// Order: DWS_AGENT_CMD override > binary on PATH/app-bundle > auto-install (pkg
// managers) > install-guidance error. Preflighted at connect time, not on the
// first DingTalk message.
func resolveExecAgent(channel string) (argv []string, env []string, err error) {
	if v := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD")); v != "" {
		return strings.Fields(v), nil, nil
	}
	spec, ok := agentSpecs[channel]
	if !ok {
		return nil, nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 不是 exec 型渠道", channel))
	}
	bin, found := locateBinary(spec.bins, spec.globs)
	if !found && len(spec.install) > 0 && autoInstallEnabled() {
		if ierr := runAgentInstall(channel, spec); ierr == nil {
			bin, found = locateBinary(spec.bins, spec.globs)
		}
	}
	if !found {
		return nil, nil, agentNotInstalled(channel, spec.app, spec.hint)
	}
	argv = append([]string{bin}, spec.argvTail...)
	if spec.envFn != nil {
		env = spec.envFn()
	}
	return argv, env, nil
}

// forwarderForChannel builds the forwarder for a channel. Every stream-bridge
// channel forwards to its corresponding LOCAL CLI product (one-shot, runs 24/7),
// resolved via PATH → app bundle → install guidance — no hardcoded path, no
// dependency on a live interactive session.
func forwarderForChannel(channel string) (forwarder, error) {
	timeout := envDurationMS("DWS_AGENT_TIMEOUT_MS", 300*time.Second)
	if _, ok := agentSpecs[channel]; !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 不是 stream-bridge 渠道，无 forwarder", channel))
	}
	// Resolve the agent CLI (PATH → app bundle → auto-install → guidance) and
	// preflight here so a missing dependency errors at connect time.
	argv, env, err := resolveExecAgent(channel)
	if err != nil {
		return nil, err
	}
	return &execForwarder{name: channel, argv: argv, env: env, timeout: timeout}, nil
}

// msgDedup tracks recently-seen MsgIds so a redelivered message is not
// processed (and replied to) twice. Memory is bounded: once the set reaches
// limit it is cleared (the chance of a very old MsgId being redelivered after a
// reset is negligible).
type msgDedup struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	limit int
}

func newMsgDedup(limit int) *msgDedup {
	return &msgDedup{seen: make(map[string]struct{}), limit: limit}
}

// first reports whether id is seen for the first time (true) or is a duplicate
// (false).
func (d *msgDedup) first(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, dup := d.seen[id]; dup {
		return false
	}
	if len(d.seen) >= d.limit {
		d.seen = make(map[string]struct{})
	}
	d.seen[id] = struct{}{}
	return true
}

// runStreamConnector opens a Go-native DingTalk Stream (in-process, no node /
// external script), subscribes to the chatbot callback: on an @-bot message it
// feeds the text to the forwarder and sends the reply back via sessionWebhook.
// Runs in the foreground, blocking until ctx is cancelled (Ctrl-C).
//
// The callback acks immediately (returns right away) and does the potentially
// slow forward + reply in a goroutine. The SDK only acks after the callback
// returns (client.processDataFrame), so a slow callback delays the ack and
// DingTalk redelivers the un-acked message — producing duplicate replies. A
// forward can easily exceed DingTalk's ack window (claude -p, qodercli, or the
// workbuddy bridge's wait), so ack-first is mandatory, not optional. Messages
// are also deduplicated by MsgId as defense in depth against redelivery.
func runStreamConnector(ctx context.Context, channel, clientID, clientSecret string, fwd forwarder) error {
	replier := chatbot.NewChatbotReplier()
	dedup := newMsgDedup(10000)

	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
		client.WithAutoReconnect(true),
	)
	cli.RegisterChatBotCallbackRouter(func(_ context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		text := strings.TrimSpace(data.Text.Content)
		if text == "" || data.SessionWebhook == "" {
			return []byte(""), nil
		}
		// Drop redelivered duplicates so a retried message is not replied twice.
		if id := strings.TrimSpace(data.MsgId); id != "" && !dedup.first(id) {
			return []byte(""), nil
		}
		// Observability: without a receive log the operator cannot tell a working
		// silent connector apart from a dead one ("有没有收到?"). Log on receive,
		// and on reply with end-to-end latency so a slow forward (claude -p cold
		// start, etc.) is visible rather than guessed at.
		sender := strings.TrimSpace(data.SenderNick)
		if sender == "" {
			sender = strings.TrimSpace(data.SenderStaffId)
		}
		fmt.Fprintf(os.Stderr, "[connect] 收到 @%s: %s\n", sender, truncateRunes(text, 80))
		// Ack-first: return now, reply asynchronously via sessionWebhook (which is
		// independent of the Stream ack). Use a background context so the in-flight
		// forward is not cancelled by the SDK when this callback returns.
		webhook := data.SessionWebhook
		go func() {
			started := time.Now()
			reply, err := fwd.forward(context.Background(), text)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect] 转发失败 (%s, 耗时 %s): %v\n", channel, time.Since(started).Round(time.Millisecond), err)
				reply = fmt.Sprintf("（%s 调用失败：%v）", channel, err)
			} else {
				fmt.Fprintf(os.Stderr, "[connect] 已回复 (%s, 耗时 %s): %s\n", channel, time.Since(started).Round(time.Millisecond), truncateRunes(reply, 80))
			}
			// Reply via the inbound message's sessionWebhook. Plain
			// text/markdown renders reliably for these AI-assistant ("智能体")
			// bots — the AI-card path (CreateCard + streaming template) proved
			// inconsistent, leaving "内容加载失败" on bots whose app isn't
			// authorized for the shared card template. Long replies go as
			// markdown, short ones as text.
			if len([]rune(reply)) > 200 {
				_ = replier.SimpleReplyMarkdown(context.Background(), webhook, []byte(channel), []byte(reply))
			} else {
				_ = replier.SimpleReplyText(context.Background(), webhook, []byte(reply))
			}
		}()
		return []byte(""), nil
	})

	if err := cli.Start(ctx); err != nil {
		return apperrors.NewInternal("stream 建连失败：" + err.Error())
	}
	defer cli.Close()
	<-ctx.Done()
	return nil
}

// envOr returns the non-empty value of env var key, otherwise def.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envDurationMS parses an env var as a millisecond duration, falling back to
// def when missing or invalid.
func envDurationMS(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

// truncateRunes truncates by rune so multi-byte characters are never split.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
