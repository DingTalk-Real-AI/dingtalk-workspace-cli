// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

type employeeDSHState struct {
	ProtocolVersion   int    `json:"protocolVersion"`
	AgentUUID         string `json:"agentUuid"`
	Profile           string `json:"dwsProfile"`
	BindingRevision   uint64 `json:"bindingRevision"`
	RuntimeInstanceID string `json:"runtimeInstanceId"`
	RuntimeState      string `json:"runtimeState"`
	TransportReady    bool   `json:"transportReady"`
	ExecutorReady     bool   `json:"executorReady"`
	Released          bool   `json:"released"`
	Prepared          bool   `json:"prepared"`
	ObservedAt        string `json:"observedAt"`
}

var employeeDSHControl = runEmployeeDSHControl

func runEmployeeDSHControl(ctx context.Context, b digitalEmployeeBinding, action string) (employeeDSHState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result employeeDSHState
	input, _ := json.Marshal(map[string]any{"protocolVersion": 1, "action": action, "agentUuid": b.AgentUUID, "dwsProfile": b.DWSProfile, "bindingRevision": b.BindingRevision})
	ctx, cancel := context.WithTimeout(ctx, 32*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dsh-dingtalk", "digital-employee", "runtime", "--stdin", "--json")
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "CREDENTIAL") || strings.Contains(upper, "AUTH_CODE") || upper == "DWS_CLIENT_ID" || upper == "DWS_DUMP_RAW" {
			continue
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &employeeControlWriter{buf: &out, limit: 64 * 1024}
	cmd.Stderr = &employeeControlWriter{limit: 64 * 1024}
	if err := cmd.Run(); err != nil {
		return result, fmt.Errorf("DSH 宿主未确认运行状态，请启动或升级宿主后重试；不能视为已停止")
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || result.ProtocolVersion != 1 || result.AgentUUID != b.AgentUUID || result.Profile != b.DWSProfile || result.BindingRevision != b.BindingRevision {
		return result, fmt.Errorf("invalid DSH runtime response identity")
	}
	return result, nil
}

type employeeControlWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *employeeControlWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return 0, fmt.Errorf("runtime output exceeds limit")
	}
	w.limit -= len(p)
	if w.buf != nil {
		return w.buf.Write(p)
	}
	return len(p), nil
}

func updateEmployeeBinding(b digitalEmployeeBinding) error {
	return writeEmployeeJSON(digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile), b)
}

func stopEmployeeRuntime(ctx context.Context, b digitalEmployeeBinding) error {
	if bindingChannel(b) == "dsh" {
		s, err := employeeDSHControl(ctx, b, "stop")
		if err != nil {
			return err
		}
		if !s.Released || s.RuntimeState != "stopped" {
			return fmt.Errorf("DSH 尚未释放该员工")
		}
		return confirmEmployeeRuntimeReleased(ctx, b, s.RuntimeInstanceID)
	}
	dir := digitalEmployeeRuntimeDir(b.DWSProfile)
	activation, err := auth.AcquireDualLock(ctx, filepath.Join(dir, "activation"))
	if err != nil {
		return err
	}
	defer activation.Release()
	s, err := readDigitalEmployeeState(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.Profile != b.DWSProfile || s.AgentUUID != b.AgentUUID {
		return fmt.Errorf("employee runtime identity mismatch")
	}
	if !employeeStateAlive(s) {
		return nil
	}
	if s.RunID == "" {
		return fmt.Errorf("运行实例不明，拒绝停止")
	}
	if err := writeEmployeeJSON(filepath.Join(dir, "stop.json"), map[string]string{"runId": s.RunID}); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		current, err := readDigitalEmployeeState(dir)
		if err == nil && current.RunID == s.RunID && employeeStopComplete(current, s.SupervisorPID) {
			return nil
		}
		if err := waitForDigitalEmployeeReceipt(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("等待员工释放超时；保留绑定，禁止启动新 Adapter")
}

func confirmEmployeeRuntimeReleased(ctx context.Context, b digitalEmployeeBinding, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock, err := auth.AcquireDualLock(ctx, filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "worker"))
	if err != nil {
		return fmt.Errorf("员工 Profile 运行锁仍被占用，未确认释放")
	}
	defer lock.Release()
	return clearEmployeeLeaseGuard(b, instance)
}

func employeeLifecycleStatus(ctx context.Context, b digitalEmployeeBinding) map[string]any {
	state := map[string]any{"agentUuid": b.AgentUUID, "dwsProfile": b.DWSProfile, "channel": bindingChannel(b), "bindingRevision": b.BindingRevision, "bindingState": employeeBindingState(b), "desiredState": employeeDesiredState(b), "status": "stopped", "runtimeState": "stopped", "transportReady": false, "executorReady": false, "runtimeInstanceId": "", "observedAt": time.Now().UTC().Format(time.RFC3339Nano)}
	state["deviceId"], state["runtimeBindingId"] = b.DeviceID, b.RuntimeBindingID
	state["serverBindingState"] = "unregistered"
	if b.RuntimeBindingID != "" {
		state["serverBindingState"] = "bound"
		if employeeBindingState(b) == "unbound" {
			state["serverBindingState"] = "unbound"
		}
	}
	if raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile)); err == nil {
		var serverOp employeeServerOperation
		if json.Unmarshal(raw, &serverOp) != nil || serverOp.Phase == "pending" {
			state["serverBindingState"] = "unknown"
		}
		if serverOp.Phase == "confirmed" {
			state["serverBindingState"] = "commit_pending"
		}
	} else if !os.IsNotExist(err) {
		state["serverBindingState"] = "unknown"
	}
	var op employeeBindingOperation
	if data, err := os.ReadFile(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation.json")); err == nil && json.Unmarshal(data, &op) == nil {
		state["operationId"] = op.ID
		if op.ReasonCode != "" {
			state["reasonCode"] = op.ReasonCode
			state["nextAction"] = "重试原 connect unbind 或 connect；若新绑定已提交则使用 connect restart"
		}
	}
	if employeeBindingState(b) == "unbound" {
		return state
	}
	if bindingChannel(b) == "dsh" {
		r, err := employeeDSHControl(ctx, b, "status")
		if err != nil {
			state["status"] = "unknown"
			state["runtimeState"] = "unknown"
			state["reasonCode"] = "dsh_host_unreachable"
			state["nextAction"] = "启动或升级 DSH 宿主后重试；不要强行覆盖绑定"
			return state
		}
		state["status"] = r.RuntimeState
		state["runtimeState"] = r.RuntimeState
		state["runtimeInstanceId"] = r.RuntimeInstanceID
		state["transportReady"] = r.TransportReady
		state["executorReady"] = r.ExecutorReady
		state["observedAt"] = r.ObservedAt
	} else {
		r, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(b.DWSProfile))
		if err == nil && r.AgentUUID == b.AgentUUID && r.Profile == b.DWSProfile {
			state["runtimeInstanceId"] = r.RunID
			state["pid"] = r.PID
			state["logPath"] = r.LogPath
			if employeeStateAlive(r) || r.Status == "blocked" {
				state["status"] = r.Status
				state["runtimeState"] = r.Status
				state["transportReady"] = r.Status == "running"
				state["executorReady"] = r.Status == "running"
			}
			if r.Code != "" {
				state["reasonCode"] = r.Code
			}
		} else if !os.IsNotExist(err) {
			state["status"] = "unknown"
			state["runtimeState"] = "unknown"
			state["reasonCode"] = "runtime_state_invalid"
		}
	}
	return state
}

func employeeBindingState(b digitalEmployeeBinding) string {
	if b.BindingState == "" {
		return "bound"
	}
	return b.BindingState
}
func employeeDesiredState(b digitalEmployeeBinding) string {
	if b.DesiredState == "" {
		return "running"
	}
	return b.DesiredState
}
func sameEmployeeBinding(a, b digitalEmployeeBinding) bool {
	return a.AgentUUID == b.AgentUUID && a.DWSProfile == b.DWSProfile && bindingChannel(a) == bindingChannel(b) && a.OperatorOpenDingTalkID == b.OperatorOpenDingTalkID && a.BindingRevision == b.BindingRevision
}
func validateEmployeeMachineRevision(cmd *cobra.Command, id string, revision uint64) error {
	if err := validateEmployeeMachineBinding(cmd, id); err != nil {
		return err
	}
	b, err := deapChannelLoadBinding(deapConnectConfigDir(), auth.RuntimeProfile())
	if err != nil || b.BindingRevision != revision || employeeBindingState(b) == "unbound" {
		return fmt.Errorf("employee binding revision is no longer authorized")
	}
	return nil
}

func newEmployeeBindingCommand() *cobra.Command {
	c := digitalEmployeeChannelContract("channel_binding", "binding", "读取数字员工权威绑定；用于宿主启动前校验，不访问凭据或其他宿主。", "DSH 启动员工之前核验绑定版本与运行期望")
	c.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"agentUuid":{"type":"string","description":"员工 ID"},"dwsProfile":{"type":"string","description":"精确员工 Profile"},"channel":{"type":"string","description":"绑定 Adapter"},"bindingRevision":{"type":"integer","description":"绑定代数"},"bindingState":{"type":"string","description":"绑定状态"},"desiredState":{"type":"string","description":"运行期望"}}}`)}
	return NewLeafCommand(LeafSpec{Use: "binding", Short: "读取员工绑定权威状态", PostMount: deapAgentNoArgs, OutputRollout: output.RolloutUnifiedActive,
		Flags:  []LeafFlag{{Name: "channel", Required: true, Enum: digitalEmployeeChannels(), Usage: "绑定 Adapter"}, {Name: "stdin", Kind: LeafBool, Required: true, Usage: "员工身份与绑定版本 JSON"}},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, Contract: c,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var in struct {
				AgentUUID       string `json:"agentUuid"`
				BindingRevision uint64 `json:"bindingRevision"`
			}
			if err := decodeBoundedDigitalEmployeeStdin(cmd, &in); err != nil {
				return err
			}
			if err := validateEmployeeMachineRevision(cmd, in.AgentUUID, in.BindingRevision); err != nil {
				return err
			}
			b, err := deapChannelLoadBinding(deapConnectConfigDir(), auth.RuntimeProfile())
			if err != nil {
				return err
			}
			return writeDWSMachineEnvelope(cmd, map[string]any{"agentUuid": b.AgentUUID, "dwsProfile": b.DWSProfile, "channel": bindingChannel(b), "bindingRevision": b.BindingRevision, "bindingState": employeeBindingState(b), "desiredState": employeeDesiredState(b)})
		},
	})
}

func newEmployeeUnbindCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "unbind", Short: "解绑数字员工并保留本地身份", PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{{Name: "agent-uuid", Required: true, Usage: "本地已绑定员工 ID"}, {Name: "runtime-binding-id", Usage: "明确指定服务端绑定 ID；必须与本地记录一致"}}, OutputRollout: output.RolloutUnifiedActive,
		Safety:   contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{Identity: contract.ToolIdentitySpec{ProductID: dingtalkTagProductID, Name: "connect_unbind", CanonicalPath: "dingtalk-tag.connect_unbind", CLIPath: "dingtalk-tag connect unbind", PrimaryCLIPath: "dingtalk-tag connect unbind", Group: "connect"}, Description: "停止并确认员工实例释放后调用服务端解绑；保留 Profile、凭据、审计和去重记录。未知实例不强行解绑。", Parameters: []contract.ParamDecl{{Name: "agent-uuid", Property: "agentUuid"}, {Name: "runtime-binding-id", Property: "runtimeBindingId"}}, Result: digitalEmployeeResultSpec(), DryRun: deapAgentPlanDryRun, Interface: &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "服务端解绑回执、本机绑定事务及宿主控制"}, Selection: contract.SelectionSpec{AgentSummary: "安全解除已有数字员工的本机 Adapter 绑定", UseWhen: []string{"解绑数字员工并保留 Profile", "更换 Agent 或设备时先解绑，成功后再 connect"}, AvoidWhen: []string{"仅暂停使用 connect stop；首次接入使用 connect；机器人使用 dev connect"}, Examples: []string{"dws dingtalk-tag connect unbind --agent-uuid <agentUuid>"}}},
		RunE:     func(cmd *cobra.Command, _ []string) error { return runEmployeeUnbind(cmd) },
	})
}

type employeeBindingOperation struct {
	ID           string `json:"operationId"`
	Action       string `json:"action"`
	FromRevision uint64 `json:"fromRevision"`
	Target       string `json:"target"`
	ReasonCode   string `json:"reasonCode,omitempty"`
}

func runEmployeeUnbind(cmd *cobra.Command) (runErr error) {
	bindings, err := employeeFindBindings(devAppStringFlag(cmd, "agent-uuid"))
	if err != nil {
		return err
	}
	if len(bindings) != 1 {
		return fmt.Errorf("员工必须对应唯一的本地绑定")
	}
	b := bindings[0]
	// 预览不获取文件锁、不生成设备 ID、不修复回执，也不接触宿主。
	if commandDryRun(cmd) {
		steps := []string{"stop_old", "confirm_released", "server_unbind", "unbind_keep_profile"}
		channel := bindingChannel(b)
		return writeDWSMachinePlan(cmd, map[string]any{"status": "planned", "agentUuid": b.AgentUUID, "channel": channel, "steps": steps})
	}
	dir := digitalEmployeeRuntimeDir(b.DWSProfile)
	lock, err := auth.AcquireDualLock(cmd.Context(), filepath.Join(dir, "operation"))
	if err != nil {
		return err
	}
	defer lock.Release()
	b, err = loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil {
		return err
	}
	if explicit := devAppStringFlag(cmd, "runtime-binding-id"); explicit != "" && explicit != b.RuntimeBindingID {
		return fmt.Errorf("runtime-binding-id 与本地记录不一致；禁止操作其他绑定")
	}
	if employeeBindingState(b) == "unbound" {
		if err := checkEmployeeServerOperation(b); err != nil {
			return err
		}
		if err := consumeEmployeeServerOperation(b.DWSProfile); err != nil {
			return err
		}
		return writeDWSMachineEnvelope(cmd, employeeLifecycleStatus(cmd.Context(), b))
	}
	if b.RuntimeBindingID == "" {
		return fmt.Errorf("绑定记录缺少 runtimeBindingId，无法确认解绑目标；请核对服务端绑定记录")
	}
	op := employeeBindingOperation{ID: uuid.NewString(), Action: "unbind", FromRevision: b.BindingRevision}
	if previous, err := os.ReadFile(filepath.Join(dir, "operation.json")); err == nil {
		var old employeeBindingOperation
		if json.Unmarshal(previous, &old) == nil && old.Action == "unbind" && old.FromRevision == b.BindingRevision && old.Target == op.Target {
			op.ID = old.ID
		}
	}
	if err := writeEmployeeJSON(filepath.Join(dir, "operation.json"), op); err != nil {
		return err
	}
	defer func() {
		if runErr != nil {
			op.ReasonCode = "binding_operation_incomplete"
			_ = writeEmployeeJSON(filepath.Join(dir, "operation.json"), op)
		}
	}()
	b.BindingState = "unbinding"
	b.DesiredState = "stopped"
	if err := updateEmployeeBinding(b); err != nil {
		return err
	}
	if err := stopEmployeeRuntime(cmd.Context(), b); err != nil {
		return err
	}
	if bindingChannel(b) == "dsh" {
		r, err := employeeDSHControl(cmd.Context(), b, "release")
		if err != nil {
			return err
		}
		if !r.Released {
			return fmt.Errorf("DSH release 未确认，保留旧绑定")
		}
	}
	if err := writeEmployeeJSON(filepath.Join(dir, fmt.Sprintf("binding-%d.json", b.BindingRevision)), b); err != nil {
		return err
	}
	if _, err := mutateEmployeeServerBinding(cmd, b, "unbind", b.DeviceID); err != nil {
		return err
	}
	b.BindingState = "unbound"
	b.BindingRevision++
	if err := updateEmployeeBinding(b); err != nil {
		return err
	}
	if err := consumeEmployeeServerOperation(b.DWSProfile); err != nil {
		return err
	}
	return writeDWSMachineEnvelope(cmd, employeeLifecycleStatus(cmd.Context(), b))
}
