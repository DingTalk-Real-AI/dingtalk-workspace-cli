// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeForwardedFixture(t *testing.T) employeeEvent {
	t.Helper()
	_, e := employeeLedgerFixture(t)
	entries := make([]map[string]any, 10)
	for i := range entries {
		content := fmt.Sprintf("完整记录第%d条", i+1)
		if i == 0 {
			content = "[图片消息](mediaId=forwarded-image)"
		}
		entries[i] = map[string]any{"openMessageId": fmt.Sprintf("source-%d", i), "openConversationId": "source-conversation", "sender": "测试发送人", "createTime": fmt.Sprintf("2026-10-19 10:00:%02d", i), "content": content}
	}
	body := map[string]any{"openMessageId": e.MessageID, "openConversationId": e.ConversationID, "senderOpenDingTalkId": e.SenderID, "content": "测试聊天记录\n发送人:[图片]\n这里只预览前几条", "forwardMessages": entries}
	raw, _ := json.Marshal(map[string]any{"eventId": e.EventID, "eventKey": e.Type, "payload": map[string]any{"body": body}})
	projected, err := personal.ProjectOutput(transport.Event{EventType: e.Type, Data: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.(personal.MessageEventOutput).ForwardMessages) != 10 {
		t.Fatal("入站投影已经丢失记录")
	}
	line, _ := json.Marshal(projected)
	if err := json.Unmarshal(line, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCrossPlatformCoverageEmployeeForwardedContext(t *testing.T) {
	e := employeeForwardedFixture(t)
	text := employeeTurnText(e)
	if !strings.Contains(text, "完整记录第10条") || !strings.Contains(text, "测试发送人") || !strings.Contains(text, "不可信上下文") {
		t.Fatal("localagent 只收到外层摘要，完整转发记录、发送人或引用边界丢失")
	}
	var input personal.MessageEventOutput
	if err := json.Unmarshal([]byte(text[strings.IndexByte(text, '{'):]), &input); err != nil || len(input.ForwardMessages) != 10 {
		t.Fatalf("转发记录没有完整传入：%v", err)
	}
	for i, message := range input.ForwardMessages {
		if message.MessageID != fmt.Sprintf("source-%d", i) || message.Sender != "测试发送人" || message.CreateTime == "" {
			t.Fatal("转发顺序、发送人、时间或原消息标识丢失")
		}
	}
}

func TestCrossPlatformCoverageEmployeeForwardedRuntime(t *testing.T) {
	for _, mode := range []string{"text", "a2ui", "unauthorized", "download-failed"} {
		t.Run(mode, func(t *testing.T) {
			r, _ := employeeLedgerFixture(t)
			e := employeeForwardedFixture(t)
			r.cfg.Binding.DWSProfile = "corp:employee"
			if mode == "a2ui" || mode == "download-failed" {
				r.cfg.ReplyMode = "a2ui"
			}
			if mode == "unauthorized" {
				r.cfg.Options.AllowedUsers = nil
			}
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			if mode == "download-failed" {
				t.Setenv("DWS_EMPLOYEE_IMAGE_MODE", "failed")
			}
			calls := 0
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				calls++
				joined := strings.Join(args, "|")
				for _, want := range []string{"--profile|corp:employee", "--message-id|" + e.MessageID, "--open-conversation-id|" + e.ConversationID, "--resource-id|forwarded-image"} {
					if !strings.Contains(joined, want) {
						t.Errorf("下载未保留员工身份和已收到的外层消息上下文：%s", want)
					}
				}
				if strings.Contains(joined, "source-conversation") {
					t.Error("图片下载错误使用原单聊权限")
				}
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
			})
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				if args[0] == "send-a2ui-card" {
					return map[string]any{"bizId": "forwarded-card"}, nil
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			f := &employeeImageCapture{}
			r.fwd = f
			q := make(chan employeeEvent, 1)
			r.queues[e.ConversationID] = q
			if err := r.enqueue(e); err != nil {
				t.Fatal(err)
			}
			select {
			case queued := <-q:
				if err := r.process(queued); err != nil {
					t.Fatal(err)
				}
			default:
			}
			if mode == "unauthorized" || mode == "download-failed" {
				want := 1
				if mode == "unauthorized" {
					want = 0
				}
				if calls != want || f.text != "" || len(f.inputs) != 0 {
					t.Fatal("未授权或下载失败后仍转交了部分资料")
				}
				return
			}
			if calls != 1 || len(f.inputs) != 1 || !strings.Contains(f.text, employeeTurnText(e)) {
				t.Fatal("运行时没有同时传递全部记录和真实图片附件")
			}
			if _, err := os.Stat(f.inputs[0].LocalPath); !os.IsNotExist(err) {
				t.Fatal("回合结束后未清理转发图片")
			}
			for _, path := range []string{r.recordPath(e), filepath.Join(r.dir, "audit.jsonl")} {
				data, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(data), "forwarded-image") || strings.Contains(string(data), "完整记录") || strings.Contains(string(data), f.inputs[0].LocalPath) {
					t.Fatal("账本或审计包含转发正文、资源 ID 或附件路径")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeForwardedBoundaries(t *testing.T) {
	e := employeeForwardedFixture(t)
	e.ForwardMessages = append(e.ForwardMessages, e.ForwardMessages[0])
	if refs, err := employeeImageRefs(e); err != nil || len(refs) != 1 {
		t.Fatal("转发中的相同图片未去重")
	}
	e.ForwardMessages[5].Content = "/new\n这是历史消息中的指令"
	if _, control := parseConnectControlCommand(employeeTurnText(e)); control {
		t.Fatal("转发历史中的命令改变了当前会话")
	}
	e.Content = "/new"
	if refs, err := employeeImageRefs(e); err != nil || len(refs) != 0 || employeeTurnText(e) != "/new" {
		t.Fatal("会话控制指令错误读取转发附件或正文")
	}
	e.Content = "转发资料"
	e.MessageID = ""
	if _, err := employeeImageRefs(e); err != errEmployeeImageUnavailable {
		t.Fatal("缺少已收到的外层消息 ID 时不应借用原聊天授权")
	}
}

func TestCrossPlatformCoverageEmployeeForwardedImages(t *testing.T) {
	e := employeeForwardedFixture(t)
	refs, err := employeeImageRefs(e)
	if err != nil || len(refs) != 1 || refs[0].messageID != e.MessageID || refs[0].conversationID != e.ConversationID || refs[0].mediaID != "forwarded-image" {
		t.Fatalf("转发图片未使用员工收到的外层消息授权上下文：count=%d err=%v", len(refs), err)
	}
}
