// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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
	Channel                string `json:"channel,omitempty"`
	DWSProfile             string `json:"dwsProfile"`
	OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId,omitempty"`
	ProfileOnly            bool   `json:"profileOnly,omitempty"`
	ProtocolVersion        int    `json:"protocolVersion"`
	RestartRequired        bool   `json:"restartRequired"`
}

type digitalEmployeePublishedIdentity struct {
	CorpID         string
	RobotUID       string
	StaffID        string
	OpenDingTalkID string
}

var (
	deapConnectConfigDir       = config.DefaultConfigDir
	deapConnectLoadProfiles    = auth.LoadProfiles
	deapConnectLoadToken       = auth.LoadTokenDataForProfile
	deapConnectManagedExchange = auth.ExchangeManagedAuthCode
	deapConnectRegisterDSH     = runDigitalEmployeeDSHRegister
	deapConnectSaveBinding     = saveDigitalEmployeeBinding
	deapChannelLoadBinding     = loadDigitalEmployeeBinding
	deapChannelReceiptWait     = waitForDigitalEmployeeReceipt
)

const (
	digitalEmployeeReceiptAttempts = 8
	digitalEmployeeReceiptInterval = 250 * time.Millisecond
)

type digitalEmployeeBinding struct {
	BindingRevision        uint64 `json:"bindingRevision,omitempty"`
	BindingState           string `json:"bindingState,omitempty"`
	DesiredState           string `json:"desiredState,omitempty"`
	SchemaVersion          int    `json:"schemaVersion"`
	AgentUUID              string `json:"agentUuid"`
	DWSProfile             string `json:"dwsProfile"`
	OperatorOpenDingTalkID string `json:"operatorOpenDingTalkId"`
	Channel                string `json:"channel,omitempty"`
}

func newDeapConnectCommand() *cobra.Command {
	cmd := NewLeafCommand(LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Use:           "connect",
		Short:         "为已发布数字员工落盘 Profile 并接入本地 Agent 或 DSH",
		Long:          "校验已发布 local_agent，以主管身份换票并保存独立 Profile，不切换主管 Current。--profile-only 仅落盘；--channel dsh 注册并请求当前宿主启动该员工，宿主不可用时提示升级或启动宿主；其他 Agent 通过 Event 收消息并以员工 Profile 回复，默认前台，--daemon --alwayson 后台常驻。运行及绑定管理使用 dingtalk-tag connect status/list/stop/restart/unbind/rebind。",
		Flags: append([]LeafFlag{
			{Name: "agent-uuid", Usage: "已存在且已发布的数字员工 ID", Required: true, Trim: true},
			{Name: "channel", Usage: "本地 Agent 类型；省略或 auto 时自动探测；profile-only 时省略", Trim: true, Enum: append([]string{"auto"}, digitalEmployeeChannels()...)},
			{Name: "profile-only", Kind: LeafBool, Usage: "仅完成授权换票与数字员工 Profile 落盘；不解析 operator、不保存 DSH binding、不注册 DSH"},
			{Name: "client-id", Usage: "传给 DEAP 的选应用提示；最终换票始终使用授权响应中的 dwsClientId", Trim: true, OmitEmpty: true},
		}, digitalEmployeeAgentFlags()...),
		Constraints: []LeafConstraint{{
			Kind: "custom", Flags: []string{"channel", "profile-only"},
			Description: "--channel 与 --profile-only 不能同时使用；省略模式时自动探测本地 Agent",
		}},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent",
		},
		Validate: func(cmd *cobra.Command, _ []string) error {
			channel := strings.TrimSpace(MustGetStringFlag(cmd, "channel"))
			profileOnly := commandBoolFlag(cmd, "profile-only")
			switch {
			case profileOnly && channel != "":
				return apperrors.NewValidation("--profile-only 与 --channel 不能同时使用")
			}
			if err := validateDigitalEmployeeAdapter(cmd); err != nil {
				return err
			}
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
			Result: digitalEmployeeResultSpec(),
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "connect",
				CanonicalPath: "dingtalk-tag.connect", CLIPath: "dingtalk-tag connect", PrimaryCLIPath: "dingtalk-tag connect",
			},
			Description: "为一个已发布的 local_agent 数字员工保存独立 Profile，并接入普通本地 Agent 或注册 DSH。",
			DryRun:      deapAgentDryRun,
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "DEAP 授权与 DWS managed exchange 的受控编排；可选本地 DSH 注册"},
			Selection: contract.SelectionSpec{
				AgentSummary: "为已有已发布数字员工保存 Profile，并可接入本地 Agent 或 DSH",
				UseWhen:      []string{"用户要把已发布数字员工转换为本地 Profile，或接入本机 Codex、Qoder 等 Agent 或 DSH"},
				AvoidWhen:    []string{"只创建、修改或发布数字员工时使用 manage；connect 本身不会更改数字员工配置"},
				Examples: []string{
					"dws dingtalk-tag connect --agent-uuid <agentUuid> --profile-only --dry-run --format json",
					"dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --daemon --alwayson --dry-run --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid"},
				{Name: "channel", Property: "channel", Enum: append([]string{"auto"}, digitalEmployeeChannels()...)},
				{Name: "profile-only", Property: "profileOnly"},
				{Name: "client-id", Property: "clientId"},
				{Name: "agent-cmd", Property: "agentCommand"},
				{Name: "agent-model", Property: "agentModel"},
				{Name: "agent-workdir", Property: "agentWorkdir"},
				{Name: "agent-memory", Property: "agentMemory"},
				{Name: "agent-timeout", Property: "agentTimeout"},
				{Name: "agent-permission-mode", Property: "agentPermissionMode"},
				{Name: "agent-approval-mode", Property: "agentApprovalMode"},
				{Name: "yolo", Property: "yolo"},
				{Name: "allowed-users", Property: "allowedUsers"},
				{Name: "allowed-groups", Property: "allowedGroups"},
				{Name: "daemon", Property: "daemon"},
				{Name: "alwayson", Property: "alwayson"},
			},
		},
	})
	cmd.AddCommand(newDigitalEmployeeStatusCommand(), newDigitalEmployeeListCommand(), newDigitalEmployeeStopCommand(), newDigitalEmployeeRestartCommand(), newEmployeeUnbindCommand(), newEmployeeRebindCommand())
	corecmd.ApplyGroupPolicy(cmd, corecmd.GroupPolicy{Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling})
	return cmd
}

func runDeapConnect(cmd *cobra.Command, _ []string) error {
	if commandBoolFlag(cmd, "local-lease") {
		return runEmployeeLease(cmd)
	}
	agentUUID := strings.TrimSpace(MustGetStringFlag(cmd, "agent-uuid"))
	channel := strings.TrimSpace(MustGetStringFlag(cmd, "channel"))
	profileOnly := commandBoolFlag(cmd, "profile-only")
	requestedClientID := strings.TrimSpace(MustGetStringFlag(cmd, "client-id"))
	if commandBoolFlag(cmd, "local-worker") || commandBoolFlag(cmd, "local-supervise") {
		return runDigitalEmployeeSaved(cmd)
	}
	if !profileOnly {
		var err error
		channel, err = resolveDigitalEmployeeChannel(cmd)
		if err != nil {
			return err
		}
	}
	if commandDryRun(cmd) {
		steps := []string{"validate_draft", "validate_published", "request_auth_code", "managed_exchange", "persist_profile"}
		if !profileOnly {
			steps = append(steps, "resolve_operator")
			if channel == "dsh" {
				steps = append(steps, "register_dsh")
			} else {
				steps = append(steps, "save_adapter", "start_event_consumer", "start_agent")
			}
		}
		plan := map[string]any{
			"status": "planned", "agentUuid": agentUUID, "profileOnly": profileOnly,
			"protocolVersion": digitalEmployeeProtocolVersion, "restartRequired": channel == "dsh",
			"steps": steps,
		}
		if channel != "" {
			plan["channel"] = channel
		}
		return writeDWSMachineEnvelope(cmd, plan)
	}

	configDir := deapConnectConfigDir()
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
	publishedIdentity, ok := publishedDigitalEmployeeIdentity(published)
	if !ok {
		return apperrors.NewInternal("数字员工发布详情缺少 profile.corpId、profile.robotUid 或 profile.staffId，无法校验身份")
	}
	var releaseRegistration func()
	if !profileOnly {
		profile := auth.ProfileSelector(auth.Profile{CorpID: publishedIdentity.CorpID, UserID: publishedIdentity.StaffID})
		// 同一员工的检查、换票、binding 和 Adapter 提交必须在同一注册事务内。
		lock, err := auth.AcquireDualLock(cmd.Context(), filepath.Join(digitalEmployeeRuntimeDir(profile), "operation"))
		if err != nil {
			return err
		}
		defer lock.Release()
		releaseRegistration = lock.Release
		if err := checkDigitalEmployeeBinding(configDir, profile, agentUUID, channel); err != nil {
			return err
		}
		if channel != "dsh" {
			if state, _ := readDigitalEmployeeState(digitalEmployeeRuntimeDir(profile)); employeeStateAlive(state) {
				return fmt.Errorf("数字员工连接已运行，请先停止后重新 connect")
			}
		} else if previous, e := loadDigitalEmployeeBinding(configDir, profile); e == nil && employeeBindingState(previous) != "unbound" {
			r, e := employeeDSHControl(cmd.Context(), previous, "status")
			if e != nil || !r.Released {
				return fmt.Errorf("已有 DSH 绑定尚未确认停止；只刷新 Profile 请使用 --profile-only，换绑请使用 connect rebind")
			}
		}
	}

	session, err := loginDigitalEmployee(cmd.Context(), configDir, agentUUID, requestedClientID, published)
	if err != nil {
		return err
	}
	digitalProfile, token := session.DigitalProfile, session.DigitalToken
	if profileOnly {
		return writeDWSMachineEnvelope(cmd, digitalEmployeeConnectResult{
			Status: "profile_saved", AgentUUID: agentUUID, DWSProfile: digitalProfile, ProfileOnly: true,
			ProtocolVersion: digitalEmployeeProtocolVersion, RestartRequired: false,
		})
	}
	operatorID, err := resolveExactOperatorOpenDingTalkID(cmd.Context(), token.AccessToken, session.SupervisorToken.UserID)
	if err != nil {
		return err
	}
	binding := digitalEmployeeBinding{
		SchemaVersion: 1, AgentUUID: agentUUID, DWSProfile: digitalProfile, OperatorOpenDingTalkID: operatorID, Channel: channel,
		BindingRevision: 1, BindingState: "bound", DesiredState: "running",
	}
	if previous, e := loadDigitalEmployeeBinding(configDir, digitalProfile); e == nil {
		binding.BindingRevision = previous.BindingRevision
		if employeeBindingState(previous) == "unbound" {
			binding.BindingRevision++
		}
	}
	cfg := digitalEmployeeAdapterConfig{Binding: binding, Name: findJSONScalar(draft, "name"), AlwaysOn: commandBoolFlag(cmd, "alwayson")}
	cfg.SelfOpenDingTalkID = publishedIdentity.OpenDingTalkID
	if channel != "dsh" {
		cfg.SelfOpenDingTalkID = publishedIdentity.OpenDingTalkID
		if !validMachineString(cfg.SelfOpenDingTalkID) {
			return fmt.Errorf("数字员工发布详情缺少自身 openDingTalkId，无法安全过滤自发消息")
		}
		cfg.Options, err = prepareDigitalEmployeeLocal(cmd, binding, token.AccessToken)
		if err != nil {
			return err
		}
	}
	if err := deapConnectSaveBinding(configDir, binding); err != nil {
		return fmt.Errorf("数字员工 Profile 已保存为 %s，但本地 operator 绑定保存失败；请重新运行同一条 connect 命令恢复: %w", digitalProfile, err)
	}
	adapter, err := digitalEmployeeAdapterFor(channel)
	if err != nil {
		return err
	}
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(digitalProfile), "adapter.json"), cfg); err != nil {
		return err
	}
	// 前台 worker 不得把注册事务锁占用整个运行生命周期。
	if releaseRegistration != nil && channel != "dsh" {
		releaseRegistration()
	}
	return adapter.Connect(cmd, cfg)
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
		return "", nil, apperrors.NewValidation("主管 Profile 缺少精确 corpId:userId 身份，无法安全登录数字员工")
	}
	exact := auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID})
	return exact, token, nil
}

func newDeapChannelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "channel", Short: "数字员工本地 Channel 机器协议", Args: cobra.NoArgs,
		TraverseChildren: true, DisableAutoGenTag: true, RunE: groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(newDeapChannelCapabilitiesCommand(), newDeapChannelReplyCommand(), newDeapChannelOperatorPrivateCommand(), newEmployeeBindingCommand())
	return cmd
}

func newDeapChannelCapabilitiesCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Use:           "capabilities", Short: "查询 DSH Channel 协议能力",
		Flags:  []LeafFlag{{Name: "channel", Usage: "已支持的数字员工 Adapter", Required: true, Trim: true, Enum: digitalEmployeeChannels()}},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeDWSMachineEnvelope(cmd, map[string]any{
				"schemaVersion": 1, "protocolVersion": 1, "channel": devAppStringFlag(cmd, "channel"), "auditMode": "local_required",
				"capabilities": map[string]any{"eventConsume": true, "replyStdin": true, "operatorPrivateStdin": true},
			})
		},
		Contract: digitalEmployeeChannelContract("channel_capabilities", "capabilities", "查询 DSH 数字员工机器协议能力", "读取本地 DSH Channel 协议能力"),
	})
}

func newDeapChannelReplyCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Use:           "reply", Short: "通过 stdin 引用回复数字员工消息",
		Flags: []LeafFlag{
			{Name: "channel", Usage: "已绑定的数字员工 Adapter", Required: true, Trim: true, Enum: digitalEmployeeChannels()},
			{Name: "stdin", Usage: "从 stdin 读取受限 JSON；正文不得进入 argv", Kind: LeafBool, Required: true},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:     runDeapChannelReply,
		Contract: digitalEmployeeChannelContract("channel_reply", "reply", "通过 stdin 安全引用回复数字员工消息", "DSH 以数字员工 Profile 引用回复事件消息时"),
	})
}

func newDeapChannelOperatorPrivateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Use:           "operator-private", Short: "通过 stdin 向固定 operator 发单聊",
		Flags: []LeafFlag{
			{Name: "channel", Usage: "已绑定的数字员工 Adapter", Required: true, Trim: true, Enum: digitalEmployeeChannels()},
			{Name: "stdin", Usage: "从 stdin 读取受限 JSON；正文不得进入 argv", Kind: LeafBool, Required: true},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:     runDeapChannelOperatorPrivate,
		Contract: digitalEmployeeChannelContract("channel_operator_private", "operator-private", "通过 stdin 安全向 connect 固化的 operator 发送单聊", "DSH 需要向数字员工主管发起私聊审批时"),
	})
}

func digitalEmployeeChannelContract(name, leaf, description, useWhen string) LeafContract {
	example := "dws dingtalk-tag channel " + leaf + " --channel dsh --format json"
	parameters := []contract.ParamDecl{{Name: "channel", Property: "channel", Enum: digitalEmployeeChannels()}}
	if leaf != "capabilities" {
		example = "dws dingtalk-tag channel " + leaf + " --channel dsh --stdin --format json"
		parameters = append(parameters, contract.ParamDecl{Name: "stdin", Property: "stdin", InterfaceType: "boolean"})
	}
	return LeafContract{
		Result: digitalEmployeeMachineResultSpec(leaf == "capabilities"),
		Identity: contract.ToolIdentitySpec{
			ProductID: dingtalkTagProductID, Name: name, CanonicalPath: "dingtalk-tag." + name,
			CLIPath: "dingtalk-tag channel " + leaf, PrimaryCLIPath: "dingtalk-tag channel " + leaf, Group: "channel",
		},
		Description: description,
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "受限 stdin、本地协议校验与现有 DWS 消息 MCP 能力组合"},
		Selection: contract.SelectionSpec{
			AgentSummary: description, UseWhen: []string{useWhen},
			AvoidWhen: []string{"面向终端用户的普通消息发送使用 chat；本命令只供已绑定 Adapter 的机器协议调用"},
			Examples:  []string{example},
		},
		Parameters: parameters,
	}
}

type digitalEmployeeReplyInput struct {
	BindingRevision    uint64 `json:"bindingRevision,omitempty"`
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
	BindingRevision        uint64 `json:"bindingRevision,omitempty"`
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
	if err := validateEmployeeMachineRevision(cmd, input.AgentUUID, input.BindingRevision); err != nil {
		return err
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
	delivery, err := resolveDigitalEmployeeDelivery(cmd.Context(), result, input.ConversationID, input.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("resolve digital employee reply receipt: %w", err)
	}
	return writeDWSMachineEnvelope(cmd, delivery)
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
	if err := validateEmployeeMachineRevision(cmd, input.AgentUUID, input.BindingRevision); err != nil {
		return err
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
	delivery, err := resolveDigitalEmployeeDelivery(cmd.Context(), result, findJSONScalar(result, "openConvThreadId"), input.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("resolve digital employee operator message receipt: %w", err)
	}
	return writeDWSMachineEnvelope(cmd, delivery)
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

func resolveDigitalEmployeeDelivery(ctx context.Context, sendResult map[string]any, conversationID, idempotencyKey string) (map[string]any, error) {
	delivery := digitalEmployeeDeliveryResult(sendResult, conversationID, idempotencyKey)
	if strings.TrimSpace(jsonScalar(delivery["openMessageId"])) != "" {
		return delivery, nil
	}
	taskID := firstJSONScalar(sendResult, "openTaskId", "taskId")
	if taskID == "" {
		return nil, apperrors.NewInternal("数字员工发送响应缺少 openMessageId 和 openTaskId，无法确认投递结果")
	}
	for attempt := 0; attempt < digitalEmployeeReceiptAttempts; attempt++ {
		if attempt > 0 {
			if err := deapChannelReceiptWait(ctx, digitalEmployeeReceiptInterval); err != nil {
				return nil, err
			}
		}
		status, err := callMachineMCPJSON(ctx, "im", "query_message_send_status", map[string]any{"openTaskId": taskID})
		if err != nil {
			return nil, fmt.Errorf("query digital employee message status: %w", err)
		}
		delivery = digitalEmployeeDeliveryResult(status, conversationID, idempotencyKey)
		if strings.TrimSpace(jsonScalar(delivery["openMessageId"])) != "" {
			return delivery, nil
		}
	}
	return nil, apperrors.NewInternal("数字员工发送任务未在有限等待时间内返回 openMessageId，无法确认投递结果")
}

func waitForDigitalEmployeeReceipt(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func writeDWSMachineEnvelope(cmd *cobra.Command, data any) error {
	ctx, _ := output.WithResultStore(cmd.Context())
	cmd.SetContext(ctx)
	if err := output.StoreResult(ctx, output.Success(data)); err != nil {
		return err
	}
	_, _, err := output.EmitStoredResult(cmd)
	return err
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

func publishedDigitalEmployeeIdentity(value map[string]any) (digitalEmployeePublishedIdentity, bool) {
	data := businessDataMap(value)
	profile, ok := data["profile"].(map[string]any)
	if !ok {
		return digitalEmployeePublishedIdentity{}, false
	}
	identity := digitalEmployeePublishedIdentity{
		CorpID:         jsonScalar(profile["corpId"]),
		RobotUID:       jsonScalar(profile["robotUid"]),
		StaffID:        jsonScalar(profile["staffId"]),
		OpenDingTalkID: jsonScalar(profile["openDingTalkId"]),
	}
	return identity, identity.CorpID != "" && identity.RobotUID != "" && identity.StaffID != ""
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
