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
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// This file wires the confirmation gate to DingTalk:
//   - approvalCardSender: the dependency-injected boundary that delivers the
//     [Approve]/[Reject] card to the owner and updates it with the outcome.
//     Tests inject a fake; production uses dingtalkApprovalCardSender (HTTP).
//   - approvalOrchestrator: the connector-side glue. It owns the gate + sender +
//     runner, decides whether an agent reply is an action request, drives the
//     Submit → card → Await → execute → reply loop, and routes button callbacks
//     into gate.Decide. connect_stream.go calls into it; it never imports stream
//     internals back, so it stays unit-testable without a live connection.

// Button action constants. The card's two buttons each carry a fixed action
// id; the approval id and the decision travel as private action params so a
// callback can be associated and resolved without any server-side lookup table.
const (
	approvalActionApprove   = "dws_approval_approve"
	approvalActionReject    = "dws_approval_reject"
	approvalParamID         = "dwsApprovalId" // approval request id
	approvalParamDecision   = "dwsDecision"   // "approve" | "reject"
	approvalDecisionApprove = "approve"
	approvalDecisionReject  = "reject"
)

// approvalCardSender delivers and updates the owner-facing confirmation card.
// It is an interface so the orchestrator and its tests never touch the network:
// the real sender talks to the DingTalk card API, the fake records calls.
type approvalCardSender interface {
	// SendApprovalCard delivers an interactive card with [Approve]/[Reject]
	// buttons (each carrying req.ID + its decision in the action params) to the
	// owner's 1:1 chat with the bot, returning the delivered card instance id
	// (outTrackId) for later result updates and callback association.
	SendApprovalCard(ctx context.Context, ownerUserID string, req *ApprovalRequest) (string, error)
	// UpdateApprovalCard replaces the card body with the final outcome text
	// (best-effort; an error must not fail the surrounding flow).
	UpdateApprovalCard(ctx context.Context, outTrackID, text string) error
}

// approvalRunner is the subset of executor.Runner the orchestrator needs. Kept
// as a named alias so the dependency is explicit and easy to fake.
type approvalRunner interface {
	Run(context.Context, executor.Invocation) (executor.Result, error)
}

// approvalReplier abstracts "send a plain text line back into the conversation"
// (the group reply after approve/reject). connect_stream.go supplies a closure
// over the chatbot replier + sessionWebhook; tests supply a recorder.
type approvalReplier func(ctx context.Context, convID, text string) error

// ownerNotifier sends a proactive 1:1 message to specific users (the owner, the
// requester), independent of any inbound sessionWebhook. Text approval needs it
// because the approval conversation (owner's 1:1 with the bot) is not the
// conversation the request arrived on, and because the decision can land minutes
// later when the original webhook is stale. *aiCardClient implements it.
type ownerNotifier interface {
	sendOTOText(ctx context.Context, userIDs []string, text string) error
}

// auditSink records a terminal-state request to a durable, reviewable place
// (e.g. a DingTalk online sheet) so every action the twin takes is auditable
// beyond the local approvals JSON. Best-effort: a sink failure must never block
// or fail the action.
type auditSink interface {
	record(ctx context.Context, req *ApprovalRequest)
}

// sheetAuditSink appends one row per action to a DingTalk online sheet (axls)
// via the sheet `append_rows` tool, run under the connector's (bot) identity.
// Columns: 时间 | 摘要 | 请求人 | 状态 | 批准人 | 是否自动 | 失败原因 | 单据ID.
type sheetAuditSink struct {
	runner  approvalRunner
	nodeID  string // axls doc id / URL
	sheetID string // worksheet id or name (e.g. "Sheet1")
}

func (s *sheetAuditSink) record(ctx context.Context, req *ApprovalRequest) {
	if s == nil || s.runner == nil || req == nil {
		return
	}
	auto := ""
	if req.AutoApproved {
		auto = "自动"
	}
	decidedAt := ""
	if !req.DecidedAt.IsZero() {
		decidedAt = req.DecidedAt.Format("2006-01-02 15:04:05")
	}
	row := []any{
		req.CreatedAt.Format("2006-01-02 15:04:05"),
		req.Summary,
		req.Requester,
		string(req.State),
		req.DecidedBy,
		auto,
		req.ExecErr,
		req.ID,
		decidedAt,
	}
	inv := executor.NewHelperInvocation("sheet append", "sheet", "append_rows", map[string]any{
		"nodeId":  s.nodeID,
		"sheetId": s.sheetID,
		"values":  [][]any{row},
	})
	// The online-sheet API throttles (e.g. THREADPOOL_BUSY) under bursts, dropping
	// an audit row on a transient error. Retry a few times with backoff so the
	// trail stays complete; still best-effort — give up (log) after the last try.
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err = s.runner.Run(ctx, inv); err == nil {
			return
		}
		if attempt < 3 && isTransientSheetErr(err) {
			time.Sleep(time.Duration(attempt) * 600 * time.Millisecond)
			continue
		}
		break
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval][audit] 写审计表格失败（已重试）approvalId=%s: %v\n", req.ID, err)
	}
}

// isTransientSheetErr reports whether a sheet-append error is a transient
// throttle/timeout worth retrying (vs. a permanent error like a bad node id).
func isTransientSheetErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToUpper(err.Error())
	for _, sig := range []string{"BUSY", "THREADPOOL", "TIMEOUT", "RATE", "LIMIT", "TOO MANY", "503", "429"} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// approvalOrchestrator is the connector-side controller for the gate. It is
// constructed per connector with the owner's userId, the gate, the card sender
// and the runner. Zero owner / nil sender disables the gate (the connector then
// behaves exactly as before — plain Q&A).
type approvalOrchestrator struct {
	gate        *approvalGate
	sender      approvalCardSender
	runner      approvalRunner
	ownerUserID string
	awaitWindow time.Duration
	// textMode swaps the interactive [Approve]/[Reject] card for a private
	// text confirmation: the bot DMs the OWNER "X 请求执行……" and the owner
	// replies "同意"/"拒绝" in their own 1:1 chat with the bot. The requester
	// never sees the approval — they only get the final result. Used when no
	// approval-card template is configured, so the gate works with zero
	// card-platform setup. The decision is captured asynchronously (see
	// handleOwnerDecision), not by blocking on Await.
	textMode bool
	// notifier delivers the proactive owner/requester 1:1 messages text mode
	// needs (nil in card mode).
	notifier ownerNotifier
	// audit, when set, records every terminal-state request to a durable sink
	// (e.g. a DingTalk online sheet) on top of the local approvals JSON. nil =
	// local-file audit only.
	audit auditSink
	// allowedScopes is the role's capability allowlist (product names). When
	// non-empty, an action whose product is not listed is refused before it ever
	// reaches the gate, keeping the role in its lane. Empty = no restriction.
	allowedScopes []string
	// confirmPolicy governs how OTHERS' requests are confirmed: "manual" (ask
	// every time, the default), "auto" (run without asking, still audited),
	// "remember" (ask once per action verb then reuse). The owner's own requests
	// always auto-run regardless.
	confirmPolicy string
	// remembered caches the owner's decision per action verb for the "remember"
	// policy (verb → approved). Guarded by rememberMu.
	rememberMu sync.Mutex
	remembered map[string]bool
}

// gateDecision decides how to handle a write action: "auto" (run now), "ask"
// (route to the owner), or "reject" (a remembered rejection). The owner's own
// requests always auto-run. For others it follows confirmPolicy.
func (o *approvalOrchestrator) gateDecision(requester, verb string) string {
	if strings.TrimSpace(requester) != "" && strings.TrimSpace(requester) == o.ownerUserID {
		return "auto"
	}
	switch strings.ToLower(strings.TrimSpace(o.confirmPolicy)) {
	case "auto":
		return "auto"
	case "remember":
		o.rememberMu.Lock()
		decided, ok := o.remembered[verb]
		o.rememberMu.Unlock()
		if !ok {
			return "ask"
		}
		if decided {
			return "auto"
		}
		return "reject"
	default: // manual / empty
		return "ask"
	}
}

// rememberDecision records the owner's decision for an action verb so the
// "remember" policy can reuse it. No-op unless the policy is "remember".
func (o *approvalOrchestrator) rememberDecision(verb string, approved bool) {
	if strings.ToLower(strings.TrimSpace(o.confirmPolicy)) != "remember" || strings.TrimSpace(verb) == "" {
		return
	}
	o.rememberMu.Lock()
	if o.remembered == nil {
		o.remembered = make(map[string]bool)
	}
	o.remembered[verb] = approved
	o.rememberMu.Unlock()
}

// scopeAllows reports whether the role may use the given product. An empty
// allowlist means no restriction (allow all).
func (o *approvalOrchestrator) scopeAllows(product string) bool {
	if len(o.allowedScopes) == 0 {
		return true
	}
	product = strings.ToLower(strings.TrimSpace(product))
	for _, s := range o.allowedScopes {
		if strings.ToLower(strings.TrimSpace(s)) == product {
			return true
		}
	}
	return false
}

// recordAudit writes the request's current (terminal) state to the audit sink,
// fetching the fresh snapshot so state/exec_err reflect the final outcome.
// No-op when no sink is configured; best-effort.
func (o *approvalOrchestrator) recordAudit(ctx context.Context, id string) {
	if o == nil || o.audit == nil {
		return
	}
	if r := o.gate.Get(id); r != nil {
		o.audit.record(ctx, r)
	}
}

// newApprovalOrchestrator builds the card-mode controller. awaitWindow caps how
// long the connector blocks for the owner's decision before giving up (the
// request stays Pending on disk, so a late tap still records and executes
// nothing twice).
func newApprovalOrchestrator(gate *approvalGate, sender approvalCardSender, runner approvalRunner, ownerUserID string) *approvalOrchestrator {
	return &approvalOrchestrator{
		gate:        gate,
		sender:      sender,
		runner:      runner,
		ownerUserID: strings.TrimSpace(ownerUserID),
		awaitWindow: 10 * time.Minute,
	}
}

// newTextApprovalOrchestrator builds the text-mode controller (no card sender).
// notifier delivers the private owner/requester messages. The owner confirms by
// replying "同意"/"拒绝" in their 1:1 chat with the bot; the decision is captured
// by handleOwnerDecision on the next inbound message, so handleReply must NOT
// block waiting for it.
func newTextApprovalOrchestrator(gate *approvalGate, runner approvalRunner, ownerUserID string, notifier ownerNotifier) *approvalOrchestrator {
	o := newApprovalOrchestrator(gate, nil, runner, ownerUserID)
	o.textMode = true
	o.notifier = notifier
	return o
}

// enabled reports whether the gate is active. Card mode needs an owner + a card
// sender; text mode needs an owner + a notifier to reach them privately.
func (o *approvalOrchestrator) enabled() bool {
	if o == nil || o.ownerUserID == "" || o.gate == nil {
		return false
	}
	if o.textMode {
		return o.notifier != nil
	}
	return o.sender != nil
}

// agentSystemHint is appended to the forwarded prompt so the agent emits the
// structured action marker (see parseActionMarker) instead of executing
// anything itself. It is the simplified "intent detection" for the slice.
const agentSystemHint = "\n\n[系统提示] 如果用户的请求是要执行一个动作（而不是单纯提问），" +
	"不要假装已经完成，而是在回复末尾用下面其中一个标记声明这个动作，由主人确认后再执行：\n" +
	`· 建待办：[[ACTION:todo.create title="待办标题" due="2026-06-14T18:00:00+08:00"]]（due 可省略）` + "\n" +
	`· 建日程：[[ACTION:calendar.create title="日程标题" start="2026-06-20T10:00:00+08:00" end="2026-06-20T11:00:00+08:00"]]` + "\n" +
	`· 建文档：[[ACTION:doc.create name="文档名"]]` + "\n" +
	"时间一律用 ISO-8601（带时区）。如果只是普通提问，正常回答即可，不要输出任何标记。"

// decorateForActionDetection appends the action-marker instruction to a prompt
// when the gate is enabled. A no-op when disabled, so plain Q&A is untouched.
func (o *approvalOrchestrator) decorateForActionDetection(prompt string) string {
	if !o.enabled() {
		return prompt
	}
	return prompt + agentSystemHint
}

// handleReply is the orchestration hook the connector calls with the agent's
// raw reply. It returns (finalReply, handled): when handled is true the gate
// took over (it has already sent the card / will reply asynchronously via
// reply) and the connector must NOT send finalReply itself. When false the
// connector replies normally with finalReply (the marker, if any, is stripped).
//
// requester is the staffId who asked; convID is where the group answer goes;
// reply is the function used to post the approve/reject outcome back.
func (o *approvalOrchestrator) handleReply(ctx context.Context, requester, convID, agentReply string, reply approvalReplier) (string, bool) {
	if !o.enabled() {
		return agentReply, false
	}
	act, cleaned, found := parseActionMarker(agentReply)
	if !found {
		return agentReply, false // plain Q&A
	}
	pa, summary, ok := toPlannedAction(act, o.ownerUserID)
	if !ok {
		// Unknown/blank action: degrade to a plain reply (marker stripped) so a
		// malformed marker never silently swallows the answer.
		return cleaned, false
	}

	// Role scope: an action outside the role's capability allowlist is refused
	// up front (never reaches the gate or the owner). The requester is told it is
	// out of lane — this is a capability boundary, not an approval, so it is fine
	// to surface to them.
	if !o.scopeAllows(pa.Product) {
		fmt.Fprintf(os.Stderr, "[connect][approval] 越权拦截：角色无 %q 能力，拒绝动作 %s\n", pa.Product, summary)
		return fmt.Sprintf("（该数字员工没有「%s」能力，无法执行此操作：%s）", pa.Product, summary), false
	}

	// 写类才拦：route the gate-or-not decision through the read/write classifier
	// instead of gating every detected marker. A definitively read-class action
	// is safe and bypasses the gate (the marker is stripped and the reply goes
	// out normally). Write — and Unknown, per the CmdClass safety contract — keep
	// the owner's sign-off requirement.
	if classifyPlannedAction(pa) == CmdClassRead {
		return cleaned, false
	}

	// Confirmation strategy: owner-self always auto-runs; others follow
	// confirmPolicy (manual=ask / auto=run / remember=reuse last decision).
	decision := o.gateDecision(requester, act.Verb)
	req := o.gate.Submit(ApprovalRequest{
		Requester:    requester,
		ConvID:       convID,
		Summary:      summary,
		Verb:         act.Verb,
		Action:       pa,
		AutoApproved: decision == "auto",
	})

	switch decision {
	case "auto":
		// Asking IS the authorization (owner-self), or the policy pre-authorized
		// it (auto / remembered-approve). Execute directly; still fully audited.
		return o.autoApproveAndExecute(ctx, req, convID, reply)
	case "reject":
		// Remembered rejection for this action kind — decline without bothering
		// the owner, and tell the requester.
		o.gate.Decide(req.ID, false, o.ownerUserID)
		o.recordAudit(ctx, req.ID)
		fmt.Fprintf(os.Stderr, "[connect][approval] 记忆策略：%q 此前被拒，自动拒绝 approvalId=%s\n", act.Verb, req.ID)
		return fmt.Sprintf("（主人此前已拒绝这类操作，本次未执行：%s）", summary), false
	default: // "ask"
		// Text mode: privately DM the owner and return immediately — the owner's
		// "同意/拒绝" reply is captured asynchronously by handleOwnerDecision
		// (blocking here would deadlock the per-conversation worker that must also
		// process that reply). The requester (this conversation) sees nothing.
		if o.textMode {
			return o.handleReplyText(ctx, req)
		}
		return o.handleReplyCard(ctx, req, convID, reply)
	}
}

// autoApproveAndExecute runs an owner's own request without a second
// confirmation, while still recording the full lifecycle (Decide by the owner →
// execute) so the on-disk approvals log audits it like any other action. It
// replies into the conversation the owner asked in (the inbound webhook is still
// fresh — the owner just messaged). Used for both card and text mode.
func (o *approvalOrchestrator) autoApproveAndExecute(ctx context.Context, req *ApprovalRequest, convID string, reply approvalReplier) (string, bool) {
	decided, _ := o.gate.Decide(req.ID, true, o.ownerUserID)
	if decided == nil {
		decided = req
	}
	fmt.Fprintf(os.Stderr, "[connect][approval][audit] 主人自助操作，自动执行 approvalId=%s owner=%s: %s\n",
		decided.ID, o.ownerUserID, decided.Summary)
	out, execErr := o.execute(ctx, decided)
	if execErr != nil {
		// Do not lose it: hold for retry and tell the owner to recover. Most
		// likely the connector's dws login is not the bot owner, so an
		// owner-scoped action can't run yet.
		o.gate.markDeferred(decided.ID, execErr.Error())
		o.recordAudit(ctx, decided.ID)
		o.notifyOwnerDeferred(ctx, decided, execErr)
		if reply != nil {
			_ = reply(ctx, convID, "收到，但现在没能直接完成（已记下，稍后会补做）。")
		}
		return "", true
	}
	o.gate.markExecuted(decided.ID)
	o.recordAudit(ctx, decided.ID)
	if reply != nil {
		_ = reply(ctx, convID, "已为你完成："+decided.Summary+"\n"+out)
	}
	return "", true
}

// notifyOwnerDeferred privately tells the owner that an action could not run now
// and is being held, with the likely cause and how to recover (log dws in as the
// bot owner, then reply "重试"). Best-effort.
func (o *approvalOrchestrator) notifyOwnerDeferred(ctx context.Context, req *ApprovalRequest, execErr error) {
	if o.notifier == nil {
		return
	}
	who := strings.TrimSpace(req.Requester)
	if who == "" || who == o.ownerUserID {
		who = "你"
	}
	msg := fmt.Sprintf("⚠️ %s 请求执行：%s\n但我现在没能以你的身份完成（很可能这台连接器的 dws 没有登录成你本人的账号）。\n"+
		"已先记下，不会丢。把 dws 登录成机器人主人账号后会自动补做；想立即补做可回复「重试」。\n（错误：%s）",
		who, req.Summary, truncateRunes(execErr.Error(), 120))
	_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, msg)
}

// handleReplyText privately DMs the OWNER the approval request and hands control
// back to the connector (handled=true). The requester sees nothing — the
// approval happens only between the owner and the bot. It never blocks: the
// owner's decision arrives later as an inbound message routed through
// handleOwnerDecision.
func (o *approvalOrchestrator) handleReplyText(ctx context.Context, req *ApprovalRequest) (string, bool) {
	who := strings.TrimSpace(req.Requester)
	if who == "" {
		who = "有人"
	}
	prompt := fmt.Sprintf("🔔 %s 请求执行一个操作，需要你确认：\n%s\n回复「同意」执行，或「拒绝」取消。", who, req.Summary)
	if err := o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, prompt); err != nil {
		// Cannot reach the owner: the action does not run. Do NOT leak the
		// approval to the requester — text mode's whole point is privacy — so
		// just log and swallow (handled), leaving the request Pending on disk.
		fmt.Fprintf(os.Stderr, "[connect][approval] 私聊主人 %s 失败，本次未执行: %v\n", o.ownerUserID, err)
		return "", true
	}
	fmt.Fprintf(os.Stderr, "[connect][approval] 已私聊主人 %s 待确认 approvalId=%s requester=%s: %s\n",
		o.ownerUserID, req.ID, req.Requester, req.Summary)
	return "", true
}

// handleOwnerDecision intercepts an inbound message that is the OWNER replying
// "同意/拒绝" (in their own 1:1 chat with the bot) to the pending request. It
// decides, executes-or-declines, privately acks the owner, and sends the
// outcome to the original requester. Returns true when it consumed the message
// (so the connector skips forwarding it to the agent). A non-owner sender, a
// non-decision message, or no pending request all return false (ordinary
// message). Only active in text mode.
func (o *approvalOrchestrator) handleOwnerDecision(ctx context.Context, senderStaffID, text string) bool {
	if o == nil || !o.textMode || !o.enabled() {
		return false
	}
	if senderStaffID == "" || senderStaffID != o.ownerUserID {
		return false
	}
	// "重试/恢复": the owner has recovered — flush the deferred backlog.
	if isRetryWord(text) {
		return o.flushDeferred(ctx)
	}
	approve, ok := parseDecisionWord(text)
	if !ok {
		return false
	}
	req := o.gate.latestPending()
	if req == nil {
		return false
	}
	decided, deciding := o.gate.Decide(req.ID, approve, senderStaffID)
	if !deciding {
		// Already decided (e.g. a double reply): consume the keyword, do not
		// re-execute.
		return decided != nil
	}
	// "remember" policy: cache this decision for the action kind so the next
	// same-verb request reuses it without asking again.
	o.rememberDecision(decided.Verb, approve)
	if !decided.approved() {
		o.recordAudit(ctx, decided.ID)
		_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, "好的，已拒绝，未执行。")
		o.notifyRequester(ctx, decided, "你的请求未获主人批准，本次未执行。")
		return true
	}
	out, execErr := o.execute(ctx, decided)
	if execErr != nil {
		// Approved but could not run now → hold for retry, tell the owner how to
		// recover. The requester is NOT told it failed (their task is not lost).
		o.gate.markDeferred(decided.ID, execErr.Error())
		o.recordAudit(ctx, decided.ID)
		o.notifyOwnerDeferred(ctx, decided, execErr)
		return true
	}
	o.gate.markExecuted(decided.ID)
	o.recordAudit(ctx, decided.ID)
	_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, "已同意并执行完成："+decided.Summary)
	o.notifyRequester(ctx, decided, "已为你完成："+decided.Summary+"\n"+out)
	return true
}

// flushDeferred replays the deferred backlog after the owner recovers. Each
// request is re-executed; on success it is marked executed and the original
// requester gets the outcome, on failure it stays deferred (still not lost).
// The owner gets a summary. Returns true (the "重试" message is always consumed).
func (o *approvalOrchestrator) flushDeferred(ctx context.Context) bool {
	pending := o.gate.allDeferred()
	if len(pending) == 0 {
		_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, "当前没有积压的请求。")
		return true
	}
	done, stuck := o.flushDeferredOnce(ctx, pending)
	// Manual "重试" always reports back (the owner asked), naming what was done.
	var b strings.Builder
	fmt.Fprintf(&b, "已补做 %d 个积压请求", len(done))
	if len(done) > 0 {
		b.WriteString("：\n")
		for i, s := range done {
			fmt.Fprintf(&b, "%d. %s\n", i+1, s)
		}
	} else {
		b.WriteString("。")
	}
	if stuck > 0 {
		fmt.Fprintf(&b, "仍有 %d 个未成功（身份可能还没对上，确认 dws 登录后再回复「重试」）。", stuck)
	}
	_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, strings.TrimRight(b.String(), "\n"))
	return true
}

// flushDeferredOnce re-executes each given deferred request once. Successful
// ones are marked executed and their requester is notified; failed ones stay
// deferred. Returns the completed summaries and the still-stuck count. Shared by
// the manual "重试" path and the background auto-retry.
func (o *approvalOrchestrator) flushDeferredOnce(ctx context.Context, pending []*ApprovalRequest) (done []string, stuck int) {
	for _, req := range pending {
		out, execErr := o.execute(ctx, req)
		if execErr != nil {
			o.gate.markDeferred(req.ID, execErr.Error())
			o.recordAudit(ctx, req.ID)
			stuck++
			continue
		}
		o.gate.markExecuted(req.ID)
		o.recordAudit(ctx, req.ID)
		o.notifyRequester(ctx, req, "（已恢复）已为你完成："+req.Summary+"\n"+out)
		done = append(done, req.Summary)
	}
	return done, stuck
}

// autoFlushDeferred is the background-retry pass: it replays the backlog and
// messages the owner ONLY when something actually completed (so a periodic tick
// while the identity is still wrong stays silent — no spam). The requester is
// still notified per completed item inside flushDeferredOnce.
func (o *approvalOrchestrator) autoFlushDeferred(ctx context.Context) {
	pending := o.gate.allDeferred()
	if len(pending) == 0 {
		return
	}
	done, _ := o.flushDeferredOnce(ctx, pending)
	if len(done) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "（已自动恢复）补做了 %d 个积压请求：\n", len(done))
	for i, s := range done {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	_ = o.notifier.sendOTOText(ctx, []string{o.ownerUserID}, strings.TrimRight(b.String(), "\n"))
}

// startAutoRetry runs autoFlushDeferred on an interval until ctx is cancelled,
// so a deferred backlog drains by itself once the owner's identity comes back —
// no manual "重试" needed. A no-op when there is no notifier (card mode) or a
// non-positive interval.
func (o *approvalOrchestrator) startAutoRetry(ctx context.Context, interval time.Duration) {
	if o == nil || o.notifier == nil || interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				o.autoFlushDeferred(ctx)
			}
		}
	}()
}

// notifyRequester sends the outcome to the original requester, unless they are
// the owner themselves (who already got the owner-side ack — no need to double
// message). Best-effort.
func (o *approvalOrchestrator) notifyRequester(ctx context.Context, req *ApprovalRequest, text string) {
	who := strings.TrimSpace(req.Requester)
	if who == "" || who == o.ownerUserID {
		return
	}
	_ = o.notifier.sendOTOText(ctx, []string{who}, text)
}

// handleReplyCard runs the interactive-card flow: deliver the [Approve]/[Reject]
// card to the owner, block for the decision, then execute-or-decline and report
// the outcome into the conversation.
func (o *approvalOrchestrator) handleReplyCard(ctx context.Context, req *ApprovalRequest, convID string, reply approvalReplier) (string, bool) {
	outTrackID, err := o.sender.SendApprovalCard(ctx, o.ownerUserID, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval] 确认卡片投放失败: %v\n", err)
		// Cannot ask the owner → tell the requester the gate could not engage,
		// rather than silently executing or silently dropping.
		return "（需要主人确认，但确认卡片发送失败，本次未执行）", false
	}
	o.gate.setOutTrackID(req.ID, outTrackID)
	fmt.Fprintf(os.Stderr, "[connect][approval] 已向主人 %s 发送确认卡片 approvalId=%s outTrackId=%s: %s\n",
		o.ownerUserID, req.ID, outTrackID, req.Summary)

	// Block for the owner's decision off the connector's per-conversation
	// worker is fine: the worker already serializes one conversation, and the
	// gate persists so a restart mid-wait loses only the in-memory block.
	decided, gotDecision := o.gate.Await(ctx, req.ID, o.awaitWindow)
	if !gotDecision || decided == nil {
		_ = reply(ctx, convID, "（等待主人确认超时，本次未执行）")
		return "", true
	}
	if !decided.approved() {
		o.recordAudit(ctx, decided.ID)
		_ = o.sender.UpdateApprovalCard(ctx, decided.OutTrackID, "已拒绝：未执行。")
		_ = reply(ctx, convID, "主人暂时没批准。")
		return "", true
	}

	// Approved → the orchestrator runs the planned command directly (no agent
	// round-trip), then reports the outcome to the group and the card.
	out, execErr := o.execute(ctx, decided)
	if execErr != nil {
		o.gate.markFailed(decided.ID, execErr.Error())
		o.recordAudit(ctx, decided.ID)
		_ = o.sender.UpdateApprovalCard(ctx, decided.OutTrackID, "已同意，但执行失败："+execErr.Error())
		_ = reply(ctx, convID, "主人已同意，但执行失败："+execErr.Error())
		return "", true
	}
	o.gate.markExecuted(decided.ID)
	o.recordAudit(ctx, decided.ID)
	_ = o.sender.UpdateApprovalCard(ctx, decided.OutTrackID, "已同意并执行完成。")
	_ = reply(ctx, convID, "主人已同意，已执行："+decided.Summary+"\n"+out)
	return "", true
}

// execute runs the request's planned action via the runner and returns a short
// human-readable result line. It is the only place the gate touches the
// executor, so adding a new approvable verb is a matter of toPlannedAction +
// the runner already supporting that tool.
func (o *approvalOrchestrator) execute(ctx context.Context, req *ApprovalRequest) (string, error) {
	if o.runner == nil {
		return "", fmt.Errorf("no runner configured")
	}
	inv := executor.NewHelperInvocation(req.Action.LegacyPath, req.Action.Product, req.Action.Tool, req.Action.Params)
	res, err := o.runner.Run(ctx, inv)
	if err != nil {
		return "", err
	}
	if !res.Invocation.Implemented {
		// The helper override was not wired at runtime (e.g. dry-run / discovery
		// gap): surface it rather than claim success.
		return "（动作已受理，但运行时未实际执行——请检查命令是否在当前环境可用）", nil
	}
	return "（待办已创建）", nil
}

// handleCardCallback maps a button tap to gate.Decide. It pulls the approval id
// and decision from the action params (primary association), falling back to the
// card instance id (OutTrackId) when the params are absent. Returns a card
// response that flips the buttons to a decided state. Safe to call repeatedly:
// a second tap after a decision is a no-op (Decide is idempotent).
func (o *approvalOrchestrator) handleCardCallback(ctx context.Context, req *card.CardRequest) (*card.CardResponse, error) {
	if o == nil || o.gate == nil || req == nil {
		return &card.CardResponse{}, nil
	}
	id, approve, ok := decodeCardAction(req)
	if !ok {
		fmt.Fprintf(os.Stderr, "[connect][approval] 卡片回调无法解析动作 outTrackId=%s\n", req.OutTrackId)
		return &card.CardResponse{}, nil
	}
	if id == "" {
		// No approval id in the params: associate by the card instance id.
		if found := o.gate.findByOutTrackID(req.OutTrackId); found != nil {
			id = found.ID
		}
	}
	if id == "" {
		fmt.Fprintf(os.Stderr, "[connect][approval] 卡片回调无法关联审批 outTrackId=%s\n", req.OutTrackId)
		return &card.CardResponse{}, nil
	}
	decided, deciding := o.gate.Decide(id, approve, strings.TrimSpace(req.UserId))
	verb := "同意"
	if !approve {
		verb = "拒绝"
	}
	if deciding {
		fmt.Fprintf(os.Stderr, "[connect][approval] 主人 %s 点了[%s] approvalId=%s\n", req.UserId, verb, id)
	} else if decided != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval] 审批 %s 已是终态(%s)，忽略重复点击\n", id, decided.State)
	}
	// Echo the decision back into the card's private data so the renderer can
	// show a decided state. Best-effort; the gate state is the source of truth.
	return &card.CardResponse{
		UserPrivateData: &card.CardDataDto{
			CardParamMap: map[string]string{"dwsDecision": verb},
		},
	}, nil
}

// decodeCardAction extracts (approvalId, approve, ok) from a card callback. It
// prefers the explicit decision param, then the tapped action id, so the card
// can carry the decision either way. ok is false when neither identifies a
// decision (a non-approval card, or a malformed payload).
func decodeCardAction(req *card.CardRequest) (id string, approve bool, ok bool) {
	id = strings.TrimSpace(req.GetActionString(approvalParamID))
	decision := strings.TrimSpace(req.GetActionString(approvalParamDecision))
	switch decision {
	case approvalDecisionApprove:
		return id, true, true
	case approvalDecisionReject:
		return id, false, true
	}
	// Fall back to which button was pressed (its action id).
	for _, a := range req.CardActionData.CardPrivateData.ActionIdList {
		switch strings.TrimSpace(a) {
		case approvalActionApprove:
			return id, true, true
		case approvalActionReject:
			return id, false, true
		}
	}
	return id, false, false
}

// ---- Real (HTTP) card sender ----

// dingtalkApprovalCardSender delivers the interactive approval card via the
// DingTalk card API, reusing aiCardClient for auth + the create/deliver/update
// HTTP plumbing. The card template must define two buttons whose action params
// the connector fills in (approval id + decision); see the docstring on
// SendApprovalCard for the contract.
type dingtalkApprovalCardSender struct {
	cli        *aiCardClient
	templateID string
}

// newDingtalkApprovalCardSender builds the real sender. An empty templateID
// returns nil: without an interactive-card template the connector cannot render
// buttons, so the caller must keep the gate disabled rather than deliver a card
// the owner cannot act on.
func newDingtalkApprovalCardSender(clientID, clientSecret, templateID string) approvalCardSender {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil
	}
	return &dingtalkApprovalCardSender{
		cli:        newAICardClient(clientID, clientSecret, templateID),
		templateID: templateID,
	}
}

// SendApprovalCard creates an interactive card instance and delivers it to the
// owner's 1:1 chat with the bot. The approval id and per-button decision are
// passed in cardData.cardParamMap so the template's [Approve]/[Reject] buttons
// echo them back as action params on tap (see decodeCardAction). Best-effort
// result-check via callChecked surfaces a per-target deliver failure that the
// card API hides inside an HTTP 200.
func (s *dingtalkApprovalCardSender) SendApprovalCard(ctx context.Context, ownerUserID string, req *ApprovalRequest) (string, error) {
	outTrackID := "dws_approval_" + req.ID
	create := map[string]any{
		"cardTemplateId": s.templateID,
		"outTrackId":     outTrackID,
		"cardData": map[string]any{
			"cardParamMap": map[string]any{
				"title":           "需要你确认",
				"summary":         req.Summary,
				"requester":       req.Requester,
				approvalParamID:   req.ID,
				"approveDecision": approvalDecisionApprove,
				"rejectDecision":  approvalDecisionReject,
				"approveActionId": approvalActionApprove,
				"rejectActionId":  approvalActionReject,
				"config":          `{"autoLayout":true}`,
			},
		},
		"callbackType":          "STREAM",
		"imRobotOpenSpaceModel": map[string]any{"supportForward": true},
	}
	if err := s.cli.call(ctx, http.MethodPost, "/v1.0/card/instances", create); err != nil {
		return "", err
	}
	deliver := map[string]any{
		"outTrackId":  outTrackID,
		"userIdType":  1,
		"openSpaceId": "dtv1.card//IM_ROBOT." + ownerUserID,
		"imRobotOpenDeliverModel": map[string]any{
			"spaceType": "IM_ROBOT",
			"robotCode": s.cli.clientID,
		},
	}
	if err := s.cli.callChecked(ctx, http.MethodPost, "/v1.0/card/instances/deliver", deliver); err != nil {
		return "", err
	}
	return outTrackID, nil
}

// UpdateApprovalCard rewrites the card's result line after a decision. It is
// best-effort: a stuck card is bad UX but must never fail the gate flow, so
// callers ignore the error.
func (s *dingtalkApprovalCardSender) UpdateApprovalCard(ctx context.Context, outTrackID, text string) error {
	return s.cli.call(ctx, http.MethodPut, "/v1.0/card/instances", map[string]any{
		"outTrackId":        outTrackID,
		"cardData":          map[string]any{"cardParamMap": map[string]any{"result": text, "summary": text}},
		"cardUpdateOptions": map[string]any{"updateCardDataByKey": true},
	})
}
