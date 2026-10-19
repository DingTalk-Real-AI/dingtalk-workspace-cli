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

func employeeCardResponseError(stdout, stderr []byte) error {
	var response struct {
		Success *bool `json:"success"`
		Error   struct {
			Category         string `json:"category"`
			Type             string `json:"type"`
			ExecutionStarted *bool  `json:"execution_started"`
		} `json:"error"`
	}
	// Only stable structured fields establish pre-execution rejection. Never
	// classify from message text, or assume a generic server failure had no effect.
	if json.Unmarshal(stderr, &response) == nil {
		e := response.Error
		if e.ExecutionStarted != nil {
			if !*e.ExecutionStarted {
				return employeeTerminal("card_write_rejected")
			}
			return employeeTerminal("card_write_unconfirmed")
		}
		if e.Category == "validation" || e.Category == "auth" || e.Type == "validation" || e.Type == "auth" {
			return employeeTerminal("card_write_rejected")
		}
		if e.Category != "" && e.Category != "network" || response.Success != nil && !*response.Success {
			return employeeTerminal("card_response_failed")
		}
	}
	response.Success = nil
	if json.Unmarshal(stdout, &response) == nil && response.Success != nil && !*response.Success {
		return employeeTerminal("card_response_failed")
	}
	return employeeTerminal("card_write_unconfirmed")
}

func (r *employeeRuntime) cardFailure(e employeeEvent, record *employeeTaskRecord, stage string, err error) error {
	record.Status, record.CardError = "needs_review", stage
	record.Delivery = "unknown"
	record.CardErrorCode = "card_write_unconfirmed"
	var classified *employeeRunError
	if errors.As(err, &classified) {
		record.CardErrorCode = classified.Code
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
	data, err := employeeMarshalJSON(map[string]any{"code": code, "level": "warning", "failures": count, "stage": stage, "reason": record.CardErrorCode, "task": filepath.Base(r.recordPath(e)), "time": time.Now().Format(time.RFC3339Nano)})
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

func employeeCardPanel(title string, children []string, running bool) map[string]any {
	panel := map[string]any{"id": "execution", "component": "CollapsiblePanel", "title": title, "children": children, "variant": "reasoning", "fallbackMarkdown": title}
	if running {
		// DingTalk's public progress pattern renders this marker as its loading
		// animation. flowStatus alone does not decorate an A2UI panel title.
		panel["title"] = "[AIGenerating]" + title
		panel["textEffect"] = "shimmer"
	}
	return panel
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
	// Keep progress bounded without forwarding raw tool names, input or output.
	start := 0
	if len(v.items) > 32 {
		start = len(v.items) - 32
	}
	for index, p := range v.items[start:] {
		id := "step-" + jsonScalar(v.itemBase+index+start)
		text := p.Text
		if p.Kind == "tool" {
			text = "正在调用工具…"
			if p.Done {
				text = "工具调用已结束"
			}
		}
		children = append(children, id)
		components = append(components, map[string]any{"id": id, "component": "Markdown", "content": text})
	}
	components = append(components, employeeCardPanel(title, children, running))
	rootChildren := []string{"execution"}
	if v.answer != "" {
		rootChildren = append(rootChildren, "answer")
		components = append(components, map[string]any{"id": "answer", "component": "Markdown", "content": map[string]any{"path": "/answer"}})
	}
	components = append(components, map[string]any{"id": "root", "component": "Column", "children": rootChildren})
	return []map[string]any{
		employeeCardMessage("updateDataModel", surface, map[string]any{"path": "/answer", "value": v.answer}),
		employeeCardMessage("updateComponents", surface, map[string]any{"components": components}),
	}
}

func (r *employeeRuntime) processCard(e employeeEvent, record *employeeTaskRecord, fwd progressForwarder, attachments ...connectMediaAttachment) error {
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
	initial := []map[string]any{
		employeeCardMessage("createSurface", record.SurfaceID, map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{"answer": ""}}),
		employeeCardMessage("updateComponents", record.SurfaceID, map[string]any{"components": []map[string]any{
			{"id": "root", "component": "Column", "children": []string{"execution"}}, employeeCardPanel("思考中…", []string{}, true),
		}}),
	}
	result, err := employeeA2UICall(r.ctx, r.cfg.Binding, initial, append([]string{"send-a2ui-card"}, target...)...)
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
	confirmedPanel := employeeCardPanel("思考中…", []string{}, true)
	var updateErr error
	writeCtx, cancelWrites := context.WithCancel(r.ctx)
	defer cancelWrites()
	update := func(flow, title string) error {
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
						if component["id"] == "execution" {
							confirmedPanel = component
						}
					}
				}
			}
			record.CardUpdates++
			record.FlowStatus, record.Status = batchFlow, "card_streaming"
			view.lastSend = time.Now()
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
		delete(panel, "textEffect")
		record.Status = "card_updating"
		if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
			return employeeTerminal("ledger_unavailable")
		}
		result, err := employeeA2UICall(r.ctx, r.cfg.Binding, []map[string]any{
			employeeCardMessage("updateComponents", record.SurfaceID, map[string]any{"components": []map[string]any{panel}}),
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
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		dirty := false
		for {
			select {
			case p, ok := <-progress:
				if !ok {
					return
				}
				if updateErr != nil {
					continue
				}
				view.accept(p)
				dirty = true
			case <-ticker.C:
			case <-writeCtx.Done():
				return
			}
			if dirty && updateErr == nil && time.Since(view.lastSend) >= time.Second {
				updateErr = update("INPUTTING", "正在处理…")
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
	prompt := employeeImagePrompt(employeeTurnText(e), attachments)
	answer, agentErr := fwd.forwardProgress(r.ctx, e.ConversationID, prompt, emit, attachments...)
	if errors.Is(agentErr, context.Canceled) || errors.Is(agentErr, context.DeadlineExceeded) {
		cancelWrites()
	}
	close(progress) // forwardProgress guarantees callbacks have stopped.
	<-done          // No view/ledger access races and no updates can follow FINISH.
	record.Execution = "success"
	if agentErr != nil {
		record.Execution = "failure"
		if errors.Is(agentErr, context.Canceled) || r.ctx.Err() != nil {
			record.Execution = "cancelled"
		}
	}
	if updateErr != nil {
		return updateFailure("update_unconfirmed", updateErr)
	}
	// A model timeout may cancel an in-flight write. Only finish if every
	// previous write was confirmed; use the still-live runtime for the terminal.
	writeCtx = r.ctx
	title, status := "处理完成", "card_finished"
	if agentErr != nil || strings.TrimSpace(answer) == "" {
		title, status = "任务未完成，已保留现有结果", "card_incomplete"
	} else {
		view.answer = answer
	}
	if err := update("FINISH", title); err != nil {
		return updateFailure("finish_unconfirmed", err)
	}
	record.Status = status
	record.Delivery = "accepted"
	r.mu.Lock()
	delete(r.cardFailures, e.ConversationID)
	r.mu.Unlock()
	return nil
}
