// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/google/uuid"
)

const employeeCardCatalog = "https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"

// Existing CLI leaves own profile selection, validation and MCP dispatch.
// Do not mutate the parent process's global profile or introduce another API.
var employeeA2UICall = runEmployeeA2UICall

func runEmployeeA2UICall(ctx context.Context, binding digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
	b, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), binding.DWSProfile)
	if err != nil || !sameEmployeeBinding(b, binding) || employeeBindingState(b) != "bound" || employeeDesiredState(b) != "running" {
		return nil, employeeTerminal("binding_not_authorized")
	}
	content, err := encodeEmployeeCardMessages(messages)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	argv := append([]string{"chat", "message"}, args...)
	// Keep the payload out of argv, including Windows' 32767-character limit.
	argv = append(argv, "--content", "-", "--format", "json")
	cmd, err := employeeCommand(ctx, binding.DWSProfile, argv...)
	if err != nil {
		return nil, employeeTerminal("dws_command_unavailable")
	}
	var stdout, stderr employeeBoundedBuffer
	cmd.Stdin = bytes.NewReader(content)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var result struct {
		Success bool           `json:"success"`
		Result  map[string]any `json:"result"`
	}
	if err != nil || stdout.overflow || json.Unmarshal(stdout.Bytes(), &result) != nil || !result.Success || result.Result == nil {
		// A lost response may already have committed. Never retry or recreate.
		if ctx.Err() == nil && !stdout.overflow && !stderr.overflow {
			return nil, employeeCardResponseError(stdout.Bytes(), stderr.Bytes())
		}
		return nil, employeeTerminal("card_write_unconfirmed")
	}
	return result.Result, nil
}

func employeeCardResponseError(stdout, stderr []byte) *employeeRunError {
	var response employeeCardErrorEnvelope
	classified := func(code string) *employeeRunError {
		e := response.Error
		if e.ExecutionID == "" {
			e.ExecutionID = e.Details.ExecutionID
		}
		d := &employeeCardDiagnostic{Code: code, Category: employeeCardSafeScalar(e.Category), Reason: employeeCardSafeScalar(e.Reason), Stage: employeeCardSafeScalar(e.Stage), ExecutionStarted: e.ExecutionStarted, Retryable: e.Retryable, ExecutionID: employeeCardSafeScalar(e.ExecutionID), TraceID: employeeCardSafeScalar(e.TraceID)}
		// 只保留 CLI 传输错误的脱敏原因；业务拒绝、工具正文和非 JSON 输出均不落日志。
		if e.Category == "api" && employeeCardTransportReason(e.Reason) && (e.Stage == "request" || e.Stage == "") {
			d.Message, d.Cause = employeeCardSafeDiagnosticText(e.Message), employeeCardSafeDiagnosticText(e.Cause)
		}
		no := false
		return &employeeRunError{Code: code, Retryable: &no, CardDiagnostic: d}
	}
	// Only stable structured fields establish pre-execution rejection. Never
	// classify from message text, or assume a generic server failure had no effect.
	if json.Unmarshal(stderr, &response) == nil {
		e := response.Error
		if e.ExecutionStarted != nil {
			if !*e.ExecutionStarted {
				if e.Category == "api" && e.Stage == "request" && e.Retryable != nil && *e.Retryable && employeeCardTransportReason(e.Reason) {
					return classified("card_request_not_sent")
				}
				return classified("card_write_rejected")
			}
			return classified("card_write_unconfirmed")
		}
		if e.Category == "validation" || e.Category == "auth" || e.Type == "validation" || e.Type == "auth" {
			return classified("card_write_rejected")
		}
		if e.Category != "" && e.Category != "network" || response.Success != nil && !*response.Success {
			return classified("card_response_failed")
		}
	}
	response = employeeCardErrorEnvelope{}
	if json.Unmarshal(stdout, &response) == nil && response.Success != nil && !*response.Success {
		return classified("card_response_failed")
	}
	response = employeeCardErrorEnvelope{}
	return classified("card_write_unconfirmed")
}

func (r *employeeRuntime) cardFailure(e employeeEvent, record *employeeTaskRecord, stage string, err error) error {
	record.Status, record.CardError = "needs_review", stage
	record.Delivery = "unknown"
	record.CardErrorCode = "card_write_unconfirmed"
	var classified *employeeRunError
	if errors.As(err, &classified) {
		record.CardErrorCode = classified.Code
	}
	record.LastCardError = &employeeCardDiagnostic{Code: record.CardErrorCode}
	if classified != nil && classified.CardDiagnostic != nil {
		record.LastCardError = classified.CardDiagnostic
	}
	if record.CardErrorCode == "card_request_not_sent" || record.CardErrorCode == "card_write_rejected" {
		record.Status, record.Delivery = "card_delivery_failed", "failure"
	}
	if record.CardErrorCode == "card_write_rejected" && stage == "create_unconfirmed" {
		record.Status, record.CardError = "card_rejected", "create_rejected"
		record.Delivery = "rejected"
	}
	r.mu.Lock()
	if r.cardFailures == nil {
		r.cardFailures = make(map[string]int)
	}
	r.cardFailures[e.ConversationID]++
	count := r.cardFailures[e.ConversationID]
	r.mu.Unlock()
	if count != 1 && count%3 != 0 {
		return nil
	}
	code := "card_reply_failure"
	if count >= 3 {
		code = "card_reply_continuous_failures"
	}
	// Operational alert contains no conversation IDs, credentials or tool output.
	data, err := employeeMarshalJSON(map[string]any{"code": code, "level": "warning", "failures": count, "stage": stage, "reason": record.CardErrorCode, "diagnostic": record.LastCardError, "task": filepath.Base(r.recordPath(e)), "time": time.Now().Format(time.RFC3339Nano)})
	if err != nil {
		return employeeTerminal("card_alert_unavailable")
	}
	f, err := employeeOpenFile(filepath.Join(r.dir, "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return employeeTerminal("card_alert_unavailable")
	}
	defer f.Close()
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	if err != nil {
		return employeeTerminal("card_alert_unavailable")
	}
	return nil
}

func employeeCardMessage(kind, surface string, value map[string]any) map[string]any {
	value["surfaceId"] = surface
	return map[string]any{"version": "v1.0", kind: value}
}

const employeeCardFlushInterval = 250 * time.Millisecond

func employeeCardPanel(title string, children []string, running bool) map[string]any {
	panel := map[string]any{"id": employeeCardPanelID(running), "component": "CollapsiblePanel", "title": title, "children": children, "variant": "reasoning", "fallbackMarkdown": title, "defaultExpanded": running, "colorToken": "common_level2_base_color"}
	if running {
		// DingTalk's public progress pattern renders this marker as its loading
		// animation. flowStatus alone does not decorate an A2UI panel title.
		panel["title"] = "[AIGenerating]" + title
		panel["textEffect"] = "shimmer"
	}
	return panel
}

func employeeCardPanelID(running bool) string {
	if running {
		return "execution"
	}
	// defaultExpanded 只在首次挂载时生效。终态使用新组件，才能收起手动展开的过程。
	return "execution-complete"
}

type employeeCardView struct {
	items    []agentProgress
	itemBase int
	answer   string
	revision int
	lastSend time.Time
}

func (v *employeeCardView) accept(p agentProgress) {
	if p.Kind == "answer" {
		v.answer = publicProgressPreview(p.Text)
		return
	}
	if p.Kind != "progress" && p.Kind != "tool" {
		return
	}
	p.Text = boundedPublicText(p.Text, 256, "…")
	if v.items == nil {
		v.items = make([]agentProgress, 0, 32)
	}
	for i := range v.items {
		if v.items[i].ID == p.ID {
			v.items[i] = p
			return
		}
	}
	if len(v.items) == 32 {
		copy(v.items, v.items[1:])
		v.items[len(v.items)-1] = p
		v.itemBase++
		return
	}
	v.items = append(v.items, p)
}

func (v *employeeCardView) messages(surface, title string, running bool) []map[string]any {
	children := make([]string, 0, len(v.items))
	components := make([]map[string]any, 0, len(v.items)+3)
	// 只展示接入层生成的公开工具类型和状态，不读取原始输入输出。
	start := 0
	if len(v.items) > 32 {
		start = len(v.items) - 32
	}
	for index, p := range v.items[start:] {
		id := "step-" + jsonScalar(v.itemBase+index+start)
		if p.Kind == "tool" {
			children = append(children, "tool-group-"+id)
			components = append(components, employeeCardToolComponents(id, p, running)...)
			continue
		}
		children = append(children, id)
		components = append(components, employeeCardProgressText(id, p.Text))
	}
	if len(children) == 0 {
		children = append(children, "step-initial")
		components = append(components, employeeCardProgressText("step-initial", title))
	}
	components = append(components,
		map[string]any{"id": "executionTimeline", "component": "Column", "children": children, "gap": 6},
		employeeCardPanel(title, []string{"executionTimeline"}, running))
	rootChildren := []string{employeeCardPanelID(running)}
	if v.answer != "" {
		rootChildren = append(rootChildren, "result")
		components = append(components,
			map[string]any{"id": "answer", "component": "Markdown", "content": map[string]any{"path": "/answer"}},
			map[string]any{"id": "result", "component": "Column", "children": []string{"answer"}, "gap": 6})
	}
	components = append(components, map[string]any{"id": "root", "component": "Column", "children": rootChildren, "gap": 6})
	return []map[string]any{
		employeeCardMessage("updateDataModel", surface, map[string]any{"path": "/answer", "value": v.answer}),
		employeeCardMessage("updateComponents", surface, map[string]any{"components": components}),
	}
}

func (r *employeeRuntime) processCard(e employeeEvent, record *employeeTaskRecord, fwd forwarder, attachments ...connectMediaAttachment) error {
	if !r.accept(e) {
		return employeeTerminal("reply_not_authorized")
	}
	record.SurfaceID = "employee-" + uuid.NewString()
	record.Status = "card_creating"
	record.Execution, record.Delivery = "not_started", "pending"
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	target := []string{"--open-dingtalk-id", e.SenderID}
	if e.Type != "user_im_message_receive_o2o" && e.Type != "user_im_message_receive_o2o_all" {
		target = []string{"--conversation-id", e.ConversationID}
	}
	initialView := employeeCardView{}
	initial := []map[string]any{
		employeeCardMessage("createSurface", record.SurfaceID, map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{"answer": ""}}),
		initialView.messages(record.SurfaceID, "思考中…", true)[1],
	}
	createArgs := append([]string{"send-a2ui-card"}, target...)
	if r.cfg.ReplyCardForward {
		createArgs = append(createArgs, "--support-forward")
	}
	result, err := employeeA2UICall(r.ctx, r.cfg.Binding, initial, createArgs...)
	if err != nil {
		return r.cardFailure(e, record, "create_unconfirmed", err)
	}
	rawBizID, isString := result["bizId"].(string)
	record.BizID, err = chatmsg.NormalizeCardBizID(rawBizID)
	if !isString || err != nil {
		return r.cardFailure(e, record, "create_missing_biz_id", employeeTerminal("create_missing_biz_id"))
	}
	record.FlowStatus, record.Status = "PROCESSING", "card_streaming"
	record.Execution, record.Delivery = "running", "accepted"
	record.CardCreatedAt = time.Now().Format(time.RFC3339Nano)
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	view := employeeCardView{}
	confirmedPanel := employeeCardPanel("思考中…", []string{"executionTimeline"}, true)
	confirmedChildren := []string{"execution"}
	var updateErr error
	writeCtx, cancelWrites := context.WithCancel(r.ctx)
	defer cancelWrites()
	update := func(flow, title string) error {
		// 从发送开始节流，慢请求已消耗的时间不再追加到下一帧等待中。
		view.lastSend = time.Now()
		batches, err := view.batches(record.SurfaceID, title, flow != "FINISH")
		if err != nil {
			return err
		}
		for i, messages := range batches {
			batchFlow := flow
			if i < len(batches)-1 {
				batchFlow = "INPUTTING"
			}
			record.Status = "card_updating"
			if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
				return employeeTerminal("ledger_unavailable")
			}
			args := []string{"update-a2ui-card", "--biz-id", record.BizID, "--flow-status", batchFlow}
			if i == len(batches)-1 && view.answer != "" {
				annotation, _ := json.Marshal([]map[string]string{{"surfaceId": record.SurfaceID, "componentId": "answer", "type": "artifact"}})
				args = append(args, "--a2ui-annotations", string(annotation))
			}
			result, err := employeeA2UICall(writeCtx, r.cfg.Binding, messages, args...)
			if err != nil {
				return err
			}
			if actual := jsonScalar(result["flowStatus"]); actual != "" && actual != batchFlow {
				return employeeTerminal("card_status_unconfirmed")
			}
			for _, message := range messages {
				if update, ok := message["updateComponents"].(map[string]any); ok {
					for _, component := range update["components"].([]map[string]any) {
						if component["component"] == "CollapsiblePanel" {
							confirmedPanel = component
						}
						if component["id"] == "root" {
							confirmedChildren = component["children"].([]string)
						}
					}
				}
			}
			record.CardUpdates++
			record.FlowStatus, record.Status = batchFlow, "card_streaming"
			if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
				return employeeTerminal("ledger_unavailable")
			}
		}
		return nil
	}
	// Only a deterministic local size rejection permits a compact terminal frame.
	// Preserve the last confirmed panel children and all already delivered answers.
	updateFailure := func(stage string, cause error) error {
		var classified *employeeRunError
		if !errors.As(cause, &classified) || classified.Code != "card_payload_too_large" {
			return r.cardFailure(e, record, stage, cause)
		}
		panel := make(map[string]any, len(confirmedPanel))
		for key, value := range confirmedPanel {
			panel[key] = value
		}
		panel["title"] = "内容超出卡片容量，完整结果未交付"
		panel["fallbackMarkdown"] = panel["title"]
		panel["id"], panel["defaultExpanded"] = employeeCardPanelID(false), false
		delete(panel, "textEffect")
		children := append([]string(nil), confirmedChildren...)
		children[0] = employeeCardPanelID(false)
		record.Status = "card_updating"
		if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
			return employeeTerminal("ledger_unavailable")
		}
		result, err := employeeA2UICall(r.ctx, r.cfg.Binding, []map[string]any{
			employeeCardMessage("updateComponents", record.SurfaceID, map[string]any{"components": []map[string]any{panel, {"id": "root", "component": "Column", "children": children}}}),
		}, "update-a2ui-card", "--biz-id", record.BizID, "--flow-status", "FINISH")
		if err != nil {
			return r.cardFailure(e, record, "finish_unconfirmed", err)
		}
		if actual := jsonScalar(result["flowStatus"]); actual != "" && actual != "FINISH" {
			return r.cardFailure(e, record, "finish_unconfirmed", employeeTerminal("card_status_unconfirmed"))
		}
		record.CardUpdates++
		record.FlowStatus = "FINISH"
		if err := r.cardFailure(e, record, "result_undelivered", cause); err != nil {
			return err
		}
		record.Status = "card_incomplete"
		return nil
	}

	// One consumer owns the view, ledger and writes. The model's event reader
	// only enqueues bounded public events and never waits for a network write.
	progress := make(chan agentProgress, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(employeeCardFlushInterval)
		defer ticker.Stop()
		dirty := false
		for {
			select {
			case p, ok := <-progress:
				if !ok {
					return
				}
				view.accept(p)
				dirty = true
			case <-ticker.C:
			case <-writeCtx.Done():
				return
			}
			// 请求阻塞期间可能积压多个累计快照；只发布队列中的最新状态。
		drain:
			for drained := 0; drained < cap(progress); drained++ {
				select {
				case p, ok := <-progress:
					if !ok {
						break drain
					}
					view.accept(p)
					dirty = true
				default:
					break drain
				}
			}
			if dirty && updateErr == nil && time.Since(view.lastSend) >= employeeCardFlushInterval {
				updateErr = update("INPUTTING", "处理中…")
				dirty = false
			}
		}
	}()
	emit := func(p agentProgress) {
		select {
		case progress <- p:
			return
		default:
		}
		// Under backpressure retain recent public state, never block the RPC loop.
		select {
		case <-progress:
		default:
		}
		select {
		case progress <- p:
		default:
		}
	}
	// Confirm the placeholder before downloading images. Explicit attachments
	// supplied by an integration caller retain their original native transport.
	var agentErr error
	cleanup := func() {}
	if attachments == nil {
		attachments, cleanup, agentErr = prepareEmployeeImages(r.ctx, r.cfg.Binding.DWSProfile, r.dir, e)
	}
	defer cleanup()
	var answer string
	if agentErr == nil {
		ctx := context.WithValue(r.turnContext(e), agentProgressKey{}, emit)
		answer, agentErr = forwardEmployeeTurn(ctx, fwd, e.ConversationID, employeeTurnText(e), attachments...)
	}
	if errors.Is(agentErr, context.Canceled) || errors.Is(agentErr, context.DeadlineExceeded) {
		cancelWrites()
	}
	close(progress) // 本轮返回后回调已停止，终态之前等待所有卡片写入结束。
	<-done          // No view/ledger access races and no updates can follow FINISH.
	// A model timeout may cancel an in-flight write. Only finish if every
	// previous write was confirmed; use the still-live runtime for the terminal.
	writeCtx = r.ctx
	title, status := "处理完成", "card_finished"
	record.Execution = "success"
	if agentErr != nil || strings.TrimSpace(answer) == "" {
		record.Execution = "failure"
		title, status = "任务未完成，已保留现有结果", "card_incomplete"
		if failure := employeeSharedTurnFailure(agentErr); failure != nil {
			record.Execution, record.ExecutionErrorCode = failure.Execution, failure.Code
			title, view.answer = failure.Title, failure.Reply
		}
	} else {
		view.answer = answer
	}
	// 仅恢复已有卡片的交付，不重新调用 Agent；每轮至多补交一次最终快照。
	recoveryUsed := false
	recoverWrite := func(stage string, cause error) error {
		if !employeeCardCanRecover(cause) || recoveryUsed || r.ctx.Err() != nil {
			return cause
		}
		result := answer
		if result == "" {
			result = view.answer
		}
		if err := r.preserveCardResult(e, record, result); err != nil {
			return err
		}
		if err := r.cardFailure(e, record, stage, cause); err != nil {
			return err
		}
		recoveryUsed = true
		record.CardRecoveryAttempts++
		return update("FINISH", title)
	}
	fail := func(stage string, cause error) error {
		var classified *employeeRunError
		if errors.As(cause, &classified) && (classified.Code == "ledger_unavailable" || classified.Code == "card_alert_unavailable") {
			return cause
		}
		result := answer
		if result == "" {
			result = view.answer
		}
		if err := r.preserveCardResult(e, record, result); err != nil {
			return err
		}
		return updateFailure(stage, cause)
	}
	if updateErr != nil {
		if err := recoverWrite("update_unconfirmed", updateErr); err != nil {
			return fail("update_unconfirmed", err)
		}
	} else if err := update("FINISH", title); err != nil {
		if err = recoverWrite("finish_unconfirmed", err); err != nil {
			return fail("finish_unconfirmed", err)
		}
	}
	record.Status = status
	record.Delivery = "accepted"
	record.CardError, record.CardErrorCode = "", ""
	r.mu.Lock()
	delete(r.cardFailures, e.ConversationID)
	r.mu.Unlock()
	return nil
}
