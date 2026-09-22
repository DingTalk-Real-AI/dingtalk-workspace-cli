// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

type digitalEmployeeLoginSession struct {
	DigitalProfile    string
	DigitalToken      *auth.TokenData
	SupervisorProfile string
	SupervisorToken   *auth.TokenData
}

type digitalEmployeeLoginResult struct {
	Status                  string `json:"status"`
	AgentUUID               string `json:"agentUuid"`
	DWSProfile              string `json:"dwsProfile"`
	CurrentProfilePreserved bool   `json:"currentProfilePreserved"`
	UseOnce                 string `json:"useOnce"`
	SelectProfile           string `json:"selectProfile"`
}

func runDeapAgentLogin(cmd *cobra.Command, _ []string) error {
	agentUUID := strings.TrimSpace(MustGetStringFlag(cmd, "agent-uuid"))
	requestedClientID := strings.TrimSpace(MustGetStringFlag(cmd, "client-id"))
	if commandDryRun(cmd) {
		return writeDWSMachinePlan(cmd, map[string]any{
			"status": "planned", "agentUuid": agentUUID,
			"steps": []string{"validate_published", "request_auth_code", "managed_exchange", "verify_online_identity", "persist_exact_profile"},
		})
	}

	published, err := queryPublishedDigitalEmployee(cmd.Context(), agentUUID)
	if err != nil {
		return err
	}
	session, err := loginDigitalEmployee(cmd.Context(), deapConnectConfigDir(), agentUUID, requestedClientID, published)
	if err != nil {
		return err
	}
	return writeDWSMachineEnvelope(cmd, digitalEmployeeLoginResult{
		Status: "profile_saved", AgentUUID: agentUUID, DWSProfile: session.DigitalProfile,
		CurrentProfilePreserved: true,
		UseOnce:                 fmt.Sprintf("dws --profile %s <command>", session.DigitalProfile),
		SelectProfile:           fmt.Sprintf("dws profile use %s", session.DigitalProfile),
	})
}

// loginDigitalEmployee 是 manage login 与 connect 共用的安全登录内核。它只做
// 授权码换票、在线身份核验和精确 Profile 落盘，不包含 local_agent、DSH 或
// Bridge 逻辑，并始终保留发起操作的主管 Profile 为当前 Profile。
func loginDigitalEmployee(ctx context.Context, configDir, agentUUID, requestedClientID string, published map[string]any) (_ *digitalEmployeeLoginSession, resultErr error) {
	started := time.Now()
	stage := "published_identity"
	slog.DebugContext(ctx, "dingtalk_tag_login_started", "hasClientHint", requestedClientID != "")
	defer func() {
		slog.DebugContext(ctx, "dingtalk_tag_login_completed", "stage", stage, "success", resultErr == nil, "durationMs", time.Since(started).Milliseconds())
	}()
	supervisorSelector, supervisor, err := currentSupervisorProfile(ctx, configDir)
	if err != nil {
		return nil, err
	}
	publishedIdentity, ok := publishedDigitalEmployeeIdentity(published)
	if !ok {
		return nil, apperrors.NewInternal("数字员工发布详情缺少登录所需的内部身份信息")
	}

	authArgs := map[string]any{"agentUuid": agentUUID}
	if requestedClientID != "" {
		authArgs["clientId"] = requestedClientID
	}
	stage = "request_auth_code"
	authorization, err := callDeapJSON(ctx, deapAgentAuthCodeTool, authArgs, true)
	if err != nil {
		return nil, fmt.Errorf("request digital employee authorization: %w", err)
	}
	stage = "authorization_response"
	authData := businessDataMap(authorization)
	dwsClientID := requiredJSONScalar(authData, "dwsClientId")
	dwsAuthCode := requiredJSONScalar(authData, "dwsAuthCode")
	if dwsClientID == "" || dwsAuthCode == "" {
		return nil, apperrors.NewInternal("数字员工授权响应缺少登录所需的内部身份或凭证信息")
	}
	// 当前详情公开 corpId/userId；授权工具仅提供换票凭证。在线身份核验仍须
	// 与发布详情一致，不再依赖旧 uid/staffId/orgId 字段。
	stage = "managed_exchange"
	token, err := deapConnectManagedExchange(ctx, configDir, auth.ManagedExchangeRequest{
		ClientID: dwsClientID, AuthCode: dwsAuthCode,
		ExpectedUserID: publishedIdentity.UserID, ExpectedCorpID: publishedIdentity.CorpID,
		PreserveProfile: supervisorSelector, ResolveIdentity: resolveDigitalEmployeeManagedIdentity,
	})
	// 尽早清空本地变量，后续错误和输出都不再接触授权码。
	dwsAuthCode = ""
	if err != nil {
		return nil, err
	}
	stage = "profile_identity"
	digitalProfile := auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID})
	if digitalProfile == "" || token.UserID != publishedIdentity.UserID || token.CorpID != publishedIdentity.CorpID {
		return nil, apperrors.NewInternal("数字员工 Profile 身份校验失败")
	}
	stage = "profile_saved"
	return &digitalEmployeeLoginSession{
		DigitalProfile: digitalProfile, DigitalToken: token,
		SupervisorProfile: supervisorSelector, SupervisorToken: supervisor,
	}, nil
}
