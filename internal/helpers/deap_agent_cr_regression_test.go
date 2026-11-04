// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCRTextUnknown(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "text"
	r.fwd = &employeeBoundaryForwarder{err: fmt.Errorf("%w: %w", errCodexEmployeeResumeFailed, &codexDesktopTurnError{Code: "codex_desktop_result_unknown", Submitted: true})}
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, filepath.Join(r.dir, "no-network-audit"))
	})
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "audit"}); err != nil {
		t.Fatal(err)
	}
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.recordPath(e))
	if err != nil {
		t.Fatal(err)
	}
	var got employeeTaskRecord
	if json.Unmarshal(raw, &got) != nil {
		t.Fatal("账本不可读")
	}
	if got.Execution != "unknown" || got.ExecutionErrorCode != "codex_desktop_result_unknown" {
		t.Errorf("已提交未知被错误归类：execution=%q code=%q status=%q", got.Execution, got.ExecutionErrorCode, got.Status)
	}
}

func TestCrossPlatformCoverageEmployeeCRStreamFailure(t *testing.T) {
	for _, mode := range []string{"nonzero", "timeout", "result-error", "session-error"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &connectStreamCommandContext, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				calls++
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCRStreamProcess$", "--", mode)
			})
			f := &execForwarder{name: "claudecode", argv: []string{"audit"}, streamArgv: []string{"audit"}, parser: "cc", workDir: t.TempDir(), env: []string{"DWS_EMPLOYEE_CR_PROCESS=1"}, timeout: 2 * time.Second}
			if runtime.GOOS == "windows" {
				f.timeout = 10 * time.Second
			}
			if mode == "session-error" {
				f.sessions = newConvSessions("")
			}
			var seen string
			answer, err := f.forwardStream(context.Background(), "audit", "audit", func(s string) { seen = s })
			if calls != 1 {
				t.Fatalf("已有部分输出后不应自动重试: %d", calls)
			}
			if seen == "" {
				t.Fatal("没有触发部分正文路径")
			}
			if err == nil {
				t.Errorf("%s 后仍返回成功：answer=%q", mode, answer)
			}
			if mode == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("deadline 身份丢失: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCRLegacyAsk(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	cfg := digitalEmployeeAdapterConfig{SelfOpenDingTalkID: "self", SelfIdentityProfile: "corp:employee", Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:employee", OperatorOpenDingTalkID: "owner", Channel: "codex"}}
	cfg.Options.Yolo = false
	if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json")
	if err := writeEmployeeJSON(p, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDigitalEmployeeConfig(cfg.Binding.DWSProfile)
	if err != nil {
		t.Fatal(err)
	}
	bad := cfg
	bad.Options.CodexInitialPermissionMode = "unknown"
	if err := writeEmployeeJSON(p, bad); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDigitalEmployeeConfig(cfg.Binding.DWSProfile); err == nil {
		t.Fatal("未知保存模式被恢复为默认权限")
	}
	if _, err := digitalEmployeeNewForwarder(context.Background(), bad); err == nil {
		t.Fatal("未知模式启动了 forwarder")
	}
	f := &codexAppServerForwarder{developerInstructions: "employee", initialPermissionMode: loaded.Options.CodexInitialPermissionMode}
	got := f.threadParams("")
	if loaded.Options.CodexInitialPermissionMode != "legacy-ask" || got["sandbox"] != "read-only" || got["approvalsReviewer"] != "user" {
		t.Errorf("旧 Yolo=false 缺保守迁移：mode=%q sandbox=%v reviewer=%v", loaded.Options.CodexInitialPermissionMode, got["sandbox"], got["approvalsReviewer"])
	}
}

func TestCrossPlatformCoverageEmployeeCRCredentialDisplay(t *testing.T) {
	for _, s := range []string{"credential=CR_AUDIT_FAKE", "AWS_SECRET_ACCESS_KEY=CR_AUDIT_FAKE", "SECRET_KEY=CR_AUDIT_FAKE", "Cookie: CR_AUDIT_FAKE"} {
		t.Run(s, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"command": "echo " + s})
			_, fields, safe := employeeApprovalDetails("item/commandExecution/requestApproval", raw)
			if safe || len(fields) != 0 {
				t.Errorf("常见凭据形式仍允许原文展示和批准：safe=%v fields=%d", safe, len(fields))
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCRCredentialContext(t *testing.T) {
	for _, text := range []string{
		"grep -rn token src/", "npm run test-token-refresh", "需要刷新 token 缓存", "rotate secret rotation job",
		"echo headers envs umid", "echo text=data content=data", "grep --include token src/",
	} {
		t.Run(text, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"command": "grep -rn token src/", "reason": text})
			if _, fields, safe := employeeApprovalDetails("item/commandExecution/requestApproval", raw); !safe || len(fields) != 2 {
				t.Fatal("普通命令或申请理由被误判为凭据")
			}
		})
	}
	for _, text := range []string{
		"TOKEN=CR_FAKE", `{"credential": "CR_FAKE"}`, "--password CR_FAKE", "--api-token=CR_FAKE",
		"Cookie: CR_FAKE", "AWS_SECRET_ACCESS_KEY=CR_FAKE", "SECRET_KEY=CR_FAKE", "Authorization: Basic CR_FAKE", "--password -CR_FAKE",
	} {
		t.Run(text, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"command": "echo " + text})
			if _, fields, safe := employeeApprovalDetails("item/commandExecution/requestApproval", raw); safe || len(fields) != 0 {
				t.Fatal("真实赋值或参数形式的凭据仍允许展示和批准")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCRLegacyActionBinding(t *testing.T) {
	r, _ := employeeLedgerFixture(t)
	r.cfg.Binding.OperatorOpenDingTalkID = "owner"
	pending := &employeeApproval{BizID: "biz", decision: make(chan bool, 1)}
	r.approvals = map[string]*employeeApproval{"audit": pending}
	raw := []byte(`{"type":"user_card_action_triggered","payload":{"body":{"actionData":{"name":"wrong-action","surfaceId":"wrong-surface","sourceComponentId":"wrong-button","context":{"approvalId":"audit","decision":"approve"}},"bizInfoDTO":{"bizId":"biz"},"operatorDTO":{"openDingTalkId":"owner"}}}}`)
	r.handleApprovalEvent(raw)
	r.wg.Wait()
	select {
	case allowed := <-pending.decision:
		if allowed {
			t.Error("旧回调在动作/组件均不匹配时仍放行")
		}
	default:
	}
}

func TestCrossPlatformCoverageEmployeeCREmptyPermissionBaseline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := newCodexDesktopApprovals(ctx)
	defer a.close()
	a.syncPermissions(codexDesktopSnapshot{})
	wait, stop := context.WithCancel(ctx)
	a.waits["audit"] = &codexDesktopApprovalWait{cancel: stop, result: make(chan bool, 1)}
	a.syncPermissions(codexDesktopSnapshot{CurrentPermissions: json.RawMessage(`{"approvalPolicy":"never","sandboxPolicy":{"type":"dangerFullAccess"}}`)})
	if wait.Err() == nil || !a.waits["audit"].invalidated {
		t.Error("权限空基线首次出现时，旧审批未失效")
	}
}

// 真正启动测试子进程，验证 stdout 已产生后非零退出或被 deadline 终止的路径。
func TestEmployeeCRStreamProcess(t *testing.T) {
	if os.Getenv("DWS_EMPLOYEE_CR_PROCESS") != "1" {
		return
	}
	fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"CR_PARTIAL"}}}`)
	if os.Args[len(os.Args)-1] == "session-error" {
		fmt.Fprintln(os.Stderr, "session not found")
		os.Exit(1)
	}
	if os.Args[len(os.Args)-1] == "nonzero" {
		os.Exit(1)
	}
	if os.Args[len(os.Args)-1] == "result-error" {
		fmt.Println(`{"type":"result","is_error":true,"subtype":"error_during_execution","result":""}`)
		os.Exit(0)
	}
	time.Sleep(60 * time.Second)
}

func TestCrossPlatformCoverageEmployeeCRModesAndApprovalFields(t *testing.T) {
	for _, tc := range []struct {
		mode string
		yolo bool
		want string
	}{{"", false, "legacy-ask"}, {"", true, ""}, {"ask", false, "ask"}, {"bypass", false, "bypass"}, {"legacy-ask", false, "legacy-ask"}} {
		got, err := employeeCodexInitialMode(connectAgentOptions{CodexInitialPermissionMode: tc.mode, Yolo: tc.yolo})
		if err != nil || got != tc.want {
			t.Fatalf("权限迁移: %q %v", got, err)
		}
	}
	if _, err := employeeCodexInitialMode(connectAgentOptions{CodexInitialPermissionMode: "unknown"}); err == nil {
		t.Fatal("未知旧配置不能默认自动审核")
	}
	for _, value := range []string{"Bearer CR_FAKE", "https://user:CR_FAKE@example.invalid"} {
		if !employeeApprovalHasSensitiveData(value) {
			t.Fatal("认证值未拒绝")
		}
	}
	for _, v := range []string{"echo safe", "echo text data", "echo content data"} {
		raw, _ := json.Marshal(map[string]any{"command": v})
		if _, _, safe := employeeApprovalDetails("item/commandExecution/requestApproval", raw); !safe {
			t.Fatal("普通正文误判为凭据", v)
		}
	}
	r, _ := employeeLedgerFixture(t)
	r.cfg.Binding.OperatorOpenDingTalkID = "owner"
	for _, scenario := range []string{"valid", "name", "surface", "button", "missing"} {
		pending := &employeeApproval{BizID: "biz", decision: make(chan bool, 1)}
		r.approvals = map[string]*employeeApproval{"id": pending}
		action := map[string]any{"name": "dws_employee_approval", "surfaceId": "approval-id", "sourceComponentId": "approve", "context": map[string]string{"approvalId": "id", "decision": "approve"}}
		switch scenario {
		case "name":
			action["name"] = "wrong"
		case "surface":
			action["surfaceId"] = "wrong"
		case "button":
			action["sourceComponentId"] = "decline"
		case "missing":
			delete(action, "name")
		}
		raw, _ := json.Marshal(map[string]any{"type": "user_card_action_triggered", "payload": map[string]any{"body": map[string]any{"actionData": action, "bizInfoDTO": map[string]string{"bizId": "biz"}, "operatorDTO": map[string]string{"openDingTalkId": "owner"}}}})
		r.handleApprovalEvent(raw)
		r.wg.Wait()
		allowed := false
		select {
		case allowed = <-pending.decision:
		default:
		}
		if allowed != (scenario == "valid") {
			t.Fatal("旧回调组件绑定错误", scenario)
		}
	}
}

func TestCrossPlatformCoverageEmployeeCRResultClassification(t *testing.T) {
	for _, tc := range []struct {
		code      string
		submitted bool
		execution string
	}{{"codex_desktop_unavailable", false, "failure"}, {"codex_desktop_turn_failed", true, "failure"}, {"codex_desktop_result_unknown", true, "unknown"}} {
		f := employeeSharedTurnFailure(&codexDesktopTurnError{Code: tc.code, Submitted: tc.submitted})
		if f == nil || f.Code != tc.code || f.Execution != tc.execution || f.Reply == "" {
			t.Fatal("文本与卡片错误分类不一致", f)
		}
	}
	if employeeSharedTurnFailure(errors.New("unrelated")) != nil {
		t.Fatal("非桌面错误被误分类")
	}
	if got := employeeTaskLabel(employeeTaskRecord{Status: "needs_review", Execution: "unknown", ExecutionErrorCode: "codex_desktop_result_unknown", Delivery: "accepted"}); got != "结果待确认" {
		t.Fatal(got)
	}
}

type crSettingsReader struct {
	*bytes.Reader
	reads, seeks int
	failSeek     int
	failRead     bool
	emptyEnd     bool
}

func (r *crSettingsReader) Read(p []byte) (int, error) {
	if r.failRead && r.seeks > 1 {
		return 0, io.ErrUnexpectedEOF
	}
	n, e := r.Reader.Read(p)
	r.reads += n
	return n, e
}
func (r *crSettingsReader) Seek(o int64, w int) (int64, error) {
	r.seeks++
	if r.seeks == r.failSeek {
		return 0, io.ErrClosedPipe
	}
	if r.emptyEnd && w == io.SeekEnd {
		return 0, nil
	}
	return r.Reader.Seek(o, w)
}

func TestCrossPlatformCoverageCodexCRSavedSettingsTail(t *testing.T) {
	s := codexPermissionFixture(t)
	meta := []byte(`{"type":"session_meta","payload":{"id":"thread"}}` + "\n")
	settings, _ := json.Marshal(map[string]any{"type": "turn_context", "payload": s})
	latest, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "thread_settings_applied", "thread_id": "thread", "thread_settings": s}})
	history := bytes.Repeat([]byte(`{"type":"event_msg","payload":{"type":"history"}}`+"\n"), 100000)
	source := append(append(append([]byte{}, meta...), history...), append(latest, '\n')...)
	reader := &crSettingsReader{Reader: bytes.NewReader(source)}
	got, err := readCodexSavedSettingsFrom(reader, "thread")
	if err != nil || got.Cwd != s.Cwd || reader.reads > 160*1024 {
		t.Fatalf("未从最新尾部读取或读取了整个历史: read=%d err=%v", reader.reads, err)
	}
	long, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "history", "text": strings.Repeat("x", 80000)}})
	source = append(append(append(append([]byte{}, meta...), settings...), '\n'), long...)
	if got, err := readCodexSavedSettingsFrom(bytes.NewReader(source), "thread"); err != nil || got.Cwd != s.Cwd {
		t.Fatal("跨块回退未找到最新完整设置", err)
	}
	behind := append(append(append(append([]byte{}, meta...), settings...), '\n'), history...)
	if got, err := readCodexSavedSettingsFrom(bytes.NewReader(behind), "thread"); err != nil || got.Cwd != s.Cwd {
		t.Fatal("多块历史后的最新设置未正确读取", err)
	}
	for _, tail := range []string{`!`, `{"type":"turn_context","payload":42}`, `{"type":"event_msg","payload":42}`, `{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_id":"other"}}`, `{"type":"event_msg","payload":{"type":"history"}}`, strings.Repeat("x", codexDesktopFrameLimit+64*1024+1), strings.Repeat("x", codexDesktopFrameLimit+1) + "\n", `{"type":"event_msg","payload":{"text":"` + strings.Repeat("x", codexDesktopFrameLimit) + `"}}`} {
		raw := append(append([]byte{}, meta...), tail...)
		if _, err := readCodexSavedSettingsFrom(bytes.NewReader(raw), "thread"); err == nil {
			t.Fatal("最新权限残缺或缺失时必须停止恢复")
		}
	}
	for _, tc := range []struct {
		seek        int
		read, empty bool
	}{{1, false, false}, {2, false, false}, {0, true, false}, {0, false, true}} {
		raw := append(append([]byte{}, meta...), settings...)
		r := &crSettingsReader{Reader: bytes.NewReader(raw), failSeek: tc.seek, failRead: tc.read, emptyEnd: tc.empty}
		if _, err := readCodexSavedSettingsFrom(r, "thread"); err == nil {
			t.Fatal("权限读取 IO 失败被忽略")
		}
	}
}

func TestCrossPlatformCoverageCodexCREmptyRootsAndPolicies(t *testing.T) {
	s := codexPermissionFixture(t)
	s.Roots = []string{}
	for _, policy := range []string{`"on-request"`, `"never"`, `"untrusted"`, `"on-failure"`} {
		s.Approval = json.RawMessage(policy)
		params, err := s.resumeParams("thread")
		if err != nil {
			t.Fatal(err)
		}
		raw := codexPermissionResumeFixture(t, "thread", s)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		delete(out, "runtimeWorkspaceRoots")
		raw, _ = json.Marshal(out)
		if err := verifyCodexResumedPermissions(raw, params); err != nil {
			t.Fatal("空根目录省略时不应误报不一致", err)
		}
		params["runtimeWorkspaceRoots"] = []string{s.Cwd}
		if verifyCodexResumedPermissions(raw, params) == nil {
			t.Fatal("非空根目录缺失不得放行")
		}
	}
	for _, policy := range []string{`"future-policy"`, `{"future":true}`} {
		s.Approval = json.RawMessage(policy)
		if _, err := s.resumeParams("thread"); err == nil {
			t.Fatal("未知权限策略不得自动恢复")
		}
	}
}

func TestCrossPlatformCoverageEmployeeCRUnknownDelivery(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "text"
	r.fwd = &employeeBoundaryForwarder{err: fmt.Errorf("%w: %w", errCodexEmployeeResumeFailed, &codexDesktopTurnError{Code: "codex_desktop_result_unknown", Submitted: true, Cause: context.DeadlineExceeded})}
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCRReplyProcess$", "--", "CR_REPLY")
		cmd.Env = append(os.Environ(), "DWS_EMPLOYEE_CR_REPLY=1")
		return cmd
	})
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "reply"}); err != nil {
		t.Fatal(err)
	}
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.recordPath(e))
	if err != nil {
		t.Fatal(err)
	}
	var record employeeTaskRecord
	if json.Unmarshal(raw, &record) != nil {
		t.Fatal("账本格式错误")
	}
	if record.Execution != "unknown" || record.Status != "needs_review" || record.Delivery != "accepted" || employeeTaskLabel(record) != "结果待确认" {
		t.Fatalf("通知送达不得变成任务成功或取消: %+v", record)
	}
}
func TestEmployeeCRReplyProcess(t *testing.T) {
	if os.Args[len(os.Args)-1] == "CR_REPLY" {
		fmt.Println(`{"ok":true,"data":{"openMessageId":"reply","deliveryStatus":"delivered"}}`)
		os.Exit(0)
	}
}

func TestCrossPlatformCoverageEmployeeCRQoderAttachmentFailure(t *testing.T) {
	testseam.Swap(t, &connectStreamCommandContext, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCRStreamProcess$", "--", "nonzero")
		cmd.Env = append(os.Environ(), "DWS_EMPLOYEE_CR_PROCESS=1")
		return cmd
	})
	sessions := newConvSessions("")
	old := sessions.id("conversation")
	f := &qoderStreamForwarder{name: "qoder", bin: os.Args[0], sessions: sessions, timeout: 5 * time.Second, env: []string{"DWS_EMPLOYEE_CR_PROCESS=1"}}
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(agentProgress) {})
	if _, err := f.forwardWithAttachments(ctx, "conversation", "fixture", []connectMediaAttachment{{MediaType: "image", LocalPath: filepath.Join(t.TempDir(), "image.png")}}); err == nil {
		t.Fatal("附件执行失败不能显示完成")
	}
	if sessions.id("conversation") == old {
		t.Fatal("附件临时 forwarder 失败后仍复用坏会话")
	}
}

func TestCrossPlatformCoverageEmployeeCREmptySessionRecovery(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &connectStreamCommandContext, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				calls++
				mode := "CR_RETRY_MISSING"
				if calls == 2 && success {
					mode = "CR_RETRY_OK"
				}
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCRRetryProcess$", "--", mode)
			})
			sessions := newConvSessions("")
			old := sessions.id("conversation")
			f := &execForwarder{name: "claudecode", argv: []string{"fixture"}, streamArgv: []string{"fixture"}, parser: "cc", sessions: sessions, workDir: t.TempDir(), timeout: 10 * time.Second}
			answer, err := f.forwardStream(context.Background(), "conversation", "fixture", func(string) {})
			if calls != 2 || sessions.id("conversation") == old {
				t.Fatalf("无正文的确定会话缺失只允许恢复一次: calls=%d", calls)
			}
			if success && (err != nil || answer != "CR_RETRY_OK") {
				t.Fatalf("会话恢复失败: %q %v", answer, err)
			}
			if !success && err == nil {
				t.Fatal("第二次失败不得报告成功")
			}
		})
	}
}
func TestEmployeeCRRetryProcess(t *testing.T) {
	mode := os.Args[len(os.Args)-1]
	if mode == "CR_RETRY_MISSING" {
		fmt.Fprintln(os.Stderr, "session not found: stale")
		os.Exit(1)
	}
	if mode == "CR_RETRY_OK" {
		fmt.Println(`{"type":"result","result":"CR_RETRY_OK"}`)
		os.Exit(0)
	}
}
