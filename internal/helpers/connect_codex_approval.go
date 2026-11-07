// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"strings"
)

// 审批独立于公开进度；原始请求不得进入进度卡或普通日志。
type codexApprovalKey struct{}
type codexApprovalHandler func(context.Context, string, json.RawMessage) bool

func codexApprovalSink(ctx context.Context) codexApprovalHandler {
	h, _ := ctx.Value(codexApprovalKey{}).(codexApprovalHandler)
	return h
}

func codexApprovalMethod(method string) bool {
	return method == "item/commandExecution/requestApproval" || method == "item/fileChange/requestApproval" || method == "item/permissions/requestApproval"
}

func codexApprovalResult(method string, params json.RawMessage, allow bool) any {
	if method == "item/permissions/requestApproval" {
		var p struct {
			Permissions map[string]any `json:"permissions"`
		}
		_ = json.Unmarshal(params, &p)
		if !allow || p.Permissions == nil {
			p.Permissions = map[string]any{}
		}
		// 只回传请求原有的权限，授权仅限当前回合。
		return map[string]any{"permissions": p.Permissions, "scope": "turn"}
	}
	decision := "decline"
	if allow {
		decision = "accept"
	}
	return map[string]string{"decision": decision}
}

func codexApprovalMatches(params json.RawMessage, thread, turn string) bool {
	var p struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
	}
	return json.Unmarshal(params, &p) == nil && p.ThreadID == thread && strings.TrimSpace(p.TurnID) != "" && p.ItemID != "" && (turn == "" || p.TurnID == turn)
}

func codexApprovalWithChanges(params, changes json.RawMessage) json.RawMessage {
	if len(changes) == 0 || string(changes) == "null" {
		return params
	}
	var p map[string]any
	if json.Unmarshal(params, &p) != nil {
		return params
	}
	p["_dwsFileChanges"] = changes
	b, _ := json.Marshal(p)
	return b
}
