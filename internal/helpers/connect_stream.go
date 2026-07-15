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
	"encoding/json"
	"errors"
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
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"
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

func (streamSDKLogger) Debugf(format string, args ...interface{}) {
	_ = format
	_ = args
}
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

type connectStreamClient interface {
	RegisterChatBotCallbackRouter(chatbot.IChatBotMessageHandler)
	RegisterCardCallbackRouter(card.ICardCallbackHandler)
	Start(context.Context) error
	Close()
}

type connectChatReplier interface {
	SimpleReplyMarkdown(context.Context, string, []byte, []byte) error
	SimpleReplyText(context.Context, string, []byte) error
}

type connectMediaClient interface {
	downloadMessageFile(context.Context, string, string) (string, error)
	downloadMessageFileNamed(context.Context, string, string, string) (string, error)
	downloadRecoveredChatRecordFile(context.Context, fileInboundInfo) (string, error)
	getUserUnionID(context.Context, string) (string, error)
	downloadDentryFile(context.Context, int64, int64, string, string) (string, error)
}

var (
	newConnectStreamClient = func(clientID, clientSecret string, keepAlive time.Duration) connectStreamClient {
		return client.NewStreamClient(
			client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
			client.WithAutoReconnect(true),
			client.WithKeepAlive(keepAlive),
		)
	}
	newConnectChatReplier = func() connectChatReplier { return chatbot.NewChatbotReplier() }
	newConnectMediaClient = func(clientID, clientSecret string) connectMediaClient {
		return newAICardClient(clientID, clientSecret, "")
	}
)

type streamingForwarder interface {
	forwarder
	canStream() bool
	forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error)
}

// connectMediaAttachment is a DingTalk attachment that has already been
// authenticated and downloaded by the common Stream ingress. Keeping the
// attachment separate from the textual prompt lets multimodal backends pass
// the original bytes through their native protocol instead of asking the
// model to infer a local path from prose.
type connectMediaAttachment struct {
	LocalPath string
	FileName  string
	MediaType string
}

// attachmentForwarder is implemented by backends with a native attachment
// transport (for example OpenCode file parts or Gemini inlineData). Backends
// without one still receive the absolute local path in the prompt and can use
// their read tool, preserving compatibility with custom agents.
type attachmentForwarder interface {
	forwardWithAttachments(ctx context.Context, convID, text string, attachments []connectMediaAttachment) (string, error)
}

type streamingAttachmentForwarder interface {
	attachmentForwarder
	canStream() bool
	forwardStreamWithAttachments(ctx context.Context, convID, text string, attachments []connectMediaAttachment, onDelta func(string)) (string, error)
}

func forwardConnectTurn(ctx context.Context, fwd forwarder, convID, prompt string, attachments []connectMediaAttachment, onDelta func(string)) (string, error) {
	if af, ok := fwd.(streamingAttachmentForwarder); ok {
		return af.forwardStreamWithAttachments(ctx, convID, prompt, attachments, onDelta)
	}
	if af, ok := fwd.(attachmentForwarder); ok {
		return af.forwardWithAttachments(ctx, convID, prompt, attachments)
	}
	if sf, ok := fwd.(streamingForwarder); ok {
		return sf.forwardStream(ctx, convID, prompt, onDelta)
	}
	return fwd.forward(ctx, convID, prompt)
}

// sessionResetter is an optional capability: a forwarder that can forget a
// conversation's agent session, so a built-in /new or /clear command starts a
// fresh context. Forwarders with per-conversation memory (Claude-family exec,
// codex app-server) implement it; a stateless channel does not, and the main
// loop tells the user the command is unsupported there.
type sessionResetter interface {
	resetSession(convID string)
}

// sessionClearer is an optional capability beyond sessionResetter: a forwarder
// that can actively dispose a conversation's session on the agent side (e.g.
// opencode's DELETE /session/:id), so /clear truly wipes it instead of only
// forgetting the local id mapping. Channels whose agent exposes no delete in
// the mode DWS drives it fall back to sessionResetter, so /clear behaves like
// /new there.
type sessionClearer interface {
	clearSession(ctx context.Context, convID string) error
}

type forwarderCloser interface {
	close() error
}

// connectAgentOptions carries the user-facing agent tuning exposed on
// `dev connect` (and mirrored env vars). Zero value = defaults:
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
	// your app in the developer console for reliable branded rendering. Empty
	// means no cards — replies stay plain text/markdown with the Thinking/Done
	// chips (see newAICardClient); pass "public" to opt into the shared
	// openclaw template (best-effort: shared templates may not render for
	// every app).
	CardTemplate string
	// KnowledgeDir is a local directory of .md/.txt answering material
	// (--knowledge-dir / DWS_KNOWLEDGE_DIR): per-message top-k retrieval is
	// prepended to the prompt while the agent keeps running from the clean
	// scratch dir (see connect_knowledge.go). Empty = off.
	KnowledgeDir string
	// KnowledgeSource is a typed knowledge source (--knowledge-source):
	// "wiki:<spaceId>" / "doc:<docId>" pulls a DingTalk knowledge base on
	// startup, caches it locally, and feeds it into the same retriever as
	// KnowledgeDir (see connect_knowledge_wiki.go). A bare value is treated as
	// a local directory. Empty = off; coexists with KnowledgeDir.
	KnowledgeSource string
	// AllowedUsers / AllowedGroups are staffId / openConversationId
	// allowlists (--allowed-users / --allowed-groups, comma-separated; env
	// DWS_ALLOWED_USERS / DWS_ALLOWED_GROUPS). Empty = everyone.
	AllowedUsers  []string
	AllowedGroups []string
	// UserRateLimit caps messages per sender per minute
	// (--user-rate-limit / DWS_USER_RATE_LIMIT); 0 = unlimited.
	UserRateLimit int
	// OwnerUserID is the digital-twin owner's staffId (--owner-user-id /
	// DWS_OWNER_USER_ID). When set together with an interactive-card template
	// (ApprovalCardTemplate), the confirmation gate is active: action requests
	// are routed to this user for [Approve]/[Reject] before executing. Empty =
	// gate off (plain Q&A).
	OwnerUserID string
	// ApprovalCardTemplate is the interactive-card template ID for the approval
	// card (--approval-card-template / DWS_APPROVAL_CARD_TEMPLATE). Required for
	// the gate to render [Approve]/[Reject] buttons; empty = gate off.
	ApprovalCardTemplate string
	// RoleConfigPath is a digital-employee role YAML (--role-config /
	// DWS_ROLE_CONFIG). When set, the role's owner / persona / knowledge sources
	// fill the corresponding options that were not given explicitly (an explicit
	// flag/env always wins). The role's client_id must match the connecting bot.
	// See connect_role.go for the schema. Empty = no role profile.
	RoleConfigPath string
	// Persona is a system-prompt fragment prepended to every forwarded prompt to
	// shape the bot's tone/expertise. Normally sourced from the role config's
	// `persona`; there is no standalone flag for it. Empty = no persona prefix.
	Persona string
	// KnowledgeSources are additional typed knowledge sources (same grammar as
	// KnowledgeSource) merged into the retriever on startup. Sourced from the
	// role config's `knowledge_sources`; each is loaded and its chunks appended
	// to the same knowledge base as KnowledgeDir/KnowledgeSource. Empty = none.
	KnowledgeSources []string
	// AuditSheetNode / AuditSheetTab point the approval gate's audit trail at a
	// DingTalk online sheet (axls): one row per action (--audit-sheet /
	// DWS_AUDIT_SHEET is the doc id/URL, --audit-sheet-tab the worksheet, default
	// "Sheet1"). Empty AuditSheetNode = local approvals JSON only.
	AuditSheetNode string
	AuditSheetTab  string
	// RoleScopes is the role's capability allowlist (RoleConfig.AllowedScopes),
	// e.g. ["todo", "approval"]. When non-empty the approval gate refuses an
	// action whose product is not in the list, so a role stays in its lane (an
	// HR assistant can't touch code/drive). Empty = no scope restriction.
	RoleScopes []string
	// ConfirmPolicy is the role's confirmation strategy for OTHERS' action
	// requests (the owner's own requests always auto-run): "manual" asks the
	// owner every time, "auto" runs without asking (full trust, still audited),
	// "remember" asks once per action kind then reuses that decision. Sourced
	// from RoleConfig.confirm_policy; empty = manual.
	ConfirmPolicy string
	// Timeout caps each agent turn (--agent-timeout seconds /
	// DWS_AGENT_TIMEOUT_MS milliseconds). 0 = no limit (default).
	Timeout time.Duration
	// Yolo enables highest-permission mode for the agent. It is the default for
	// dev connect; pass --agent-permission-mode ask or --agent-approval-mode ask
	// to opt into the restricted/confirmation mode. Each channel maps yolo to
	// its own permission flag:
	// Claude Code / codebuddy / workbuddy get --dangerously-skip-permissions;
	// Codex switches sandbox from read-only to workspace-write; Qoder
	// re-enables skills and user settings. Use with caution.
	Yolo bool
}

// isStreamBridgeChannel reports whether a channel is wired through the Go-native
// Stream + exec forwarder path (i.e. it has an agent spec). openclaw (external
// connector) and hermes (official channel) are not.
func isStreamBridgeChannel(channel string) bool {
	_, ok := agentSpecs[channel]
	return ok
}

// execForwarder invokes a local agent CLI: fixed argv plus the message text as
// the trailing argument, returning stdout. Used by claudecode / codebuddy and
// generic/custom one-shot channels.
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

// resetSession drops the conversation's agent session so the next message
// starts a fresh one. Implements sessionResetter for the built-in /new and
// /clear commands. A no-op when this channel has no addressable sessions
// (sessions == nil).
func (f *execForwarder) resetSession(convID string) {
	if f.sessions != nil && strings.TrimSpace(convID) != "" {
		f.sessions.reset(convID)
	}
}

func (f *execForwarder) label() string {
	memo := "stateless"
	if f.sessions != nil {
		memo = "session-memory"
	}
	return fmt.Sprintf("exec:%s (%s, %s)", f.name, f.argv[0], memo)
}

func (f *execForwarder) hasSession(convID string) bool {
	return f.sessions != nil && strings.TrimSpace(convID) != ""
}

func (f *execForwarder) commandArgs(argv []string, convID, text string) []string {
	// Session args go right after the binary, before the spec tail — some specs
	// (qoder) end the tail with `-p` so the prompt must stay the trailing
	// positional argument.
	var args []string
	if f.hasSession(convID) {
		args = append(args, f.sessions.args(convID)...)
	}
	args = append(args, argv[1:]...)
	args = append(args, text)
	return args
}

func (f *execForwarder) configureCommand(cmd *exec.Cmd) {
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
}

func (f *execForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	ctx, cancel := applyTimeout(ctx, f.timeout)
	defer cancel()

	run := func() (string, string, error) {
		args := f.commandArgs(f.argv, convID, text)
		cmd := exec.CommandContext(ctx, f.argv[0], args...)
		f.configureCommand(cmd)
		out, err := cmd.Output()
		s := strings.TrimSpace(string(out))
		// Guard against a backend error being mistaken for the answer: some agent
		// CLIs (claude) print "API Error: 4xx ..." to stdout and still exit 0, so a
		// non-empty stdout is not proof of a real reply. If stdout is a bare backend
		// error, return an actionable hint instead of forwarding the raw error into
		// the chat (issue #14: a custom-provider 422 was echoed to the group).
		if s != "" && !agentReplyIsError(s) {
			return brandReply(f.name, s), "", nil
		}
		if s != "" && agentReplyIsError(s) {
			if f.hasSession(convID) {
				f.sessions.reset(convID)
			}
			return agentBackendErrorReply(s), "", nil
		}
		if err != nil {
			msg := execErrorMessage(err)
			return "", msg, err
		}
		return "（本地 agent 无文本输出）", "", nil
	}

	reply, msg, err := run()
	if err == nil {
		return reply, nil
	}

	if f.hasSession(convID) && agentSessionMissingError(msg) {
		f.sessions.reset(convID)
		reply, msg, err = run()
		if err == nil {
			return reply, nil
		}
	}

	if f.hasSession(convID) {
		// Self-heal session state: if this conversation's session is broken
		// (e.g. --resume of a session that was never created or got cleaned),
		// drop the mapping so the next message starts a fresh session instead
		// of failing forever.
		f.sessions.reset(convID)
	}
	return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
}

// forwardWithAttachments grants the Claude-family CLIs read-only access to the
// exact directories that contain this turn's downloaded attachments. These
// agents otherwise run from an isolated scratch directory, so an absolute path
// in prose can still be rejected by their external-directory permission gate.
// The custom channel is intentionally left untouched because DWS cannot assume
// flags understood by an arbitrary user command.
func (f *execForwarder) forwardWithAttachments(ctx context.Context, convID, text string, attachments []connectMediaAttachment) (string, error) {
	switch f.name {
	case "claudecode", "codebuddy", "workbuddy":
	default:
		return f.forward(ctx, convID, text)
	}
	seen := make(map[string]struct{})
	var dirs []string
	for _, attachment := range attachments {
		path := strings.TrimSpace(attachment.LocalPath)
		if path == "" {
			continue
		}
		dir := filepath.Dir(path)
		if _, exists := seen[dir]; exists {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	if len(dirs) == 0 {
		return f.forward(ctx, convID, text)
	}
	clone := *f
	clone.argv = append([]string{f.argv[0]}, "--allowedTools", "Read")
	for _, dir := range dirs {
		clone.argv = append(clone.argv, "--add-dir", dir)
	}
	clone.argv = append(clone.argv, f.argv[1:]...)
	return clone.forward(ctx, convID, text)
}

// convSessions maps a DingTalk conversation to a stable agent session ID, so a
// channel CLI with addressable sessions keeps multi-turn context per chat.
// First message of a conversation mints a UUID and passes `--session-id <id>`
// (create); subsequent messages pass `--resume <id>` (continue). Claude Code,
// codebuddy and qodercli share these exact flags in one-shot CLI mode. Qoder's
// persistent stream-json mode reuses the same UUID values as JSON session_id.
//
// The map is persisted to disk (path, scoped per clientId) so a connector
// restart resumes every chat's context instead of starting fresh — the whole
// point of an always-on digital employee. An empty path keeps the map purely
// in memory (persistence disabled, e.g. --agent-memory off or no clientId),
// preserving the original behaviour exactly. Persistence is best-effort: a
// failed save only logs a warning and never blocks message handling.
type convSessions struct {
	mu   sync.Mutex
	m    map[string]string
	path string // on-disk store; empty disables persistence
}

// newConvSessions builds the session map, restoring any persisted state from
// path. A missing or corrupt file degrades to an empty map (see
// loadConvSessionMap) — it never panics or blocks startup. An empty path means
// in-memory only.
func newConvSessions(path string) *convSessions {
	return &convSessions{m: loadConvSessionMap(path), path: path}
}

// persistLocked writes the current map to disk. The caller must hold s.mu, so
// the snapshot marshalled here is race-free and the write is serialized with
// every other map mutation. It is best-effort (see saveConvSessionMap).
func (s *convSessions) persistLocked() {
	saveConvSessionMap(s.path, s.m)
}

func convSessionKey(convID string) string {
	key := strings.TrimSpace(convID)
	if key == "" {
		key = "_default"
	}
	return key
}

// id returns the stable agent session ID for one conversation, minting and
// persisting one on first sight. It is used by transports that carry the session
// id in a JSON field rather than argv flags.
func (s *convSessions) id(convID string) string {
	key := convSessionKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.m[key]; ok {
		return id
	}
	id := uuid.NewString()
	s.m[key] = id
	s.persistLocked()
	return id
}

// args returns the session argv fragment for one conversation, minting a new
// session on first sight. A newly minted mapping is persisted so a restart can
// resume it.
func (s *convSessions) args(convID string) []string {
	key := convSessionKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.m[key]; ok {
		return []string{"--resume", id}
	}
	id := uuid.NewString()
	s.m[key] = id
	s.persistLocked()
	return []string{"--session-id", id}
}

// reset forgets a conversation's session so the next message starts a fresh
// one (a new UUID — the old one may or may not exist on the agent side, and a
// fresh ID is safe either way). The removal is persisted so a restart does not
// resurrect the dropped session.
func (s *convSessions) reset(convID string) {
	key := convSessionKey(convID)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	s.persistLocked()
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

func execErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		msg = strings.TrimSpace(string(ee.Stderr))
	}
	return msg
}

// agentReplyIsError reports whether an agent's stdout is a bare backend error
// rather than a real answer. Claude Code prints provider failures as
// "API Error: <status> ..." on stdout and may still exit 0, so the connector
// cannot rely on exit code / non-empty stdout alone. This is a heuristic on the
// leading marker; a legitimate reply never starts with "API Error:".
func agentReplyIsError(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "API Error:")
}

// agentSessionMissingError recognizes stale addressable sessions. Agent CLIs
// differ in wording, but the operational meaning is the same: the connector's
// persisted session id points at a conversation the CLI can no longer resume.
func agentSessionMissingError(msg string) bool {
	lower := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(lower, "no conversation found with session id") ||
		strings.Contains(lower, "conversation not found") ||
		strings.Contains(lower, "session not found")
}

// agentBackendErrorReply turns a bare backend error into a short, actionable
// Chinese message for the chat, instead of echoing the raw provider error
// (issue #14). It keeps only the first line of the raw error, truncated.
func agentBackendErrorReply(raw string) string {
	first := strings.TrimSpace(raw)
	if i := strings.IndexByte(first, byte('\n')); i >= 0 {
		first = strings.TrimSpace(first[:i])
	}
	return fmt.Sprintf("AI 后端调用失败（原始错误：%s）。如果你用的是自定义模型供应商，请用 --agent-model <你供应商支持的模型名> 指定模型后重连。",
		truncateRunes(first, 160))
}

// providerBaseURLInjected reports whether a custom Anthropic-compatible provider
// base URL is in effect for the claude subprocess: either injected via the
// user's Claude settings (env, see claudeUserSettingsEnv) or already present in
// the connector's own environment. When it is, the built-in haiku pin must be
// dropped so the provider's default model is used (it may not map haiku).
func providerBaseURLInjected(env []string) bool {
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok && strings.TrimSpace(k) == "ANTHROPIC_BASE_URL" {
			return true
		}
	}
	if v, ok := os.LookupEnv("ANTHROPIC_BASE_URL"); ok && strings.TrimSpace(v) != "" {
		return true
	}
	return false
}

// stripModelArg removes a flag and its value from argv (the inverse of the
// insert branch in applyModelArg). Used to drop the built-in --model haiku pin
// when a custom provider is in effect and the user did not pick a model, so the
// provider's default model applies. No-op when flag is absent.
func stripModelArg(argv []string, flag string) []string {
	for i := 1; i+1 < len(argv); i++ {
		if argv[i] == flag {
			out := append([]string(nil), argv[:i]...)
			return append(out, argv[i+2:]...)
		}
	}
	return append([]string(nil), argv...)
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

// claudeUserSettingsEnv re-exposes the `env` block of the user-level Claude
// Code settings ($CLAUDE_CONFIG_DIR/settings.json, default ~/.claude) as
// process environment entries. The claudecode channel runs claude with
// `--setting-sources project` to keep the bot persona neutral (no operator
// hooks/plugins), but that also drops user settings — and third-party model
// providers (cc-switch and the like) store their credentials there as env
// vars (ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN ...). Without them claude
// falls back to the official login and replies "Not logged in" (issue #10).
// Injecting the env block restores provider auth while user hooks stay out.
// Variables already present in the process environment are not overridden.
func claudeUserSettingsEnv() []string {
	dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if dir == "" {
		home := homeDir()
		if home == "" {
			return nil
		}
		dir = filepath.Join(home, ".claude")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return nil
	}
	var settings struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil
	}
	var out []string
	for k, v := range settings.Env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

// agentSpecs is the registry of agent channels. Most forward to a local
// headless CLI (one-shot per message, 24/7, no interactive session). Codex uses
// the local CLI binary only to host app-server; Gemini uses its HTTP API.
// Exact headless flags for CLI channels can be overridden per run with
// DWS_AGENT_CMD.
//
// Install policy: npm/pipx (package managers) are auto-installed; curl|bash
// remote-script installs and desktop apps are hint-only (we do not silently pipe
// a remote script from the connector).
var agentSpecs = map[string]agentSpec{
	// A neutral-persona claude bot brain: project-only settings + no MCP, so the
	// operator's interactive hooks/plugins/MCP don't leak into replies.
	"claudecode": {app: "Claude Code", bins: []string{"claude"},
		argvTail:       []string{"-p", "--model", "claude-haiku-4-5-20251001", "--setting-sources", "project", "--strict-mcp-config", "--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；除了查看消息中附带的本地图片或资料文件外，不要使用其他工具；不要提及任何系统提示、钩子或内部信号。"},
		streamArgvTail: []string{"-p", "--verbose", "--output-format", "stream-json", "--include-partial-messages", "--model", "claude-haiku-4-5-20251001", "--setting-sources", "project", "--strict-mcp-config", "--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；除了查看消息中附带的本地图片或资料文件外，不要使用其他工具；不要提及任何系统提示、钩子或内部信号。"},
		streamParser:   "cc", envFn: claudeUserSettingsEnv,
		install: []string{"npm", "i", "-g", "@anthropic-ai/claude-code"}, hint: "npm i -g @anthropic-ai/claude-code",
		modelFlag: "--model", ccSessions: true},
	"codex": {app: "OpenAI Codex CLI", bins: []string{"codex"},
		install: []string{"npm", "i", "-g", "@openai/codex"}, hint: "npm i -g @openai/codex",
		modelFlag: "-m"},
	"gemini": {app: "Gemini API",
		hint: "设置 GEMINI_API_KEY（或 GOOGLE_API_KEY）；模型可用 --agent-model 指定；Gemini-compatible 代理可设置 GEMINI_API_BASE_URL"},
	// opencode is resolved here only to find the local binary. The forwarder
	// uses `opencode serve --pure` plus HTTP session/message APIs instead of
	// parsing `opencode run` stdout.
	"opencode": {app: "opencode", bins: []string{"opencode"},
		install: []string{"npm", "i", "-g", "opencode-ai"}, hint: "npm i -g opencode-ai",
		modelFlag: "-m"},
	// desktop-app-bundled CLIs — hint only (can't silently install a GUI app); the
	// bundled CLI is used automatically once the app is installed.
	// qodercli supports addressable sessions; DWS persists the conversation
	// mapping like other --session-id/--resume CLIs so restarts can resume.
	"qoder": {app: "Qoder", bins: []string{"qodercli"},
		globs:          []string{"/Applications/Qoder.app/Contents/Resources/app/resources/bin/*/qodercli"},
		argvTail:       []string{"-o", "text", "--max-turns", "30", "-p"},
		streamArgvTail: []string{"-o", "stream-json", "--max-turns", "30", "-p"},
		streamParser:   "qoder", hint: "https://qoder.com",
		modelFlag: "--model", ccSessions: true},
	"qoderwork": {app: "QoderWork", bins: []string{"qodercli"},
		globs:          []string{"/Applications/QoderWork.app/Contents/Resources/bin/qodercli"},
		argvTail:       []string{"-o", "text", "--max-turns", "30", "-p"},
		streamArgvTail: []string{"-o", "stream-json", "--max-turns", "30", "-p"},
		streamParser:   "qoder", hint: "https://qoder.com",
		modelFlag: "--model", ccSessions: true},
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
			"你叫「WorkBuddy 助手」，是钉钉群里的智能助手。无论被问到你是谁，都只能自称 WorkBuddy 助手，绝不能提到 CodeBuddy 这个名字。请用简洁自然的中文直接回答问题；不要主动使用工具、读写文件或执行命令。仅当用户消息明确附带了本地附件路径时，可以使用 Read 工具只读该附件，不得访问其它文件。",
			"-p"},
		streamArgvTail: []string{"--append-system-prompt",
			"你叫「WorkBuddy 助手」，是钉钉群里的智能助手。无论被问到你是谁，都只能自称 WorkBuddy 助手，绝不能提到 CodeBuddy 这个名字。请用简洁自然的中文直接回答问题；不要主动使用工具、读写文件或执行命令。仅当用户消息明确附带了本地附件路径时，可以使用 Read 工具只读该附件，不得访问其它文件。",
			"-p", "--output-format", "stream-json", "--include-partial-messages"},
		streamParser: "cc", envFn: codebuddyEnv, hint: "https://www.codebuddy.cn/work/",
		modelFlag: "--model", ccSessions: true},
	// custom: an escape hatch for self-built / not-yet-supported agent CLIs
	// (issue #37, e.g. 网易有道龙虾 LobsterAI). It has NO built-in binary — the
	// full command comes from --agent-cmd (which sets DWS_AGENT_CMD) or
	// DWS_AGENT_CMD directly. dws starts the Stream and forwards each @-bot
	// message to that command with the question appended as the trailing
	// argument, using stdout as the reply. One-shot only (no streaming/session/
	// model override, since the CLI's protocol is unknown), so any headless AI
	// tool can be onboarded without code changes.
	"custom": {app: "自定义 AI 工具（--agent-cmd / DWS_AGENT_CMD）", bins: nil,
		hint: "用 --agent-cmd \"<可执行命令>\" 指定你的 AI 工具（无头/一次性模式：问题作为最后一个参数追加，答案打到 stdout），或设环境变量 DWS_AGENT_CMD"},
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
	case "custom":
		// custom has no built-in binary; "installed" means a command was supplied
		// via --agent-cmd / DWS_AGENT_CMD.
		command := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD"))
		status := map[string]any{
			"required":    "自定义命令（--agent-cmd / DWS_AGENT_CMD）",
			"installed":   command != "",
			"autoInstall": false,
			"installHint": "用 --agent-cmd \"<可执行命令>\" 指定你的 AI 工具",
		}
		if command != "" {
			status["command"] = command
		}
		return status
	case "gemini":
		status := map[string]any{
			"required":    "GEMINI_API_KEY or GOOGLE_API_KEY",
			"installed":   geminiAPIKey() != "",
			"autoInstall": false,
			"installHint": "设置 GEMINI_API_KEY（或 GOOGLE_API_KEY）；模型可用 --agent-model 指定；Gemini-compatible 代理可设置 GEMINI_API_BASE_URL",
		}
		if base := strings.TrimSpace(geminiAPIBaseURL()); base != "" {
			status["baseURL"] = base
		}
		return status
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
// Order for non-Codex channels: DWS_AGENT_CMD override > binary on
// PATH/app-bundle > auto-install (pkg managers) > install-guidance error. Codex
// ignores DWS_AGENT_CMD because its channel is app-server only. Preflighted at
// connect time, not on the first DingTalk message.
func resolveExecAgent(channel string) (argv []string, env []string, err error) {
	if v := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD")); v != "" && channel != "codex" {
		return strings.Fields(v), nil, nil
	}
	if channel == "custom" {
		// custom has no built-in binary: its argv MUST come from --agent-cmd /
		// DWS_AGENT_CMD (handled above). Reaching here means neither was set.
		return nil, nil, apperrors.NewValidation("custom 渠道需要用 --agent-cmd \"<可执行命令>\"（或环境变量 DWS_AGENT_CMD）指定你的 AI 工具：无头/一次性模式，用户问题作为最后一个参数追加，回答打到 stdout")
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
func forwarderForChannel(channel, clientID string, opts connectAgentOptions) (forwarder, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = envDurationMS("DWS_AGENT_TIMEOUT_MS", 0)
	}
	spec, ok := agentSpecs[channel]
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 不是 stream-bridge 渠道，无 forwarder", channel))
	}
	overridden := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD")) != "" && channel != "codex"
	if channel == "gemini" && !overridden {
		return newGeminiAPIForwarder(timeout, opts)
	}
	// Resolve the agent CLI (PATH → app bundle → auto-install → guidance) and
	// preflight here so a missing dependency errors at connect time.
	argv, env, err := resolveExecAgent(channel)
	if err != nil {
		return nil, err
	}
	// A DWS_AGENT_CMD override is a fully user-controlled argv: do not splice in
	// model or session flags we cannot know are valid for it. Codex ignores the
	// override because its channel is app-server only; custom commands belong on
	// --channel custom.
	userPickedModel := opts.Model != "" || strings.TrimSpace(os.Getenv("DWS_AGENT_MODEL")) != ""
	if !overridden && opts.Model != "" {
		if spec.modelFlag == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 的 agent CLI 不支持模型覆盖（--agent-model）", channel))
		}
		argv = applyModelArg(argv, spec.modelFlag, opts.Model)
	}
	// Custom-provider default-model fix (issue #14): when a third-party
	// Anthropic-compatible provider is in effect (ANTHROPIC_BASE_URL injected via
	// the user's Claude settings or already in our env) and the user did not pick
	// a model, drop the spec's built-in model pin (claudecode's haiku) so the
	// provider's default model applies — the provider may not map that exact pin
	// and would otherwise return an error (a 422 that got echoed into the chat).
	// Official-login users (no base URL) keep the built-in pin unchanged.
	dropBuiltinModel := !overridden && !userPickedModel && spec.modelFlag != "" && providerBaseURLInjected(env)
	if dropBuiltinModel {
		argv = stripModelArg(argv, spec.modelFlag)
	}
	var sessions *convSessions
	if !overridden && opts.Memory && spec.ccSessions {
		// Scope the on-disk session store by clientId so multiple bots on one
		// machine stay isolated; an empty clientId disables persistence and the
		// map stays in memory.
		sessionStore := connectSessionStorePath(clientID)
		sessions = newConvSessions(sessionStore)
	}
	// Incremental-output argv: same binary, the spec's stream tail, same model
	// override. A DWS_AGENT_CMD override disables streaming (unknown argv).
	var streamArgv []string
	parser := ""
	if !overridden && len(spec.streamArgvTail) > 0 {
		streamArgv = append([]string{argv[0]}, spec.streamArgvTail...)
		if opts.Model != "" && spec.modelFlag != "" {
			streamArgv = applyModelArg(streamArgv, spec.modelFlag, opts.Model)
		} else if dropBuiltinModel {
			streamArgv = stripModelArg(streamArgv, spec.modelFlag)
		}
		parser = spec.streamParser
	}
	if channel == "codex" {
		return newCodexAppServerForwarder(argv[0], env, timeout, opts, clientID), nil
	}
	// opencode uses its official local HTTP server; DWS keeps the
	// conversation→session mapping and sends one-shot group replies.
	if channel == "opencode" && !overridden {
		return newOpencodeForwarder(argv[0], env, timeout, opts, clientID), nil
	}
	if (channel == "qoder" || channel == "qoderwork") && !overridden {
		return newQoderStreamForwarder(channel, argv[0], env, timeout, opts, sessions), nil
	}
	// Yolo: splice per-agent permission flags for exec-based forwarders.
	if !overridden && opts.Yolo {
		switch channel {
		case "claudecode", "codebuddy", "workbuddy":
			argv = append(argv, "--permission-mode", "bypassPermissions", "--dangerously-skip-permissions")
			if len(streamArgv) > 0 {
				streamArgv = append(streamArgv, "--permission-mode", "bypassPermissions", "--dangerously-skip-permissions")
			}
		}
	}
	base := &execForwarder{name: channel, argv: argv, env: env, timeout: timeout,
		workDir: opts.WorkDir, sessions: sessions,
		streamArgv: streamArgv, parser: parser}
	return base, nil
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
// connectExtras carries the optional Q&A hardening features into the stream
// connector; nil/zero fields are off.
type connectExtras struct {
	gate     *connectGate
	kb       *knowledgeBase
	approval *approvalOrchestrator
	// onTurnDone is an optional lifecycle observer invoked after one queued
	// batch has completely finished. It is nil in production and lets focused
	// tests wait for ack-first work without timing sleeps.
	onTurnDone func()
	// persona is a role's system-prompt fragment prepended to every forwarded
	// prompt (empty = no prefix). Sourced from RoleConfig.Persona.
	persona string
}

type connectQueuedTurn struct {
	convID            string
	text              string
	picCodes          []string
	fileInfos         []fileInboundInfo
	chatRecordLookups []chatRecordLookup
	webhook           string
	msgID             string
	msgType           string
	senderStaffID     string
	conversationID    string
	conversationType  string
	callbackData      chatbot.BotCallbackDataModel
}

func mergeConnectQueuedTurns(turns []connectQueuedTurn) connectQueuedTurn {
	if len(turns) == 0 {
		return connectQueuedTurn{}
	}
	if len(turns) == 1 {
		return turns[0]
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if connectTurnShouldStayStandalone(turns[i]) {
			return turns[i]
		}
	}
	merged := turns[len(turns)-1]
	lines := make([]string, 0, len(turns)+1)
	lines = append(lines, "用户在上一轮处理期间连续发送了以下消息，请把它们作为同一个最新请求一起处理：")
	for i, turn := range turns {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, connectTurnSummary(turn)))
	}
	merged.text = strings.Join(lines, "\n")
	merged.picCodes = nil
	merged.fileInfos = nil
	merged.chatRecordLookups = nil
	for i := range turns {
		merged.picCodes = append(merged.picCodes, turns[i].picCodes...)
		merged.fileInfos = append(merged.fileInfos, turns[i].fileInfos...)
		merged.chatRecordLookups = append(merged.chatRecordLookups, turns[i].chatRecordLookups...)
	}
	return merged
}

func connectTurnShouldStayStandalone(turn connectQueuedTurn) bool {
	if _, ok := parseConnectControlCommand(turn.text); ok {
		return true
	}
	if _, ok := parseDecisionWord(turn.text); ok {
		return true
	}
	return isRetryWord(turn.text)
}

func connectTurnSummary(turn connectQueuedTurn) string {
	if text := strings.TrimSpace(turn.text); text != "" {
		if len(turn.picCodes) > 0 {
			return text + " [同时附有图片]"
		}
		return text
	}
	if len(turn.picCodes) > 0 {
		return "[图片]"
	}
	if len(turn.fileInfos) > 0 {
		if len(turn.fileInfos) == 1 {
			if name := strings.TrimSpace(turn.fileInfos[0].FileName); name != "" {
				return "[附件: " + name + "]"
			}
		}
		return fmt.Sprintf("[%d 个附件]", len(turn.fileInfos))
	}
	return "[空消息]"
}

func connectApprovalGroupReply(replier connectChatReplier, webhook, channel string) approvalReplier {
	return func(ctx context.Context, _ string, text string) error {
		if len([]rune(text)) > 200 {
			return replier.SimpleReplyMarkdown(ctx, webhook, []byte(channel), []byte(text))
		}
		return replier.SimpleReplyText(ctx, webhook, []byte(text))
	}
}

func runStreamConnector(ctx context.Context, channel, clientID, clientSecret string, fwd forwarder, cardCli *aiCardClient, extras *connectExtras) error {
	if extras == nil {
		extras = &connectExtras{}
	}
	checkFDLimit()
	if closer, ok := fwd.(forwarderCloser); ok {
		defer func() {
			if err := closer.close(); err != nil {
				fmt.Fprintf(os.Stderr, "[connect] 关闭本地 agent 失败（忽略）: %v\n", err)
			}
		}()
	}
	// One connector per robot per machine — duplicate Stream connections on
	// one clientId get messages load-balanced between them (bot answers
	// intermittently), see acquireConnectLock.
	release, err := acquireConnectLock(clientID)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	defer release()

	// Health heartbeat: record connect/receive/reply/error so `connect status`
	// can tell a live connector from a dead one (see connect_health.go). Nil
	// when no clientId identity is available; all calls below are no-ops then.
	health := newConnectHealth(clientID, channel)
	health.start(ctx)

	streamLoggerOnce.Do(func() { sdklogger.SetLogger(streamSDKLogger{}) })
	replier := newConnectChatReplier()
	dedup := newMsgDedup(10000)
	queue := newConvQueue()
	// Media downloads need an authenticated API client even when cards are
	// off (aiCardClient with an empty template is creds+HTTP only).
	var mediaCli connectMediaClient
	if cardCli != nil {
		mediaCli = cardCli
	} else {
		mediaCli = newConnectMediaClient(clientID, clientSecret)
	}

	keepAlive := envDurationMS("DWS_CONNECT_KEEPALIVE_MS", 30*time.Second)
	fmt.Fprintf(os.Stderr, "[connect] keepAlive=%s autoReconnect=true\n", keepAlive)
	cli := newConnectStreamClient(clientID, clientSecret, keepAlive)
	cli.RegisterChatBotCallbackRouter(func(_ context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		text := strings.TrimSpace(data.Text.Content)
		msgtype := strings.TrimSpace(data.Msgtype)
		// Discover attachments by their locator fields, never by a msgtype
		// allowlist. msgtype is only a classification hint for the agent prompt.
		picCodes, fileInfos, unrecoverableCount := callbackInboundMedia(msgtype, data.Content)
		var chatRecordLookups []chatRecordLookup
		if unrecoverableCount > 0 {
			indexes := chatRecordUnknownIndexes(data.Content)
			if strings.TrimSpace(data.MsgId) != "" && len(indexes) > 0 {
				chatRecordLookups = append(chatRecordLookups, chatRecordLookup{
					MsgID:          strings.TrimSpace(data.MsgId),
					UnknownIndexes: indexes,
				})
				fmt.Fprintf(os.Stderr, "[connect][media] 转发记录中有 %d 条 unknownMsgType，将在 ACK 后补拉原始内容 (msgId=%s)\n", unrecoverableCount, data.MsgId)
			} else {
				fmt.Fprintf(os.Stderr, "[connect][media] 转发记录中有 %d 条 unknownMsgType，但缺少外层消息 ID，保留原始 JSON 降级处理\n", unrecoverableCount)
			}
		}
		// Structured-text fallback: DingTalk leaves data.Text.Content blank on
		// markdown / richText callbacks (the body ships in data.Content). Without
		// this, `dws chat message send --group ... --text ...` — which defaults
		// to msgType=markdown — hits the drop branch below and the bot looks
		// dead to the sender.
		if text == "" {
			// Forwarded records must keep their complete JSON, even if the outer
			// msgtype is renamed or a title-like field could be extracted as text.
			// Detect the record by payload shape rather than message type.
			if hasChatRecordPayload(data.Content) {
				text = rawCallbackPrompt(msgtype, data.Content)
			}
			// interactiveCard (a bot @-mentioning this bot) nests the body in
			// content.cardContent and carries the mention as its own leading
			// leaf; the leaf-aware extractor drops it so the agent gets the
			// clean instruction. Detection is based on the payload shape so a
			// renamed/new type is handled identically.
			if text == "" {
				text = extractInteractiveCardText(data.Content)
			}
			if text == "" {
				if fallback := extractCallbackText(data.Content); fallback != "" {
					text = fallback
				}
			}
			if text == "" {
				text = rawCallbackPrompt(msgtype, data.Content)
			}
		}
		if data.SessionWebhook == "" {
			// A session webhook is required for the fallback reply path. Message
			// payload shape is deliberately not filtered here: unknown and complex
			// types are forwarded as raw JSON for the backend model to interpret.
			fmt.Fprintf(os.Stderr, "[connect] 丢弃消息 msgtype=%q staffId=%s convId=%s msgId=%s content=%s (sessionWebhook 为空，无法回复)\n",
				msgtype, data.SenderStaffId, data.ConversationId, data.MsgId, summarizeContent(data.Content))
			return []byte(""), nil
		}
		// Drop redelivered duplicates so a retried message is not replied twice.
		if id := strings.TrimSpace(data.MsgId); id != "" && !dedup.first(id) {
			return []byte(""), nil
		}
		// Access policy (--allowed-users / --allowed-groups /
		// --user-rate-limit): denied messages are dropped with a log line —
		// replying to them would itself be a spend amplifier.
		if extras.gate.enabled() {
			if ok, reason := extras.gate.allow(
				strings.TrimSpace(data.SenderStaffId),
				strings.TrimSpace(data.ConversationType),
				strings.TrimSpace(data.ConversationId),
			); !ok {
				fmt.Fprintf(os.Stderr, "[connect] 已拦截消息（%s）: staffId=%s convId=%s\n",
					reason, data.SenderStaffId, data.ConversationId)
				return []byte(""), nil
			}
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
		health.onPush()
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
		msgID := strings.TrimSpace(data.MsgId)
		turn := connectQueuedTurn{
			convID:            convID,
			text:              text,
			picCodes:          picCodes,
			fileInfos:         fileInfos,
			chatRecordLookups: chatRecordLookups,
			webhook:           webhook,
			msgID:             msgID,
			msgType:           msgtype,
			senderStaffID:     strings.TrimSpace(data.SenderStaffId),
			conversationID:    strings.TrimSpace(data.ConversationId),
			conversationType:  strings.TrimSpace(data.ConversationType),
			callbackData:      *data,
		}
		// Same-conversation agent calls never run in parallel; messages received
		// while a turn is running are merged into one pending follow-up instead
		// of forming an unbounded stale FIFO.
		queue.submit(turn, func(turns []connectQueuedTurn) {
			if extras.onTurnDone != nil {
				defer extras.onTurnDone()
			}
			turn := mergeConnectQueuedTurns(turns)
			if len(turns) > 1 {
				fmt.Fprintf(os.Stderr, "[connect] 合并 %d 条待处理消息 (convId=%s, latestMsgId=%s)\n", len(turns), turn.convID, turn.msgID)
			}
			text := turn.text
			picCodes := turn.picCodes
			fileInfos := turn.fileInfos
			chatRecordLookups := turn.chatRecordLookups
			webhook := turn.webhook
			convID := turn.convID
			msgID := turn.msgID
			msgtype := turn.msgType
			callbackData := &turn.callbackData
			// Digital-twin text approval: if this is the OWNER replying
			// 「同意」/「拒绝」 (in their 1:1 chat with the bot) to the pending
			// request, route it to the gate (decide → execute/decline, with
			// private owner ack + requester outcome) instead of forwarding it to
			// the agent. Returns true only when it consumed the message.
			if extras.approval.handleOwnerDecision(context.Background(),
				strings.TrimSpace(callbackData.SenderStaffId), text) {
				return
			}
			// Built-in slash commands (/new, /clear): reset this conversation's
			// session and ack, instead of forwarding to the agent. Matches only
			// when the whole message is the command (parseConnectControlCommand),
			// so a normal question is unaffected. Costs no agent turn / tokens.
			if action, isCmd := parseConnectControlCommand(text); isCmd {
				ackText := action.ack
				if action.resetsSession() {
					cleared := false
					// /clear prefers a real server-side session delete when the
					// channel's agent supports one (opencode). /new always just
					// drops the local mapping so the old session stays resumable.
					if action.name == "clear" {
						if c, canClear := fwd.(sessionClearer); canClear {
							cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
							if err := c.clearSession(cctx, convID); err != nil {
								fmt.Fprintf(os.Stderr, "[connect] /clear 删除会话失败 (%s, msgId=%s): %v\n", channel, msgID, err)
							}
							cancel()
							cleared = true
						}
					}
					if !cleared {
						if r, canReset := fwd.(sessionResetter); canReset {
							r.resetSession(convID)
						} else {
							ackText = "当前渠道暂不支持会话指令（/new、/clear）。"
						}
					}
				}
				if serr := replier.SimpleReplyText(context.Background(), webhook, []byte(ackText)); serr != nil {
					fmt.Fprintf(os.Stderr, "[connect] 指令回复发送失败 (%s, msgId=%s): %v\n", channel, msgID, serr)
				}
				return
			}
			started := time.Now()
			// Assemble the forwarded prompt: resolve an attached picture (the
			// top Q&A inbound is an error screenshot), then knowledge-augment.
			prompt, attachments := assembleConnectTurnMedia(text, clientID, callbackData.SenderStaffId, mediaCli, picCodes, fileInfos, chatRecordLookups)
			originalAttachments := append([]connectMediaAttachment(nil), attachments...)
			defer cleanupConnectMediaAttachments(originalAttachments)
			prompt, attachments = prepareConnectForwarderAttachments(fwd, prompt, attachments)
			defer cleanupConnectMediaAttachments(attachments)
			if extras.kb != nil {
				prompt = extras.kb.augment(prompt)
			}
			// Role persona: prepend the role's system-prompt fragment so the bot
			// answers in its configured lane/tone. Sits above the knowledge and the
			// question; a no-op when no role config supplied one.
			if p := strings.TrimSpace(extras.persona); p != "" {
				prompt = p + "\n\n" + prompt
			}
			// Confirmation gate: when active, ask the agent to declare any
			// action it would take via a structured [[ACTION:...]] marker
			// instead of executing it, so the orchestrator can route the
			// action through the owner's approval card (see handleReply).
			gateOn := extras.approval.enabled()
			if gateOn {
				prompt = extras.approval.decorateForActionDetection(prompt)
			}
			// hermes-UX reply sequence (gateway/platforms/dingtalk.py):
			//   ① on receive: "🤔Thinking" reaction chip on the user's message
			//      (no card yet — the thinking phase is the chip, not a card);
			//   ② agent runs;
			//   ③ on done: deliver the AI card with the final content;
			//   ④ swap the chip to "🥳Done".
			// Reactions attach to the triggering message, but DingTalk rejects a
			// reaction on an interactiveCard message (a bot @-mentioning this bot)
			// with a 500 system.error — reactions are only supported on human
			// messages. Skip the chip for those turns so we don't fire a call the
			// platform always rejects; the reply itself is unaffected.
			canReact := !strings.EqualFold(msgtype, "interactiveCard")
			thinking := false
			if cardCli != nil && canReact {
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
			sf, streamable := fwd.(streamingForwarder)
			if cardCli != nil && cardCli.hasTemplate() && streamable && sf.canStream() && !gateOn {
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

			reply, err := forwardConnectTurn(context.Background(), fwd, convID, prompt, attachments, onDelta)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect] 转发失败 (%s, 耗时 %s): %v\n", channel, time.Since(started).Round(time.Millisecond), err)
				if errors.Is(err, context.DeadlineExceeded) {
					// Self-recovery: drop the conversation's session so the next
					// message starts fresh instead of reusing a stuck one.
					if r, ok := fwd.(sessionResetter); ok {
						r.resetSession(convID)
					}
					reply = fmt.Sprintf("（%s 回复超时，已自动重置会话，请重试。如需调整超时上限可用 --agent-timeout）", channel)
				} else {
					reply = fmt.Sprintf("（%s 调用失败：%v）", channel, err)
				}
				health.onError(err)
			} else {
				fmt.Fprintf(os.Stderr, "[connect] agent 已生成回复 (%s, 耗时 %s): %s\n", channel, time.Since(started).Round(time.Millisecond), truncateRunes(reply, 80))
				health.onReply()
			}

			// Confirmation gate orchestration: if the agent's reply declared
			// an action, this submits it, sends the owner an approval card,
			// blocks for the decision, executes on approve (or declines), and
			// posts the outcome into the group itself. handled==true means the
			// gate owns the reply and the normal delivery below is skipped.
			if err == nil && gateOn {
				groupReply := connectApprovalGroupReply(replier, webhook, channel)
				out, handled := extras.approval.handleReply(context.Background(), strings.TrimSpace(callbackData.SenderStaffId), convID, reply, groupReply)
				if handled {
					if thinking {
						cardCli.swapThinkingToDone(context.Background(), callbackData.ConversationId, msgID)
					}
					return
				}
				reply = out
			}

			delivered := false
			if cardCli != nil && cardCli.hasTemplate() {
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
						runConnectCardRepair(func() {
							cardCli.repair(context.Background(), cardInst, reply)
						})
					}
				}
			}

			if !delivered {
				// Fallback path: plain reply via the inbound sessionWebhook.
				// Retry up to 3 times with exponential backoff (1s, 2s, 4s)
				// to handle transient network errors (EOF, timeout).
				var sendErr error
				backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
				for attempt := 0; attempt <= len(backoffs); attempt++ {
					if len([]rune(reply)) > 200 {
						sendErr = replier.SimpleReplyMarkdown(context.Background(), webhook, []byte(channel), []byte(reply))
					} else {
						sendErr = replier.SimpleReplyText(context.Background(), webhook, []byte(reply))
					}
					if sendErr == nil {
						break
					}
					if attempt < len(backoffs) {
						fmt.Fprintf(os.Stderr, "[connect] 普通消息发送失败 (%s, attempt %d/%d, msgId=%s): %v，%v 后重试\n",
							channel, attempt+1, len(backoffs)+1, msgID, sendErr, backoffs[attempt])
						helperSleep(backoffs[attempt])
					}
				}
				if sendErr != nil {
					fmt.Fprintf(os.Stderr, "[connect] 普通消息发送失败（重试耗尽） (%s, msgId=%s): %v\n", channel, msgID, sendErr)
					delivered = false
				} else {
					fmt.Fprintf(os.Stderr, "[connect] 普通消息已发送 (%s, msgId=%s)\n", channel, msgID)
					delivered = true
				}
			}

			if thinking {
				if delivered {
					cardCli.swapThinkingToDone(context.Background(), callbackData.ConversationId, msgID)
				} else {
					fmt.Fprintf(os.Stderr, "[connect] 回复发送失败，保留思考表态不切换完成 (%s, msgId=%s)\n", channel, msgID)
				}
			}
		})
		return []byte(""), nil
	})

	// Confirmation gate: button taps on the owner's approval card arrive on the
	// same Stream long-connection (no public webhook). Register the card callback
	// router only when the gate is active; it routes [Approve]/[Reject] into
	// gate.Decide, which wakes the blocked Await in the forward goroutine.
	if extras.approval.enabled() {
		cli.RegisterCardCallbackRouter(func(ctx context.Context, creq *card.CardRequest) (*card.CardResponse, error) {
			return extras.approval.handleCardCallback(ctx, creq)
		})
	}

	if err := cli.Start(ctx); err != nil {
		return apperrors.NewInternal("stream 建连失败：" + err.Error())
	}
	defer cli.Close()
	health.onConnected()
	<-ctx.Done()
	return nil
}

var runConnectCardRepair = func(repair func()) {
	go repair()
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

// applyTimeout returns a context bounded by timeout when timeout > 0, or the
// original context unchanged when timeout == 0 (no limit). The returned cancel
// func is always safe to defer.
func applyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}

// truncateRunes truncates by rune so multi-byte characters are never split.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
