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
	// textMode swaps the interactive [Approve]/[Reject] card for a plain-text
	// confirmation: the bot posts "需要主人确认……" into the conversation and the
	// owner replies "同意"/"拒绝". Used when no approval-card template is
	// configured, so the gate works with zero card-platform setup. In this mode
	// there is no card sender and the decision is captured asynchronously (see
	// handleOwnerDecision) rather than by blocking on Await.
	textMode bool
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
// The owner confirms by replying "同意"/"拒绝" in the conversation; the decision
// is captured by handleOwnerDecision on the next inbound message, so handleReply
// must NOT block waiting for it.
func newTextApprovalOrchestrator(gate *approvalGate, runner approvalRunner, ownerUserID string) *approvalOrchestrator {
	o := newApprovalOrchestrator(gate, nil, runner, ownerUserID)
	o.textMode = true
	return o
}

// enabled reports whether the gate is active. Card mode needs an owner + a card
// sender; text mode needs an owner only (the conversation is the channel).
func (o *approvalOrchestrator) enabled() bool {
	if o == nil || o.ownerUserID == "" || o.gate == nil {
		return false
	}
	return o.sender != nil || o.textMode
}

// agentSystemHint is appended to the forwarded prompt so the agent emits the
// structured action marker (see parseActionMarker) instead of executing
// anything itself. It is the simplified "intent detection" for the slice.
const agentSystemHint = "\n\n[系统提示] 如果用户的请求是要执行一个动作（而不是单纯提问），" +
	"不要假装已经完成，而是在回复末尾用如下标记声明这个动作，由主人确认后再执行：" +
	`[[ACTION:todo.create title="待办标题" due="2026-06-14T18:00:00+08:00"]]（due 可省略）。` +
	"如果只是普通提问，正常回答即可，不要输出该标记。"

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

	// 写类才拦：route the gate-or-not decision through the read/write classifier
	// instead of gating every detected marker. A definitively read-class action
	// is safe and bypasses the gate (the marker is stripped and the reply goes
	// out normally). Write — and Unknown, per the CmdClass safety contract — keep
	// the owner's sign-off requirement.
	if classifyPlannedAction(pa) == CmdClassRead {
		return cleaned, false
	}

	req := o.gate.Submit(ApprovalRequest{
		Requester: requester,
		ConvID:    convID,
		Summary:   summary,
		Action:    pa,
	})

	// Text mode: post a plain-text confirmation into the conversation and return
	// immediately — the owner's "同意/拒绝" reply is captured asynchronously by
	// handleOwnerDecision (blocking here would deadlock the per-conversation
	// worker that must also process that reply).
	if o.textMode {
		return o.handleReplyText(ctx, req, reply)
	}
	return o.handleReplyCard(ctx, req, convID, reply)
}

// handleReplyText posts the text-mode confirmation prompt and hands control back
// to the connector (handled=true). It never blocks: the decision arrives later
// as an ordinary inbound message routed through handleOwnerDecision.
func (o *approvalOrchestrator) handleReplyText(ctx context.Context, req *ApprovalRequest, reply approvalReplier) (string, bool) {
	prompt := fmt.Sprintf("⚠️ 这是执行类操作，需要主人确认：%s\n主人请回复「同意」执行，或「拒绝」取消。", req.Summary)
	if err := reply(ctx, req.ConvID, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "[connect][approval] 文本确认提示发送失败: %v\n", err)
		// Could not even ask: surface it rather than silently dropping or running.
		return "（需要主人确认，但提示发送失败，本次未执行）", false
	}
	fmt.Fprintf(os.Stderr, "[connect][approval] 待主人文本确认 approvalId=%s conv=%s: %s\n",
		req.ID, req.ConvID, req.Summary)
	return "", true
}

// handleOwnerDecision intercepts an inbound message that is the owner replying
// "同意/拒绝" to a pending text-mode confirmation in this conversation. It
// returns true when it consumed the message (decided + executed/declined),
// telling the connector to skip forwarding it to the agent. A non-owner sender,
// a non-decision message, or no pending request all return false (ordinary
// message). Only active in text mode.
func (o *approvalOrchestrator) handleOwnerDecision(ctx context.Context, senderStaffID, convID, text string, reply approvalReplier) bool {
	if o == nil || !o.textMode || !o.enabled() {
		return false
	}
	if senderStaffID == "" || senderStaffID != o.ownerUserID {
		return false
	}
	approve, ok := parseDecisionWord(text)
	if !ok {
		return false
	}
	req := o.gate.pendingForConv(convID)
	if req == nil {
		return false
	}
	decided, deciding := o.gate.Decide(req.ID, approve, senderStaffID)
	if !deciding {
		// Already decided (e.g. a double reply): consume the keyword but do not
		// re-execute.
		return decided != nil
	}
	if !decided.approved() {
		_ = reply(ctx, convID, "好的，已拒绝，本次不执行。")
		return true
	}
	out, execErr := o.execute(ctx, decided)
	if execErr != nil {
		o.gate.markFailed(decided.ID, execErr.Error())
		_ = reply(ctx, convID, "已同意，但执行失败："+execErr.Error())
		return true
	}
	o.gate.markExecuted(decided.ID)
	_ = reply(ctx, convID, "已同意，已执行："+decided.Summary+"\n"+out)
	return true
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
		_ = o.sender.UpdateApprovalCard(ctx, decided.OutTrackID, "已拒绝：未执行。")
		_ = reply(ctx, convID, "主人暂时没批准。")
		return "", true
	}

	// Approved → the orchestrator runs the planned command directly (no agent
	// round-trip), then reports the outcome to the group and the card.
	out, execErr := o.execute(ctx, decided)
	if execErr != nil {
		o.gate.markFailed(decided.ID, execErr.Error())
		_ = o.sender.UpdateApprovalCard(ctx, decided.OutTrackID, "已同意，但执行失败："+execErr.Error())
		_ = reply(ctx, convID, "主人已同意，但执行失败："+execErr.Error())
		return "", true
	}
	o.gate.markExecuted(decided.ID)
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
