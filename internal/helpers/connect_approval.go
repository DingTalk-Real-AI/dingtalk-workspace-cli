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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// The "digital twin" confirmation gate. When someone @-mentions the bot with an
// *action* request ("create a todo for me"), the bot does NOT execute it: it
// records an ApprovalRequest, sends an interactive card with [Approve]/[Reject]
// buttons to the owner, and only runs the planned action after the owner taps
// Approve. A Reject (or timeout) declines without executing.
//
// This file owns the gate engine: the request/state model, the thread-safe
// store with crash-safe on-disk persistence (so a Pending request survives a
// connector restart), and the Submit/Decide/Await/Get API. Card delivery and
// the connector wiring live in connect_approval_card.go and connect_stream.go.

// approvalState is the request lifecycle: Pending → Approved|Rejected →
// Executed|Failed. A decision (Approve/Reject) is the owner's call; execution
// is what the orchestrator does afterwards on an approved request.
type approvalState string

const (
	approvalPending  approvalState = "pending"
	approvalApproved approvalState = "approved"
	approvalRejected approvalState = "rejected"
	approvalExecuted approvalState = "executed"
	approvalFailed   approvalState = "failed"
)

// plannedAction is the structured command the gate will run once approved. It
// is intentionally generic (product + tool + params) so it maps straight onto
// executor.NewHelperInvocation without the gate knowing about any specific
// command. For the M2 vertical slice the orchestrator only emits todo.create,
// but the shape already supports any helper tool.
type plannedAction struct {
	// Product/Tool/LegacyPath feed executor.NewHelperInvocation. Product is the
	// canonical product (e.g. "todo"); Tool is the RPC name (e.g.
	// "create_personal_todo"); LegacyPath is the human command path for logs.
	Product    string         `json:"product"`
	Tool       string         `json:"tool"`
	LegacyPath string         `json:"legacy_path,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

// ApprovalRequest is one pending/decided confirmation. It is the on-disk record
// too (marshalled as-is), so every field is JSON-tagged and self-describing.
type ApprovalRequest struct {
	ID        string        `json:"id"`
	Requester string        `json:"requester"` // staffId of who asked
	ConvID    string        `json:"conv_id"`   // conversation to reply into
	Summary   string        `json:"summary"`   // human-readable "what will happen"
	Action    plannedAction `json:"action"`    // structured command to run on approve
	State     approvalState `json:"state"`
	// OutTrackID is the delivered card's instance id, recorded so a button
	// callback that only carries the card id (not the approval id in its action
	// params) can still be mapped back to this request.
	OutTrackID string    `json:"out_track_id,omitempty"`
	DecidedBy  string    `json:"decided_by,omitempty"` // staffId who approved/rejected
	ExecErr    string    `json:"exec_err,omitempty"`   // failure detail when State=failed
	CreatedAt  time.Time `json:"created_at"`
	DecidedAt  time.Time `json:"decided_at,omitempty"`
}

// decided reports whether the owner has made a call (approved or rejected),
// regardless of any later execution outcome.
func (r *ApprovalRequest) decided() bool {
	switch r.State {
	case approvalApproved, approvalRejected, approvalExecuted, approvalFailed:
		return true
	}
	return false
}

// approved reports whether the owner approved (in any post-approval state).
func (r *ApprovalRequest) approved() bool {
	switch r.State {
	case approvalApproved, approvalExecuted, approvalFailed:
		return true
	}
	return false
}

// approvalGate is the thread-safe confirmation-gate store. It keeps every
// request in memory and mirrors each one to <config>/connect/<clientId>/
// approvals/<id>.json so a Pending request survives a connector restart.
// Persistence is best-effort and never blocks a decision: a write failure only
// logs (mirroring connect_sessions_store's contract). An empty clientId means
// in-memory only (used by tests).
type approvalGate struct {
	clientID string

	mu      sync.Mutex
	reqs    map[string]*ApprovalRequest
	waiters map[string]chan struct{} // id → closed when the request is decided
}

// newApprovalGate builds the gate and eagerly loads any persisted requests from
// disk (so a restart can still resolve a card tapped while the bot was down).
func newApprovalGate(clientID string) *approvalGate {
	g := &approvalGate{
		clientID: strings.TrimSpace(clientID),
		reqs:     make(map[string]*ApprovalRequest),
		waiters:  make(map[string]chan struct{}),
	}
	g.loadAll()
	return g
}

// approvalDir returns the on-disk directory for this gate's requests, or "" when
// persistence is disabled (empty clientId). The clientId is sanitized exactly
// like the session store / lock file so it is always filesystem-safe.
func (g *approvalGate) approvalDir() string {
	if g.clientID == "" {
		return ""
	}
	return filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(g.clientID), "approvals")
}

// Submit records a new request in the Pending state, persists it, and returns
// the stored request (with a generated ID and CreatedAt). A blank requester or
// summary is allowed — the gate does not police content, it only sequences the
// approval — but an empty action means "nothing to run on approve", which the
// orchestrator must guard before calling Submit.
func (g *approvalGate) Submit(req ApprovalRequest) *ApprovalRequest {
	g.mu.Lock()
	defer g.mu.Unlock()

	if strings.TrimSpace(req.ID) == "" {
		req.ID = uuid.NewString()
	}
	req.State = approvalPending
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	stored := req // copy
	g.reqs[stored.ID] = &stored
	g.persist(&stored)
	return &stored
}

// setOutTrackID records the delivered card instance id for a request so a
// callback that carries only the card id can be mapped back. No-op for unknown
// ids; persists on success.
func (g *approvalGate) setOutTrackID(id, outTrackID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.reqs[id]
	if !ok {
		return
	}
	r.OutTrackID = outTrackID
	g.persist(r)
}

// findByOutTrackID returns the request whose delivered card has the given
// instance id, used as the fallback association when a callback's action params
// did not carry the approval id. Returns nil when none match.
func (g *approvalGate) findByOutTrackID(outTrackID string) *ApprovalRequest {
	outTrackID = strings.TrimSpace(outTrackID)
	if outTrackID == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, r := range g.reqs {
		if r.OutTrackID == outTrackID {
			cp := *r
			return &cp
		}
	}
	return nil
}

// pendingForConv returns a snapshot of the most-recent still-Pending request in
// the given conversation, or nil when none is awaiting a decision there. It is
// the lookup the text-approval mode needs to map an owner's "同意/拒绝" reply
// (which only carries the conversation, not an approval id) back to its request.
func (g *approvalGate) pendingForConv(convID string) *ApprovalRequest {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var best *ApprovalRequest
	for _, r := range g.reqs {
		if r.State == approvalPending && r.ConvID == convID {
			if best == nil || r.CreatedAt.After(best.CreatedAt) {
				best = r
			}
		}
	}
	if best == nil {
		return nil
	}
	cp := *best
	return &cp
}

// Decide records the owner's call on a Pending request and wakes any Await.
// It is idempotent and race-safe: a second decision (e.g. a double-tap, or a
// reject after an approve) is ignored once the request has already been
// decided, and the function reports whether THIS call was the deciding one.
func (g *approvalGate) Decide(id string, approve bool, by string) (*ApprovalRequest, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	r, ok := g.reqs[id]
	if !ok {
		return nil, false
	}
	if r.decided() {
		cp := *r
		return &cp, false // already decided; not the deciding call
	}
	if approve {
		r.State = approvalApproved
	} else {
		r.State = approvalRejected
	}
	r.DecidedBy = strings.TrimSpace(by)
	r.DecidedAt = time.Now()
	g.persist(r)
	g.wake(id)
	cp := *r
	return &cp, true
}

// markExecuted / markFailed record the post-approval execution outcome. They do
// not gate or wake anything (Await already returned on the decision); they exist
// so the on-disk record reflects what actually happened, for audit and restart.
func (g *approvalGate) markExecuted(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.reqs[id]; ok && r.approved() {
		r.State = approvalExecuted
		g.persist(r)
	}
}

func (g *approvalGate) markFailed(id string, cause string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.reqs[id]; ok && r.approved() {
		r.State = approvalFailed
		r.ExecErr = truncateRunes(strings.TrimSpace(cause), 500)
		g.persist(r)
	}
}

// Get returns a snapshot copy of a request, or nil if unknown.
func (g *approvalGate) Get(id string) *ApprovalRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.reqs[id]; ok {
		cp := *r
		return &cp
	}
	return nil
}

// Await blocks until the request is decided (approved or rejected), the context
// is cancelled, or the timeout elapses, and returns the final snapshot. A
// request already decided returns immediately. On timeout the request stays
// Pending (a late tap can still resolve it) and Await returns (snapshot, false).
// The bool reports whether a decision was observed within the wait.
func (g *approvalGate) Await(ctx context.Context, id string, timeout time.Duration) (*ApprovalRequest, bool) {
	// Fast path + waiter registration under the lock so we never miss a Decide
	// that lands between the check and the channel subscribe.
	g.mu.Lock()
	r, ok := g.reqs[id]
	if !ok {
		g.mu.Unlock()
		return nil, false
	}
	if r.decided() {
		cp := *r
		g.mu.Unlock()
		return &cp, true
	}
	ch, exists := g.waiters[id]
	if !exists {
		ch = make(chan struct{})
		g.waiters[id] = ch
	}
	g.mu.Unlock()

	var timer *time.Timer
	var timeout_c <-chan time.Time
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
		timeout_c = timer.C
	}

	select {
	case <-ch:
		return g.Get(id), true
	case <-ctx.Done():
		return g.Get(id), false
	case <-timeout_c:
		return g.Get(id), false
	}
}

// wake closes and clears the waiter channel for id (must hold g.mu). Safe to
// call when there is no waiter.
func (g *approvalGate) wake(id string) {
	if ch, ok := g.waiters[id]; ok {
		close(ch)
		delete(g.waiters, id)
	}
}

// persist atomically writes one request to disk (must hold g.mu). Best-effort:
// a failure only logs and never blocks the decision path, exactly like
// saveConvSessionMap. No-op when persistence is disabled.
func (g *approvalGate) persist(r *ApprovalRequest) {
	dir := g.approvalDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 创建审批目录失败，跳过落盘：%v\n", err)
		return
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 序列化审批失败，跳过落盘：%v\n", err)
		return
	}
	path := filepath.Join(dir, sanitizeLockID(r.ID)+".json")
	tmp, err := os.CreateTemp(dir, "approval-*.json.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 创建审批临时文件失败，跳过落盘：%v\n", err)
		return
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(config.FilePerm); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 设置审批文件权限失败，跳过落盘：%v\n", err)
		return
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 写入审批临时文件失败，跳过落盘：%v\n", err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 关闭审批临时文件失败，跳过落盘：%v\n", err)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][approval][warn] 原子替换审批存档失败，跳过落盘：%v\n", err)
	}
}

// loadAll reads every persisted request back into memory at startup. It is
// forgiving like loadConvSessionMap: a missing dir (first run) is silent, and a
// single corrupt file is skipped with a warning rather than aborting the load.
func (g *approvalGate) loadAll() {
	dir := g.approvalDir()
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[connect][approval][warn] 读取审批目录失败，按空起：%v\n", err)
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[connect][approval][warn] 读取审批存档 %s 失败，跳过：%v\n", e.Name(), err)
			continue
		}
		var r ApprovalRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			fmt.Fprintf(os.Stderr, "[connect][approval][warn] 审批存档 %s 已损坏，跳过：%v\n", e.Name(), err)
			continue
		}
		if strings.TrimSpace(r.ID) == "" {
			continue
		}
		g.reqs[r.ID] = &r
	}
}

// ---- "Execution-class" request detection (simplified for the M2 slice) ----
//
// We do NOT build a general NLP intent classifier here (that is a later task).
// Instead the orchestrator instructs the agent to emit a structured marker for
// any action it would take, and we parse that marker out of the reply:
//
//   [[ACTION:todo.create title="交方案" due="2026-06-14T18:00:00+08:00"]]
//
// Matched → route through the gate. No marker → ordinary Q&A, replied directly.
// The grammar is deliberately tiny: ACTION:<verb> followed by key="value"
// pairs. The only verb wired end-to-end in this slice is todo.create.

var actionMarkerRe = regexp.MustCompile(`\[\[ACTION:\s*([a-zA-Z0-9_.]+)\s*(.*?)\]\]`)

// actionKVRe captures key="value" (or key='value') pairs inside a marker.
var actionKVRe = regexp.MustCompile(`([a-zA-Z0-9_]+)\s*=\s*"([^"]*)"|([a-zA-Z0-9_]+)\s*=\s*'([^']*)'`)

// detectedAction is one parsed [[ACTION:...]] marker: its verb (e.g.
// "todo.create") and its key/value arguments.
type detectedAction struct {
	Verb string
	Args map[string]string
}

// parseActionMarker extracts the first [[ACTION:...]] marker from an agent
// reply, returning (action, cleaned-reply, found). cleanedReply is the reply
// with the marker stripped (and surrounding whitespace tidied) so it can be
// shown to a human without the machine syntax leaking through.
func parseActionMarker(reply string) (detectedAction, string, bool) {
	loc := actionMarkerRe.FindStringSubmatchIndex(reply)
	if loc == nil {
		return detectedAction{}, reply, false
	}
	verb := strings.TrimSpace(reply[loc[2]:loc[3]])
	argStr := reply[loc[4]:loc[5]]
	act := detectedAction{Verb: verb, Args: map[string]string{}}
	for _, m := range actionKVRe.FindAllStringSubmatch(argStr, -1) {
		if m[1] != "" {
			act.Args[m[1]] = m[2]
		} else if m[3] != "" {
			act.Args[m[3]] = m[4]
		}
	}
	cleaned := strings.TrimSpace(reply[:loc[0]] + reply[loc[1]:])
	return act, cleaned, true
}

// toPlannedAction maps a detected marker onto the structured command the gate
// will execute. It returns (action, summary, ok): ok is false for an unknown or
// malformed verb (e.g. todo.create without a title), so the orchestrator can
// fall back to a plain reply instead of submitting a no-op approval.
//
// ownerStaffID is the todo executor (the owner is who the reminder is for); the
// slice intentionally creates the todo for the owner, matching "create a todo
// for me" where "me" is the digital-twin owner.
func toPlannedAction(act detectedAction, ownerStaffID string) (plannedAction, string, bool) {
	switch act.Verb {
	case "todo.create":
		title := strings.TrimSpace(act.Args["title"])
		if title == "" {
			title = strings.TrimSpace(act.Args["subject"])
		}
		if title == "" {
			return plannedAction{}, "", false
		}
		vo := map[string]any{
			"subject":     title,
			"executorIds": []string{ownerStaffID},
		}
		summary := fmt.Sprintf("创建待办：%s", title)
		if due := strings.TrimSpace(act.Args["due"]); due != "" {
			if ms, err := parseDueToMillis(due); err == nil {
				vo["dueTime"] = ms
				summary += fmt.Sprintf("（截止 %s）", due)
			}
		}
		pa := plannedAction{
			Product:    "todo",
			Tool:       "create_personal_todo",
			LegacyPath: "todo task create",
			Params:     map[string]any{"PersonalTodoCreateVO": vo},
		}
		return pa, summary, true
	default:
		return plannedAction{}, "", false
	}
}

// approveWords / rejectWords are the whole-message replies the text-approval
// mode treats as the owner's decision. The match is on the ENTIRE trimmed
// message (case-insensitive) so a casual mention ("同意他的看法") never
// accidentally approves an action — only a bare "同意" / "拒绝" decides.
var (
	approveWords = map[string]struct{}{
		"同意": {}, "通过": {}, "批准": {}, "确认": {}, "同意执行": {},
		"approve": {}, "ok": {}, "yes": {}, "y": {},
	}
	rejectWords = map[string]struct{}{
		"拒绝": {}, "驳回": {}, "不行": {}, "不同意": {}, "取消": {}, "否决": {},
		"reject": {}, "no": {}, "n": {},
	}
)

// parseDecisionWord classifies a whole message as an approve/reject decision.
// It returns (approve, ok): ok is false when the message is not a bare decision
// keyword, so the caller forwards it to the agent as an ordinary message.
func parseDecisionWord(msg string) (approve bool, ok bool) {
	m := strings.ToLower(strings.TrimSpace(msg))
	if _, yes := approveWords[m]; yes {
		return true, true
	}
	if _, no := rejectWords[m]; no {
		return false, true
	}
	return false, false
}

// classifyPlannedAction maps a planned command onto its read/write class so the
// gate can honor the "写类才拦" design: only a write (or Unknown — per the
// CmdClass safety contract) action needs the owner's sign-off; a read-class
// action is safe to let through without gating.
//
// It classifies on the human command path (LegacyPath, e.g. "todo task create")
// first, since that is exactly the space-joined segment shape ClassifyDwsCommand
// expects. When LegacyPath is absent or yields Unknown, it falls back to the
// product + RPC tool name (e.g. "todo" + "create_personal_todo"), whose leading
// verb token ("create") the classifier can still recognise.
func classifyPlannedAction(pa plannedAction) CmdClass {
	if path := strings.TrimSpace(pa.LegacyPath); path != "" {
		if c := ClassifyDwsCommand(strings.Fields(path)...); c != CmdClassUnknown {
			return c
		}
	}
	return ClassifyDwsCommand(pa.Product, pa.Tool)
}

// parseDueToMillis converts an ISO-8601 due string to epoch millis, accepting
// the common RFC3339 shapes the agent is asked to emit. Kept local to the gate
// so detection has no dependency on the cobra todo command's flag plumbing.
func parseDueToMillis(due string) (int64, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, due); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unrecognized due time %q", due)
}
