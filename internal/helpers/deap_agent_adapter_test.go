package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func installEmployeeReplyBinding(t *testing.T) {
	t.Helper()
	previous := auth.RuntimeProfile()
	auth.SetRuntimeProfile("employee-corp:employee-user")
	t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
	testseam.Swap(t, &deapChannelLoadBinding, func(_, profile string) (digitalEmployeeBinding, error) {
		return digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: profile, OperatorOpenDingTalkID: "operator-open"}, nil
	})
}

func TestCrossPlatformCoverageEmployeeAdapterDetectionAndBoundaries(t *testing.T) {
	for _, name := range digitalEmployeeChannels() {
		t.Run(name, func(t *testing.T) {
			cmd := newDeapConnectCommand()
			_ = cmd.Flags().Set("channel", name)
			if name == "custom" {
				_ = cmd.Flags().Set("agent-cmd", "echo")
			}
			if err := validateDigitalEmployeeAdapter(cmd); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, channel := range []string{"dsh", "hermes", "openclaw"} {
		cmd := newDeapConnectCommand()
		_ = cmd.Flags().Set("channel", channel)
		_ = cmd.Flags().Set("daemon", "true")
		if err := validateDigitalEmployeeAdapter(cmd); err == nil {
			t.Fatalf("accepted %s daemon", channel)
		}
	}
	cmd := newDeapConnectCommand()
	_ = cmd.Flags().Set("agent-cmd", "echo")
	if name, err := resolveDigitalEmployeeChannel(cmd); err != nil || name != "custom" {
		t.Fatalf("%q %v", name, err)
	}
	_ = cmd.Flags().Set("alwayson", "true")
	if err := validateDigitalEmployeeAdapter(cmd); err == nil {
		t.Fatal("alwayson without daemon accepted")
	}
}

func TestCrossPlatformCoverageEmployeeSavedWorkerDoesNotRequireOriginalCustomCommand(t *testing.T) {
	cmd := newDeapConnectCommand()
	_ = cmd.Flags().Set("channel", "custom")
	_ = cmd.Flags().Set("local-worker", "true")
	if err := validateDigitalEmployeeAdapter(cmd); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeRuntimeScopeAndBindingCompatibility(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: "corp:employee", OperatorOpenDingTalkID: "operator"}
	if err := saveDigitalEmployeeBinding(dir, b); err != nil {
		t.Fatal(err)
	}
	if err := checkDigitalEmployeeBinding(dir, b.DWSProfile, b.AgentUUID, "dsh"); err != nil {
		t.Fatal(err)
	}
	if err := checkDigitalEmployeeBinding(dir, b.DWSProfile, b.AgentUUID, "codex"); err == nil {
		t.Fatal("overwrote legacy DSH binding")
	}
	if digitalEmployeeScope("corp:a") == digitalEmployeeScope("corp:b") {
		t.Fatal("employee collision")
	}
	if strings.Contains(digitalEmployeeScope("corp:a"), ":") {
		t.Fatal("unsafe scope")
	}
}

func TestCrossPlatformCoverageEmployeeCommonAgentOptionsMatchDevConnect(t *testing.T) {
	cmd := newDeapConnectCommand()
	for k, v := range map[string]string{"channel": "codex", "agent-model": "model-test", "agent-workdir": t.TempDir(), "agent-memory": "false", "agent-timeout": "9", "agent-permission-mode": "ask"} {
		_ = cmd.Flags().Set(k, v)
	}
	opts, err := digitalEmployeeOptions(cmd)
	if err != nil {
		t.Fatal(err)
	}
	want, err := connectAgentOptionsFromCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != want.Model || opts.WorkDir != want.WorkDir || opts.Memory != want.Memory || opts.Timeout != want.Timeout || opts.Yolo != want.Yolo {
		t.Fatalf("options drift: %#v %#v", opts, want)
	}
}

func TestCrossPlatformCoverageEmployeeRetryBudgetPersistsAcrossRestarts(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name    string
		hint    *bool
		retries int
	}{{"terminal", &no, 0}, {"retryable", &yes, 2}, {"unknown", nil, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			s := employeeRetryState{}
			count := 0
			for i := 0; i < 5; i++ {
				next, allowed := employeeNextRetry(s, &employeeRunError{Code: "test", Retryable: tc.hint, RetryAfter: 30 * time.Second}, time.Now())
				raw, _ := json.Marshal(next)
				_ = json.Unmarshal(raw, &s)
				if !allowed {
					break
				}
				count++
				if time.Until(s.NextAttempt) < 29*time.Second {
					t.Fatal("ignored cooldown")
				}
			}
			if count != tc.retries {
				t.Fatalf("retries %d", count)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeRuntimeOwnerAndGroupACL(t *testing.T) {
	r := employeeRuntime{cfg: digitalEmployeeAdapterConfig{Options: connectAgentOptions{AllowedUsers: []string{"owner"}, AllowedGroups: []string{"allowed-group"}}}}
	e := employeeEvent{Type: "user_im_message_receive_o2o_all", EventID: "e", MessageID: "m", ConversationID: "dm", SenderID: "owner", Content: "hello"}
	if !r.accept(e) {
		t.Fatal("owner denied")
	}
	e.SenderID = "guest"
	if r.accept(e) {
		t.Fatal("guest allowed")
	}
	e.SenderID = "owner"
	e.Type = "user_im_message_receive_group_all"
	if r.accept(e) {
		t.Fatal("unlisted group allowed")
	}
	e.ConversationID = "allowed-group"
	if !r.accept(e) {
		t.Fatal("allowed group denied")
	}
	e.SenderID = "employee"
	r.cfg.SelfOpenDingTalkID = "employee"
	r.cfg.Options.AllowedUsers = append(r.cfg.Options.AllowedUsers, "employee")
	if r.accept(e) {
		t.Fatal("self message allowed")
	}
}

func TestCrossPlatformCoverageEmployeePrivateDiagnosticsReachOpenCode(t *testing.T) {
	fwd := newOpencodeForwarder("opencode", nil, time.Second, connectAgentOptions{WorkDir: t.TempDir(), PrivateDiagnostics: true}, "isolated")
	if !fwd.(*opencodeForwarder).server.privateDiagnostics {
		t.Fatal("OpenCode diagnostics bypass private boundary")
	}
	dev := newOpencodeForwarder("opencode", nil, time.Second, connectAgentOptions{WorkDir: t.TempDir()}, "robot")
	if dev.(*opencodeForwarder).server.privateDiagnostics {
		t.Fatal("changed dev connect diagnostics")
	}
}

func TestCrossPlatformCoverageEmployeeStopWaitsForSupervisorExit(t *testing.T) {
	state := digitalEmployeeRunState{Status: "stopped"}
	if employeeStopComplete(state, os.Getpid()) {
		t.Fatal("reported stop before supervisor exited")
	}
	if !employeeStopComplete(state, 0) {
		t.Fatal("foreground stopped was not complete")
	}
}

func TestCrossPlatformCoverageEmployeeConnectRegistrationLockPrecedesAuthorizationAndBinding(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), "")
	InitDepsForTest(t, caller)
	setupConnectSupervisorSeams(t)
	dir := deapConnectConfigDir()
	lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(digitalEmployeeRuntimeDir("employee-corp:employee-user"), "operation"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cmd := newConnectTestCommand(t, false)
	cmd.SetContext(ctx)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("concurrent registration bypassed lock")
	}
	for _, call := range caller.calls {
		if call.toolName == deapAgentAuthCodeTool {
			t.Fatal("obtained authorization before registration lock")
		}
	}
	if _, err := os.Stat(digitalEmployeeBindingPath(dir, "employee-corp:employee-user")); !os.IsNotExist(err) {
		t.Fatalf("binding changed: %v", err)
	}
}

func TestCrossPlatformCoverageEmployeeMachineCommandProfileAndPrivateInput(t *testing.T) {
	t.Setenv("DWS_CLIENT_SECRET", "must-not-leak")
	t.Setenv("DWS_DUMP_RAW", "1")
	cmd, err := employeeCommand(context.Background(), "corp:employee", "dingtalk-tag", "channel", "reply", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cmd.Args[1:3], []string{"--profile", "corp:employee"}) {
		t.Fatal(cmd.Args)
	}
	for _, v := range cmd.Env {
		if strings.Contains(v, "must-not-leak") || v == "DWS_DUMP_RAW=1" {
			t.Fatal("credential/raw dump leaked")
		}
	}
	var buffer employeeBoundedBuffer
	_, _ = buffer.Write(bytes.Repeat([]byte("x"), digitalEmployeeStdinLimit+1))
	if !buffer.overflow || buffer.Len() != digitalEmployeeStdinLimit {
		t.Fatal("unbounded subprocess output")
	}
}

type employeeTestForwarder struct{ calls int }

func (f *employeeTestForwarder) label() string { return "test" }
func (f *employeeTestForwarder) forward(context.Context, string, string) (string, error) {
	f.calls++
	return "answer-not-in-audit", nil
}

func TestCrossPlatformCoverageEmployeeRuntimeRealNDJSONAndReceiptEnvelope(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	t.Setenv("DWS_EMPLOYEE_FIXTURE", "reply")
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if !strings.Contains(strings.Join(args, " "), "channel reply") {
			t.Fatalf("unexpected command %v", args)
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeSubprocessFixture$")
	})
	profile := "corp:employee"
	runtimeDir := digitalEmployeeRuntimeDir(profile)
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	fwd := &employeeTestForwarder{}
	r := employeeRuntime{cfg: digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: profile, Channel: "custom"}}, dir: runtimeDir, fwd: fwd, ctx: context.Background()}
	e := employeeEvent{EventID: "event", MessageID: "message", ConversationID: "conversation", Content: "private-question"}
	record := employeeTaskRecord{IdempotencyKey: "idempotent"}
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		t.Fatal(err)
	}
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.recordPath(e))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"status":"delivered"`)) {
		t.Fatalf("receipt: %s", data)
	}
	audit, _ := os.ReadFile(filepath.Join(runtimeDir, "audit.jsonl"))
	if bytes.Contains(audit, []byte("private-question")) || bytes.Contains(audit, []byte("answer-not-in-audit")) {
		t.Fatal("audit leaked body")
	}
	if fwd.calls != 1 {
		t.Fatal("wrong turn count")
	}
}

func TestEmployeeSubprocessFixture(t *testing.T) {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "employee-consume-fixture" {
		switch os.Getenv("DWS_EMPLOYEE_EVENT_FIXTURE") {
		case "transport-lifecycle":
			fmt.Fprintln(os.Stderr, "[event] ready event_count=1 bus_pid=123")
			fmt.Fprintln(os.Stderr, `[event] transport {"state":"connecting","observed":true}`)
			for _, phase := range []struct{ trigger, state string }{{"connect", "connected"}, {"disconnect", "reconnecting"}, {"recover", "idle"}} {
				for {
					data, _ := os.ReadFile(os.Getenv("DWS_EMPLOYEE_PHASE_FILE"))
					if string(data) == phase.trigger {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				fmt.Fprintf(os.Stderr, "[event] transport {\"state\":%q,\"observed\":true}\n", phase.state)
			}
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		case "ipc-only", "legacy-ready":
			fmt.Fprintln(os.Stderr, "[event] ready event_count=1 bus_pid=123")
			if os.Getenv("DWS_EMPLOYEE_EVENT_FIXTURE") == "legacy-ready" {
				fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"inferred"}`)
			}
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		case "no-ready":
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		case "invalid":
			fmt.Fprintln(os.Stderr, "[event] ready event_count=1 bus_pid=123")
			fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"inferred","observed":true}`)
			fmt.Println("{invalid}")
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "[event] ready event_count=1 bus_pid=123")
		fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"inferred","observed":true}`)
		line := `{"type":"user_im_message_receive_o2o_all","event_id":"event","message_id":"message","conversation_id":"conversation","sender_open_dingtalk_id":"owner","content":"private-question"}`
		fmt.Println(line)
		fmt.Println(line)
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if os.Getenv("DWS_EMPLOYEE_FIXTURE") != "reply" {
		return
	}
	var input digitalEmployeeReplyInput
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || input.Text != "answer-not-in-audit" {
		os.Exit(2)
	}
	fmt.Print(`{"ok":true,"data":{"openMessageId":"reply","deliveryStatus":"delivered"}}`)
	os.Exit(0)
}

func TestCrossPlatformCoverageEmployeeConsumerFaultsFailClosed(t *testing.T) {
	for _, tc := range []struct{ mode, code string }{{"no-ready", "ready_timeout"}, {"invalid", "invalid_event"}, {"audit", "audit_unavailable"}} {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
			t.Setenv("DWS_EMPLOYEE_EVENT_FIXTURE", tc.mode)
			// race 二进制冷启动可能超过 300ms；只有超时用例缩短 ready 窗口。
			readyTimeout := 3 * time.Second
			if tc.mode == "no-ready" {
				readyTimeout = 300 * time.Millisecond
			}
			testseam.Swap(t, &digitalEmployeeReadyTimeout, readyTimeout)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeSubprocessFixture$", "--", "employee-consume-fixture")
			})
			fwd := &employeeTestForwarder{}
			testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) { return fwd, nil })
			cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{SchemaVersion: 1, OperatorOpenDingTalkID: "owner", AgentUUID: "agent", DWSProfile: "corp:employee", Channel: "custom"}}
			if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
				t.Fatal(err)
			}
			if tc.mode == "audit" {
				if err := os.MkdirAll(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "audit.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			err := runEmployeeWorker(ctx, cfg, 0)
			if err == nil || err.Error() != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
			if fwd.calls != 0 {
				t.Fatal("fault started Agent turn")
			}
			state, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile))
			if err != nil || state.Status != "blocked" || state.Code != tc.code {
				t.Fatalf("state=%+v err=%v", state, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeRetryHintTerminalDominatesAndKeepsLongestDelay(t *testing.T) {
	for i := 0; i < 30; i++ {
		err := &employeeRunError{}
		employeeReadRetryHint(`{"retryable":true,"state":"terminal_hold","retry_after_seconds":30,"child":{"retryable":true,"retry_after_seconds":1}}`, err)
		if err.Retryable == nil || *err.Retryable || err.RetryAfter != 30*time.Second {
			t.Fatalf("hint=%+v", err)
		}
	}
}

func TestCrossPlatformCoverageEmployeeSupervisorCanStopDuringCooldown(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{AgentUUID: "agent", DWSProfile: "corp:employee", Channel: "custom"}, AlwaysOn: true}
	runtimeDir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	if err := writeEmployeeJSON(filepath.Join(runtimeDir, "retry.json"), employeeRetryState{NextAttempt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- superviseDigitalEmployee(ctx, cfg) }()
	for {
		state, err := readDigitalEmployeeState(runtimeDir)
		if err == nil && state.Status == "retry_wait" {
			if err := writeEmployeeJSON(filepath.Join(runtimeDir, "stop.json"), map[string]string{"runId": state.RunID}); err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("exited early: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cooldown ignored stop")
	}
	state, err := readDigitalEmployeeState(runtimeDir)
	if err != nil || state.Status != "stopped" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

func TestCrossPlatformCoverageEmployeeEventConsumerReadyDedupeAndGracefulStop(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	t.Setenv("DWS_EMPLOYEE_FIXTURE", "reply")
	consumerArgs := make(chan []string, 1)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		argv := []string{"-test.run=^TestEmployeeSubprocessFixture$"}
		if strings.Contains(strings.Join(args, " "), "event consume") {
			consumerArgs <- append([]string(nil), args...)
			argv = append(argv, "--", "employee-consume-fixture")
		}
		return exec.CommandContext(ctx, os.Args[0], argv...)
	})
	fwd := &employeeTestForwarder{}
	testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) { return fwd, nil })
	cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:employee", Channel: "custom", OperatorOpenDingTalkID: "owner"}, Options: connectAgentOptions{AllowedUsers: []string{"owner"}}}
	if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runEmployeeWorker(ctx, cfg, 0) }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	var gotConsumerArgs []string
	for {
		select {
		case err := <-done:
			t.Fatalf("worker exited before delivery: %v", err)
		case <-timer.C:
			t.Fatal("consumer did not deliver")
		case <-ticker.C:
			if gotConsumerArgs == nil {
				select {
				case gotConsumerArgs = <-consumerArgs:
				default:
				}
			}
			s, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile))
			if err == nil && s.Status == "running" {
				ready = true
			}
			entries, _ := filepath.Glob(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "tasks", "*.json"))
			if len(entries) == 1 {
				data, _ := os.ReadFile(entries[0])
				if bytes.Contains(data, []byte(`"status":"delivered"`)) {
					cancel()
					select {
					case err := <-done:
						if err != nil {
							t.Fatal(err)
						}
					case <-time.After(3 * time.Second):
						t.Fatal("stdin EOF did not stop consumer")
					}
					if !ready || fwd.calls != 1 {
						t.Fatalf("ready=%v calls=%d", ready, fwd.calls)
					}
					if len(gotConsumerArgs) == 0 || slices.Contains(gotConsumerArgs, "--stream-source-id") {
						t.Fatalf("event consumer args = %v", gotConsumerArgs)
					}
					return
				}
			}
		}
	}
}
