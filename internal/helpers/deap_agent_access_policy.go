// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// 正式 connect 仅为已发布 local_agent 建立服务端绑定；无此回执的旧通道
// 继续使用本地白名单。平台在可见范围撤销或员工类型变更后停止旧绑定投递，
// 类型变更需重新绑定。历史 supervisorProfile 不是平台授权的证据。
func employeeBindingAccessPolicy(b digitalEmployeeBinding) string {
	if validMachineString(b.RuntimeBindingID) {
		return "platform"
	}
	return "local_allowlist"
}

var employeeSelfIdentityToken = func(ctx context.Context, profile string) (*auth.TokenData, error) {
	return auth.NewOAuthProvider(deapConnectConfigDir(), nil).GetTokenSnapshotForProfile(ctx, profile)
}

func validEmployeeSelfIdentity(id string) bool {
	return id == strings.TrimSpace(id) && validMachineString(id) && strings.IndexFunc(id, unicode.IsControl) < 0
}

// 自身开放 ID 必须来自员工 Token 上下文，发布详情中的同名字段属于另一身份作用域。
// 旧配置没有作用域标记时，只在启动解析员工自身一次，不查主管或可见范围、不改绑定。
func employeeBindingSelfIdentity(parent context.Context, b digitalEmployeeBinding, required bool) (string, error) {
	var cfg digitalEmployeeAdapterConfig
	raw, err := os.ReadFile(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"))
	if err == nil && json.Unmarshal(raw, &cfg) == nil && sameEmployeeBinding(cfg.Binding, b) && cfg.Binding.RuntimeBindingID == b.RuntimeBindingID && cfg.Binding.DeviceID == b.DeviceID && validEmployeeSelfIdentity(cfg.SelfOpenDingTalkID) && (!required || cfg.SelfIdentityProfile == b.DWSProfile) {
		return cfg.SelfOpenDingTalkID, nil
	}
	if !required {
		return "", nil
	}
	failure := func() (string, error) {
		return "", apperrors.NewValidation("员工自身身份暂不可用，已停止启动", apperrors.WithReason("self_identity_unavailable"), apperrors.WithRetryable(false))
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	token, err := employeeSelfIdentityToken(ctx, b.DWSProfile)
	if err != nil || token == nil || auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID}) != b.DWSProfile {
		return failure()
	}
	self, err := resolveExactOperatorOpenDingTalkID(ctx, token.AccessToken, token.UserID)
	if err != nil || !validEmployeeSelfIdentity(self) {
		return failure()
	}
	return self, nil
}
