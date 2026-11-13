// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/google/uuid"
)

type employeeApproval struct {
	BizID    string
	decision chan bool
}

type employeeApprovalRecord struct {
	ID        string                  `json:"id"`
	Status    string                  `json:"status"`
	BizID     string                  `json:"bizId,omitempty"`
	UpdatedAt time.Time               `json:"updatedAt"`
	ExpiresAt time.Time               `json:"expiresAt"`
	Origin    *employeeApprovalOrigin `json:"origin,omitempty"`
}

type employeeApprovalOrigin struct {
	SenderName       string `json:"senderName,omitempty"`
	SenderID         string `json:"senderId"`
	ConversationID   string `json:"conversationId"`
	ConversationType string `json:"conversationType"`
	MessageID        string `json:"messageId"`
}

type employeeApprovalOriginKey struct{}

func employeeApprovalsEnabled(cfg digitalEmployeeAdapterConfig) bool {
	// 当前会话随时可能由主管切回人工审批，不能按启动时的 Yolo 布尔值停掉监听。
	return cfg.Binding.Channel == "codex"
}

func (r *employeeRuntime) turnContext(events ...employeeEvent) context.Context {
	if !employeeApprovalsEnabled(r.cfg) {
		return r.ctx
	}
	ctx := context.WithValue(r.ctx, codexApprovalKey{}, codexApprovalHandler(r.requestApproval))
	if len(events) != 0 {
		e := events[0]
		kind := "私聊"
		if e.Type == "user_im_message_receive_at" || strings.Contains(e.Type, "group") {
			kind = "群聊"
		}
		// 来源来自已接收事件，禁止模型通过审批参数冒充其他发起人。
		ctx = context.WithValue(ctx, employeeApprovalOriginKey{}, &employeeApprovalOrigin{SenderName: e.SenderName, SenderID: e.SenderID, ConversationID: e.ConversationID, ConversationType: kind, MessageID: e.MessageID})
	}
	return ctx
}

func employeeApprovalSource(origin *employeeApprovalOrigin) string {
	if origin == nil {
		return ""
	}
	line := func(value string) string { return truncateRunes(strings.Join(strings.Fields(value), " "), 256) }
	return "发起人：" + line(origin.SenderName) + "（" + line(origin.SenderID) + "）\n来源会话：" + origin.ConversationType + "（" + line(origin.ConversationID) + "）\n来源消息：" + line(origin.MessageID) + "\n\n"
}

// 无法完整、安全展示的请求只能拒绝，避免批准被截断或脱敏隐藏的操作。
var employeeApprovalAuthValue = regexp.MustCompile(`(?i)(bearer\s+\S+|https?://[^\s/]+:[^\s@]+@[^\s]+)`)

// 只在赋值、字段或带值长参数中识别凭据名，避免把正文中普通的 token 等词当作键。
var employeeApprovalKey = regexp.MustCompile(`(?i)([a-z_][a-z0-9_-]*)["']?\s*[=:]|--([a-z_][a-z0-9_-]*)\s+\S`)

func employeeApprovalHasSensitiveData(text string) bool {
	if employeeApprovalAuthValue.MatchString(text) {
		return true
	}
	for _, key := range employeeApprovalKey.FindAllStringSubmatch(text, -1) {
		// 日志中的正文键不是凭据名，正文里的实际凭据仍由本轮扫描识别。
		name := strings.ToLower(key[1] + key[2])
		if name != "text" && name != "content" && logging.IsSensitiveKey(name) {
			return true
		}
	}
	return false
}

func employeeApprovalSummary(method string, raw json.RawMessage) (string, bool) {
	summary, _, safe := employeeApprovalDetails(method, raw)
	return summary, safe
}

func employeeApprovalDetails(method string, raw json.RawMessage) (string, []employeeApprovalCardField, bool) {
	var p map[string]any
	if json.Unmarshal(raw, &p) != nil {
		return "审批请求格式无法识别", nil, false
	}
	if method == "item/fileChange/requestApproval" && p["_dwsFileChanges"] == nil {
		return "文件修改详情未提供，请在 Codex 中核对；本卡仅允许拒绝。", nil, false
	}
	if method == "item/commandExecution/requestApproval" && strings.TrimSpace(jsonScalar(p["command"])) == "" && p["networkApprovalContext"] == nil {
		return "未提供具体命令或网络目标，本卡仅允许拒绝。", nil, false
	}
	fields := map[string]any{}
	for _, key := range []string{"command", "cwd", "reason", "grantRoot", "networkApprovalContext", "additionalPermissions", "permissions", "_dwsFileChanges"} {
		if v := p[key]; v != nil {
			fields[key] = v
		}
	}
	var detail strings.Builder
	var cardFields []employeeApprovalCardField
	labels := map[string]string{"command": "执行命令", "cwd": "工作目录", "reason": "申请理由", "grantRoot": "授权根目录", "networkApprovalContext": "网络访问目标", "additionalPermissions": "额外权限", "permissions": "申请权限", "_dwsFileChanges": "文件变更"}
	for _, key := range []string{"command", "cwd", "reason", "grantRoot", "networkApprovalContext", "additionalPermissions", "permissions", "_dwsFileChanges"} {
		value, exists := fields[key]
		if !exists {
			continue
		}
		detail.WriteString(labels[key] + "：\n")
		text, ok := value.(string)
		if !ok {
			b, _ := json.MarshalIndent(value, "", "  ")
			text = string(b)
		}
		detail.WriteString(text)
		label := labels[key]
		if key == "command" {
			label = "待审核指令"
		}
		cardFields = append(cardFields, employeeApprovalCardField{Key: key, Label: label, Value: text, Code: key == "command" || key == "_dwsFileChanges"})
		detail.WriteString("\n")
	}
	text := detail.String()
	safe := len(text) <= 4096 && !employeeApprovalHasSensitiveData(text) && len(fields) > 0
	if !safe {
		return "请求包含无法完整展示的操作或敏感信息，请在 Codex 中核对；本卡仅允许拒绝。", nil, false
	}
	title := map[string]string{"item/commandExecution/requestApproval": "命令执行", "item/fileChange/requestApproval": "文件修改", "item/permissions/requestApproval": "权限申请"}[method]
	return title + "\n" + text, cardFields, true
}

func (r *employeeRuntime) requestApproval(parent context.Context, method string, params json.RawMessage) bool {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	id := uuid.NewString()
	surface := "approval-" + id
	summary, fields, canAllow := employeeApprovalDetails(method, params)
	origin, _ := parent.Value(employeeApprovalOriginKey{}).(*employeeApprovalOrigin)
	expires, _ := ctx.Deadline()
	cardInfo := employeeApprovalCardInfo{Origin: origin, Method: method, ExpiresAt: expires, Fields: fields}
	record := employeeApprovalRecord{ID: id, Status: "creating", UpdatedAt: time.Now(), ExpiresAt: expires, Origin: origin}
	path := filepath.Join(r.dir, "approvals", id+".json")
	if writeEmployeeJSON(path, record) != nil {
		return false
	}
	messages := []map[string]any{employeeCardMessage("createSurface", surface, map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{}})}
	messages = append(messages, employeeApprovalMessages(surface, id, summary, "等待主管审批（仅批准本次请求）", canAllow, cardInfo)...)
	pending := &employeeApproval{decision: make(chan bool, 1)}
	// 发送前登记随机 ID；只有服务端 bizId 已确认后的回调才有资格放行。
	r.mu.Lock()
	if r.approvals == nil {
		r.approvals = map[string]*employeeApproval{}
	}
	r.approvals[id] = pending
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.approvals, id); r.mu.Unlock() }()
	result, err := employeeA2UICall(ctx, r.cfg.Binding, messages, "send-a2ui-card", "--open-dingtalk-id", r.cfg.Binding.OperatorOpenDingTalkID, "--support-forward=false")
	if err != nil {
		return false
	}
	biz, err := chatmsg.NormalizeCardBizID(jsonScalar(result["bizId"]))
	if err != nil {
		return false
	}
	r.mu.Lock()
	pending.BizID = biz
	r.mu.Unlock()
	record.BizID = biz
	record.Status = "pending"
	record.UpdatedAt = time.Now()
	if writeEmployeeJSON(path, record) != nil {
		return false
	}
	allow := false
	record.Status = "expired"
	select {
	case allow = <-pending.decision:
		record.Status = "declined"
		if allow && canAllow {
			record.Status = "approved"
		}
	case <-ctx.Done():
	}
	allow = allow && canAllow && ctx.Err() == nil
	binding, bindErr := loadDigitalEmployeeBinding(deapConnectConfigDir(), r.cfg.Binding.DWSProfile)
	allow = allow && bindErr == nil && sameEmployeeBinding(binding, r.cfg.Binding) && employeeBindingState(binding) == "bound" && employeeDesiredState(binding) == "running"
	if !allow && record.Status == "approved" {
		record.Status = "expired"
	}
	record.UpdatedAt = time.Now()
	if writeEmployeeJSON(path, record) != nil {
		allow = false
		record.Status = "record_failed"
	}
	status := map[string]string{"approved": "主管已允许本次请求", "declined": "主管已拒绝", "expired": "审批已失效，未放行", "record_failed": "审批记录未能保存，未放行"}[record.Status]
	// 去掉按钮；更新失败不重发卡片，也不扩大已确认的审批范围。
	updateCtx, stop := context.WithTimeout(r.ctx, 10*time.Second)
	defer stop()
	_, _ = employeeA2UICall(updateCtx, r.cfg.Binding, employeeApprovalMessages(surface, id, summary, status, false, cardInfo), "update-a2ui-card", "--biz-id", biz, "--flow-status", "FINISH")
	return allow
}

var employeeApprovalOperator = func(ctx context.Context, b digitalEmployeeBinding, uid string) (string, error) {
	token, err := employeeSelfIdentityToken(ctx, b.DWSProfile)
	if err != nil || token == nil {
		return "", employeeTerminal("approval_identity_unavailable")
	}
	return resolveExactOperatorOpenDingTalkID(ctx, token.AccessToken, uid)
}

// A2UI 的操作者取自受信事件信封；action.context 中自报的身份不能用于授权。
type employeeApprovalAction struct {
	Name    string `json:"name"`
	Context struct {
		ID       string `json:"approvalId"`
		Decision string `json:"decision"`
	} `json:"context"`
	SurfaceID   string `json:"surfaceId"`
	ComponentID string `json:"sourceComponentId"`
}

func (r *employeeRuntime) handleApprovalEvent(line []byte) bool {
	var event struct {
		Type    string `json:"type"`
		Payload struct {
			Body struct {
				ActionData employeeApprovalAction `json:"actionData"`
				A2UI       struct {
					Version string                 `json:"version"`
					Action  employeeApprovalAction `json:"action"`
				} `json:"a2uiEvent"`
				BizInfo struct {
					BizID string `json:"bizId"`
				} `json:"bizInfoDTO"`
				Operator struct {
					UID    json.Number `json:"uid"`
					OpenID string      `json:"openDingTalkId"`
				} `json:"operatorDTO"`
			} `json:"body"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &event) != nil || event.Type != "user_card_action_triggered" {
		return false
	}
	body := event.Payload.Body
	action := body.ActionData
	if body.A2UI.Version != "" {
		action = body.A2UI.Action
		if body.A2UI.Version != "1.0" {
			return true
		}
	}
	// 兼容分支也必须证明来自当前审批卡的指定动作与按钮，缺字段的旧事件拒绝。
	if action.Name != "dws_employee_approval" || action.SurfaceID != "approval-"+action.Context.ID || action.ComponentID != action.Context.Decision {
		return true
	}
	id, decision := action.Context.ID, action.Context.Decision
	r.mu.Lock()
	pending := r.approvals[id]
	valid := pending != nil && pending.BizID != "" && pending.BizID == body.BizInfo.BizID
	r.mu.Unlock()
	if !valid || (decision != "approve" && decision != "decline") {
		return true
	}
	operator := body.Operator.OpenID
	uid, err := strconv.ParseInt(string(body.Operator.UID), 10, 64)
	if operator == "" && (err != nil || uid <= 0) {
		return true
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// 真实 A2UI 已携带员工作用域下的操作者开放 ID；旧卡片 UID 才需要精确解析。
		if operator == "" {
			ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
			defer cancel()
			var lookupErr error
			operator, lookupErr = employeeApprovalOperator(ctx, r.cfg.Binding, strconv.FormatInt(uid, 10))
			if lookupErr != nil {
				return
			}
		}
		if !validEmployeeSelfIdentity(operator) || operator != r.cfg.Binding.OperatorOpenDingTalkID {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.approvals[id] != pending {
			return
		}
		select {
		case pending.decision <- decision == "approve":
		default:
		}
	}()
	return true
}
