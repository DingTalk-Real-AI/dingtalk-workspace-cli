// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type employeeImageCapture struct {
	inputs []connectMediaAttachment
	text   string
}

func (f *employeeImageCapture) label() string { return "image-capture" }
func (f *employeeImageCapture) forward(_ context.Context, _, text string) (string, error) {
	f.text = text
	return "", nil
}
func (f *employeeImageCapture) forwardWithAttachments(_ context.Context, _, text string, attachments []connectMediaAttachment) (string, error) {
	f.text = text
	f.inputs = append([]connectMediaAttachment(nil), attachments...)
	for _, a := range attachments {
		if _, err := os.ReadFile(a.LocalPath); err != nil {
			return "", err
		}
	}
	return "", nil
}

func TestCrossPlatformCoverageEmployeeImageInput(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "quoted"}[quoted], func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.Binding.DWSProfile = "corp:employee"
			e.Content = "[图片消息](mediaId=fixture-image) 注意：如需下载使用dws chat message download-media命令下载"
			wantMessage, wantConversation := e.MessageID, e.ConversationID
			if quoted {
				e.QuotedMessage = &personal.MessageEventContext{MessageID: "source-message", ConversationID: "source-conversation", Content: e.Content}
				e.Content = "请识别引用图片"
				wantMessage, wantConversation = "source-message", "source-conversation"
			}
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			calls := 0
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				calls++
				joined := strings.Join(args, "|")
				for _, want := range []string{"--profile|corp:employee", "--resource-id|fixture-image", "--message-id|" + wantMessage, "--open-conversation-id|" + wantConversation} {
					if !strings.Contains(joined, want) {
						t.Errorf("图片下载未使用员工身份或原消息定位：缺少 %s", want)
					}
				}
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
			})
			fwd := &employeeImageCapture{}
			r.fwd = fwd
			// 从真实事件投影开始，验证定位字段没有在入站解码中丢失。
			body := map[string]any{"openMessageId": e.MessageID, "openConversationId": e.ConversationID, "senderOpenDingTalkId": e.SenderID, "content": e.Content}
			if e.QuotedMessage != nil {
				body["quotedMessage"] = map[string]any{"openMessageId": wantMessage, "openConversationId": wantConversation, "content": e.QuotedMessage.Content}
			}
			raw, _ := json.Marshal(map[string]any{"eventId": e.EventID, "eventKey": e.Type, "payload": map[string]any{"body": body}})
			projected, err := personal.ProjectOutput(transport.Event{EventType: e.Type, Data: string(raw)})
			if err != nil {
				t.Fatal(err)
			}
			line, _ := json.Marshal(projected)
			if err := json.Unmarshal(line, &e); err != nil {
				t.Fatal(err)
			}
			q := make(chan employeeEvent, 1)
			r.queues[e.ConversationID] = q
			if err := r.enqueue(e); err != nil {
				t.Fatal(err)
			}
			if err := r.process(<-q); err != nil {
				t.Fatal(err)
			}
			if len(fwd.inputs) != 1 || fwd.inputs[0].MediaType != "image" || calls != 1 {
				t.Fatalf("Agent 只收到 mediaId 文本，未收到图片内容：attachments=%v download_calls=%d", fwd.inputs, calls)
			}
			path := fwd.inputs[0].LocalPath
			if !strings.Contains(fwd.text, path) {
				t.Fatal("Agent 文本缺少可读附件路径")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("回合结束未清理图片：%v", err)
			}
			for _, name := range []string{r.recordPath(e), filepath.Join(r.dir, "audit.jsonl")} {
				stored, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(stored), "fixture-image") || strings.Contains(string(stored), path) {
					t.Fatal("账本或审计泄漏图片引用")
				}
			}
		})
	}
}

func TestEmployeeImageDownloadHelper(t *testing.T) {
	if os.Getenv("DWS_EMPLOYEE_IMAGE_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg != "--output" || i+1 >= len(os.Args) {
			continue
		}
		path := os.Args[i+1]
		mode := os.Getenv("DWS_EMPLOYEE_IMAGE_MODE")
		if mode == "failed" {
			os.Exit(6)
		}
		if mode == "cancelled" {
			time.Sleep(10 * time.Second)
			os.Exit(7)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(2)
		}
		switch mode {
		case "webp":
			data, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
			_, _ = f.Write(data)
		case "jpeg":
			_ = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
		case "gif":
			_ = gif.Encode(f, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}), nil)
		case "non-image":
			_, _ = f.WriteString("this is not an image")
		case "oversized":
			_ = f.Truncate(employeeImageMaxBytes + 1)
		default:
			if png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))) != nil {
				os.Exit(3)
			}
		}
		if f.Close() != nil {
			os.Exit(4)
		}
		if mode == "invalid-json" {
			_, _ = os.Stdout.WriteString("not json")
			os.Exit(0)
		}
		if mode == "not-success" {
			_, _ = os.Stdout.WriteString(`{"success":false}`)
			os.Exit(0)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"success": true, "output": path, "downloadUrl": "https://example.invalid/private?token=secret"})
		os.Exit(0)
	}
	os.Exit(5)
}

func TestCrossPlatformCoverageEmployeeImageBoundaries(t *testing.T) {
	for _, mode := range []string{"success", "failed", "invalid-json", "not-success", "non-image", "oversized", "cancelled", "unauthorized", "control"} {
		t.Run(mode, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			e.Content = "[图片消息](mediaId=private-image)"
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			t.Setenv("DWS_EMPLOYEE_IMAGE_MODE", mode)
			calls := 0
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				if strings.Contains(strings.Join(args, "|"), "|download-media|") {
					calls++
					return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
				}
				// 隔离真实下行，失败事实仍通过任务账本验证。
				return exec.CommandContext(ctx, filepath.Join(r.dir, "missing-reply-binary"))
			})
			if mode == "success" || mode == "cancelled" {
				ctx := context.Background()
				cancel := func() {}
				if mode == "cancelled" {
					ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				}
				defer cancel()
				attachments, cleanup, err := prepareEmployeeImages(ctx, "corp:employee", r.dir, e)
				defer cleanup()
				if mode == "success" {
					if err != nil || len(attachments) != 1 {
						t.Fatalf("图片准备失败：%v", err)
					}
					for _, path := range []string{attachments[0].LocalPath, filepath.Dir(attachments[0].LocalPath), filepath.Join(r.dir, "media")} {
						info, err := os.Stat(path)
						if err != nil || info.Mode().Perm()&0077 != 0 {
							t.Fatalf("图片不是私有文件：%s %v", path, err)
						}
					}
				} else if !errors.Is(err, errEmployeeImageUnavailable) {
					t.Fatalf("下载取消未报告失败：%v", err)
				}
				cleanup()
			} else {
				fwd := &employeeImageCapture{}
				r.fwd = fwd
				if mode == "unauthorized" {
					r.cfg.Options.AllowedUsers = nil
				}
				if mode == "control" {
					e.QuotedMessage = &personal.MessageEventContext{Content: e.Content}
					e.Content = "/new"
				}
				q := make(chan employeeEvent, 1)
				r.queues[e.ConversationID] = q
				if err := r.enqueue(e); err != nil {
					t.Fatal(err)
				}
				select {
				case next := <-q:
					if err := r.process(next); err != nil {
						t.Fatal(err)
					}
				default:
				}
				if len(fwd.inputs) != 0 || fwd.text != "" {
					t.Fatal("失败、未授权或控制消息进入读图 Agent")
				}
				if mode != "unauthorized" && mode != "control" {
					raw, err := os.ReadFile(r.recordPath(e))
					if err != nil {
						t.Fatal(err)
					}
					var record employeeTaskRecord
					if json.Unmarshal(raw, &record) != nil || record.Execution != "failure" || record.Status != "agent_failed" {
						t.Fatalf("图片错误被当作无正文成功：%s", raw)
					}
				}
			}
			wantCalls := 1
			if mode == "control" || mode == "unauthorized" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("下载调用次数=%d，希望=%d", calls, wantCalls)
			}
			entries, _ := os.ReadDir(filepath.Join(r.dir, "media"))
			if len(entries) != 0 {
				t.Fatal("回合结束残留图片文件")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeImageReferences(t *testing.T) {
	for _, tc := range []struct {
		name, content, message, conversation string
		want                                 int
		bad                                  bool
	}{
		{"plain-media-id", "mediaId=secret", "m", "c", 0, false},
		{"non-image", "[视频消息](mediaId=secret)", "m", "c", 0, false},
		{"duplicate", strings.Repeat("[图片消息](mediaId=a)", 10), "m", "c", 1, false},
		{"over-limit", "[图片消息](mediaId=a)[图片消息](mediaId=b)[图片消息](mediaId=c)[图片消息](mediaId=d)[图片消息](mediaId=e)", "m", "c", 0, true},
		{"missing-message", "[图片消息](mediaId=a)", "", "c", 0, true},
		{"missing-conversation", "[图片消息](mediaId=a)", "m", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs, err := employeeImageRefs(employeeEvent{Content: tc.content, MessageID: tc.message, ConversationID: tc.conversation})
			if (err != nil) != tc.bad || len(refs) != tc.want {
				t.Fatalf("refs=%v err=%v", refs, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeImageCustomCommand(t *testing.T) {
	dir := t.TempDir()
	bin := writeShellExecutable(t, dir, "image-custom", `printf '%s\n' "$#" "$@"`)
	fwd := &execForwarder{name: "custom", argv: []string{bin, "--fixture-option"}, timeout: 5 * time.Second}
	path := filepath.Join(dir, "图片.png")
	answer, err := forwardEmployeeTurn(context.Background(), fwd, "conversation", "看图", connectMediaAttachment{LocalPath: path, MediaType: "image"})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(answer, "\n")
	if len(lines) < 3 || lines[0] != "2" || lines[1] != "--fixture-option" || !strings.Contains(answer, path) {
		t.Fatalf("custom 注入未知参数或丢失附件路径：%q", answer)
	}
}

func TestCrossPlatformCoverageEmployeeImageCodexInput(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	fwd := newCodexAppServerForwarder("fixture", nil, 0, connectAgentOptions{Memory: true}, "employee").(*codexAppServerForwarder)
	fwd.developerInstructions = codexEmployeeDeveloperInstructions
	input := &bufferWriteCloser{}
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) {
		return unitCodexClient(input,
			codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
			codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"image-thread"}}`)},
			codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"image-thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"图片识别成功"}]}}`)},
		), nil
	})
	path := filepath.Join(t.TempDir(), "image.png")
	answer, err := forwardEmployeeTurn(context.Background(), fwd, "conversation", "看图", connectMediaAttachment{LocalPath: path, MediaType: "image"})
	if err != nil || answer != "图片识别成功" {
		t.Fatalf("员工 Codex 读图失败：%q %v", answer, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(input.String()), "\n") {
		var request struct {
			Method string `json:"method"`
			Params struct {
				Input []struct{ Type, Path string } `json:"input"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &request) != nil {
			t.Fatal("无效 RPC")
		}
		if request.Method != "turn/start" {
			continue
		}
		for _, item := range request.Params.Input {
			if item.Type == "localImage" && item.Path == path {
				return
			}
		}
		t.Fatal("员工回合未传递 Codex 原生 localImage")
	}
	t.Fatal("未调用 turn/start")
}

func TestCrossPlatformCoverageEmployeeImageIOFailures(t *testing.T) {
	for _, mode := range []string{"mkdir", "turn-dir", "create-file", "close-file", "command", "open", "read", "rename"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			e := employeeEvent{MessageID: "m", ConversationID: "c", Content: "[图片消息](mediaId=fixture)"}
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			boom := errors.New("private IO failure")
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
			})
			switch mode {
			case "mkdir":
				dir = filepath.Join(dir, "not-a-directory")
				if err := os.WriteFile(dir, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			case "turn-dir":
				testseam.Swap(t, &employeeImageMkdirTemp, func(string, string) (string, error) { return "", boom })
			case "create-file":
				testseam.Swap(t, &employeeImageCreateTemp, func(string, string) (*os.File, error) { return nil, boom })
			case "close-file":
				testseam.Swap(t, &employeeImageCreateTemp, func(dir, pattern string) (*os.File, error) {
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					if err := f.Close(); err != nil {
						t.Fatal(err)
					}
					return f, nil
				})
			case "command":
				testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", boom })
			case "open":
				testseam.Swap(t, &employeeImageOpen, func(string) (*os.File, error) { return nil, boom })
			case "read":
				testseam.Swap(t, &employeeImageOpen, func(path string) (*os.File, error) {
					f, err := os.Open(path)
					if err != nil {
						return nil, err
					}
					if err := f.Close(); err != nil {
						t.Fatal(err)
					}
					return f, nil
				})
			case "rename":
				testseam.Swap(t, &employeeImageRename, func(string, string) error { return boom })
			}
			attachments, cleanup, err := prepareEmployeeImages(context.Background(), "corp:employee", dir, e)
			cleanup()
			if !errors.Is(err, errEmployeeImageUnavailable) || len(attachments) != 0 {
				t.Fatalf("文件错误未阻止附件交付：%v %v", attachments, err)
			}
			entries, _ := os.ReadDir(filepath.Join(dir, "media"))
			if len(entries) != 0 {
				t.Fatal("失败后残留图片回合")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeImageFormatAndQuoteFailures(t *testing.T) {
	for _, format := range []string{"jpeg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			t.Setenv("DWS_EMPLOYEE_IMAGE_MODE", format)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
			})
			attachments, cleanup, err := prepareEmployeeImages(context.Background(), "corp:employee", t.TempDir(), employeeEvent{MessageID: "m", ConversationID: "c", Content: "[图片消息](mediaId=fixture)"})
			defer cleanup()
			want := map[string]string{"jpeg": ".jpg", "gif": ".gif", "webp": ".webp"}[format]
			if err != nil || len(attachments) != 1 || filepath.Ext(attachments[0].LocalPath) != want {
				t.Fatalf("图片格式准备失败：%v %v", attachments, err)
			}
		})
	}
	e := employeeEvent{MessageID: "m", ConversationID: "c", Content: "当前问题", QuotedMessage: &personal.MessageEventContext{Content: "[图片消息](mediaId=fixture)"}}
	attachments, cleanup, err := prepareEmployeeImages(context.Background(), "corp:employee", t.TempDir(), e)
	defer cleanup()
	if !errors.Is(err, errEmployeeImageUnavailable) || len(attachments) != 0 {
		t.Fatal("缺少引用原消息定位仍交付图片")
	}
}
