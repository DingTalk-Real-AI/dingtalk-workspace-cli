// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// 显式 opt-in 的真实卡片链路验收。进度为标记清晰的演示数据，只发给员工主管。
// 不修改员工绑定，也不调用模型或执行工具；finish 文件用于在客户端观察后结束演示。
func TestEmployeeCardAcceptanceLive(t *testing.T) {
	dir := os.Getenv("DWS_CARD_ACCEPTANCE_DIR")
	if dir == "" {
		t.Skip("set DWS_CARD_ACCEPTANCE_DIR, BIN and ADAPTER for an authorized self-only card test")
	}
	bin, adapter := os.Getenv("DWS_CARD_ACCEPTANCE_BIN"), os.Getenv("DWS_CARD_ACCEPTANCE_ADAPTER")
	if !filepath.IsAbs(bin) || !filepath.IsAbs(adapter) || !filepath.IsAbs(dir) {
		t.Fatal("需要明确的二进制、员工配置和独立测试目录")
	}
	raw, err := os.ReadFile(adapter)
	var cfg digitalEmployeeAdapterConfig
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.Binding.OperatorOpenDingTalkID == "" {
		t.Fatal("员工运行配置不可读或主管未确认")
	}
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return bin, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	r := &employeeRuntime{cfg: cfg, dir: dir, ctx: ctx}
	sequence := 0
	realCall := employeeA2UICall
	persistent := os.Getenv("DWS_CARD_ACCEPTANCE_PERSISTENT_FAILURE") == "1"
	fault := persistent || os.Getenv("DWS_CARD_ACCEPTANCE_BEFORE_SEND_FAILURE") == "1"
	var faultServer *httptest.Server
	if fault {
		faultServer = httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("未受信任证书不应收到 HTTP 请求") }))
		faultServer.Config.ErrorLog = log.New(io.Discard, "", 0)
		faultServer.StartTLS()
		defer faultServer.Close()
	}
	injected := false
	testseam.Swap(t, &employeeA2UICall, func(ctx context.Context, binding digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		started := time.Now()
		var result map[string]any
		var err error
		if fault && args[0] == "update-a2ui-card" && (persistent || !injected && args[4] == "INPUTTING") {
			// 真实本机 TLS 握手失败，经正式 transport/JSON 错误边界；不向业务网关提交这一帧。
			injected = true
			client := transport.NewClient(&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}).WithMaxRetries(0)
			_, networkErr := client.CallTool(ctx, faultServer.URL, "card_recovery_test", nil)
			var encoded bytes.Buffer
			if networkErr == nil {
				t.Fatal("预期的证书校验失败未发生")
			}
			if jsonErr := apperrors.PrintJSON(&encoded, networkErr); jsonErr != nil {
				t.Fatal(jsonErr)
			}
			err = employeeCardResponseError(nil, encoded.Bytes())
			if !employeeCardCanRecover(err) {
				t.Fatalf("真实握手失败没有安全恢复证据：%v", err)
			}
		} else {
			result, err = realCall(ctx, binding, messages, args...)
		}
		sequence++
		label := fmt.Sprintf("frame-%02d", sequence)
		_ = writeEmployeeJSON(filepath.Join(dir, label+".json"), messages)
		receipt := map[string]any{"started": started, "finished": time.Now(), "args": args, "confirmed": err == nil, "result": result}
		var classified *employeeRunError
		if errors.As(err, &classified) {
			receipt["diagnostic"] = classified.CardDiagnostic
		}
		_ = writeEmployeeJSON(filepath.Join(dir, label+"-receipt.json"), receipt)
		return result, err
	})
	e := employeeEvent{Type: "user_im_message_receive_o2o", EventID: "c2-local-demo", MessageID: "c2-local-demo", ConversationID: "c2-local-demo", SenderID: cfg.Binding.OperatorOpenDingTalkID, Content: "C2 本地卡片演示验收"}
	if persistent {
		e.MessageID, e.ConversationID = os.Getenv("DWS_CARD_ACCEPTANCE_MESSAGE_ID"), os.Getenv("DWS_CARD_ACCEPTANCE_CONVERSATION_ID")
		if !validMachineString(e.MessageID) || !validMachineString(e.ConversationID) {
			t.Fatal("持续故障验收需要主管单聊中的真实原消息 ID 和会话 ID")
		}
	}
	fwd := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "demo", Kind: "progress", Text: "C2 本地验收演示：核对展开、浅色过程和终态收起。"})
		emit(agentProgress{ID: "tool", Kind: "tool", Text: "文件读取（演示）", Status: "running"})
		for _, answer := range []string{"**C2 本地验收演示**\n\n第一段已到达。", "**C2 本地验收演示**\n\n第一段已到达。\n\n第二段已到达，正文位于过程面板外。"} {
			emit(agentProgress{ID: "answer", Kind: "answer", Text: answer})
			time.Sleep(time.Second)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "finish")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
		emit(agentProgress{ID: "tool", Kind: "tool", Text: "文件读取（演示）", Status: "completed"})
		if fault {
			return "**卡片故障恢复验收完成**\n\n本轮在本机真实触发了一次 TLS 证书校验失败，随后向原卡片补交完整结果并结束。\n\n本次使用演示进度验证恢复链路，没有重跑 Agent 或执行业务工具。", nil
		}
		return "**C2 本地验收演示完成**\n\n过程应已自动收起；展开后可见浅色的演示过程与工具完成状态。\n\n本次只验证卡片显示，没有执行真实工具。", nil
	}}
	key := sha256.Sum256([]byte(dir))
	record := employeeTaskRecord{IdempotencyKey: fmt.Sprintf("%x", key[:16])}
	wantStatus := "card_finished"
	if persistent {
		wantStatus = "card_delivery_failed"
	}
	if err := r.processCard(e, &record, fwd); err != nil || record.Status != wantStatus {
		t.Fatalf("卡片验收未完成：status=%s error=%v", record.Status, err)
	}
	if persistent {
		if err := r.notifyCardFailure(e, &record); err != nil || record.CardNoticeDelivery != "delivered" {
			_ = writeEmployeeJSON(filepath.Join(dir, "result.json"), record)
			t.Fatalf("独立终态通知没有确认送达：%s %v", record.CardNoticeDelivery, err)
		}
	}
	if fault && (!injected || record.CardRecoveryAttempts != 1) {
		t.Fatalf("未验证单次安全补交：%+v", record)
	}
	_ = writeEmployeeJSON(filepath.Join(dir, "result.json"), record)
	t.Logf("bizId=%s flowStatus=%s updates=%d", record.BizID, record.FlowStatus, record.CardUpdates)
}
