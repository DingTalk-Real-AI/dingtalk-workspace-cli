// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// 显式 opt-in，使用真实员工身份、传输及审批处理器；只复用已有卡片订阅。
// 人工点击后才执行隔离目录内的一次 printf，不修改 Agent 会话权限。
func TestEmployeeLiveApprovalCardReplacement(t *testing.T) {
	dir := os.Getenv("DWS_APPROVAL_CARD_LIVE_DIR")
	if dir == "" {
		t.Skip("set DWS_APPROVAL_CARD_LIVE_DIR and the explicit binary, adapter, subscription to opt in")
	}
	bin, adapter, subscribe := os.Getenv("DWS_APPROVAL_CARD_LIVE_BIN"), os.Getenv("DWS_APPROVAL_CARD_LIVE_ADAPTER"), os.Getenv("DWS_APPROVAL_CARD_LIVE_SUBSCRIBE")
	if bin == "" || adapter == "" || subscribe == "" {
		t.Fatal("真实审批验收缺少明确的二进制、员工配置或既有订阅")
	}
	raw, err := os.ReadFile(adapter)
	var cfg digitalEmployeeAdapterConfig
	if err != nil || json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("员工运行配置不可读")
	}
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return bin, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	// 生产员工在审批回合超时后继续运行，测试不能同时取消原卡的收尾上下文。
	runtimeCtx, stopRuntime := context.WithCancel(context.WithoutCancel(ctx))
	defer stopRuntime()
	r := &employeeRuntime{cfg: cfg, dir: dir, ctx: runtimeCtx}
	var receiptMu sync.Mutex
	terminalConfirmed := false
	realCall := employeeA2UICall
	testseam.Swap(t, &employeeA2UICall, func(ctx context.Context, b digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		result, err := realCall(ctx, b, messages, args...)
		label := "pending"
		if args[0] == "update-a2ui-card" {
			label = "terminal"
			receiptMu.Lock()
			terminalConfirmed = err == nil && jsonScalar(result["flowStatus"]) == "FINISH"
			receiptMu.Unlock()
		}
		_ = writeEmployeeJSON(filepath.Join(dir, label+"-card.json"), messages)
		_ = writeEmployeeJSON(filepath.Join(dir, label+"-receipt.json"), map[string]any{"confirmed": err == nil, "result": result})
		return result, err
	})
	// 观察者由自身时长和 SIGTERM 收尾，避免回合取消触发 CommandContext 的硬终止。
	observer, err := employeeCommand(context.WithoutCancel(ctx), cfg.Binding.DWSProfile, "event", "consume", "--subscribe-id", subscribe, "--flatten", "--duration", "8m", "--format", "ndjson")
	if err != nil {
		t.Fatal("无法启动既有订阅的观察者")
	}
	stdin, _ := observer.StdinPipe()
	stdout, _ := observer.StdoutPipe()
	stderr, _ := observer.StderrPipe()
	if err := observer.Start(); err != nil {
		t.Fatalf("回调观察者启动失败：%T", err)
	}
	observerDone := make(chan error, 1)
	go func() { observerDone <- observer.Wait() }()
	var readers sync.WaitGroup
	readers.Add(2)
	defer func() {
		_ = observer.Process.Signal(syscall.SIGTERM)
		_ = stdin.Close()
		select {
		case <-observerDone:
		case <-time.After(5 * time.Second):
			t.Error("观察者没有及时退出；保留既有订阅")
		}
		_ = stdout.Close()
		_ = stderr.Close()
		readers.Wait()
		r.wg.Wait()
	}()
	ready := make(chan struct{}, 1)
	go func() {
		defer readers.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "[event] ready") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
	}()
	go func() {
		defer readers.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		for scanner.Scan() {
			r.handleApprovalEvent(scanner.Bytes())
		}
	}()
	select {
	case <-ready:
	case <-time.After(40 * time.Second):
		t.Fatal("回调观察者未就绪，不发送卡片")
	}
	resultFile := filepath.Join(dir, "approved-result.txt")
	if _, err := os.Stat(resultFile); !os.IsNotExist(err) {
		t.Fatal("验收结果文件已存在，停止本轮测试")
	}
	command := "printf '%s\\n' 'A2UI_APPROVAL_V2_OK' > '" + strings.ReplaceAll(resultFile, "'", "'\\''") + "'"
	params, _ := json.Marshal(map[string]any{"command": command, "cwd": dir, "reason": "新版单条授权卡测试：只有主管真实点击允许后，才在本轮隔离目录写入一行测试标记。"})
	done := make(chan bool, 1)
	go func() {
		done <- r.requestApproval(ctx, "item/commandExecution/requestApproval", params)
		close(done)
	}()
	defer func() {
		cancel()
		for range done {
		}
	}()
	var record employeeApprovalRecord
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		paths, _ := filepath.Glob(filepath.Join(dir, "approvals", "*.json"))
		if len(paths) == 1 {
			data, _ := os.ReadFile(paths[0])
			_ = json.Unmarshal(data, &record)
			if record.Status == "pending" {
				break
			}
		}
		select {
		case <-done:
			t.Fatal("卡片未确认进入待审批状态；不自动重发")
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	if record.Status != "pending" {
		t.Fatal("卡片未确认进入待审批状态；不自动重发")
	}
	state := map[string]any{"state": "awaiting_click", "approvalId": record.ID, "bizId": record.BizID, "expiresAt": record.ExpiresAt, "resultFile": resultFile, "resultExistsBeforeClick": false}
	if err := writeEmployeeJSON(filepath.Join(dir, "live-state.json"), state); err != nil {
		t.Fatal(err)
	}
	t.Logf("APPROVAL_CARD_AWAITING_CLICK approvalId=%s bizId=%s", record.ID, record.BizID)
	allowed := <-done
	if allowed {
		if err := exec.CommandContext(ctx, "/bin/zsh", "-c", command).Run(); err != nil {
			t.Fatalf("批准后的测试命令失败：%T", err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "approvals", record.ID+".json"))
	_ = json.Unmarshal(data, &record)
	content, readErr := os.ReadFile(resultFile)
	if allowed && (readErr != nil || strings.TrimSpace(string(content)) != "A2UI_APPROVAL_V2_OK") || !allowed && !os.IsNotExist(readErr) {
		t.Error("审批决定与实际执行结果不一致")
	}
	receiptMu.Lock()
	state["terminalUpdateConfirmed"] = terminalConfirmed
	receiptMu.Unlock()
	state["state"], state["approvalStatus"], state["allowed"], state["resultExists"] = "finished", record.Status, allowed, readErr == nil
	_ = writeEmployeeJSON(filepath.Join(dir, "live-state.json"), state)
	if state["terminalUpdateConfirmed"] != true {
		t.Error("终态卡片更新未确认")
	}
	t.Log(fmt.Sprintf("真实审批卡验收结束：decision=%s executed=%t", record.Status, allowed))
}
