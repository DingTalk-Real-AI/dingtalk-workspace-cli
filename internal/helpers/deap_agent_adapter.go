// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// 数字员工直接复用 Agent 协议注册表，不包含机器人专用的外部连接器。
func digitalEmployeeChannels() []string {
	channels := []string{"dsh"}
	for name := range agentSpecs {
		channels = append(channels, name)
	}
	sort.Strings(channels)
	return channels
}

type digitalEmployeeAdapterConfig struct {
	Binding  digitalEmployeeBinding `json:"binding"`
	Options  connectAgentOptions    `json:"options"`
	AlwaysOn bool                   `json:"alwaysOn"`
	Name     string                 `json:"name,omitempty"`
}

// digitalEmployeeAdapter 接收已经完成身份准备的非敏感配置。
// DSH 交给外部宿主；local Adapter 在 DWS 内维护 Event 与 Agent。
type digitalEmployeeAdapter interface {
	Connect(*cobra.Command, digitalEmployeeAdapterConfig) error
}
type digitalEmployeeLocalAdapter struct{}
type digitalEmployeeDSHAdapter struct{}

func digitalEmployeeAdapterFor(channel string) (digitalEmployeeAdapter, error) {
	if channel == "dsh" {
		return digitalEmployeeDSHAdapter{}, nil
	}
	if _, ok := agentSpecs[channel]; ok {
		return digitalEmployeeLocalAdapter{}, nil
	}
	return nil, fmt.Errorf("unsupported digital employee adapter")
}

func (digitalEmployeeDSHAdapter) Connect(cmd *cobra.Command, cfg digitalEmployeeAdapterConfig) error {
	b := cfg.Binding
	registration := map[string]any{"schemaVersion": 1, "agentUuid": b.AgentUUID, "dwsProfile": b.DWSProfile, "operatorOpenDingTalkId": b.OperatorOpenDingTalkID, "protocolVersion": 1}
	if cfg.Name != "" {
		registration["name"] = cfg.Name
	}
	status, err := deapConnectRegisterDSH(cmd.Context(), registration)
	if err != nil {
		return fmt.Errorf("数字员工 Profile 已保存为 %s，但 DSH 注册失败；请重新运行同一条 connect 命令以获取新授权码并幂等重试注册: %w", b.DWSProfile, err)
	}
	return writeDWSMachineEnvelope(cmd, digitalEmployeeConnectResult{Status: status, AgentUUID: b.AgentUUID, Channel: "dsh", DWSProfile: b.DWSProfile, OperatorOpenDingTalkID: b.OperatorOpenDingTalkID, ProtocolVersion: 1, RestartRequired: true})
}

func digitalEmployeeScope(profile string) string {
	return "employee-" + strings.TrimSuffix(filepath.Base(digitalEmployeeBindingPath("", profile)), ".json")
}

func digitalEmployeeRuntimeDir(profile string) string {
	return filepath.Join(deapConnectConfigDir(), "digital-employees", digitalEmployeeScope(profile))
}

func bindingChannel(b digitalEmployeeBinding) string {
	if b.Channel == "" {
		return "dsh"
	}
	return b.Channel
}

func checkDigitalEmployeeBinding(dir, profile, uuid, channel string) error {
	b, err := loadDigitalEmployeeBinding(dir, profile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if b.AgentUUID != uuid || bindingChannel(b) != channel {
		return fmt.Errorf("数字员工已有 %s binding；不能覆盖不同员工或 Adapter 的绑定", bindingChannel(b))
	}
	return nil
}

func resolveDigitalEmployeeChannel(cmd *cobra.Command) (string, error) {
	requested := devAppStringFlag(cmd, "channel")
	if (requested == "" || requested == "auto") && devAppStringFlag(cmd, "agent-cmd") != "" {
		requested = "custom"
	}
	channel, _ := resolveConnectChannel(requested)
	for _, supported := range digitalEmployeeChannels() {
		if supported == channel {
			return channel, nil
		}
	}
	return "", fmt.Errorf("无法选择受支持的数字员工 Agent，请指定 --channel（%s）或 --profile-only；OpenClaw/Hermes 暂未适配", strings.Join(digitalEmployeeChannels(), "|"))
}

func validateDigitalEmployeeAdapter(cmd *cobra.Command) error {
	if commandBoolFlag(cmd, "local-worker") || commandBoolFlag(cmd, "local-supervise") {
		if commandBoolFlag(cmd, "local-worker") && commandBoolFlag(cmd, "local-supervise") {
			return fmt.Errorf("invalid worker mode")
		}
		if commandBoolFlag(cmd, "profile-only") || commandBoolFlag(cmd, "daemon") || commandDryRun(cmd) {
			return fmt.Errorf("invalid internal worker options")
		}
		return nil
	}
	if commandBoolFlag(cmd, "profile-only") {
		for _, name := range digitalEmployeeAdapterFlagNames() {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("--profile-only 不接受 --%s", name)
			}
		}
		return nil
	}
	channel, err := resolveDigitalEmployeeChannel(cmd)
	if err != nil {
		return err
	}
	if channel == "dsh" {
		for _, name := range digitalEmployeeAdapterFlagNames() {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("DSH 由外部宿主管理，不接受 --%s", name)
			}
		}
		return nil
	}
	if commandBoolFlag(cmd, "alwayson") && !commandBoolFlag(cmd, "daemon") {
		return fmt.Errorf("--alwayson 必须与 --daemon 同时使用")
	}
	if commandBoolFlag(cmd, "daemon") && !daemonDetachSupported {
		return fmt.Errorf("当前平台不支持 --daemon")
	}
	_, err = digitalEmployeeOptions(cmd)
	return err
}

func digitalEmployeeAdapterFlagNames() []string {
	return []string{"agent-cmd", "agent-model", "agent-workdir", "agent-memory", "agent-timeout", "agent-permission-mode", "agent-approval-mode", "yolo", "allowed-users", "allowed-groups", "daemon", "alwayson", "local-worker", "local-supervise"}
}

func digitalEmployeeOptions(cmd *cobra.Command) (connectAgentOptions, error) {
	// 共用 dev connect 的模型/目录/权限解析；未挂载的机器人选项保持零值。
	opts, err := connectAgentOptionsFromCommand(cmd)
	if err != nil {
		return opts, err
	}
	opts.Command = devAppStringFlag(cmd, "agent-cmd")
	if opts.Command == "" {
		opts.Command = strings.TrimSpace(os.Getenv("DWS_AGENT_CMD"))
	}
	channel, _ := resolveDigitalEmployeeChannel(cmd)
	if channel == "custom" && opts.Command == "" {
		return opts, fmt.Errorf("custom 需要 --agent-cmd")
	}
	if channel != "custom" && opts.Command != "" {
		return opts, fmt.Errorf("自定义命令请使用 --channel custom")
	}
	if opts.Timeout < 0 {
		return opts, fmt.Errorf("agent-timeout 不能为负数")
	}
	if opts.WorkDir != "" {
		opts.WorkDir, err = filepath.Abs(opts.WorkDir)
		if err != nil {
			return opts, err
		}
		info, e := os.Stat(opts.WorkDir)
		if e != nil || !info.IsDir() {
			return opts, fmt.Errorf("agent-workdir 必须是存在的目录")
		}
	}
	// 不继承机器人白名单环境变量：staffId 与员工开放 ID 不可混用。
	opts.AllowedUsers = splitCommaList(devAppStringFlag(cmd, "allowed-users"))
	opts.AllowedGroups = splitCommaList(devAppStringFlag(cmd, "allowed-groups"))
	opts.ReplyCard = false
	return opts, nil
}

func digitalEmployeeAgentFlags() []LeafFlag {
	return []LeafFlag{
		{Name: "agent-cmd", Usage: "custom 命令；问题作为末参，stdout 作为回复"},
		{Name: "agent-model", Usage: "Agent 模型；同 dev connect"},
		{Name: "agent-workdir", Usage: "Agent 工作目录；同 dev connect"},
		{Name: "agent-memory", Kind: LeafBool, Default: "true", Usage: "按员工和会话保留 Agent 上下文"},
		{Name: "agent-timeout", Kind: LeafInt, Usage: "Agent 每轮超时秒数；0 不限制"},
		{Name: "agent-permission-mode", Enum: []string{"ask", "bypass"}, Usage: "Agent 权限模式；同 dev connect"},
		{Name: "agent-approval-mode", Enum: []string{"ask", "yolo"}, Usage: "Agent 审批模式；同 dev connect"},
		{Name: "yolo", Kind: LeafBool, Usage: "显式选择 Agent 最高权限模式"},
		{Name: "allowed-users", Usage: "额外允许的精确 userId，以逗号分隔；在员工身份下解析"},
		{Name: "allowed-groups", Usage: "允许的员工上下文 openConversationId，以逗号分隔；群内仍检查发送人"},
		{Name: "daemon", Kind: LeafBool, Usage: "在后台启动 Event 和 Agent（Windows 不支持）"},
		{Name: "alwayson", Kind: LeafBool, Usage: "配合 --daemon 在允许的重试预算内恢复 worker"},
		{Name: "local-worker", Kind: LeafBool, Hidden: true, Usage: "internal: 从员工 Profile 的已保存配置运行"},
		{Name: "local-supervise", Kind: LeafBool, Hidden: true, Usage: "internal: 监督员工 worker"},
	}
}

func newDigitalEmployeeConnectionCommand() *cobra.Command {
	cmd := newGroupCommand(&cobra.Command{Use: "connection", Short: "数字员工本地连接运行管理"})
	cmd.AddCommand(newDigitalEmployeeStatusCommand(), newDigitalEmployeeListCommand(), newDigitalEmployeeStopCommand(), newDigitalEmployeeRestartCommand())
	return cmd
}

func prepareDigitalEmployeeLocal(cmd *cobra.Command, binding digitalEmployeeBinding, accessToken string) (connectAgentOptions, error) {
	opts, err := digitalEmployeeOptions(cmd)
	if err != nil {
		return opts, err
	}
	users := []string{binding.OperatorOpenDingTalkID}
	for _, uid := range opts.AllowedUsers {
		id, e := resolveExactOperatorOpenDingTalkID(cmd.Context(), accessToken, uid)
		if e != nil {
			return opts, fmt.Errorf("allowed-users 无法精确解析: %w", e)
		}
		users = append(users, id)
	}
	opts.AllowedUsers = users
	return opts, nil
}

func (digitalEmployeeLocalAdapter) Connect(cmd *cobra.Command, cfg digitalEmployeeAdapterConfig) error {
	binding := cfg.Binding
	dir := digitalEmployeeRuntimeDir(binding.DWSProfile)
	lock, err := auth.AcquireDualLock(cmd.Context(), filepath.Join(dir, "registration"))
	if err != nil {
		return err
	}
	defer lock.Release()
	if s, _ := readDigitalEmployeeState(dir); employeeStateAlive(s) {
		return fmt.Errorf("数字员工连接已运行，请先停止后修改配置")
	}
	if err := writeEmployeeJSON(filepath.Join(dir, "adapter.json"), cfg); err != nil {
		return err
	}
	// 只有显式 connect 完成新一轮主管授权后开始新预算；worker/restart 不重置。
	if err := writeEmployeeJSON(filepath.Join(dir, "retry.json"), employeeRetryState{}); err != nil {
		return err
	}
	if commandBoolFlag(cmd, "daemon") {
		return startDigitalEmployeeDaemon(cmd, cfg)
	}
	return runDigitalEmployeeForeground(cmd, cfg)
}

func employeeStateAlive(s digitalEmployeeRunState) bool {
	return (s.SupervisorPID > 0 && processAlive(s.SupervisorPID)) || (s.PID > 0 && processAlive(s.PID))
}

func writeEmployeeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return AtomicWriteJSON(path, data)
}

func loadDigitalEmployeeConfig(profile string) (digitalEmployeeAdapterConfig, error) {
	var cfg digitalEmployeeAdapterConfig
	data, err := os.ReadFile(filepath.Join(digitalEmployeeRuntimeDir(profile), "adapter.json"))
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid adapter configuration")
	}
	b, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), profile)
	if err != nil || b != cfg.Binding || b.DWSProfile != profile || bindingChannel(b) == "dsh" {
		return cfg, fmt.Errorf("adapter configuration does not match employee binding")
	}
	return cfg, nil
}

func digitalEmployeeResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"连接状态"},"agentUuid":{"type":"string","description":"数字员工 ID"},"channel":{"type":"string","description":"Adapter 类型"},"dwsProfile":{"type":"string","description":"员工精确 Profile"},"pid":{"type":"integer","description":"本地运行进程"},"logPath":{"type":"string","description":"无正文运行日志"},"restartRequired":{"type":"boolean","description":"是否需要外部宿主重启"},"items":{"type":"array","description":"连接列表","items":{"type":"object"}}}}`)}
}

func digitalEmployeeMachineResultSpec(capabilities bool) *contract.ResultSpec {
	if capabilities {
		return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"schemaVersion":{"type":"integer","description":"输入 Schema 版本"},"protocolVersion":{"type":"integer","description":"机器协议版本"},"channel":{"type":"string","description":"Adapter 类型"},"auditMode":{"type":"string","description":"审计要求"},"capabilities":{"type":"object","description":"支持的协议操作","properties":{"eventConsume":{"type":"boolean","description":"支持 Event Consumer"},"replyStdin":{"type":"boolean","description":"支持 stdin 引用回复"},"operatorPrivateStdin":{"type":"boolean","description":"支持 stdin 主管私聊"}}}}}`)}
	}
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"openMessageId":{"type":"string","description":"已确认的开放消息 ID"},"conversationId":{"type":"string","description":"目标会话 ID"},"deliveryStatus":{"type":"string","description":"delivered 或 unknown"},"idempotencyKey":{"type":"string","description":"发送幂等键"}}}`)}
}

func newDigitalEmployeeStatusCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "status", Short: "数字员工连接 status", Flags: []LeafFlag{{Name: "agent-uuid", Usage: "本地已绑定的数字员工 ID", Required: true}},
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:          func(cmd *cobra.Command, _ []string) error { return runDigitalEmployeeLifecycle(cmd, "status") },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_status", CanonicalPath: "dingtalk-tag.connect_status", CLIPath: "dingtalk-tag connection status", PrimaryCLIPath: "dingtalk-tag connection status", Group: "connection"},
			Description: "管理已绑定数字员工的本地进程；DSH 显示外部管理状态；不重新获取主管授权码。",
			Result:      digitalEmployeeResultSpec(),
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "读取本地绑定并管理所属进程"},
			Parameters:  []contract.ParamDecl{{Name: "agent-uuid", Property: "agentUuid"}},
			Selection:   contract.SelectionSpec{AgentSummary: "数字员工本地连接 status", UseWhen: []string{"需要对数字员工连接执行 status"}, AvoidWhen: []string{"机器人连接管理使用 dev connect；创建员工使用 manage"}, Examples: []string{"dws dingtalk-tag connection status --agent-uuid <agentUuid>"}},
		},
	})
}

func newDigitalEmployeeListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "list", Short: "数字员工连接 list", Flags: nil,
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:          func(cmd *cobra.Command, _ []string) error { return runDigitalEmployeeLifecycle(cmd, "list") },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_list", CanonicalPath: "dingtalk-tag.connect_list", CLIPath: "dingtalk-tag connection list", PrimaryCLIPath: "dingtalk-tag connection list", Group: "connection"},
			Description: "管理已绑定数字员工的本地进程；DSH 显示外部管理状态；不重新获取主管授权码。",
			Result:      digitalEmployeeResultSpec(),
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "读取本地绑定并管理所属进程"},
			Parameters:  nil,
			Selection:   contract.SelectionSpec{AgentSummary: "数字员工本地连接 list", UseWhen: []string{"需要对数字员工连接执行 list"}, AvoidWhen: []string{"机器人连接管理使用 dev connect；创建员工使用 manage"}, Examples: []string{"dws dingtalk-tag connection list"}},
		},
	})
}

func newDigitalEmployeeStopCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "stop", Short: "数字员工连接 stop", Flags: []LeafFlag{{Name: "agent-uuid", Usage: "本地已绑定的数字员工 ID", Required: true}},
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:          func(cmd *cobra.Command, _ []string) error { return runDigitalEmployeeLifecycle(cmd, "stop") },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_stop", CanonicalPath: "dingtalk-tag.connect_stop", CLIPath: "dingtalk-tag connection stop", PrimaryCLIPath: "dingtalk-tag connection stop", Group: "connection"},
			Description: "管理已绑定数字员工的本地进程；DSH 显示外部管理状态；不重新获取主管授权码。",
			Result:      digitalEmployeeResultSpec(),
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "读取本地绑定并管理所属进程"},
			Parameters:  []contract.ParamDecl{{Name: "agent-uuid", Property: "agentUuid"}},
			Selection:   contract.SelectionSpec{AgentSummary: "数字员工本地连接 stop", UseWhen: []string{"需要对数字员工连接执行 stop"}, AvoidWhen: []string{"机器人连接管理使用 dev connect；创建员工使用 manage"}, Examples: []string{"dws dingtalk-tag connection stop --agent-uuid <agentUuid>"}},
		},
	})
}

func newDigitalEmployeeRestartCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "restart", Short: "数字员工连接 restart", Flags: []LeafFlag{{Name: "agent-uuid", Usage: "本地已绑定的数字员工 ID", Required: true}},
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		RunE:          func(cmd *cobra.Command, _ []string) error { return runDigitalEmployeeLifecycle(cmd, "restart") },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_restart", CanonicalPath: "dingtalk-tag.connect_restart", CLIPath: "dingtalk-tag connection restart", PrimaryCLIPath: "dingtalk-tag connection restart", Group: "connection"},
			Description: "管理已绑定数字员工的本地进程；DSH 显示外部管理状态；不重新获取主管授权码。",
			Result:      digitalEmployeeResultSpec(),
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "读取本地绑定并管理所属进程"},
			Parameters:  []contract.ParamDecl{{Name: "agent-uuid", Property: "agentUuid"}},
			Selection:   contract.SelectionSpec{AgentSummary: "数字员工本地连接 restart", UseWhen: []string{"需要对数字员工连接执行 restart"}, AvoidWhen: []string{"机器人连接管理使用 dev connect；创建员工使用 manage"}, Examples: []string{"dws dingtalk-tag connection restart --agent-uuid <agentUuid>"}},
		},
	})
}

// 保留此接口参数中的 context，所有运行操作必须服从宿主取消。
var digitalEmployeeNewForwarder = func(ctx context.Context, cfg digitalEmployeeAdapterConfig) (forwarder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg.Options.PrivateDiagnostics = true
	return newLocalAgentForwarder(cfg.Binding.Channel, digitalEmployeeScope(cfg.Binding.DWSProfile)+"-"+cfg.Binding.Channel, cfg.Options)
}

var digitalEmployeeReadyTimeout = 45 * time.Second
