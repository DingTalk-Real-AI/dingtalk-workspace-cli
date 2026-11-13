// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type codexApprovalWriter struct{ write func([]byte) (int, error) }

func (w codexApprovalWriter) Write(p []byte) (int, error) { return w.write(p) }
func (w codexApprovalWriter) Close() error                { return nil }

func TestCrossPlatformCoverageCodexApprovalProtocol(t *testing.T) {
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval"} {
		for _, allow := range []bool{false, true} {
			t.Run(method+"/"+jsonScalar(allow), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				called := false
				ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(_ context.Context, m string, p json.RawMessage) bool {
					called = true
					if m != method || !codexApprovalMatches(p, "thread", "turn") {
						t.Error("approval leaked across turns")
					}
					if m == "item/fileChange/requestApproval" {
						var fields map[string]any
						_ = json.Unmarshal(p, &fields)
						if fields["_dwsFileChanges"] == nil {
							t.Error("file approval omitted changes")
						}
					}
					return allow
				}))
				c := unitCodexClient(nil)
				c.stdin = codexApprovalWriter{write: func(b []byte) (int, error) {
					var m struct {
						ID     int            `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
						Result map[string]any `json:"result"`
					}
					_ = json.Unmarshal(b, &m)
					if m.Method == "turn/start" {
						if m.Params["approvalPolicy"] != nil || m.Params["approvalsReviewer"] != nil || m.Params["sandboxPolicy"] != nil {
							t.Error("turn must inherit thread permissions")
						}
						go func() {
							c.msgs <- codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{"turn":{"id":"turn"}}`)}
							c.msgs <- codexRPCMessage{Method: "item/started", Params: json.RawMessage(`{"threadId":"thread","item":{"id":"item","type":"fileChange","changes":[{"path":"/workspace/a","diff":"+hello"}]}}`)}
							c.msgs <- codexRPCMessage{ID: codexIntPtr(77), Method: method, Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item","command":"echo hello","permissions":{"network":{"enabled":true}}}`)}
						}()
					} else if m.ID == 77 {
						if !called {
							t.Error("response sent before decision")
						}
						if method == "item/permissions/requestApproval" {
							permissions := m.Result["permissions"].(map[string]any)
							if (len(permissions) > 0) != allow || m.Result["scope"] != "turn" {
								t.Error("permission grant expanded")
							}
						} else {
							want := "decline"
							if allow {
								want = "accept"
							}
							if m.Result["decision"] != want {
								t.Error("wrong decision")
							}
						}
						go func() {
							c.msgs <- codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"id":"turn","status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)}
						}()
					}
					return len(b), nil
				}}
				answer, err := c.runTurn(ctx, "thread", "hello", nil, nil)
				if err != nil || answer != "done" {
					t.Fatalf("%q %v", answer, err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageCodexApprovalResolution(t *testing.T) {
	for _, scenario := range []string{"resolved", "cancelled", "send-error", "duplicate", "wrong-turn", "unknown", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(ctx context.Context, _ string, _ json.RawMessage) bool {
				if scenario == "resolved" || scenario == "cancelled" {
					<-ctx.Done()
				}
				return true
			}))
			c := unitCodexClient(codexApprovalWriter{write: func(b []byte) (int, error) {
				if scenario == "send-error" {
					return 0, errors.New("closed")
				}
				return len(b), nil
			}})
			c.approvalReplies = make(chan map[string]any, 2)
			c.approvalCancels = map[int]context.CancelFunc{}
			msg := codexRPCMessage{ID: codexIntPtr(1), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item"}`)}
			if scenario == "unknown" {
				msg.Method = "unsupported"
			}
			if scenario == "wrong-turn" {
				msg.Params = json.RawMessage(`{"threadId":"thread","turnId":"other","itemId":"item"}`)
			}
			if scenario == "disabled" {
				ctx = context.Background()
			}
			started := c.startApproval(ctx, msg, "thread", "turn")
			if scenario == "unknown" || scenario == "wrong-turn" || scenario == "disabled" {
				if started {
					t.Fatal("unsupported request accepted")
				}
				return
			}
			if !started {
				t.Fatal("approval missing")
			}
			if scenario == "duplicate" && !c.startApproval(ctx, msg, "thread", "turn") {
				t.Fatal("duplicate should not restart")
			}
			if scenario == "resolved" {
				c.approvalCancels[1]()
				delete(c.approvalCancels, 1)
				c.approvalReplies <- map[string]any{"id": 1}
				c.readErr <- io.EOF
			}
			if scenario == "cancelled" {
				cancel()
			}
			_, _ = c.next(ctx)
			for _, stop := range c.approvalCancels {
				stop()
			}
			c.approvalWG.Wait()
		})
	}
	// 畸形和空权限请求不能生成额外权限。
	if codexApprovalMatches(json.RawMessage(`!`), "x", "") {
		t.Fatal("invalid request")
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`!`), json.RawMessage(`{}`)} {
		_ = codexApprovalResult("item/permissions/requestApproval", raw, true)
	}
	_ = codexApprovalWithChanges(json.RawMessage(`!`), json.RawMessage(`[]`))
	_ = codexApprovalWithChanges(json.RawMessage(`{}`), nil)
}

func TestCrossPlatformCoverageCodexDesktopApproval(t *testing.T) {
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(context.Context, string, json.RawMessage) bool { return true }))
			a := newCodexDesktopApprovals(ctx)
			defer a.close()
			x, y := net.Pipe()
			defer x.Close()
			defer y.Close()
			c := &codexDesktopClient{conn: x, client: "follower", owner: "owner", thread: "thread", frames: make(chan codexDesktopFrame, 4)}
			go c.read(ctx)
			done := make(chan codexDesktopMessage, 1)
			go func() {
				var h [4]byte
				if _, err := io.ReadFull(y, h[:]); err != nil {
					return
				}
				size := int(h[0]) | int(h[1])<<8 | int(h[2])<<16 | int(h[3])<<24
				b := make([]byte, size)
				_, _ = io.ReadFull(y, b)
				var m codexDesktopMessage
				_ = json.Unmarshal(b, &m)
				done <- m
				server := &codexDesktopClient{conn: y}
				_ = server.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, Method: m.Method, HandledByClientID: "owner", ResultType: "success", Result: json.RawMessage(`{"ok":true}`)})
			}()
			r := codexDesktopApproval{ID: json.RawMessage(`"rpc"`), Method: method, Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item","permissions":{"network":{"enabled":true}}}`)}
			for !a.handled[`"rpc"`] {
				if err := a.update(ctx, c, []codexDesktopApproval{r}, "turn"); err != nil {
					t.Fatal(err)
				}
				if ctx.Err() != nil {
					t.Fatal("approval did not resolve")
				}
				time.Sleep(time.Millisecond)
			}
			m := <-done
			if m.Version != 1 {
				t.Fatal("unversioned decision")
			}
			b, _ := json.Marshal(m.Params)
			var p map[string]any
			_ = json.Unmarshal(b, &p)
			if p["requestId"] != "rpc" || p["conversationId"] != "thread" {
				t.Fatal("wrong request scope")
			}
			if err := a.update(ctx, c, []codexDesktopApproval{r}, "turn"); err != nil || len(a.waits) != 0 {
				t.Fatal("resolved request asked again")
			}
		})
	}
	// 已在 Codex 回答/撤销的请求应取消卡片等待。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(ctx context.Context, _ string, _ json.RawMessage) bool { <-ctx.Done(); return false }))
	a := newCodexDesktopApprovals(ctx)
	defer a.close()
	c := &codexDesktopClient{thread: "thread"}
	r := codexDesktopApproval{ID: json.RawMessage(`1`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item"}`)}
	if err := a.update(ctx, c, []codexDesktopApproval{r, r, {Method: "unknown"}}, "turn"); err != nil {
		t.Fatal(err)
	}
	if err := a.update(ctx, c, nil, "turn"); err != nil || len(a.waits) != 0 {
		t.Fatal("stale approval retained")
	}
	if err := newCodexDesktopApprovals(context.Background()).update(ctx, c, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageCodexApprovalCleanupAndForwarder(t *testing.T) {
	for _, scenario := range []string{"resolved", "turn-finished"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			entered := make(chan struct{})
			ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(ctx context.Context, _ string, _ json.RawMessage) bool {
				close(entered)
				<-ctx.Done()
				return false
			}))
			c := unitCodexClient(nil)
			c.stdin = codexApprovalWriter{write: func(b []byte) (int, error) {
				var m struct {
					Method string `json:"method"`
				}
				_ = json.Unmarshal(b, &m)
				if m.Method == "turn/start" {
					go func() {
						c.msgs <- codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{"turn":{"id":"turn"}}`)}
						c.msgs <- codexRPCMessage{ID: codexIntPtr(77), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item"}`)}
						<-entered
						if scenario == "resolved" {
							c.msgs <- codexRPCMessage{Method: "serverRequest/resolved", Params: json.RawMessage(`{"threadId":"thread","requestId":77}`)}
						}
						c.msgs <- codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)}
					}()
				}
				return len(b), nil
			}}
			if _, err := c.runTurn(ctx, "thread", "input", nil, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
	writer := &bufferWriteCloser{}
	c := unitCodexClient(writer, codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)}, codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)}, codexRPCMessage{ID: codexIntPtr(3), Result: json.RawMessage(`{"turn":{"id":"turn"}}`)}, codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)})
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return c, nil })
	ctx := context.WithValue(context.Background(), codexApprovalKey{}, codexApprovalHandler(func(context.Context, string, json.RawMessage) bool { return false }))
	f := &codexAppServerForwarder{developerInstructions: "employee"}
	if _, err := f.forwardAppServer(ctx, "conversation", "hello", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(writer.String(), `"approvalPolicy":"on-request"`) || !strings.Contains(writer.String(), `"approvalsReviewer":"auto_review"`) {
		t.Fatal("thread did not default to auto review")
	}
	a := newCodexDesktopApprovals(context.WithValue(context.Background(), codexApprovalKey{}, codexApprovalHandler(func(ctx context.Context, _ string, _ json.RawMessage) bool { <-ctx.Done(); return false })))
	if err := a.update(context.Background(), &codexDesktopClient{thread: "thread"}, []codexDesktopApproval{{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item"}`)}}, "turn"); err != nil {
		t.Fatal(err)
	}
	a.close()
}
