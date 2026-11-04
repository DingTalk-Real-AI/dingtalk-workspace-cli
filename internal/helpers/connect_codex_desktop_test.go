// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageCodexDesktopWriterRouting(t *testing.T) {
	for _, scenario := range []string{"shared", "other-rpc", "not-employee"} {
		t.Run(scenario, func(t *testing.T) {
			writer := &bufferWriteCloser{}
			code := -32600
			if scenario == "other-rpc" {
				code = 123
			}
			c := unitCodexClient(writer,
				codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
				codexRPCMessage{ID: codexIntPtr(2), Error: &codexRPCError{Code: code, Message: "thread old already has an active writer"}},
				codexRPCMessage{ID: codexIntPtr(3), Error: &codexRPCError{Message: "fixture stop"}},
			)
			testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return c, nil })
			called := 0
			testseam.Swap(t, &codexForwardDesktopTurn, func(ctx context.Context, thread, text string, attachments []connectMediaAttachment, _ func(string), instructions string) (string, error) {
				called++
				if thread != "old" || text != "next" || len(attachments) != 1 || agentProgressSink(ctx) == nil || instructions != "employee" {
					t.Fatal("共享接续丢失线程、输入、附件或公开进度")
				}
				return "same context", nil
			})
			f := &codexAppServerForwarder{sessions: newCodexThreadSessions(""), developerInstructions: "employee"}
			if scenario == "not-employee" {
				f.developerInstructions = ""
			}
			key := codexManagedConversationKey("conv")
			f.sessions.setThreadID(key, "old")
			ctx := context.WithValue(context.Background(), agentProgressKey{}, func(agentProgress) {})
			answer, err := f.forwardAppServer(ctx, "conv", "next", []connectMediaAttachment{{LocalPath: "image"}}, nil)
			if scenario == "shared" {
				if err != nil || answer != "same context" || called != 1 || f.sessions.threadID(key) != "old" || strings.Contains(writer.String(), `"turn/start"`) {
					t.Fatalf("未保留单一写入方及原映射：%q %v %d", answer, err, called)
				}
			} else if called != 0 || err == nil {
				t.Fatal("不应对其他 RPC 错误或机器人静默启用共享接入")
			}
		})
	}
	if codexThreadHasActiveWriter(errors.New("already has an active writer")) || codexThreadHasActiveWriter(&codexRPCError{Code: -32600, Message: "different"}) {
		t.Fatal("必须同时核对 RPC 类型、错误码和原因")
	}
}

// 使用真实长度帧和 net.Pipe 验证协议、回合关联、断线不重放及公开输出边界。
func TestCrossPlatformCoverageCodexDesktopSharedTurn(t *testing.T) {
	for _, scenario := range []string{"approval", "approval-error", "canonical", "legacy", "busy", "failed", "empty", "drop-after-start", "bad-receipt", "no-client", "no-owner", "empty-owner", "version", "init-drop", "init-timeout", "busy-timeout", "result-timeout", "busy-limit", "result-limit", "in-progress", "completed-error", "post-disconnect"} {
		t.Run(scenario, func(t *testing.T) {
			if strings.HasSuffix(scenario, "limit") {
				testseam.Swap(t, &codexDesktopWithTimeout, func(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
					if scenario == "busy-limit" && d == codexDesktopReadyTimeout || scenario == "result-limit" && d == codexDesktopTurnTimeout {
						d = 80 * time.Millisecond
					}
					return context.WithTimeout(ctx, d)
				})
			}
			timeout := 4 * time.Second
			if strings.HasSuffix(scenario, "timeout") {
				timeout = 80 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			client, server := net.Pipe()
			defer server.Close()
			testseam.Swap(t, &codexDesktopDial, func(context.Context) (net.Conn, error) { return client, nil })
			var starts atomic.Int32
			var partial atomic.Bool
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer server.Close()
				codec := &codexDesktopClient{conn: server}
				snapshots := 0
				approvalResolved := false
				for {
					var h [4]byte
					if _, err := io.ReadFull(server, h[:]); err != nil {
						return
					}
					b := make([]byte, binary.LittleEndian.Uint32(h[:]))
					if _, err := io.ReadFull(server, b); err != nil {
						return
					}
					var m codexDesktopMessage
					if json.Unmarshal(b, &m) != nil {
						return
					}
					result := any(map[string]any{})
					switch m.Method {
					case "initialize":
						if scenario == "init-drop" {
							return
						}
						if scenario == "init-timeout" {
							<-ctx.Done()
							return
						}
						if scenario != "no-client" {
							result = map[string]string{"clientId": "client"}
						}
					case "thread-owner-discovery":
						if scenario == "empty-owner" {
							_ = codec.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, ResultType: "success", Method: m.Method})
							continue
						}
						if scenario == "no-owner" {
							_ = codec.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, ResultType: "error", Error: "no-client-found"})
							continue
						}
					case "thread-follower-start-turn":
						starts.Add(1)
						if scenario == "drop-after-start" {
							return
						}
						p := m.Params.(map[string]any)
						turnStart := p["turnStart"].(map[string]any)
						request := turnStart["request"].(map[string]any)
						if request["approvalPolicy"] != nil || request["approvalsReviewer"] != nil || request["sandboxPolicy"] != nil {
							t.Error("shared turn overrode human permission selection")
						}
						policy := request["additionalContext"].(map[string]any)["dws.digital_employee_policy"].(map[string]any)
						if policy["kind"] != "application" || policy["value"] != "employee" || len(request["input"].([]any)[0].(map[string]any)["text_elements"].([]any)) != 0 {
							t.Error("共享轮次缺少员工应用策略或文本元素协议")
						}
						if m.TargetClientID != "desktop" || m.Version != 2 || request["threadId"] != "thread" || request["clientUserMessageId"] == "" || len(request["input"].([]any)) != 2 || len(turnStart["context"].(map[string]any)) != 1 {
							t.Error("提交必须定向原持有者，保留权限并带唯一消息标识和附件")
						}
						if scenario != "bad-receipt" {
							result = map[string]any{"result": map[string]any{"turn": map[string]string{"id": "ours"}}}
						}
					case "thread-follower-file-approval-decision":
						if m.Params.(map[string]any)["decision"] != "accept" {
							t.Error("wrong approval")
						}
						approvalResolved = true
						if scenario == "approval-error" {
							_ = codec.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, ResultType: "error", Method: m.Method, HandledByClientID: "desktop"})
							continue
						}
					case "thread-stream-following-changed":
						snapshots++
						if scenario == "in-progress" && snapshots == 3 && !partial.Load() {
							t.Error("共享正文必须在回合完成前发出")
						}
						if scenario == "post-disconnect" && starts.Load() > 0 {
							return
						}
						state := map[string]any{"id": "thread", "threadRuntimeStatus": map[string]string{"type": "idle"}, "currentPermissions": map[string]any{"approvalPolicy": "on-request", "approvalsReviewer": "user", "sandboxPolicy": map[string]string{"type": "workspaceWrite"}}}
						if (scenario == "busy" || scenario == "busy-timeout" || scenario == "busy-limit") && snapshots == 1 {
							state["threadRuntimeStatus"] = map[string]string{"type": "active"}
						}
						if starts.Load() > 0 {
							status := "completed"
							if strings.HasPrefix(scenario, "approval") && !approvalResolved {
								status = "inProgress"
								state["requests"] = []any{map[string]any{"id": "approval-rpc", "method": "item/fileChange/requestApproval", "params": map[string]string{"threadId": "thread", "turnId": "ours", "itemId": "change"}}, map[string]any{"id": "foreign", "method": "unknown"}}
							}
							if scenario == "result-timeout" || scenario == "result-limit" || scenario == "in-progress" && snapshots == 2 {
								status = "inProgress"
							}
							if scenario == "failed" {
								status = "failed"
							}
							items := []any{map[string]string{"id": "private", "type": "reasoning", "text": "PRIVATE_REASONING"}, map[string]string{"id": "tool", "type": "mcpToolCall", "text": "PRIVATE_TOOL", "status": "failed"}, map[string]string{"id": "comment", "type": "agentMessage", "phase": "commentary", "text": "已核对"}}
							if scenario != "empty" {
								text := "共享答案"
								if scenario == "in-progress" && snapshots == 2 {
									text = "共享"
									items[1] = map[string]string{"id": "tool", "type": "mcpToolCall", "status": "inProgress"}
								}
								items = append(items, map[string]string{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": text})
							}
							if strings.HasPrefix(scenario, "approval") {
								items = append(items, map[string]any{"id": "change", "type": "fileChange", "changes": []any{map[string]string{"path": "/workspace/a", "diff": "+hello"}}})
							}
							turn := map[string]any{"turnId": "ours", "status": status, "items": items}
							if scenario == "completed-error" {
								turn["error"] = map[string]string{"message": "PRIVATE_ERROR"}
							}
							other := map[string]any{"turnId": "someone-else", "status": "completed", "items": []any{map[string]string{"type": "agentMessage", "text": "错误回合的答案"}}}
							if scenario == "legacy" {
								state["turns"] = []any{turn, other}
							} else {
								state["turnHistory"] = map[string]any{"history": map[string]any{"entitiesByKey": map[string]any{"a": turn, "b": other}}}
							}
						}
						version := 11
						if scenario == "version" {
							version = 12
						}
						for _, source := range []string{"unrelated-client", "desktop"} {
							if codec.send(codexDesktopMessage{Type: "broadcast", SourceClientID: source, Method: "thread-stream-state-changed", Version: version, Params: map[string]any{"hostId": "local", "conversationId": "thread", "change": map[string]any{"type": "snapshot", "conversationState": state}}}) != nil {
								return
							}
						}
						continue
					default:
						t.Error("意外的 IPC 方法", m.Method)
						return
					}
					raw, _ := json.Marshal(result)
					if codec.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, ResultType: "success", Method: m.Method, HandledByClientID: "desktop", Result: raw}) != nil {
						return
					}
				}
			}()
			var progress []agentProgress
			ctx = context.WithValue(ctx, agentProgressKey{}, func(p agentProgress) {
				progress = append(progress, p)
				if p.Kind == "answer" && p.Text == "共享" && !p.Done {
					partial.Store(true)
				}
			})
			if strings.HasPrefix(scenario, "approval") {
				ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(_ context.Context, _ string, p json.RawMessage) bool {
					var fields map[string]any
					_ = json.Unmarshal(p, &fields)
					if fields["_dwsFileChanges"] == nil {
						t.Error("shared file approval omitted diff")
					}
					return true
				}))
			}
			delta := ""
			answer, err := forwardCodexDesktopTurn(ctx, "thread", "new input", []connectMediaAttachment{{MediaType: "image", LocalPath: "/fixture.png"}, {MediaType: "file", LocalPath: "/ignored"}}, func(s string) { delta = s }, "employee")
			<-done
			switch scenario {
			case "approval", "canonical", "legacy", "busy", "in-progress":
				if err != nil || answer != "共享答案" || delta != answer || starts.Load() != 1 {
					t.Fatalf("共享执行失败：%q %v %d", answer, err, starts.Load())
				}
			case "empty":
				if !errors.Is(err, errCodexCompletedWithoutReply) {
					t.Fatalf("空结果身份丢失：%v", err)
				}
			default:
				var shared *codexDesktopTurnError
				if !errors.As(err, &shared) || answer != "" {
					t.Fatalf("未保留安全分类：%q %v", answer, err)
				}
				if scenario == "empty-owner" && (shared.Cause == nil || !strings.Contains(shared.Cause.Error(), "missing owner") || strings.Contains(shared.Cause.Error(), "%!w")) {
					t.Fatal("缺少 owner 应返回明确错误，不得包装 nil")
				}
				if scenario == "drop-after-start" || scenario == "bad-receipt" {
					if shared.Code != "codex_desktop_result_unknown" || !shared.Submitted || starts.Load() != 1 {
						t.Fatal("提交未知不能重放或误报未执行")
					}
				}
				if strings.HasSuffix(scenario, "limit") {
					wantStarts, wantCode := int32(0), "codex_desktop_unavailable"
					if scenario == "result-limit" {
						wantStarts, wantCode = 1, "codex_desktop_result_unknown"
					}
					if ctx.Err() != nil || !errors.Is(shared, context.DeadlineExceeded) || shared.Code != wantCode || starts.Load() != wantStarts {
						t.Fatal("独立等待上限未保留提交状态，或错误依赖调用方超时", shared, starts.Load())
					}
				}
			}
			for _, p := range progress {
				if strings.Contains(p.Text, "PRIVATE") || strings.Contains(p.Text, "错误回合") {
					t.Fatal("共享进度泄漏私有内容或串回合")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageCodexDesktopFrames(t *testing.T) {
	for _, scenario := range []string{"zero", "large", "invalid-json", "short-body", "closed", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			c := &codexDesktopClient{conn: a, frames: make(chan codexDesktopFrame, 2)}
			go c.read(ctx)
			go func() {
				defer b.Close()
				var n uint32
				switch scenario {
				case "large":
					n = codexDesktopFrameLimit + 1
				case "invalid-json", "short-body":
					n = 1
				case "closed", "cancelled":
					return
				}
				_ = binary.Write(b, binary.LittleEndian, n)
				if scenario == "invalid-json" {
					_, _ = b.Write([]byte("!"))
				}
			}()
			if scenario == "cancelled" {
				cancel()
			}
			if _, err := c.next(ctx); err == nil {
				t.Fatal("损坏、断开或取消的帧不能当作确认")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(codexDesktopPause(ctx), context.Canceled) {
		t.Fatal("等待无法取消")
	}
	// 真实平台入口在桌面端不存在或不支持时明确失败，不触碰用户的默认端点。
	t.Setenv("CODEX_HOME", t.TempDir())
	if conn, err := dialCodexDesktop(ctx); err == nil {
		conn.Close()
		t.Fatal("桌面端不可用时不能伪造共享连接")
	}
	testseam.Swap(t, &codexDesktopDial, func(context.Context) (net.Conn, error) { return nil, errors.New("private path") })
	_, err := forwardCodexDesktopTurn(context.Background(), "t", "x", nil, nil, "employee")
	if err == nil || err.Error() != "codex_desktop_unavailable" {
		t.Fatal("底层错误应保持本地安全分类")
	}
}

type codexDesktopWriteConn struct {
	net.Conn
	write func([]byte) (int, error)
}

func (c codexDesktopWriteConn) Write(b []byte) (int, error) { return c.write(b) }

func TestCrossPlatformCoverageCodexDesktopProtocolErrors(t *testing.T) {
	ctx := context.Background()
	c := &codexDesktopClient{frames: make(chan codexDesktopFrame, 4), thread: "thread", owner: "desktop"}
	c.conn = codexDesktopWriteConn{write: func(b []byte) (int, error) { return len(b), nil }}
	if err := c.send(codexDesktopMessage{Params: make(chan int)}); err == nil {
		t.Fatal("不能发送非法 JSON")
	}
	if err := c.send(codexDesktopMessage{Error: strings.Repeat("x", codexDesktopFrameLimit)}); err == nil {
		t.Fatal("必须限制出站帧大小")
	}
	c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "client-discovery-request", RequestID: "discovery"}}
	c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "broadcast"}}
	if _, err := c.next(ctx); err != nil {
		t.Fatal(err)
	}
	c.conn = codexDesktopWriteConn{write: func([]byte) (int, error) { return 0, io.ErrClosedPipe }}
	c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "client-discovery-request"}}
	if _, err := c.next(ctx); err == nil {
		t.Fatal("发现响应失败必须停止")
	}
	if _, err := c.request(ctx, "x", 1, nil); err == nil {
		t.Fatal("写入失败不可等待虚假确认")
	}
	if _, err := c.snapshot(ctx); err == nil {
		t.Fatal("订阅写入失败必须停止")
	}
	c.conn = codexDesktopWriteConn{write: func(b []byte) (int, error) {
		var m codexDesktopMessage
		_ = json.Unmarshal(b[4:], &m)
		if m.Type == "request" {
			c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "broadcast"}}
			c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "response", RequestID: m.RequestID, Method: "different", ResultType: "success"}}
		}
		return len(b), nil
	}}
	if _, err := c.request(ctx, "expected", 1, nil); err == nil {
		t.Fatal("必须关联请求及返回的方法")
	}
	for _, params := range []any{make(chan int), "malformed", map[string]any{"conversationId": "other", "hostId": "local", "change": map[string]any{"type": "snapshot"}}} {
		c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "broadcast"}}
		c.frames <- codexDesktopFrame{message: codexDesktopMessage{Type: "broadcast", Method: "thread-stream-state-changed", SourceClientID: "desktop", Params: params}}
		c.frames <- codexDesktopFrame{err: io.EOF}
		if _, err := c.snapshot(ctx); err == nil {
			t.Fatal("畸形或其他会话的数据不得用于当前回复")
		}
		for len(c.frames) > 0 {
			<-c.frames
		}
	}
	close(c.frames)
	if _, err := c.next(ctx); !errors.Is(err, io.EOF) {
		t.Fatal("必须处理读取器结束")
	}
	if _, err := c.snapshot(ctx); !errors.Is(err, io.EOF) {
		t.Fatal("断开后快照未确认")
	}
	reader, writer := net.Pipe()
	defer reader.Close()
	readCtx, cancel := context.WithCancel(ctx)
	c = &codexDesktopClient{conn: reader, frames: make(chan codexDesktopFrame)}
	done := make(chan struct{})
	go func() { c.read(readCtx); close(done) }()
	cancel()
	_ = writer.Close()
	<-done
}
