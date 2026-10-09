// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCrossPlatformCoveragePiJSONOutput(t *testing.T) {
	var updates []string
	s := &piJSONOutput{onDelta: func(v string) { updates = append(updates, v) }}
	wire := `{"type":"session"}
{"type":"message_end","message":{"role":"user","content":"用户输入"}}
{"type":"message_start","message":{"role":"assistant"}}
{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"private"}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"你好"}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"\u2028世界"}}
{"type":"message_end","message":{"role":"assistant","stopReason":"stop","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"最终答案"}]}}
{"type":"agent_end"}`
	// 逐字节写入，覆盖 UTF-8、JSON 分帧及末行无 LF。
	for _, b := range []byte(wire) {
		if _, err := s.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.finish(); err != nil {
		t.Fatal(err)
	}
	if s.final != "最终答案" || strings.Join(updates, "|") != "你好|你好\u2028世界" {
		t.Fatalf("final=%q updates=%q", s.final, updates)
	}
	for _, wire := range []string{
		"{broken}\n",
		`{"type":"message_end","message":{"role":"assistant","content":{}}}`,
		`{"type":"message_end","message":{"role":"assistant","stopReason":"error","content":[{"type":"text","text":"partial"}]}}` + "\n" + `{"type":"agent_end"}`,
		`{"type":"agent_end"}` + "\n" + `{"type":"agent_settled","aborted":true}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"partial"}}`,
	} {
		s := &piJSONOutput{}
		_, err := s.Write([]byte(wire))
		if err == nil {
			err = s.finish()
		}
		if err == nil {
			t.Fatalf("accepted incomplete/invalid output: %s", wire)
		}
	}
	if _, err := (&piJSONOutput{}).Write([]byte("\n\r\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := (&piJSONOutput{}).Write(bytes.Repeat([]byte("x"), 16*1024*1024+1)); err == nil {
		t.Fatal("oversize event accepted")
	}
}

func TestCrossPlatformCoveragePiChannelWiring(t *testing.T) {
	t.Setenv("DWS_AGENT_CMD", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	if err := writeExecStub(dir, "pi"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	fwd, err := newLocalAgentForwarder("pi", "robot", connectAgentOptions{Memory: true, Model: "provider/model"})
	if err != nil {
		t.Fatal(err)
	}
	f := fwd.(*piForwarder)
	if !f.canStream() || !strings.Contains(f.label(), "pi-json") {
		t.Fatal("Pi streaming capability missing")
	}
	args, file, err := f.commandArgs("conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--model provider/model") || !strings.Contains(strings.Join(args, " "), "--no-tools") {
		t.Fatalf("args=%v", args)
	}
	_, same, _ := f.commandArgs("conversation")
	_, other, _ := f.commandArgs("other")
	if file != same || file == other {
		t.Fatal("conversation isolation failed")
	}
	f.resetSession("conversation")
	_, reset, _ := f.commandArgs("conversation")
	if reset == file {
		t.Fatal("/new retained session")
	}
	if err := f.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.forward(context.Background(), "conversation", "hello"); err == nil {
		t.Fatal("closed forwarder accepted turn")
	}
	if got := connectAgentOptionsPayload("pi", connectAgentOptions{Memory: true})["memory"]; got != "per-conversation-pi-session" {
		t.Fatalf("memory=%v", got)
	}
	if got := connectAgentOptionsPayload("pi", connectAgentOptions{})["memory"]; got != "disabled" {
		t.Fatalf("memory=%v", got)
	}
	if buildConnectPlan("pi", "", "")["method"] != "stream-bridge-pi-json" {
		t.Fatal("Pi dry-run plan missing")
	}
	// 自定义命令仍按现有约定禁用协议和会话注入。
	custom, err := newLocalAgentForwarder("pi", "", connectAgentOptions{Command: "echo custom"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := custom.(*execForwarder); !ok {
		t.Fatalf("override=%T", custom)
	}
	if f := newPiForwarder("pi", nil, 0, connectAgentOptions{Memory: true}, "").(*piForwarder); f.sessions.path != "" {
		t.Fatal("empty scope must not persist mapping")
	}
}

func TestCrossPlatformCoveragePiProcessLifecycle(t *testing.T) {
	requirePOSIXShell(t)
	root := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", root)
	script := `import sys, json, time, os
args=sys.argv[1:]
text=sys.stdin.read()
if text == 'EXIT': sys.exit(1)
if text == 'BROKEN':
    print('{broken}', flush=True)
    sys.exit(0)
print(json.dumps({'type':'agent_start'}), flush=True)
print(json.dumps({'type':'message_start','message':{'role':'assistant'}}), flush=True)
print(json.dumps({'type':'message_update','assistantMessageEvent':{'type':'text_delta','delta':'开始'}}), flush=True)
if text == 'SLEEP': time.sleep(60)
count=1
if '--session' in args:
    path=args[args.index('--session')+1]
    if os.path.exists(path):
        with open(path) as f: count=json.load(f)['count']+1
    with open(path,'w') as f: json.dump({'count':count},f)
content='轮数:'+str(count)
if any(a.startswith('@') for a in args):
    with open(next(a[1:] for a in args if a.startswith('@'))) as f: content=f.read()
print(json.dumps({'type':'message_end','message':{'role':'assistant','stopReason':'error' if text=='FAIL' else 'stop','content':[{'type':'text','text':content}]}}),flush=True)
print(json.dumps({'type':'agent_end'}),flush=True)
`
	scriptPath := filepath.Join(root, "pi-stub.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_PI_STUB", scriptPath)
	bin := writeShellExecutable(t, root, "pi", `exec python3 "$DWS_PI_STUB" "$@"`+"\n")
	makeForwarder := func(memory bool) *piForwarder {
		return newPiForwarder(bin, nil, 5*time.Second, connectAgentOptions{Memory: memory, WorkDir: root, Yolo: true}, "test-pi").(*piForwarder)
	}
	f := makeForwarder(true)
	defer f.close()
	for i := 1; i <= 2; i++ {
		reply, err := f.forward(context.Background(), "A", "hello")
		if err != nil || reply != fmt.Sprintf("轮数:%d", i) {
			t.Fatalf("reply=%q err=%v", reply, err)
		}
	}
	stateless := makeForwarder(false)
	defer stateless.close()
	for i := 0; i < 2; i++ {
		if reply, err := stateless.forward(context.Background(), "A", "hello"); err != nil || reply != "轮数:1" {
			t.Fatalf("stateless=%q %v", reply, err)
		}
	}
	attachment := filepath.Join(root, "文件.md")
	if err := os.WriteFile(attachment, []byte("附件正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reply, err := f.forwardWithAttachments(context.Background(), "file", "read", []connectMediaAttachment{{LocalPath: attachment, MediaType: "file"}}); err != nil || reply != "附件正文" {
		t.Fatalf("attachment=%q %v", reply, err)
	}
	for _, a := range []connectMediaAttachment{{LocalPath: attachment, MediaType: "audio"}, {LocalPath: ""}} {
		if _, err := f.forwardWithAttachments(context.Background(), "file", "read", []connectMediaAttachment{a}); err == nil {
			t.Fatal("invalid attachment accepted")
		}
	}
	for _, prompt := range []string{"FAIL", "EXIT", "BROKEN"} {
		if _, err := f.forward(context.Background(), "failure", prompt); err == nil {
			t.Fatalf("accepted %s", prompt)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := f.forward(ctx, "timeout", "SLEEP"); err == nil {
		t.Fatal("timeout accepted")
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	go func() {
		_, err := f.forwardStream(context.Background(), "close", "SLEEP", func(string) { once.Do(func() { close(started) }) })
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Pi did not start")
	}
	if err := f.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("close accepted partial answer")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close did not cancel process")
	}
	// 连接后的启动失败和存储异常不能降级为成功。
	broken := makeForwarder(false)
	broken.bin = filepath.Join(root, "missing")
	if _, err := broken.forward(context.Background(), "A", "hello"); err == nil {
		t.Fatal("missing binary accepted")
	}
	broken = makeForwarder(true)
	broken.sessions.m[convSessionKey("A")] = "../outside"
	if _, _, err := broken.commandArgs("A"); err == nil {
		t.Fatal("unsafe session ID accepted")
	}
	if _, err := broken.forward(context.Background(), "A", "hello"); err == nil {
		t.Fatal("unsafe session ID accepted during turn")
	}
	broken.sessionDir = attachment
	if _, _, err := broken.commandArgs("B"); err == nil {
		t.Fatal("non-directory session store accepted")
	}
	broken = makeForwarder(false)
	broken.opts.WorkDir = ""
	if _, err := broken.forward(context.Background(), "A", "hello"); err != nil {
		t.Fatal(err)
	}
}

// TestPiOfficialCLI 是显式开启的本地协议验收：运行真实官方 Pi，模型端使用 loopback fixture。
// 不读取个人凭证，不将 fixture 结果当成真实模型或钉钉消息送达。
func TestPiOfficialCLI(t *testing.T) {
	bin := os.Getenv("DWS_PI_TEST_BIN")
	if bin == "" {
		t.Skip("设置 DWS_PI_TEST_BIN 为官方 pi 绝对路径后执行本地协议验收")
	}
	root := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(root, "dws"))
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(root, "pi"))
	t.Setenv("PI_OFFLINE", "1")
	if err := os.MkdirAll(filepath.Join(root, "pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.Model != "dws-test" {
			http.Error(w, "wrong model", 400)
			return
		}
		count := 0
		for _, m := range req.Messages {
			if m.Role == "user" {
				count++
			}
		}
		last := fmt.Sprint(req.Messages[len(req.Messages)-1].Content)
		if strings.Contains(last, "READ_TOOL") {
			w.Header().Set("Content-Type", "text/event-stream")
			arguments, _ := json.Marshal(map[string]string{"path": filepath.Join(root, "tool-acceptance.txt")})
			chunk := map[string]any{"id": "test-tool", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call_read", "type": "function", "function": map[string]any{"name": "read", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			return
		}
		toolResult := req.Messages[len(req.Messages)-1].Role == "tool"
		if toolResult && !strings.Contains(last, "pi-tool-body") {
			http.Error(w, "read tool failed", 400)
			return
		}
		if strings.Contains(last, "READ_ATTACHMENT") && !strings.Contains(last, "pi-attachment-body") {
			http.Error(w, "missing attachment", 400)
			return
		}
		if strings.Contains(last, "FAIL_PROVIDER") {
			http.Error(w, `{"error":{"message":"private-provider-error"}}`, 400)
			return
		}
		if strings.Contains(last, "TIMEOUT_PROVIDER") {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		texts := []string{"用户轮数:", fmt.Sprint(count)}
		if toolResult {
			texts = []string{"工具读取", "通过"}
		}
		for _, text := range texts {
			chunk := map[string]any{"id": "test", "object": "chat.completion.chunk", "model": "dws-test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": nil}}}
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", b)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	models := map[string]any{"providers": map[string]any{"dws-local": map[string]any{"baseUrl": server.URL + "/v1", "api": "openai-completions", "apiKey": "local-fixture-only", "models": []any{map[string]any{"id": "dws-test", "contextWindow": 32000, "maxTokens": 1000}}}}}
	data, _ := json.Marshal(models)
	if err := os.WriteFile(filepath.Join(root, "pi", "models.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pi", "settings.json"), []byte(`{"retry":{"enabled":false},"defaultThinkingLevel":"off"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := connectAgentOptions{Memory: true, Model: "dws-local/dws-test", WorkDir: root, Yolo: false}
	makeForwarder := func(memory bool) *piForwarder {
		copy := opts
		copy.Memory = memory
		return newPiForwarder(bin, nil, 30*time.Second, copy, "pi-acceptance").(*piForwarder)
	}
	f := makeForwarder(true)
	defer f.close()
	check := func(f *piForwarder, conv, prompt, want string) {
		t.Helper()
		var updates []string
		reply, err := f.forwardStream(context.Background(), conv, prompt, func(s string) { updates = append(updates, s) })
		if err != nil {
			t.Fatal(err)
		}
		if reply != want || len(updates) == 0 {
			t.Fatalf("reply=%q want=%q updates=%q", reply, want, updates)
		}
	}
	check(f, "A", "--model @not-a-file\n中文\u2028输入", "用户轮数:1")
	check(f, "A", "follow up", "用户轮数:2")
	check(f, "B", "isolated", "用户轮数:1")
	if err := f.close(); err != nil {
		t.Fatal(err)
	}
	f = makeForwarder(true)
	defer f.close()
	check(f, "A", "after restart", "用户轮数:3")
	f.resetSession("A")
	check(f, "A", "after /new", "用户轮数:1")
	stateless := makeForwarder(false)
	defer stateless.close()
	check(stateless, "A", "stateless 1", "用户轮数:1")
	check(stateless, "A", "stateless 2", "用户轮数:1")
	if _, err := f.forward(context.Background(), "failure", "FAIL_PROVIDER"); err == nil || strings.Contains(err.Error(), "private-provider-error") {
		t.Fatalf("provider error=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := f.forward(ctx, "timeout", "TIMEOUT_PROVIDER"); err == nil {
		t.Fatal("timeout accepted")
	}
	check(f, "B", "recovered", "用户轮数:2")
	attachment := filepath.Join(root, "验收.md")
	if err := os.WriteFile(attachment, []byte("pi-attachment-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	reply, err := forwardConnectTurn(context.Background(), f, "B", "READ_ATTACHMENT", []connectMediaAttachment{{LocalPath: attachment, MediaType: "file"}}, nil)
	if err != nil || reply != "用户轮数:3" {
		t.Fatalf("official attachment=%q %v", reply, err)
	}
	if err := os.WriteFile(filepath.Join(root, "tool-acceptance.txt"), []byte("pi-tool-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	toolOpts := opts
	toolOpts.Yolo = true
	toolForwarder := newPiForwarder(bin, nil, 30*time.Second, toolOpts, "pi-tool-acceptance").(*piForwarder)
	defer toolForwarder.close()
	check(toolForwarder, "tool", "READ_TOOL", "工具读取通过")
	t.Log("PASS: 官方 Pi 流式文本、模型覆盖、会话隔离、跨重启恢复、/new、禁用记忆、附件、内置 read 工具、提供商失败、超时及恢复")
}

// 真模型验收使用已有 Pi 配置，只运行无工具文本对话。
func TestPiLiveModel(t *testing.T) {
	if os.Getenv("DWS_PI_LIVE") != "1" {
		t.Skip("DWS_PI_LIVE=1 才调用已有 Pi 配置的真实模型")
	}
	bin := os.Getenv("DWS_PI_TEST_BIN")
	if bin == "" {
		t.Fatal("需要 DWS_PI_TEST_BIN")
	}
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	f := newPiForwarder(bin, nil, 90*time.Second, connectAgentOptions{Memory: true, WorkDir: t.TempDir(), Model: os.Getenv("DWS_PI_LIVE_MODEL")}, "live-pi-acceptance").(*piForwarder)
	defer f.close()
	var updates int
	if _, err := f.forwardStream(context.Background(), "A", "记住本次验收暗号 PI-1472-LOCAL。仅回复：已记住", func(string) { updates++ }); err != nil {
		t.Fatal(err)
	}
	reply, err := f.forward(context.Background(), "A", "请仅输出刚才的验收暗号。")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "PI-1472-LOCAL") || updates == 0 {
		t.Fatalf("真实模型记忆/流式验收未通过，updates=%d", updates)
	}
	t.Log("PASS: 真实模型流式文本与同会话续聊")
}
