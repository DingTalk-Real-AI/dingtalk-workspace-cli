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
	"github.com/google/uuid"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	sdklogger "github.com/open-dingtalk/dingtalk-stream-sdk-go/logger"
)

// streamSDKLogger surfaces the Stream SDK's connection lifecycle on stderr.
// The SDK's default logger is a doNothingLogger — without SetLogger the
// operator cannot see "connect success", reconnects or read errors, making a
// dead connector indistinguishable from a healthy idle one. Debug frames stay
// silent (they dump every ping/pong).
type streamSDKLogger struct{}

func (streamSDKLogger) Debugf(format string, args ...interface{}) {}
func (streamSDKLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[stream] "+format+"\n", args...)
}
func (streamSDKLogger) Warningf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[stream][warn] "+format+"\n", args...)
}
func (streamSDKLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[stream][error] "+format+"\n", args...)
}
func (streamSDKLogger) Fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[stream][fatal] "+format+"\n", args...)
}

var streamLoggerOnce sync.Once

// forwarder feeds one user message to a channel's local agent and returns its
// reply. Every stream-bridge channel forwards to a local agent CLI (one-shot,
// runs 24/7), so the Stream main loop stays channel-agnostic. convID is the
// DingTalk conversation the message belongs to; forwarders with session memory
// use it to resume the same agent session, so follow-up questions keep context.
type forwarder interface {
	forward(ctx context.Context, convID, text string) (string, error)
	label() string
}

// connectAgentOptions carries the user-facing agent tuning exposed on
// `devapp robot connect` (and mirrored env vars). Zero value = defaults:
// channel's built-in model, per-conversation memory on (where the CLI supports
// it), empty scratch workdir.
type connectAgentOptions struct {
	// Model overrides the channel CLI's model (flag --agent-model /
	// env DWS_AGENT_MODEL). Empty keeps the spec's built-in choice.
	Model string
	// WorkDir is the directory the agent CLI runs from (flag --agent-workdir /
	// env DWS_AGENT_WORKDIR). Pointing it at a directory with knowledge files
	// (e.g. a CLAUDE.md) gives the bot context; empty uses a clean temp dir,
	// which keeps cold-start fast (a large $HOME costs ~29s vs ~4s).
	WorkDir string
	// Memory enables per-conversation session resume on channels whose CLI
	// supports addressable sessions (--session-id/--resume: claudecode,
	// codebuddy/workbuddy). Disable with --agent-memory=false.
	Memory bool
	// ReplyCard answers with a DingTalk AI card (thinking → done states, like
	// the hermes/openclaw official pipelines) instead of a plain message.
	// Any card failure falls back to the plain webhook reply. Disable with
	// --reply-card=false.
	ReplyCard bool
	// CardTemplate is the AI-card template ID (--card-template /
	// DWS_CARD_TEMPLATE). Card templates are app-scoped: register one under
	// your app in the developer console for reliable branded rendering; empty
	// uses the public openclaw template as a best-effort default.
	CardTemplate string
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
	// workDir is where the agent CLI runs from; empty falls back to the clean
	// scratch dir (see connectWorkDir).
	workDir string
	// sessions, when non-nil, maps DingTalk conversations to agent session IDs
	// for CLIs with addressable sessions (--session-id / --resume). nil =
	// stateless one-shot per message.
	sessions *convSessions
	// streamArgv, when non-empty, is the incremental-output argv (bin first),
	// with parser naming its stdout protocol (see connect_streaming.go).
	streamArgv []string
	parser     string
}

func (f *execForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "session-memory"
	}
	return fmt.Sprintf("exec:%s (%s, %s)", f.name, f.argv[0], memo)
}

func (f *execForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	// Session args go right after the binary, before the spec tail — some specs
	// (qoder) end the tail with `-p` so the prompt must stay the trailing
	// positional argument.
	var args []string
	if f.sessions != nil && strings.TrimSpace(convID) != "" {
		args = append(args, f.sessions.args(convID)...)
	}
	args = append(args, f.argv[1:]...)
	args = append(args, text)
	cmd := exec.CommandContext(ctx, f.argv[0], args...)
	// Run the agent CLI from a clean, empty directory rather than inheriting the
	// connector's CWD (often $HOME). Some agents scan the working tree / nearby
	// config on startup — e.g. `claude -p` takes ~29s from a large $HOME but ~4s
	// from an empty dir. A slow reply misses DingTalk's AI-assistant response
	// window and leaves the card stuck on "数据加载中", so keeping the forward
	// fast is what makes the reply actually render. --agent-workdir trades that
	// speed for context (knowledge files in the workdir).
	if f.workDir != "" {
		cmd.Dir = f.workDir
	} else {
		cmd.Dir = connectWorkDir()
	}
	if len(f.env) > 0 {
		cmd.Env = append(os.Environ(), f.env...)
	}
	out, err := cmd.Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		return brandReply(f.name, s), nil
	}
	if err != nil {
		// Self-heal session state: if this conversation's session is broken
		// (e.g. --resume of a session that was never created or got cleaned),
		// drop the mapping so the next message starts a fresh session instead
		// of failing forever.
		if f.sessions != nil && strings.TrimSpace(convID) != "" {
			f.sessions.reset(convID)
		}
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
	}
	return "（本地 agent 无文本输出）", nil
}

// convSessions maps a DingTalk conversation to a stable agent session ID, so a
// channel CLI with addressable sessions keeps multi-turn context per chat.
// First message of a conversation mints a UUID and passes `--session-id <id>`
// (create); subsequent messages pass `--resume <id>` (continue). Claude Code,
// codebuddy and other Claude-Code-family CLIs share these exact flags —
// verified against `claude --help` / `codebuddy --help`. qodercli only has
// `--resume` (no way to choose the ID), so the qoder family stays stateless.
// State is in-memory: a connector restart starts conversations fresh.
type convSessions struct {
	mu sync.Mutex
	m  map[string]string
}

func newConvSessions() *convSessions {
	return &convSessions{m: make(map[string]string)}
}

// args returns the session argv fragment for one conversation, minting a new
// session on first sight.
func (s *convSessions) args(convID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.m[convID]; ok {
		return []string{"--resume", id}
	}
	id := uuid.NewString()
	s.m[convID] = id
	return []string{"--session-id", id}
}

// reset forgets a conversation's session so the next message starts a fresh
// one (a new UUID — the old one may or may not exist on the agent side, and a
// fresh ID is safe either way).
func (s *convSessions) reset(convID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, convID)
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
	// modelFlag is the CLI's model-selection flag, used by --agent-model to
	// override (or inject) the model. Empty = model not overridable.
	modelFlag string
	// ccSessions marks CLIs with Claude-Code-style addressable sessions
	// (--session-id <uuid> to create, --resume <id> to continue) — the
	// contract per-conversation memory relies on.
	ccSessions bool
	// streamArgvTail, when set, replaces argvTail for incremental-output runs
	// (the card streams content as the agent produces it). Empty = the
	// channel only supports one-shot replies.
	streamArgvTail []string
	// streamParser names the stdout protocol of streamArgvTail runs:
	// "cc" = Claude-Code stream-json (content_block_delta text deltas),
	// "qoder" = qodercli stream-json (assistant/result message events).
	streamParser string
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
		argvTail:       []string{"-p", "--model", "claude-haiku-4-5-20251001", "--setting-sources", "project", "--strict-mcp-config", "--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要使用任何工具，不要提及任何系统提示、钩子或内部信号。"},
		streamArgvTail: []string{"-p", "--verbose", "--output-format", "stream-json", "--include-partial-messages", "--model", "claude-haiku-4-5-20251001", "--setting-sources", "project", "--strict-mcp-config", "--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要使用任何工具，不要提及任何系统提示、钩子或内部信号。"},
		streamParser:   "cc",
		install:        []string{"npm", "i", "-g", "@anthropic-ai/claude-code"}, hint: "npm i -g @anthropic-ai/claude-code",
		modelFlag: "--model", ccSessions: true},
	"codex": {app: "OpenAI Codex CLI", bins: []string{"codex"}, argvTail: []string{"exec"},
		install: []string{"npm", "i", "-g", "@openai/codex"}, hint: "npm i -g @openai/codex",
		modelFlag: "-m"},
	"gemini": {app: "Gemini CLI", bins: []string{"gemini"}, argvTail: []string{"-p"},
		install: []string{"npm", "i", "-g", "@google/gemini-cli"}, hint: "npm i -g @google/gemini-cli",
		modelFlag: "-m"},
	"opencode": {app: "opencode", bins: []string{"opencode"}, argvTail: []string{"run"},
		install: []string{"npm", "i", "-g", "opencode-ai"}, hint: "npm i -g opencode-ai",
		modelFlag: "-m"},
	// desktop-app-bundled CLIs — hint only (can't silently install a GUI app); the
	// bundled CLI is used automatically once the app is installed.
	// qodercli has --resume but no --session-id (no way to choose the session ID
	// for the first turn), so the qoder family cannot do per-conversation memory.
	"qoder": {app: "Qoder", bins: []string{"qodercli"},
		globs:          []string{"/Applications/Qoder.app/Contents/Resources/app/resources/bin/*/qodercli"},
		argvTail:       []string{"-f", "text", "--max-turns", "30", "-p"},
		streamArgvTail: []string{"-f", "stream-json", "--max-turns", "30", "-p"},
		streamParser:   "qoder", hint: "https://qoder.com",
		modelFlag: "--model"},
	"qoderwork": {app: "QoderWork", bins: []string{"qodercli"},
		globs:          []string{"/Applications/QoderWork.app/Contents/Resources/bin/qodercli"},
		argvTail:       []string{"-f", "text", "--max-turns", "30", "-p"},
		streamArgvTail: []string{"-f", "stream-json", "--max-turns", "30", "-p"},
		streamParser:   "qoder", hint: "https://qoder.com",
		modelFlag: "--model"},
	"codebuddy": {app: "WorkBuddy（自带 codebuddy）", bins: []string{"codebuddy"},
		globs:          []string{"/Applications/WorkBuddy.app/Contents/Resources/app.asar.unpacked/cli/bin/codebuddy"},
		argvTail:       []string{"-p"},
		streamArgvTail: []string{"-p", "--output-format", "stream-json", "--include-partial-messages"},
		streamParser:   "cc", envFn: codebuddyEnv, hint: "https://www.codebuddy.cn/work/",
		modelFlag: "--model", ccSessions: true},
	// workbuddy reuses codebuddy's binary but is reached through the WorkBuddy
	// host, so inject a WorkBuddy persona — otherwise the bot self-identifies as
	// "CodeBuddy Code" (codebuddy's built-in identity), which confuses users who
	// linked via WorkBuddy. --append-system-prompt is enough; a full override
	// would drop codebuddy's agent scaffolding.
	// The persona prompt also forbids tool use: in headless mode codebuddy
	// otherwise tries tools (e.g. writing a memory file), stalls on the
	// permission gate and replies with a permissions lecture — or nothing.
	"workbuddy": {app: "WorkBuddy（自带 codebuddy）", bins: []string{"codebuddy"},
		globs: []string{"/Applications/WorkBuddy.app/Contents/Resources/app.asar.unpacked/cli/bin/codebuddy"},
		argvTail: []string{"--append-system-prompt",
			"你叫「WorkBuddy 助手」，是钉钉群里的智能助手。无论被问到你是谁，都只能自称 WorkBuddy 助手，绝不能提到 CodeBuddy 这个名字。请用简洁自然的中文直接回答问题；不要使用任何工具，不要尝试读写文件或执行命令。",
			"-p"},
		streamArgvTail: []string{"--append-system-prompt",
			"你叫「WorkBuddy 助手」，是钉钉群里的智能助手。无论被问到你是谁，都只能自称 WorkBuddy 助手，绝不能提到 CodeBuddy 这个名字。请用简洁自然的中文直接回答问题；不要使用任何工具，不要尝试读写文件或执行命令。",
			"-p", "--output-format", "stream-json", "--include-partial-messages"},
		streamParser: "cc", envFn: codebuddyEnv, hint: "https://www.codebuddy.cn/work/",
		modelFlag: "--model", ccSessions: true},
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

// connectCliStatus reports whether a channel's local CLI dependency is
// present, without installing anything — the machine-readable preflight that
// lets an agent check (via --dry-run) and guide the user BEFORE starting the
// connector. External channels report their own onboarding tool.
func connectCliStatus(channel string) map[string]any {
	switch channel {
	case "openclaw", "hermes":
		bin := channel
		_, found := locateBinary([]string{bin}, nil)
		return map[string]any{
			"required": bin, "installed": found,
			"autoInstall": false,
			"installHint": "渠道 " + channel + " 走官方建联，请先安装并完成其 onboarding",
		}
	}
	spec, ok := agentSpecs[channel]
	if !ok {
		return map[string]any{"required": "", "installed": false}
	}
	path, found := locateBinary(spec.bins, spec.globs)
	status := map[string]any{
		"required":    spec.app,
		"installed":   found,
		"autoInstall": len(spec.install) > 0 && autoInstallEnabled(),
		"installHint": spec.hint,
	}
	if found {
		status["path"] = path
	}
	return status
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
// dependency on a live interactive session. opts applies the user-facing agent
// tuning (--agent-model / --agent-workdir / --agent-memory).
func forwarderForChannel(channel string, opts connectAgentOptions) (forwarder, error) {
	timeout := envDurationMS("DWS_AGENT_TIMEOUT_MS", 300*time.Second)
	spec, ok := agentSpecs[channel]
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 不是 stream-bridge 渠道，无 forwarder", channel))
	}
	// Resolve the agent CLI (PATH → app bundle → auto-install → guidance) and
	// preflight here so a missing dependency errors at connect time.
	argv, env, err := resolveExecAgent(channel)
	if err != nil {
		return nil, err
	}
	// A DWS_AGENT_CMD override is a fully user-controlled argv: do not splice in
	// model or session flags we cannot know are valid for it.
	overridden := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD")) != ""
	if !overridden && opts.Model != "" {
		if spec.modelFlag == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 的 agent CLI 不支持模型覆盖（--agent-model）", channel))
		}
		argv = applyModelArg(argv, spec.modelFlag, opts.Model)
	}
	var sessions *convSessions
	if !overridden && opts.Memory && spec.ccSessions {
		sessions = newConvSessions()
	}
	// Incremental-output argv: same binary, the spec's stream tail, same model
	// override. A DWS_AGENT_CMD override disables streaming (unknown argv).
	var streamArgv []string
	parser := ""
	if !overridden && len(spec.streamArgvTail) > 0 {
		streamArgv = append([]string{argv[0]}, spec.streamArgvTail...)
		if opts.Model != "" && spec.modelFlag != "" {
			streamArgv = applyModelArg(streamArgv, spec.modelFlag, opts.Model)
		}
		parser = spec.streamParser
	}
	return &execForwarder{name: channel, argv: argv, env: env, timeout: timeout,
		workDir: opts.WorkDir, sessions: sessions,
		streamArgv: streamArgv, parser: parser}, nil
}

// applyModelArg returns argv with the model flag set to model: if flag is
// already present its value is replaced (e.g. claudecode's built-in haiku
// pin), otherwise flag+model are inserted right after the binary — before the
// spec tail, because some tails end with `-p` and the prompt must stay the
// trailing positional argument.
func applyModelArg(argv []string, flag, model string) []string {
	out := append([]string(nil), argv...)
	for i := 1; i < len(out)-1; i++ {
		if out[i] == flag {
			out[i+1] = model
			return out
		}
	}
	return append(out[:1:1], append([]string{flag, model}, out[1:]...)...)
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
func runStreamConnector(ctx context.Context, channel, clientID, clientSecret string, fwd forwarder, cardCli *aiCardClient) error {
	streamLoggerOnce.Do(func() { sdklogger.SetLogger(streamSDKLogger{}) })
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
		fmt.Fprintf(os.Stderr, "[connect] 收到 @%s: %s (convType=%s convId=%s staffId=%s msgId=%s)\n",
			sender, truncateRunes(text, 80), data.ConversationType, data.ConversationId, data.SenderStaffId, data.MsgId)
		// Ack-first: return now, reply asynchronously via sessionWebhook (which is
		// independent of the Stream ack). Use a background context so the in-flight
		// forward is not cancelled by the SDK when this callback returns.
		webhook := data.SessionWebhook
		// Conversation key for session memory: the DingTalk conversation ID, so a
		// group chat shares one agent session and a 1:1 chat gets its own.
		convID := strings.TrimSpace(data.ConversationId)
		if convID == "" {
			convID = strings.TrimSpace(data.SenderStaffId)
		}
		callbackData := data
		msgID := strings.TrimSpace(data.MsgId)
		go func() {
			started := time.Now()
			// hermes-UX reply sequence (gateway/platforms/dingtalk.py):
			//   ① on receive: "🤔Thinking" reaction chip on the user's message
			//      (no card yet — the thinking phase is the chip, not a card);
			//   ② agent runs;
			//   ③ on done: deliver the AI card with the final content;
			//   ④ swap the chip to "🥳Done".
			thinking := false
			if cardCli != nil {
				if terr := cardCli.markThinking(context.Background(), callbackData.ConversationId, msgID); terr != nil {
					fmt.Fprintf(os.Stderr, "[connect][card] Thinking 表态失败（不影响回复）: %v\n", terr)
				} else {
					thinking = true
				}
			}

			// Streaming path: deliver the card up front and stream the agent's
			// output into it as it is produced — the full hermes/openclaw UX.
			// One-shot channels (or a failed card) fall back below.
			var cardInst *aiCardInstance
			var onDelta func(string)
			ef, _ := fwd.(*execForwarder)
			if cardCli != nil && ef != nil && ef.canStream() {
				if ci, cerr := cardCli.createAndDeliver(context.Background(), callbackData); cerr != nil {
					fmt.Fprintf(os.Stderr, "[connect][card] 预投卡片失败，降级一次性回复: %v\n", cerr)
				} else {
					cardInst = ci
					onDelta = func(sofar string) {
						if serr := cardCli.streamFrame(context.Background(), ci, sofar, false); serr != nil {
							fmt.Fprintf(os.Stderr, "[connect][card] 流式帧失败: %v\n", serr)
						}
					}
				}
			}

			var reply string
			var err error
			if ef != nil {
				reply, err = ef.forwardStream(context.Background(), convID, text, onDelta)
			} else {
				reply, err = fwd.forward(context.Background(), convID, text)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect] 转发失败 (%s, 耗时 %s): %v\n", channel, time.Since(started).Round(time.Millisecond), err)
				reply = fmt.Sprintf("（%s 调用失败：%v）", channel, err)
			} else {
				fmt.Fprintf(os.Stderr, "[connect] 已回复 (%s, 耗时 %s): %s\n", channel, time.Since(started).Round(time.Millisecond), truncateRunes(reply, 80))
			}

			delivered := false
			if cardCli != nil {
				if cardInst == nil {
					// One-shot path: card only appears with the final content.
					if ci, cerr := cardCli.createAndDeliver(context.Background(), callbackData); cerr != nil {
						fmt.Fprintf(os.Stderr, "[connect][card] 投放卡片失败，回退普通消息: %v\n", cerr)
					} else {
						cardInst = ci
					}
				}
				if cardInst != nil {
					if ferr := cardCli.finish(context.Background(), cardInst, reply); ferr != nil {
						// A stuck card is the worst UX: mark it failed
						// (best-effort) and fall through to the plain reply.
						fmt.Fprintf(os.Stderr, "[connect][card] 卡片完成失败，回退普通消息: %v\n", ferr)
						cardCli.markFailed(context.Background(), cardInst)
					} else {
						fmt.Fprintf(os.Stderr, "[connect][card] 卡片已完成 outTrackId=%s\n", cardInst.outTrackID)
						delivered = true
						// Client-side fetch of a fresh card occasionally misses
						// the burst and shows "内容加载失败"; a delayed repair
						// frame triggers a re-render.
						go cardCli.repair(context.Background(), cardInst, reply)
					}
				}
			}

			if !delivered {
				// Fallback path: plain reply via the inbound sessionWebhook.
				// Long replies go as markdown, short ones as text.
				if len([]rune(reply)) > 200 {
					_ = replier.SimpleReplyMarkdown(context.Background(), webhook, []byte(channel), []byte(reply))
				} else {
					_ = replier.SimpleReplyText(context.Background(), webhook, []byte(reply))
				}
			}

			if thinking {
				cardCli.swapThinkingToDone(context.Background(), callbackData.ConversationId, msgID)
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
