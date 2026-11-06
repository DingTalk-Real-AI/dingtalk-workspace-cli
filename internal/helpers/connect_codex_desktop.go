// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// 桌面端已持有写锁时，向其现有会话提交一次，不另建线程、不修改写锁。
// 权限始终继承原会话；需要人工决策的请求单独转为主管审批卡。
// IPC 是桌面端的版本化本地协议；版本不匹配必须明确失败，不能降级重放消息。
var codexForwardDesktopTurn = forwardCodexDesktopTurn
var codexDesktopDial = dialCodexDesktop
var codexDesktopWithTimeout = context.WithTimeout

const codexDesktopFrameLimit = 32 << 20
const codexDesktopReadyTimeout = 2 * time.Minute
const codexDesktopTurnTimeout = time.Hour

type codexDesktopTurnError struct {
	Code      string
	Submitted bool
	Cause     error
}

func (e *codexDesktopTurnError) Error() string { return e.Code }
func (e *codexDesktopTurnError) Unwrap() error { return e.Cause }

type codexDesktopMessage struct {
	Type              string          `json:"type"`
	RequestID         string          `json:"requestId,omitempty"`
	SourceClientID    string          `json:"sourceClientId,omitempty"`
	TargetClientID    string          `json:"targetClientId,omitempty"`
	TargetClientIDs   []string        `json:"targetClientIds,omitempty"`
	Version           int             `json:"version"`
	Method            string          `json:"method,omitempty"`
	Params            any             `json:"params,omitempty"`
	ResultType        string          `json:"resultType,omitempty"`
	HandledByClientID string          `json:"handledByClientId,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	Error             string          `json:"error,omitempty"`
	Response          any             `json:"response,omitempty"`
}

type codexDesktopFrame struct {
	message codexDesktopMessage
	err     error
}

type codexDesktopClient struct {
	conn   net.Conn
	client string
	owner  string
	thread string
	frames chan codexDesktopFrame
}

func (c *codexDesktopClient) send(m codexDesktopMessage) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > codexDesktopFrameLimit {
		return errors.New("desktop IPC frame too large")
	}
	frame := make([]byte, 4+len(b))
	binary.LittleEndian.PutUint32(frame, uint32(len(b)))
	copy(frame[4:], b)
	_, err = io.Copy(c.conn, bytes.NewReader(frame))
	return err
}

func (c *codexDesktopClient) read(ctx context.Context) {
	defer close(c.frames)
	for {
		var header [4]byte
		_, err := io.ReadFull(c.conn, header[:])
		var m codexDesktopMessage
		if err == nil {
			n := binary.LittleEndian.Uint32(header[:])
			if n == 0 || n > codexDesktopFrameLimit {
				err = errors.New("invalid desktop IPC frame length")
			} else {
				b := make([]byte, n)
				_, err = io.ReadFull(c.conn, b)
				if err == nil {
					err = json.Unmarshal(b, &m)
				}
			}
		}
		select {
		case c.frames <- codexDesktopFrame{message: m, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func (c *codexDesktopClient) next(ctx context.Context) (codexDesktopMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return codexDesktopMessage{}, ctx.Err()
		case f, ok := <-c.frames:
			if !ok {
				return codexDesktopMessage{}, io.EOF
			}
			if f.err != nil {
				return codexDesktopMessage{}, f.err
			}
			// 本接入端从不声明拥有别人的会话，也不处理其他客户端的操作请求。
			if f.message.Type == "client-discovery-request" {
				if err := c.send(codexDesktopMessage{Type: "client-discovery-response", RequestID: f.message.RequestID, Response: map[string]bool{"canHandle": false}}); err != nil {
					return codexDesktopMessage{}, err
				}
				continue
			}
			return f.message, nil
		}
	}
}

func (c *codexDesktopClient) request(ctx context.Context, method string, version int, params any) (codexDesktopMessage, error) {
	id := uuid.NewString()
	if err := c.send(codexDesktopMessage{Type: "request", RequestID: id, SourceClientID: c.client, TargetClientID: c.owner, Method: method, Version: version, Params: params}); err != nil {
		return codexDesktopMessage{}, err
	}
	for {
		m, err := c.next(ctx)
		if err != nil {
			return m, err
		}
		if m.Type != "response" || m.RequestID != id {
			continue
		}
		if m.ResultType != "success" || m.Method != method || (c.owner != "" && m.HandledByClientID != c.owner) {
			return m, errors.New("desktop IPC request unconfirmed")
		}
		return m, nil
	}
}

type codexDesktopTurn struct {
	TurnID string `json:"turnId"`
	Status string `json:"status"`
	Error  any    `json:"error"`
	Items  []struct {
		ID      string          `json:"id"`
		Type    string          `json:"type"`
		Phase   string          `json:"phase"`
		Text    string          `json:"text"`
		Status  string          `json:"status"`
		Changes json.RawMessage `json:"changes"`
	} `json:"items"`
	CompletedItemIDs map[string]any `json:"completedItemIds"`
}

type codexDesktopApproval struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type codexDesktopSnapshot struct {
	CurrentPermissions    json.RawMessage        `json:"currentPermissions"`
	LatestThreadSettings  json.RawMessage        `json:"latestThreadSettings"`
	RuntimeWorkspaceRoots []string               `json:"runtimeWorkspaceRoots"`
	Requests              []codexDesktopApproval `json:"requests"`
	ID                    string                 `json:"id"`
	ThreadRuntimeStatus   struct {
		Type string `json:"type"`
	} `json:"threadRuntimeStatus"`
	Turns       []codexDesktopTurn `json:"turns"`
	TurnHistory struct {
		History struct {
			EntitiesByKey map[string]codexDesktopTurn `json:"entitiesByKey"`
		} `json:"history"`
	} `json:"turnHistory"`
}

func (c *codexDesktopClient) snapshot(ctx context.Context) (codexDesktopSnapshot, error) {
	// 重复订阅会让持有者返回当前完整快照；不依赖私有增量补丁或推理数据格式。
	if err := c.send(codexDesktopMessage{Type: "broadcast", SourceClientID: c.client, TargetClientIDs: []string{c.owner}, Method: "thread-stream-following-changed", Version: 1, Params: map[string]any{"hostId": "local", "conversationId": c.thread, "following": true}}); err != nil {
		return codexDesktopSnapshot{}, err
	}
	for {
		m, err := c.next(ctx)
		if err != nil {
			return codexDesktopSnapshot{}, err
		}
		if m.Type != "broadcast" || m.Method != "thread-stream-state-changed" || m.SourceClientID != c.owner {
			continue
		}
		b, err := json.Marshal(m.Params)
		if err != nil {
			return codexDesktopSnapshot{}, err
		}
		var p struct {
			ConversationID string `json:"conversationId"`
			HostID         string `json:"hostId"`
			Change         struct {
				Type              string               `json:"type"`
				ConversationState codexDesktopSnapshot `json:"conversationState"`
			} `json:"change"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			return codexDesktopSnapshot{}, err
		}
		if p.ConversationID != c.thread || p.HostID != "local" || p.Change.Type != "snapshot" {
			continue
		}
		if m.Version != 11 || p.Change.ConversationState.ID != c.thread {
			return codexDesktopSnapshot{}, errors.New("unsupported desktop IPC snapshot")
		}
		return p.Change.ConversationState, nil
	}
}

func codexDesktopPause(ctx context.Context) error {
	t := time.NewTimer(employeeCardFlushInterval)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func forwardCodexDesktopTurn(ctx context.Context, thread, text string, attachments []connectMediaAttachment, onDelta func(string), instructions string) (answer string, err error) {
	parent := ctx
	// 即使调用方未设回合超时，桌面持有者失联或会话持续繁忙也不能无限轮询。
	ctx, readyCancel := codexDesktopWithTimeout(parent, codexDesktopReadyTimeout)
	defer readyCancel()
	submitted := false
	defer func() {
		if err != nil && !errors.Is(err, errCodexCompletedWithoutReply) {
			code := "codex_desktop_unavailable"
			if submitted {
				code = "codex_desktop_result_unknown"
			}
			var known *codexDesktopTurnError
			if !errors.As(err, &known) {
				err = &codexDesktopTurnError{Code: code, Submitted: submitted, Cause: err}
			}
		}
	}()
	conn, err := codexDesktopDial(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	readCtx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(readCtx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c := &codexDesktopClient{conn: conn, client: "uninitialized", thread: thread, frames: make(chan codexDesktopFrame, 16)}
	go c.read(readCtx)
	m, err := c.request(ctx, "initialize", 0, map[string]string{"clientType": "dws"})
	if err != nil {
		return "", err
	}
	var initialized struct {
		ClientID string `json:"clientId"`
	}
	if json.Unmarshal(m.Result, &initialized) != nil || initialized.ClientID == "" {
		return "", errors.New("desktop IPC initialize missing client")
	}
	c.client = initialized.ClientID
	m, err = c.request(ctx, "thread-owner-discovery", 1, map[string]string{"hostId": "local", "conversationId": thread})
	if err != nil {
		return "", fmt.Errorf("desktop thread owner unavailable: %w", err)
	}
	if m.HandledByClientID == "" {
		return "", errors.New("desktop thread owner unavailable: missing owner")
	}
	c.owner = m.HandledByClientID
	for {
		snapshot, e := c.snapshot(ctx)
		if e != nil {
			return "", e
		}
		if snapshot.ThreadRuntimeStatus.Type != "active" {
			break
		}
		if emit := agentProgressSink(ctx); emit != nil {
			emit(agentProgress{ID: "desktop-wait", Kind: "progress", Text: "同一会话正在执行，等待当前任务完成…"})
		}
		if e := codexDesktopPause(ctx); e != nil {
			return "", e
		}
	}
	readyCancel()
	ctx, turnCancel := codexDesktopWithTimeout(parent, codexDesktopTurnTimeout)
	defer turnCancel()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	input := []map[string]any{{"type": "text", "text": text, "text_elements": []any{}}}
	for _, a := range attachments {
		if a.MediaType == "image" && a.LocalPath != "" {
			input = append(input, map[string]any{"type": "localImage", "path": a.LocalPath})
		}
	}
	// 一旦开始写请求，失败也不重发；即便应答丢失，持有者可能已经执行。
	submitted = true
	request := map[string]any{"threadId": thread, "input": input, "clientUserMessageId": uuid.NewString(), "additionalContext": map[string]any{"dws.digital_employee_policy": map[string]string{"kind": "application", "value": instructions}}}
	m, err = c.request(ctx, "thread-follower-start-turn", 2, map[string]any{"conversationId": thread, "turnStart": map[string]any{"request": request, "context": map[string]bool{"inheritThreadSettings": true}}})
	if err != nil {
		return "", err
	}
	var started struct {
		Result struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		} `json:"result"`
	}
	if json.Unmarshal(m.Result, &started) != nil || started.Result.Turn.ID == "" {
		return "", errors.New("desktop turn receipt missing id")
	}
	progress := codexPublicProgress{emit: agentProgressSink(ctx)}
	seen := make(map[string]string)
	approvals := newCodexDesktopApprovals(ctx)
	defer approvals.close()
	for {
		snapshot, e := c.snapshot(ctx)
		if e != nil {
			return "", e
		}
		for i := range snapshot.Requests {
			r := &snapshot.Requests[i]
			if r.Method != "item/fileChange/requestApproval" {
				continue
			}
			var p struct {
				ItemID string `json:"itemId"`
			}
			_ = json.Unmarshal(r.Params, &p)
			for _, turn := range snapshot.TurnHistory.History.EntitiesByKey {
				if turn.TurnID != started.Result.Turn.ID {
					continue
				}
				for _, item := range turn.Items {
					if item.ID == p.ItemID {
						r.Params = codexApprovalWithChanges(r.Params, item.Changes)
					}
				}
			}
		}
		approvals.syncPermissions(snapshot)
		if err := approvals.update(ctx, c, snapshot.Requests, started.Result.Turn.ID); err != nil {
			return "", err
		}
		turns := snapshot.Turns
		for _, t := range snapshot.TurnHistory.History.EntitiesByKey {
			turns = append(turns, t)
		}
		for _, t := range turns {
			if t.TurnID != started.Result.Turn.ID {
				continue
			}
			for _, item := range t.Items {
				switch item.Type {
				case "agentMessage", "commandExecution", "mcpToolCall", "dynamicToolCall", "webSearch", "collabAgentToolCall":
				default:
					continue
				}
				_, done := t.CompletedItemIDs[item.ID]
				method := "item/started"
				if done || t.Status != "inProgress" || item.Status == "completed" || item.Status == "failed" {
					method = "item/completed"
				}
				// 只保留公开投影的去重标识，禁止把推理或工具原始内容存进进度。
				key := method + item.Type + item.Phase + item.Status
				if item.Type == "agentMessage" && codexPublicMessageKind(item.Phase) != "" {
					key += publicProgressPreview(item.Text)
				}
				if seen[item.ID] != key {
					seen[item.ID] = key
					b, _ := json.Marshal(map[string]any{"threadId": thread, "item": item})
					progress.handle(method, b, thread)
				}
				if item.Type == "agentMessage" && (item.Phase == "final_answer" || item.Phase == "") {
					answer = item.Text
				}
			}
			switch t.Status {
			case "completed":
				if t.Error != nil {
					return "", &codexDesktopTurnError{Code: "codex_desktop_turn_failed", Submitted: true}
				}
				if strings.TrimSpace(answer) == "" {
					return "", errCodexCompletedWithoutReply
				}
				if onDelta != nil {
					onDelta(answer)
				}
				return answer, nil
			case "failed", "interrupted":
				return "", &codexDesktopTurnError{Code: "codex_desktop_turn_failed", Submitted: true}
			}
		}
		if e := codexDesktopPause(ctx); e != nil {
			return "", e
		}
	}
}

type codexDesktopApprovalWait struct {
	request     codexDesktopApproval
	cancel      context.CancelFunc
	result      chan bool
	invalidated bool
}
type codexDesktopApprovals struct {
	ctx         context.Context
	wg          sync.WaitGroup
	waits       map[string]*codexDesktopApprovalWait
	handled     map[string]bool
	permissions string
}

// 仅比较权限字段；模型、标题或正文变化不能误使审批失效。
func (s codexDesktopSnapshot) permissionKey() string {
	fields := map[string]any{}
	for name, raw := range map[string]json.RawMessage{"current": s.CurrentPermissions, "settings": s.LatestThreadSettings} {
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			continue
		}
		for _, key := range []string{"approvalPolicy", "approvalsReviewer", "sandboxPolicy", "activePermissionProfile", "runtimeWorkspaceRoots"} {
			if value := values[key]; len(value) != 0 {
				fields[name+"."+key] = value
			}
		}
	}
	if len(s.RuntimeWorkspaceRoots) != 0 {
		fields["roots"] = s.RuntimeWorkspaceRoots
	}
	if len(fields) == 0 {
		return ""
	}
	b, _ := json.Marshal(fields)
	return string(b)
}

func (a *codexDesktopApprovals) syncPermissions(snapshot codexDesktopSnapshot) {
	key := snapshot.permissionKey()
	if a.permissions != key || key == "" {
		// 扩权与收紧都撤销旧卡；不得用旧点击替新权限下的请求做决定。
		// 缺失基线、权限首次出现或后续消失也不能继续使用旧审批。
		for _, wait := range a.waits {
			wait.invalidated = true
			wait.cancel()
		}
	}
	a.permissions = key
}

func newCodexDesktopApprovals(ctx context.Context) *codexDesktopApprovals {
	return &codexDesktopApprovals{ctx: ctx, waits: map[string]*codexDesktopApprovalWait{}, handled: map[string]bool{}}
}
func (a *codexDesktopApprovals) close() {
	defer a.wg.Wait()
	for _, w := range a.waits {
		w.cancel()
	}
}
func (a *codexDesktopApprovals) update(ctx context.Context, c *codexDesktopClient, requests []codexDesktopApproval, turn string) error {
	h := codexApprovalSink(a.ctx)
	if h == nil {
		return nil
	}
	present := map[string]bool{}
	for _, r := range requests {
		if !codexApprovalMethod(r.Method) || !codexApprovalMatches(r.Params, c.thread, turn) || len(r.ID) == 0 {
			continue
		}
		key := string(r.ID)
		present[key] = true
		if a.waits[key] != nil || a.handled[key] {
			continue
		}
		wait, cancel := context.WithCancel(a.ctx)
		w := &codexDesktopApprovalWait{request: r, cancel: cancel, result: make(chan bool, 1)}
		a.waits[key] = w
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			w.result <- h(wait, w.request.Method, w.request.Params) && wait.Err() == nil
		}()
	}
	for key, w := range a.waits {
		if !present[key] {
			w.cancel()
			delete(a.waits, key)
			continue
		}
		allow, ready := false, w.invalidated
		if !ready {
			select {
			case allow = <-w.result:
				ready = true
			default:
			}
		}
		if ready {
			params := map[string]any{"conversationId": c.thread, "requestId": w.request.ID}
			result := codexApprovalResult(w.request.Method, w.request.Params, allow)
			method := map[string]string{"item/commandExecution/requestApproval": "thread-follower-command-approval-decision", "item/fileChange/requestApproval": "thread-follower-file-approval-decision", "item/permissions/requestApproval": "thread-follower-permissions-request-approval-response"}[w.request.Method]
			if w.request.Method == "item/permissions/requestApproval" {
				params["response"] = result
			} else {
				params["decision"] = result.(map[string]string)["decision"]
			}
			w.cancel()
			delete(a.waits, key)
			a.handled[key] = true
			if _, err := c.request(ctx, method, 1, params); err != nil {
				return err
			}
		}
	}
	return nil
}
