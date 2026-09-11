// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"strings"

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
		return writeDWSMachineEnvelope(cmd, map[string]any{
			"status": "planned", "agentUuid": agentUUID,
			"steps": []string{"validate_published", "request_auth_code", "managed_exchange", "verify_online_identity", "persist_exact_profile"},
		})
	}

	published, err := callDeapJSON(cmd.Context(), deapAgentDetailTool, map[string]any{"agentUuid": agentUUID, "type": "published"}, false)
	if err != nil || !hasBusinessData(published) {
		return apperrors.NewValidation("数字员工尚未发布；请先发布当前草稿后再 login")
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
func loginDigitalEmployee(ctx context.Context, configDir, agentUUID, requestedClientID string, published map[string]any) (*digitalEmployeeLoginSession, error) {
	supervisorSelector, supervisor, err := currentSupervisorProfile(configDir)
	if err != nil {
		return nil, err
	}
	publishedIdentity, ok := publishedDigitalEmployeeIdentity(published)
	if !ok {
		return nil, apperrors.NewInternal("数字员工发布详情缺少 profile.corpId、profile.robotUid 或 profile.staffId，无法校验身份")
	}

	authArgs := map[string]any{"agentUuid": agentUUID}
	if requestedClientID != "" {
		authArgs["clientId"] = requestedClientID
	}
	authorization, err := callDeapJSON(ctx, deapAgentAuthCodeTool, authArgs, true)
	if err != nil {
		return nil, fmt.Errorf("request digital employee authorization: %w", err)
	}
	authData := businessDataMap(authorization)
	dwsClientID := requiredJSONScalar(authData, "dwsClientId")
	authorizedRobotUID := requiredJSONScalar(authData, "uid")
	authorizedStaffID := requiredJSONScalar(authData, "staffId")
	dwsAuthCode := requiredJSONScalar(authData, "dwsAuthCode")
	authorizationOrgID := requiredJSONScalar(authData, "orgId")
	if dwsClientID == "" || authorizedRobotUID == "" || authorizedStaffID == "" || dwsAuthCode == "" || authorizationOrgID == "" {
		return nil, apperrors.NewInternal("数字员工授权响应缺少 dwsClientId、uid、staffId、dwsAuthCode 或 orgId")
	}
	if authorizedRobotUID != publishedIdentity.RobotUID {
		return nil, apperrors.NewInternal("数字员工授权响应 uid 与发布详情 profile.robotUid 不一致")
	}
	if authorizedStaffID != publishedIdentity.StaffID {
		return nil, apperrors.NewInternal("数字员工授权响应 staffId 与发布详情 profile.staffId 不一致")
	}
	// orgId 是授权响应的上下文字段，不是 corpId；corpId 只信任发布详情。
	token, err := deapConnectManagedExchange(ctx, configDir, auth.ManagedExchangeRequest{
		ClientID: dwsClientID, AuthCode: dwsAuthCode,
		ExpectedUserID: authorizedStaffID, ExpectedCorpID: publishedIdentity.CorpID,
		PreserveProfile: supervisorSelector, ResolveIdentity: resolveDigitalEmployeeManagedIdentity,
	})
	// 尽早清空本地变量，后续错误和输出都不再接触授权码。
	dwsAuthCode = ""
	if err != nil {
		return nil, err
	}
	digitalProfile := auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID})
	if digitalProfile == "" || token.UserID != authorizedStaffID || token.CorpID != publishedIdentity.CorpID {
		return nil, apperrors.NewInternal("数字员工 Profile 身份校验失败")
	}
	return &digitalEmployeeLoginSession{
		DigitalProfile: digitalProfile, DigitalToken: token,
		SupervisorProfile: supervisorSelector, SupervisorToken: supervisor,
	}, nil
}
