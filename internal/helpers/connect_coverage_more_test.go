package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type bufferWriteCloser struct{ bytes.Buffer }

func (*bufferWriteCloser) Close() error { return nil }

type connectResponseRunner struct {
	response map[string]any
	err      error
}

func (r connectResponseRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	return executor.Result{Invocation: invocation, Response: r.response}, r.err
}

func TestCrossPlatformCoverageConnectForwarderPureCoverage(t *testing.T) {
	streamSDKLogger{}.Debugf("debug %s", "x")
	streamSDKLogger{}.Infof("info %s", "x")
	streamSDKLogger{}.Warningf("warn %s", "x")
	streamSDKLogger{}.Errorf("error %s", "x")
	streamSDKLogger{}.Fatalf("fatal %s", "x")
	_ = agentNotInstalled("channel", "app", "install")

	sessions := newConvSessions("")
	execFwd := &execForwarder{name: "agent", argv: []string{"bin"}, sessions: sessions}
	_ = execFwd.label()
	_ = execFwd.hasSession("conversation")
	execFwd.resetSession("")
	execFwd.resetSession("conversation")
	statelessExec := &execForwarder{name: "agent", argv: []string{"bin"}}
	_ = statelessExec.label()
	statelessExec.resetSession("conversation")

	qoder := newQoderStreamForwarder("qoder", "bin", nil, time.Second, connectAgentOptions{}, sessions).(*qoderStreamForwarder)
	if !qoder.canStream() || qoder.label() == "" {
		t.Fatal("qoder capabilities")
	}
	qoder.resetSession("conversation")
	qoderStateless := newQoderStreamForwarder("qoder", "bin", nil, time.Second, connectAgentOptions{}, nil).(*qoderStreamForwarder)
	_ = qoderStateless.label()
	qoderStateless.resetSession("conversation")
	_, _ = qoderStateless.forward(context.Background(), "conversation", "hello")
	var locked lockedStringBuffer
	_, _ = locked.Write([]byte("stderr"))
	if locked.String() != "stderr" {
		t.Fatal("locked string buffer")
	}

	codex := newCodexAppServerForwarder("bin", nil, time.Second, connectAgentOptions{Memory: true, Model: "model", Yolo: true}, "").(*codexAppServerForwarder)
	if !codex.canStream() || codex.label() == "" {
		t.Fatal("codex capabilities")
	}
	codex.resetSession("conversation")
	_ = codex.threadParams("")
	_ = codex.threadParams("thread")
	codexStateless := newCodexAppServerForwarder("bin", nil, time.Second, connectAgentOptions{}, "").(*codexAppServerForwarder)
	_ = codexStateless.label()
	codexStateless.resetSession("conversation")
	var codexBuffer lockedBuffer
	_, _ = codexBuffer.Write([]byte("stderr"))
	_ = codexBuffer.String()
	stdin := &bufferWriteCloser{}
	client := &codexAppServerClient{stdin: stdin, stderr: &lockedBuffer{}}
	client.rejectServerRequest(1, "request/input")
	_ = client.withStderr("prefix", errors.New("failure"))
	_, _ = client.stderr.Write([]byte("detail"))
	_ = client.withStderr("prefix", errors.New("failure"))

	opencode := newOpencodeForwarder("bin", nil, time.Second, connectAgentOptions{Memory: true}, "").(*opencodeForwarder)
	if opencode.canStream() || opencode.label() == "" {
		t.Fatal("opencode capabilities")
	}
	opencode.resetSession("conversation")
	if err := opencode.close(); err != nil {
		t.Fatal(err)
	}
	opencodeStateless := newOpencodeForwarder("bin", nil, time.Second, connectAgentOptions{}, "").(*opencodeForwarder)
	_ = opencodeStateless.label()
	opencodeStateless.resetSession("conversation")
	opencodeStateless.server = nil
	if err := opencodeStateless.close(); err != nil {
		t.Fatal(err)
	}
	opencodeBroken := newOpencodeForwarder(filepath.Join(t.TempDir(), "missing"), nil, time.Second, connectAgentOptions{}, "").(*opencodeForwarder)
	_, _ = opencodeBroken.forward(context.Background(), "conversation", "hello")
	if port, err := freeLocalPort(); err != nil || port == 0 {
		t.Fatalf("free port = %d, %v", port, err)
	}
	if randomHex(4) == "" {
		t.Fatal("random hex is empty")
	}

	cmd := exec.Command("sh", "-c", "exit 0")
	applyDetach(cmd)
	configureWorkerProcessGroup(cmd)
	cleanupWorkerProcessGroup(0)
	_ = runAgentInstall("channel", agentSpec{app: "app", install: []string{"sh", "-c", "exit 0"}})
	_ = runAgentInstall("channel", agentSpec{app: "app", install: []string{"sh", "-c", "exit 1"}})
	_ = officialChannelGuidance("openclaw")
	_ = officialChannelGuidance("hermes")
	_ = officialChannelGuidance("unknown")
}

func TestCrossPlatformCoverageGeminiAPIForwarderCoverage(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	if _, err := newGeminiAPIForwarder(0, connectAgentOptions{}); err == nil {
		t.Fatal("missing Gemini key succeeded")
	}
	t.Setenv("GOOGLE_API_KEY", " key ")
	t.Setenv("GOOGLE_GEMINI_API_BASE_URL", "https://proxy.example/")
	fwd, err := newGeminiAPIForwarder(0, connectAgentOptions{})
	if err != nil || fwd.label() == "" {
		t.Fatalf("Gemini forwarder = %#v, %v", fwd, err)
	}

	responses := []struct {
		status int
		body   string
	}{
		{http.StatusInternalServerError, `failure`},
		{http.StatusOK, `{`},
		{http.StatusOK, `{"error":{"status":"BAD","message":"failed"}}`},
		{http.StatusOK, `{"promptFeedback":{"blockReason":"SAFETY"}}`},
		{http.StatusOK, `{}`},
		{http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":" one "},{"text":"two"}]}}]}`},
	}
	for _, response := range responses {
		response := response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(response.status)
			_, _ = io.WriteString(w, response.body)
		}))
		g := &geminiAPIForwarder{model: "models/test/model", apiKey: "key", baseURL: server.URL, timeout: time.Second, httpClient: server.Client()}
		_, _ = g.forward(context.Background(), "", "hello")
		server.Close()
	}
	for _, g := range []*geminiAPIForwarder{{baseURL: "%", model: "m"}, {baseURL: "", model: ""}} {
		_, _ = g.generateContentEndpoint()
	}
}

func TestCrossPlatformCoverageConnectDaemonAndApprovalPureCoverage(t *testing.T) {
	oldOverride := connectDaemonDirOverride
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = oldOverride })

	cmd := &cobra.Command{Use: "status"}
	cmd.Flags().String("robot-client-id", "", "")
	cmd.Flags().String("unified-app-id", "", "")
	if _, err := connectDaemonDirKeyFromFlags(cmd); err == nil {
		t.Fatal("empty daemon identity succeeded")
	}
	_ = cmd.Flags().Set("robot-client-id", "client")
	if key, err := connectDaemonDirKeyFromFlags(cmd); err != nil || key == "" {
		t.Fatalf("daemon key = %q, %v", key, err)
	}
	_ = colorConnectState(healthHealthy)
	_ = colorConnectState(healthDegraded)
	_ = colorConnectState(healthDown)
	_ = colorConnectState("unknown")
	var table bytes.Buffer
	if err := writeConnectListTable(&table, []connectHealthReport{{State: healthHealthy, AppName: strings.Repeat("长", 60), ClientID: "client", Pid: 2, Channel: "codex", UptimeSec: 3}, {State: healthNotRunning}}); err != nil {
		t.Fatal(err)
	}
	daemonNotifyStateChange("", "channel", "client", "started", "")
	daemonNotifyStateChange("staff", "channel", "client", "unknown", "")

	worker := exec.Command("sh", "-c", "exit 0")
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	if err := superviseWait(context.Background(), worker); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	worker = exec.Command("sh", "-c", "sleep 10")
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = superviseWait(cancelled, worker)

	runner := connectResponseRunner{response: map[string]any{"items": []any{map[string]any{"unifiedAppId": "app", "name": "Name"}, "bad"}, "hasMore": false}}
	reports := []connectHealthReport{{UnifiedAppID: "app"}, {ClientID: "client"}}
	resolveAppNames(cmd, runner, reports)
	resolveAppNames(cmd, runner, []connectHealthReport{{ClientID: "client"}})
	_, _ = devAppNameMap(cmd, connectResponseRunner{err: errors.New("failed")})
	_ = devAppConnectList(nil)
	_ = devAppConnectList(map[string]any{"items": "bad"})

	gate := newApprovalGate("")
	req := gate.Submit(ApprovalRequest{Requester: "user", State: approvalApproved, Summary: "summary"})
	gate.markFailed(req.ID, strings.Repeat("failure", 100))
	gate.markFailed("missing", "failure")
	sink := &sheetAuditSink{}
	sink.record(context.Background(), req)
	sink = &sheetAuditSink{runner: runner, nodeID: "node", sheetID: "sheet"}
	sink.record(context.Background(), req)
	orchestrator := &approvalOrchestrator{}
	_ = orchestrator.decorateForActionDetection("prompt")
	orchestrator = &approvalOrchestrator{ownerUserID: "owner", gate: gate, textMode: true}
	_ = orchestrator.decorateForActionDetection("prompt")
}

func TestCrossPlatformCoverageAtPollTextAndCancellationCoverage(t *testing.T) {
	for _, tc := range []struct{ content, kind string }{
		{"", "text"},
		{`{"text":" hello "}`, "text"},
		{`{"content":" body "}`, "rich"},
		{`{"other":1}`, "rich"},
		{"plain", "text"},
	} {
		_ = extractAtPollText(tc.content, tc.kind)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	poller := &atMentionPoller{}
	stopped := poller.start(ctx)
	poller.poll(ctx)
	poller.handleMessage(ctx, atMentionMessage{})
	waitAtPollStop(t, stopped)

	var payload map[string]any
	_ = json.Unmarshal([]byte(`{"ok":true}`), &payload)
}

func TestCrossPlatformCoverageConnectStreamRemainingPureEdges(t *testing.T) {
	if convSessionKey("  ") != "_default" || convSessionKey(" conv ") != "conv" {
		t.Fatal("conversation key normalization failed")
	}
	if execErrorMessage(nil) != "" {
		t.Fatal("nil execution error should be empty")
	}
	cmd := exec.Command("sh", "-c", "printf detail >&2; exit 7")
	if _, err := cmd.Output(); err != nil {
		if got := execErrorMessage(err); got != "detail" {
			t.Fatalf("exit stderr = %q, want detail", got)
		}
	} else {
		t.Fatal("failing command unexpectedly succeeded")
	}

	workDir := t.TempDir()
	fwd := &execForwarder{workDir: workDir, env: []string{"DWS_TEST_CONNECT_ENV=1"}}
	execCmd := exec.Command("sh")
	fwd.configureCommand(execCmd)
	if execCmd.Dir != workDir || !containsString(execCmd.Env, "DWS_TEST_CONNECT_ENV=1") {
		t.Fatalf("configured command = dir %q env %#v", execCmd.Dir, execCmd.Env)
	}

	if got := envOr("DWS_TEST_ENV_OR_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("envOr missing = %q", got)
	}
	t.Setenv("DWS_TEST_ENV_OR", " value ")
	if got := envOr("DWS_TEST_ENV_OR", "fallback"); got != "value" {
		t.Fatalf("envOr value = %q", got)
	}
	ctx := context.Background()
	plain, cancel := applyTimeout(ctx, 0)
	cancel()
	if plain != ctx {
		t.Fatal("zero timeout should keep original context")
	}
	bounded, cancel := applyTimeout(ctx, time.Millisecond)
	cancel()
	if bounded == ctx {
		t.Fatal("positive timeout should derive a context")
	}
	if truncateRunes("中文abc", 2) != "中文…" || truncateRunes("短", 2) != "短" {
		t.Fatal("rune truncation mismatch")
	}

	turns := []connectQueuedTurn{
		{},
		{picCodes: []string{"picture-code"}},
		{fileInfos: []fileInboundInfo{{FileName: " named.txt ", DownloadCode: "download"}}},
		{fileInfos: []fileInboundInfo{{DownloadCode: "download"}}},
	}
	wants := []string{"[空消息]", "[图片]", "[附件: named.txt]", "[1 个附件]"}
	for i, turn := range turns {
		if got := connectTurnSummary(turn); got != wants[i] {
			t.Errorf("summary %d = %q, want %q", i, got, wants[i])
		}
	}
	if got := mergeConnectQueuedTurns(nil); got.text != "" || got.msgID != "" {
		t.Fatalf("empty merge = %#v", got)
	}
	one := connectQueuedTurn{text: "one", msgID: "m1"}
	if got := mergeConnectQueuedTurns([]connectQueuedTurn{one}); got.text != one.text || got.msgID != one.msgID {
		t.Fatalf("single merge = %#v", got)
	}
	mergedPicture := mergeConnectQueuedTurns([]connectQueuedTurn{
		{picCodes: []string{"old-picture"}, msgID: "m1"},
		{text: "latest", msgID: "m2"},
	})
	if len(mergedPicture.picCodes) != 1 || mergedPicture.picCodes[0] != "old-picture" {
		t.Fatalf("merged picture = %#v", mergedPicture)
	}
	mergedFile := mergeConnectQueuedTurns([]connectQueuedTurn{
		{fileInfos: []fileInboundInfo{{FileName: "old", DownloadCode: "code"}}, msgID: "m1"},
		{text: "latest", msgID: "m2"},
	})
	if len(mergedFile.fileInfos) != 1 || !mergedFile.fileInfos[0].hasActionable() {
		t.Fatalf("merged file = %#v", mergedFile)
	}
}

func TestCrossPlatformCoverageConnectCLIResolutionEdges(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	for _, channel := range []string{"openclaw", "hermes", "custom", "unknown"} {
		_ = connectCliStatus(channel)
	}
	t.Setenv("DWS_AGENT_CMD", "custom-agent --flag")
	custom := connectCliStatus("custom")
	if custom["installed"] != true || custom["command"] != "custom-agent --flag" {
		t.Fatalf("custom status = %#v", custom)
	}

	binDir := t.TempDir()
	writeShellExecutable(t, binDir, "edge-agent", "exit 0\n")
	t.Setenv("PATH", binDir)
	original, hadOriginal := agentSpecs["edge"]
	agentSpecs["edge"] = agentSpec{
		app:      "Edge Agent",
		bins:     []string{"edge-agent"},
		argvTail: []string{"--headless"},
		envFn:    func() []string { return []string{"EDGE_ENV=1"} },
		hint:     "install edge",
	}
	t.Cleanup(func() {
		if hadOriginal {
			agentSpecs["edge"] = original
		} else {
			delete(agentSpecs, "edge")
		}
	})
	t.Setenv("DWS_AGENT_CMD", "")
	argv, env, err := resolveExecAgent("edge")
	if err != nil || len(argv) != 2 || argv[1] != "--headless" || !reflect.DeepEqual(env, []string{"EDGE_ENV=1"}) {
		t.Fatalf("resolved edge agent = %#v %#v %v", argv, env, err)
	}
	if _, _, err := resolveExecAgent("missing-channel"); err == nil {
		t.Fatal("unknown exec channel should fail")
	}
	status := connectCliStatus("edge")
	if status["installed"] != true || status["path"] == "" {
		t.Fatalf("edge status = %#v", status)
	}

	t.Setenv("HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_ = homeDir()
	_ = claudeUserSettingsEnv()
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
