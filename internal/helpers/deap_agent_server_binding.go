// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// 服务端绑定 ID、本机绑定代数与运行实例 ID 是三个独立概念。
// 回执只表示服务端曾确认绑定，不表示设备在线或当前运行就绪。
type employeeServerOperation struct {
	Key              string `json:"key"`
	Action           string `json:"action"`
	Phase            string `json:"phase"`
	RuntimeBindingID string `json:"runtimeBindingId,omitempty"`
}

var employeeServerWrite = writeEmployeeJSON

func employeeServerUnknown(message string) error {
	return apperrors.NewInternal("server_binding_unknown："+message, apperrors.WithReason("server_binding_unknown"), apperrors.WithRetryable(false))
}

func employeeServerFlags() []LeafFlag {
	return []LeafFlag{
		{Name: "device-id", Usage: "稳定设备 ID；省略时使用当前 DWS 配置目录持久化的随机设备 ID", Trim: true},
		{Name: "local-agent-name", Usage: "服务端绑定的本地 Agent 名称（可选）", Trim: true},
		{Name: "extensions", Usage: "服务端绑定扩展信息字符串；不写入回执或输出"},
	}
}

func employeeServerParams() []contract.ParamDecl {
	return []contract.ParamDecl{{Name: "device-id", Property: "deviceId"}, {Name: "local-agent-name", Property: "localAgentName"}, {Name: "extensions", Property: "extensions"}}
}

func employeeDeviceID(ctx context.Context, explicit string) (string, error) {
	if explicit != "" {
		if !validMachineString(explicit) {
			return "", fmt.Errorf("无效 device-id")
		}
		return explicit, nil
	}
	dir := filepath.Join(deapConnectConfigDir(), "digital-employee-device")
	lock, err := auth.AcquireDualLock(ctx, filepath.Join(dir, "lock"))
	if err != nil {
		return "", err
	}
	defer lock.Release()
	path := filepath.Join(dir, "identity.json")
	var value struct {
		ID string `json:"deviceId"`
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(raw, &value) != nil || !validMachineString(value.ID) {
			return "", fmt.Errorf("设备标识文件损坏；禁止自动生成新 ID")
		}
		return value.ID, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	value.ID = uuid.NewString()
	if err := employeeServerWrite(path, value); err != nil {
		return "", err
	}
	return value.ID, nil
}

func employeeBindingDeviceID(ctx context.Context, b digitalEmployeeBinding, explicit string) (string, error) {
	if explicit == "" && b.DeviceID != "" {
		explicit = b.DeviceID
	}
	return employeeDeviceID(ctx, explicit)
}

func employeeServerOperationPath(profile string) string {
	return filepath.Join(digitalEmployeeRuntimeDir(profile), "server-binding-operation.json")
}

// 调用者持有员工 operation 锁。先写 pending，再发送一次；confirmed 回执允许
// 本地提交失败后的重放。pending 不能自动重放非幂等换绑，且不记录请求扩展信息。
func mutateEmployeeServerBinding(cmd *cobra.Command, b digitalEmployeeBinding, action string, deviceID string) (string, error) {
	configDir := deapConnectConfigDir()
	selector, token, err := currentSupervisorProfile(cmd.Context(), configDir)
	if err != nil {
		return "", err
	}
	if selector == b.DWSProfile {
		return "", fmt.Errorf("绑定操作需要主管 Profile，不能使用员工 Profile")
	}
	if deps == nil || deps.Caller == nil {
		return "", fmt.Errorf("MCP caller is not initialized")
	}
	caller, ok := deps.Caller.(managedIdentityTokenCaller)
	if !ok || token.AccessToken == "" {
		return "", fmt.Errorf("主管身份的 MCP 调用不可用")
	}
	request := map[string]any{"agentUuid": b.AgentUUID}
	switch action {
	case "bind", "unbind", "rebind":
	default:
		return "", fmt.Errorf("无效绑定操作")
	}
	if action != "bind" {
		if !validMachineString(b.RuntimeBindingID) {
			return "", fmt.Errorf("缺少 runtimeBindingId；旧版连接先使用 connect bind 补登记，换机时显式提供 --runtime-binding-id")
		}
		request["runtimeBindingId"] = b.RuntimeBindingID
	}
	if action != "unbind" {
		if !validMachineString(deviceID) {
			return "", fmt.Errorf("缺少稳定 deviceId")
		}
		request["deviceId"] = deviceID
		for flag, field := range map[string]string{"local-agent-name": "localAgentName", "extensions": "extensions"} {
			if value := devAppStringFlag(cmd, flag); value != "" {
				request[field] = value
			}
		}
	}
	encoded, _ := json.Marshal([]any{selector, b.AgentUUID, b.BindingRevision, action, request})
	op := employeeServerOperation{Key: fmt.Sprintf("%x", sha256.Sum256(encoded)), Action: action, Phase: "pending"}
	path := employeeServerOperationPath(b.DWSProfile)
	if raw, err := os.ReadFile(path); err == nil {
		var old employeeServerOperation
		if json.Unmarshal(raw, &old) != nil || old.Key == "" {
			return "", fmt.Errorf("服务端操作回执损坏；请核对后恢复")
		}
		if old.Phase != "pending" && old.Phase != "confirmed" && old.Phase != "consumed" && old.Phase != "rejected" {
			return "", fmt.Errorf("无效服务端操作回执状态")
		}
		if old.Phase != "consumed" && old.Phase != "rejected" {
			if old.Key != op.Key {
				return "", fmt.Errorf("存在未完成服务端绑定操作；请先恢复原请求，不可更换参数")
			}
			if old.Phase == "confirmed" {
				if !validMachineString(old.RuntimeBindingID) {
					return "", fmt.Errorf("服务端回执缺少绑定 ID")
				}
				return old.RuntimeBindingID, nil
			}
			if action != "unbind" {
				return "", employeeServerUnknown("上次绑定请求结果未知，请联系服务端核对；禁止自动重试或启动新 Agent")
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := employeeServerWrite(path, op); err != nil {
		return "", err
	}
	// identity 由网关根据主管 Token 注入；CLI 不允许覆盖身份，也不 dump 原始响应。
	result, err := callEmployeeServerBinding(cmd.Context(), caller, configDir, selector, token.AccessToken, action, request)
	if err != nil {
		if employeeServerDefinitiveRejection(err) {
			op.Phase = "rejected"
			if writeErr := employeeServerWrite(path, op); writeErr != nil {
				return "", writeErr
			}
			return "", err
		}
		return "", employeeServerUnknown("服务端请求结果未知；保留旧绑定，不启动新 Agent")
	}
	if result == nil {
		return "", employeeServerUnknown("服务端请求结果未知；保留旧绑定，不启动新 Agent")
	}
	var response struct {
		Success          *bool           `json:"success"`
		Data             json.RawMessage `json:"data"`
		RuntimeBindingID *string         `json:"runtimeBindingId"`
	}
	texts := 0
	for _, block := range result.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		texts++
		decoder := json.NewDecoder(strings.NewReader(block.Text))
		if decoder.Decode(&response) != nil || decoder.Decode(new(any)) != io.EOF {
			return "", employeeServerUnknown("服务端返回无效 JSON")
		}
	}
	if texts != 1 || response.Success == nil {
		return "", employeeServerUnknown("缺少唯一明确的服务端结果")
	}
	if !*response.Success {
		op.Phase = "rejected"
		if err := employeeServerWrite(path, op); err != nil {
			return "", err
		}
		return "", apperrors.NewAPI("server_binding_rejected：服务端拒绝操作；可能存在其他设备绑定、旧 ID 已失效、权限不足或在途/待恢复任务；旧 ID 保留，请核对后重试", apperrors.WithReason("server_binding_rejected"), apperrors.WithRetryable(false))
	}
	if action == "unbind" {
		var released bool
		if json.Unmarshal(response.Data, &released) != nil || !released {
			return "", employeeServerUnknown("解绑未返回 true")
		}
		op.RuntimeBindingID = b.RuntimeBindingID
	} else {
		// 正式契约从 data 对象读取 runtimeBindingId；兼容旧版 data 字符串
		// 和顶层映射。只读取明确字段，不递归猜测，也不使用 runtimeId。
		data := bytes.TrimSpace(response.Data)
		if len(data) > 0 && !bytes.Equal(data, []byte("null")) {
			if data[0] == '{' {
				var binding struct {
					RuntimeBindingID string `json:"runtimeBindingId"`
				}
				if json.Unmarshal(data, &binding) != nil {
					return "", employeeServerUnknown("绑定未返回有效 ID")
				}
				op.RuntimeBindingID = binding.RuntimeBindingID
			} else if json.Unmarshal(data, &op.RuntimeBindingID) != nil {
				return "", employeeServerUnknown("绑定未返回有效 ID")
			}
			// data 一旦提供就必须有效，不能用顶层兼容字段掩盖损坏结果。
			if !validMachineString(op.RuntimeBindingID) {
				return "", employeeServerUnknown("绑定未返回有效 ID")
			}
		}
		if response.RuntimeBindingID != nil {
			if op.RuntimeBindingID != "" && op.RuntimeBindingID != *response.RuntimeBindingID {
				return "", employeeServerUnknown("服务端返回的绑定 ID 冲突")
			}
			op.RuntimeBindingID = *response.RuntimeBindingID
		}
		if !validMachineString(op.RuntimeBindingID) {
			return "", employeeServerUnknown("绑定未返回有效 ID")
		}
		if action == "rebind" && op.RuntimeBindingID == b.RuntimeBindingID {
			return "", employeeServerUnknown("换绑未返回新 ID")
		}
	}
	op.Phase = "confirmed"
	if err := employeeServerWrite(path, op); err != nil {
		return "", employeeServerUnknown("服务端已成功但回执保存失败；请核对服务端结果")
	}
	return op.RuntimeBindingID, nil
}

func callEmployeeServerBinding(
	ctx context.Context,
	caller managedIdentityTokenCaller,
	configDir, selector, accessToken, action string,
	request map[string]any,
) (*edition.ToolResult, error) {
	call := func(token string) (*edition.ToolResult, error) {
		return caller.CallToolWithToken(ctx, token, deapAgentServerID, action+"_local_agent", request)
	}
	result, err := call(accessToken)
	if !employeeServerAccessTokenRejected(err) {
		return result, err
	}

	refreshed, refreshErr := deapConnectForceRefreshSupervisorToken(ctx, configDir, accessToken)
	if refreshErr != nil || strings.TrimSpace(refreshed) == "" {
		return nil, err
	}
	// ForceRefreshRejectedToken 以活动 Profile 为刷新槽。再次读取并核对精确
	// 主管身份，防止并发切换 current profile 后把其他账号 Token 用于绑定。
	verified, loadErr := deapConnectLoadToken(configDir, selector)
	if loadErr != nil || verified == nil || strings.TrimSpace(verified.AccessToken) != strings.TrimSpace(refreshed) ||
		auth.ProfileSelector(auth.Profile{CorpID: verified.CorpID, UserID: verified.UserID}) != selector {
		return nil, err
	}
	return call(strings.TrimSpace(refreshed))
}

func employeeServerAccessTokenRejected(err error) bool {
	if err == nil {
		return false
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == CodeAuthTokenExpired {
		return true
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr == nil {
		return false
	}
	if appErr.Category == apperrors.CategoryAuth {
		switch strings.ToLower(strings.TrimSpace(appErr.Reason)) {
		case "access_token_rejected", "gateway_auth_expired", "http_401":
			return true
		}
	}
	return dwsGatewayErrors[strings.ToUpper(strings.TrimSpace(appErr.ServerDiag.ServerErrorCode))]
}

func employeeServerDefinitiveRejection(err error) bool {
	if employeeServerAccessTokenRejected(err) {
		return true
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		switch cliErr.Code {
		case CodeAuthPermission, CodeMCPToolError, CodeResourceNotFound,
			CodeTableNotFound, CodeSheetNotFound, CodeFieldNotFound, CodeRecordNotFound:
			return true
		}
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr == nil {
		return false
	}
	if appErr.Category == apperrors.CategoryAuth {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(appErr.ServerDiag.ServerErrorCode), "TOOL_NOT_FOUND") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(appErr.Reason)) {
	case "business_error", "mcp_tool_error", "invalid_request":
		return true
	default:
		return false
	}
}

func consumeEmployeeServerOperation(profile string) error {
	path := employeeServerOperationPath(profile)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var op employeeServerOperation
	if json.Unmarshal(raw, &op) != nil {
		return fmt.Errorf("服务端操作回执损坏")
	}
	if op.Phase == "consumed" || op.Phase == "rejected" {
		return nil
	}
	if op.Phase != "confirmed" {
		return fmt.Errorf("服务端操作尚未确认")
	}
	op.Phase = "consumed"
	return employeeServerWrite(path, op)
}

// 仅本地提交与服务端回执完全一致时，允许恢复启动。没有回执的 #9 旧连接
// 仍可执行既有 stop/restart；登记与解绑迁移由显式 bind 处理。
func checkEmployeeServerOperation(b digitalEmployeeBinding) error {
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var op employeeServerOperation
	if json.Unmarshal(raw, &op) != nil {
		return fmt.Errorf("服务端绑定回执损坏")
	}
	switch op.Phase {
	case "consumed", "rejected":
		return nil
	case "confirmed":
		if op.RuntimeBindingID == b.RuntimeBindingID && validMachineString(b.RuntimeBindingID) && ((op.Action == "unbind") == (employeeBindingState(b) == "unbound")) {
			return nil
		}
	}
	return employeeServerUnknown("服务端绑定操作未完成，请恢复原操作；禁止启动新 Agent")
}

func newEmployeeServerBindCommand() *cobra.Command {
	flags := append([]LeafFlag{{Name: "agent-uuid", Required: true, Usage: "已有本地连接的数字员工 ID"}}, employeeServerFlags()...)
	params := append([]contract.ParamDecl{{Name: "agent-uuid", Property: "agentUuid"}}, employeeServerParams()...)
	return NewLeafCommand(LeafSpec{Use: "bind", Short: "为已有本地连接补登记服务端设备绑定", PostMount: deapAgentNoArgs, Flags: flags, OutputRollout: output.RolloutUnifiedActive,
		Safety:   contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
		Contract: LeafContract{Identity: contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_bind", CanonicalPath: "dingtalk-tag.connect_bind", CLIPath: "dingtalk-tag connect bind", PrimaryCLIPath: "dingtalk-tag connect bind", Group: "connect"}, Description: "以主管身份为已有本机连接补登记设备绑定；不启动 Agent，不判断在线。其他设备已绑定时使用 rebind。", Parameters: params, Result: digitalEmployeeResultSpec(), DryRun: deapAgentDryRun, Interface: &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "本地设备标识与服务端绑定回执"}, Selection: contract.SelectionSpec{AgentSummary: "为已有本地 Agent 补登记服务端绑定", UseWhen: []string{"旧版连接尚未登记服务端设备绑定"}, AvoidWhen: []string{"首次接入使用 connect；换绑使用 connect rebind；绑定不代表在线"}, Examples: []string{"dws dingtalk-tag connect bind --agent-uuid <agentUuid>"}}},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if commandDryRun(cmd) {
				return writeDWSMachineEnvelope(cmd, map[string]any{"status": "planned", "agentUuid": devAppStringFlag(cmd, "agent-uuid"), "steps": []string{"resolve_device", "server_bind", "save_receipt"}})
			}
			bindings, err := employeeFindBindings(devAppStringFlag(cmd, "agent-uuid"))
			if err != nil {
				return err
			}
			if len(bindings) != 1 {
				return fmt.Errorf("补登记需要唯一已有本地连接；首次接入使用 connect")
			}
			b := bindings[0]
			lock, err := auth.AcquireDualLock(cmd.Context(), filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation"))
			if err != nil {
				return err
			}
			defer lock.Release()
			b, err = loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
			if err != nil {
				return err
			}
			if employeeBindingState(b) != "bound" {
				return fmt.Errorf("本地连接不是 bound；请先恢复原生命周期操作")
			}
			device, err := employeeBindingDeviceID(cmd.Context(), b, devAppStringFlag(cmd, "device-id"))
			if err != nil {
				return err
			}
			if b.DeviceID != "" && b.DeviceID != device {
				return fmt.Errorf("目标设备不同，请使用 connect rebind")
			}
			id, err := mutateEmployeeServerBinding(cmd, b, "bind", device)
			if err != nil {
				return err
			}
			b.DeviceID, b.RuntimeBindingID = device, id
			if err := updateEmployeeBinding(b); err != nil {
				return err
			}
			if err := consumeEmployeeServerOperation(b.DWSProfile); err != nil {
				return err
			}
			return writeDWSMachineEnvelope(cmd, map[string]any{"status": "bound", "agentUuid": b.AgentUUID, "deviceId": device, "runtimeBindingId": id, "serverBindingState": "bound"})
		},
	})
}
