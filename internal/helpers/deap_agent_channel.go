// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

const (
	digitalEmployeeProtocolVersion = 1
	digitalEmployeeStdinLimit      = 256 * 1024
)

type digitalEmployeeConnectResult struct {
	Status                 string `json:"status"`
	AgentUUID              string `json:"agentUuid"`
	Channel                string `json:"channel"`
	DWSProfile             string `json:"dwsProfile"`
	OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId"`
	ProtocolVersion        int    `json:"protocolVersion"`
	RestartRequired        bool   `json:"restartRequired"`
}

var (
	deapConnectConfigDir       = config.DefaultConfigDir
	deapConnectLoadProfiles    = auth.LoadProfiles
	deapConnectLoadToken       = auth.LoadTokenDataForProfile
	deapConnectManagedExchange = auth.ExchangeManagedAuthCode
	deapConnectRegisterDSH     = runDigitalEmployeeDSHRegister
	deapConnectSaveBinding     = saveDigitalEmployeeBinding
	deapChannelLoadBinding     = loadDigitalEmployeeBinding
)

type digitalEmployeeBinding struct {
	SchemaVersion          int    `json:"schemaVersion"`
	AgentUUID              string `json:"agentUuid"`
	DWSProfile             string `json:"dwsProfile"`
	OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId"`
}

func newDeapConnectCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "connect",
		Short: "把已发布的数字员工接入本地 Agent/DSH",
		Long:  "用于企业本地 Agent 接入：校验已发布数字员工的 mainProgramType=local_agent，以当前主管身份获取一次性授权信息，使用响应中的 dwsClientId 换票并保存独立 Profile，再通过 stdin 幂等注册到 DSH。A2A 或其他需要登录数字员工 DWS 的场景不要使用 connect，应使用 dws dingtalk-tag manage login。connect 不创建、修改或发布数字员工，也不会切换当前主管 Profile；注册后由用户或宿主重启 DSH。",
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "已存在且已发布的数字员工 ID", Required: true, Trim: true},
			{Name: "channel", Usage: "本地 Agent 渠道；第一期固定为 dsh", Required: true, Trim: true, Enum: []string{"dsh"}},
			{Name: "client-id", Usage: "传给 DEAP 的选应用提示；最终换票始终使用授权响应中的 dwsClientId", Trim: true, OmitEmpty: true},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent",
		},
		Validate: func(cmd *cobra.Command, _ []string) error {
			if commandDryRun(cmd) {
				return nil
			}
			if deps == nil || deps.Caller == nil {
				return apperrors.NewInternal("MCP caller is not initialized")
			}
			return nil
		},
		RunE: runDeapConnect,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "connect",
				CanonicalPath: "dingtalk-tag.connect", CLIPath: "dingtalk-tag connect", PrimaryCLIPath: "dingtalk-tag connect",
			},
			Description: "把一个已发布的 local_agent 数字员工安全接入企业本地 Agent/DSH；只编排授权、Profile 落盘和 DSH 注册。",
			DryRun:      deapAgentDryRun,
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "DEAP 授权、DWS managed exchange 与本地 DSH 注册的受控编排"},
			Selection: contract.SelectionSpec{
				AgentSummary: "把已有且已发布的 local_agent 数字员工接入企业本地 Agent/DSH",
				UseWhen:      []string{"企业要把已有数字员工接入本地 Agent/DSH，或创建发布后继续完成本地 DSH 接入"},
				AvoidWhen:    []string{"A2A 或其他需要登录数字员工 DWS 的场景使用 manage login；只创建、修改或发布数字员工时使用 manage；connect 本身不会更改数字员工配置"},
				Examples:     []string{"dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh --dry-run --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid"},
				{Name: "channel", Property: "channel", Enum: []string{"dsh"}},
				{Name: "client-id", Property: "clientId"},
			},
		},
	})
}

func runDeapConnect(cmd *cobra.Command, _ []string) error {
	agentUUID := strings.TrimSpace(MustGetStringFlag(cmd, "agent-uuid"))
	channel := strings.TrimSpace(MustGetStringFlag(cmd, "channel"))
	requestedClientID := strings.TrimSpace(MustGetStringFlag(cmd, "client-id"))
	if commandDryRun(cmd) {
		return writeDWSMachineEnvelope(cmd, map[string]any{
			"status": "planned", "agentUuid": agentUUID, "channel": channel,
			"protocolVersion": digitalEmployeeProtocolVersion, "restartRequired": true,
			"steps": []string{"validate_draft", "validate_published", "request_auth_code", "managed_exchange", "persist_profile", "resolve_operator", "register_dsh"},
		})
	}

	configDir := deapConnectConfigDir()
	supervisorSelector, supervisor, err := currentSupervisorProfile(configDir)
	if err != nil {
		return err
	}
	draft, err := callDeapJSON(cmd.Context(), deapAgentDetailTool, map[string]any{"agentUuid": agentUUID, "type": "draft"}, false)
	if err != nil {
		return fmt.Errorf("query digital employee draft: %w", err)
	}
	mainProgramType := findJSONScalar(draft, "mainProgramType")
	if mainProgramType != "local_agent" {
		return apperrors.NewValidation("数字员工不是 local_agent；请先完整读取 draft，保留全部配置并将 mainProgramType 修改为 local_agent 后再 connect")
	}
	published, err := callDeapJSON(cmd.Context(), deapAgentDetailTool, map[string]any{"agentUuid": agentUUID, "type": "published"}, false)
	if err != nil || !hasBusinessData(published) {
		return apperrors.NewValidation("数字员工尚未发布；请先发布当前草稿后再 connect")
	}

	authArgs := map[string]any{"agentUuid": agentUUID}
	if requestedClientID != "" {
		authArgs["clientId"] = requestedClientID
	}
	authorization, err := callDeapJSON(cmd.Context(), deapAgentAuthCodeTool, authArgs, true)
	if err != nil {
		return fmt.Errorf("request digital employee authorization: %w", err)
	}
	authData := businessDataMap(authorization)
	dwsClientID := requiredJSONScalar(authData, "dwsClientId")
	uid := requiredJSONScalar(authData, "uid")
	dwsAuthCode := requiredJSONScalar(authData, "dwsAuthCode")
	orgID := requiredJSONScalar(authData, "orgId")
	if dwsClientID == "" || uid == "" || dwsAuthCode == "" || orgID == "" {
		return apperrors.NewInternal("数字员工授权响应缺少 dwsClientId、uid、dwsAuthCode 或 orgId")
	}
	token, err := deapConnectManagedExchange(cmd.Context(), configDir, auth.ManagedExchangeRequest{
		ClientID: dwsClientID, AuthCode: dwsAuthCode, UID: uid, ExpectedOrgID: orgID, PreserveProfile: supervisorSelector,
		ResolveIdentity: resolveDigitalEmployeeManagedIdentity,
	})
	// 尽早清空本地变量，后续所有错误和输出都不再接触授权码。
	dwsAuthCode = ""
	if err != nil {
		return err
	}
	digitalProfile := auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID})
	if digitalProfile == "" || token.UserID != uid || token.CorpID != orgID {
		return apperrors.NewInternal("数字员工 Profile 身份校验失败")
	}
	operatorID, err := resolveExactOperatorOpenDingTalkID(cmd.Context(), supervisor.UserID)
	if err != nil {
		return err
	}
	if err := deapConnectSaveBinding(configDir, digitalEmployeeBinding{
		SchemaVersion: 1, AgentUUID: agentUUID, DWSProfile: digitalProfile, OperatorOpenDingTalkID: operatorID,
	}); err != nil {
		return fmt.Errorf("数字员工 Profile 已保存为 %s，但本地 operator 绑定保存失败；请重新运行同一条 connect 命令恢复: %w", digitalProfile, err)
	}
	registration := map[string]any{
		"schemaVersion":          digitalEmployeeProtocolVersion,
		"agentUuid":              agentUUID,
		"dwsProfile":             digitalProfile,
		"operatorOpenDingTalkId": operatorID,
		"protocolVersion":        digitalEmployeeProtocolVersion,
	}
	if name := findJSONScalar(draft, "name"); name != "" {
		registration["name"] = name
	}
	status, err := deapConnectRegisterDSH(cmd.Context(), registration)
	if err != nil {
		return fmt.Errorf("数字员工 Profile 已保存为 %s，但 DSH 注册失败；请重新运行同一条 connect 命令以获取新授权码并幂等重试注册: %w", digitalProfile, err)
	}
	return writeDWSMachineEnvelope(cmd, digitalEmployeeConnectResult{
		Status: status, AgentUUID: agentUUID, Channel: channel, DWSProfile: digitalProfile,
		OperatorOpenDingTalkID: operatorID, ProtocolVersion: digitalEmployeeProtocolVersion, RestartRequired: true,
	})
}

func currentSupervisorProfile(configDir string) (string, *auth.TokenData, error) {
	selector := strings.TrimSpace(auth.RuntimeProfile())
	if selector == "" {
		profiles, err := deapConnectLoadProfiles(configDir)
		if err != nil {
			return "", nil, fmt.Errorf("load supervisor profiles: %w", err)
		}
		selector = strings.TrimSpace(profiles.CurrentProfile)
	}
	if selector == "" {
		return "", nil, apperrors.NewValidation("当前没有可确定的主管 Profile；请先登录或用 --profile 精确选择主管账号")
	}
	token, err := deapConnectLoadToken(configDir, selector)
	if err != nil {
		return "", nil, fmt.Errorf("load supervisor profile: %w", err)
	}
	if token == nil || strings.TrimSpace(token.CorpID) == "" || strings.TrimSpace(token.UserID) == "" {
		return "", nil, apperrors.NewValidation("主管 Profile 缺少精确 corpId:userId 身份，无法安全接入数字员工")
	}
	exact := auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID})
	return exact, token, nil
}

func newDeapChannelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "channel", Short: "数字员工本地 Channel 机器协议", Args: cobra.NoArgs,
		TraverseChildren: true, DisableAutoGenTag: true, RunE: groupRunE,
	}
	cmdutil.MarkGroup(cmd)
	cmd.AddCommand(newDeapChannelCapabilitiesCommand(), newDeapChannelReplyCommand(), newDeapChannelOperatorPrivateCommand())
	return cmd
}

func newDeapChannelCapabilitiesCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "capabilities", Short: "查询 DSH Channel 协议能力",
		Flags:  []LeafFlag{{Name: "channel", Usage: "Channel 名称；第一期固定为 dsh", Required: true, Trim: true, Enum: []string{"dsh"}}},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeDWSMachineEnvelope(cmd, map[string]any{
				"schemaVersion": 1, "protocolVersion": 1, "channel": "dsh", "auditMode": "local_required",
				"capabilities": map[string]any{"eventConsume": true, "replyStdin": true, "operatorPrivateStdin": true},
			})
		},
		Contract: digitalEmployeeChannelContract("channel_capabilities", "capabilities", "查询 DSH 数字员工机器协议能力", "读取本地 DSH Channel 协议能力"),
	})
}

func newDeapChannelReplyCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "reply", Short: "通过 stdin 引用回复数字员工消息",
		Flags: []LeafFlag{
			{Name: "channel", Usage: "Channel 名称；第一期固定为 dsh", Required: true, Trim: true, Enum: []string{"dsh"}},
			{Name: "stdin", Usage: "从 stdin 读取受限 JSON；正文不得进入 argv", Kind: LeafBool, Required: true},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:     runDeapChannelReply,
		Contract: digitalEmployeeChannelContract("channel_reply", "reply", "通过 stdin 安全引用回复数字员工消息", "DSH 以数字员工 Profile 引用回复事件消息时"),
	})
}

func newDeapChannelOperatorPrivateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "operator-private", Short: "通过 stdin 向固定 operator 发单聊",
		Flags: []LeafFlag{
			{Name: "channel", Usage: "Channel 名称；第一期固定为 dsh", Required: true, Trim: true, Enum: []string{"dsh"}},
			{Name: "stdin", Usage: "从 stdin 读取受限 JSON；正文不得进入 argv", Kind: LeafBool, Required: true},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:     runDeapChannelOperatorPrivate,
		Contract: digitalEmployeeChannelContract("channel_operator_private", "operator-private", "通过 stdin 安全向 connect 固化的 operator 发送单聊", "DSH 需要向数字员工主管发起私聊审批时"),
	})
}

func digitalEmployeeChannelContract(name, leaf, description, useWhen string) LeafContract {
	example := "dws dingtalk-tag channel " + leaf + " --channel dsh --format json"
	parameters := []contract.ParamDecl{{Name: "channel", Property: "channel", Enum: []string{"dsh"}}}
	if leaf != "capabilities" {
		example = "dws dingtalk-tag channel " + leaf + " --channel dsh --stdin --format json"
		parameters = append(parameters, contract.ParamDecl{Name: "stdin", Property: "stdin", InterfaceType: "boolean"})
	}
	return LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: dingtalkTagProductID, Name: name, CanonicalPath: "dingtalk-tag." + name,
			CLIPath: "dingtalk-tag channel " + leaf, PrimaryCLIPath: "dingtalk-tag channel " + leaf, Group: "channel",
		},
		Description: description,
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "受限 stdin、本地协议校验与现有 DWS 消息 MCP 能力组合"},
		Selection: contract.SelectionSpec{
			AgentSummary: description, UseWhen: []string{useWhen},
			AvoidWhen: []string{"面向终端用户的普通消息发送使用 chat；本命令只供 DSH 机器协议调用"},
			Examples:  []string{example},
		},
		Parameters: parameters,
	}
}

type digitalEmployeeReplyInput struct {
	SchemaVersion      int    `json:"schemaVersion"`
	ProtocolVersion    int    `json:"protocolVersion"`
	AgentUUID          string `json:"agentUuid"`
	EventID            string `json:"eventId"`
	SessionID          string `json:"sessionId"`
	ConversationID     string `json:"conversationId"`
	ReferenceMessageID string `json:"referenceMessageId"`
	Text               string `json:"text"`
	IdempotencyKey     string `json:"idempotencyKey"`
}

type digitalEmployeeOperatorInput struct {
	SchemaVersion          int    `json:"schemaVersion"`
	ProtocolVersion        int    `json:"protocolVersion"`
	AgentUUID              string `json:"agentUuid"`
	OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId"`
	Text                   string `json:"text"`
	IdempotencyKey         string `json:"idempotencyKey"`
}

func runDeapChannelReply(cmd *cobra.Command, _ []string) error {
	var input digitalEmployeeReplyInput
	if err := decodeBoundedDigitalEmployeeStdin(cmd, &input); err != nil {
		return err
	}
	if input.SchemaVersion != 1 || input.ProtocolVersion != 1 || !validMachineString(input.AgentUUID) ||
		!validMachineString(input.EventID) || !validMachineString(input.ConversationID) ||
		!validMachineString(input.ReferenceMessageID) || !validMachineString(input.IdempotencyKey) || strings.TrimSpace(input.Text) == "" {
		return apperrors.NewValidation("invalid digital employee reply payload")
	}
	lookup, err := callMachineMCPJSON(cmd.Context(), "im", "list_messages_by_ids", map[string]any{"openMsgIds": []string{input.ReferenceMessageID}})
	if err != nil {
		return fmt.Errorf("resolve referenced message sender: %w", err)
	}
	sender := findJSONScalar(lookup, "senderOpenDingTalkId")
	if sender == "" {
		sender = findJSONScalar(lookup, "sender_open_dingtalk_id")
	}
	if sender == "" {
		return apperrors.NewValidation("referenced message did not return senderOpenDingTalkId")
	}
	content, _ := json.Marshal(map[string]string{
		"referenceOpenMessageId": input.ReferenceMessageID, "srcMsgSendOpenDingTalkId": sender,
		"replyMsgType": "text", "content": input.Text,
	})
	result, err := callMachineMCPJSON(cmd.Context(), "chat", "send_personal_message", map[string]any{
		"openConversationId": input.ConversationID, "msgType": "reply", "content": string(content), "uuid": input.IdempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("send digital employee reply: %w", err)
	}
	return writeDWSMachineEnvelope(cmd, digitalEmployeeDeliveryResult(result, input.ConversationID, input.IdempotencyKey))
}

func runDeapChannelOperatorPrivate(cmd *cobra.Command, _ []string) error {
	var input digitalEmployeeOperatorInput
	if err := decodeBoundedDigitalEmployeeStdin(cmd, &input); err != nil {
		return err
	}
	if input.SchemaVersion != 1 || input.ProtocolVersion != 1 || !validMachineString(input.AgentUUID) ||
		!validMachineString(input.OperatorOpenDingTalkID) || !validMachineString(input.IdempotencyKey) || strings.TrimSpace(input.Text) == "" {
		return apperrors.NewValidation("invalid digital employee operator-private payload")
	}
	profile := strings.TrimSpace(auth.RuntimeProfile())
	if profile == "" {
		return apperrors.NewValidation("operator-private requires an explicit digital employee --profile")
	}
	binding, err := deapChannelLoadBinding(deapConnectConfigDir(), profile)
	if err != nil || binding.AgentUUID != input.AgentUUID || binding.OperatorOpenDingTalkID != input.OperatorOpenDingTalkID {
		return apperrors.NewValidation("operator-private target does not match the operator fixed by connect")
	}
	content, _ := json.Marshal(map[string]string{"title": "数字员工审批", "text": input.Text})
	result, err := callMachineMCPJSON(cmd.Context(), "chat", "send_personal_message", map[string]any{
		"receiverOpenDingTalkId": input.OperatorOpenDingTalkID, "msgType": "markdown", "content": string(content), "uuid": input.IdempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("send digital employee operator message: %w", err)
	}
	return writeDWSMachineEnvelope(cmd, digitalEmployeeDeliveryResult(result, findJSONScalar(result, "openConvThreadId"), input.IdempotencyKey))
}

func decodeBoundedDigitalEmployeeStdin(cmd *cobra.Command, target any) error {
	limited := io.LimitReader(cmd.InOrStdin(), digitalEmployeeStdinLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return apperrors.NewValidation("cannot read digital employee stdin")
	}
	if len(data) > digitalEmployeeStdinLimit {
		return apperrors.NewValidation("digital employee stdin exceeds 256 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return apperrors.NewValidation("invalid digital employee stdin JSON")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return apperrors.NewValidation("digital employee stdin must contain exactly one JSON object")
	}
	return nil
}

func validMachineString(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 512 && !strings.ContainsAny(value, "\x00\r\n")
}

func digitalEmployeeDeliveryResult(result map[string]any, conversationID, idempotencyKey string) map[string]any {
	openMessageID := firstJSONScalar(result, "openMessageId", "openMsgId", "messageId", "msgId")
	if conversationID == "" {
		conversationID = firstJSONScalar(result, "conversationId", "openConversationId", "openConvThreadId")
	}
	delivery := strings.ToLower(firstJSONScalar(result, "deliveryStatus", "sendStatus", "status"))
	if delivery != "delivered" && delivery != "success" && delivery != "accepted" {
		delivery = "unknown"
	} else {
		delivery = "delivered"
	}
	return map[string]any{
		"openMessageId": openMessageID, "conversationId": conversationID,
		"deliveryStatus": delivery, "idempotencyKey": idempotencyKey,
	}
}

func writeDWSMachineEnvelope(cmd *cobra.Command, data any) error {
	return writeCommandPayload(cmd, map[string]any{"ok": true, "outcome": "success", "data": data, "meta": map[string]any{}})
}

// callDeapJSON 的 private=true 分支专用于授权响应：直接从 ToolResult 读取，
// 永不调用 dumpRawToolResponse，也不把原始响应或底层错误放进错误文本。
func callDeapJSON(ctx context.Context, tool string, args map[string]any, private bool) (map[string]any, error) {
	if private {
		return callPrivateMCPJSON(ctx, deapAgentServerID, tool, args)
	}
	raw, err := callMCPToolReturnTextOnServer(ctx, deapAgentServerID, tool, args)
	if err != nil {
		return nil, err
	}
	return decodeMCPJSON(raw)
}

func callMachineMCPJSON(ctx context.Context, server, tool string, args map[string]any) (map[string]any, error) {
	return callPrivateMCPJSON(ctx, server, tool, args)
}

func callPrivateMCPJSON(ctx context.Context, server, tool string, args map[string]any) (map[string]any, error) {
	if deps == nil || deps.Caller == nil {
		return nil, apperrors.NewInternal("MCP caller is not initialized")
	}
	result, err := deps.Caller.CallTool(ctx, server, tool, args)
	if err != nil || result == nil {
		return nil, fmt.Errorf("private MCP operation %s/%s failed", server, tool)
	}
	for _, content := range result.Content {
		if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		value, decodeErr := decodeMCPJSON(content.Text)
		if decodeErr != nil {
			return nil, fmt.Errorf("private MCP operation %s/%s returned invalid JSON", server, tool)
		}
		if success, ok := value["success"].(bool); ok && !success {
			return nil, fmt.Errorf("private MCP operation %s/%s was rejected", server, tool)
		}
		return value, nil
	}
	return nil, fmt.Errorf("private MCP operation %s/%s returned no JSON", server, tool)
}

func decodeMCPJSON(raw string) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, fmt.Errorf("invalid MCP JSON response")
	}
	return value, nil
}

func businessDataMap(value map[string]any) map[string]any {
	for _, key := range []string{"data", "result"} {
		if nested, ok := value[key].(map[string]any); ok {
			return nested
		}
	}
	return value
}

func hasBusinessData(value map[string]any) bool {
	if success, ok := value["success"].(bool); ok && !success {
		return false
	}
	for _, key := range []string{"error", "errorMsg", "errorMessage"} {
		if strings.TrimSpace(jsonScalar(value[key])) != "" {
			return false
		}
	}
	for _, key := range []string{"data", "result"} {
		if raw, exists := value[key]; exists {
			nested, ok := raw.(map[string]any)
			return ok && len(nested) > 0
		}
	}
	return len(value) > 0
}

func requiredJSONScalar(value map[string]any, key string) string {
	return findJSONScalar(value, key)
}

func firstJSONScalar(value any, keys ...string) string {
	for _, key := range keys {
		if found := findJSONScalar(value, key); found != "" {
			return found
		}
	}
	return ""
}

func findJSONScalar(value any, target string) string {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed[target]; ok {
			if scalar := jsonScalar(raw); scalar != "" {
				return scalar
			}
		}
		for _, nested := range typed {
			if found := findJSONScalar(nested, target); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := findJSONScalar(nested, target); found != "" {
				return found
			}
		}
	}
	return ""
}

func jsonScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64, float32, int, int64, int32:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}
