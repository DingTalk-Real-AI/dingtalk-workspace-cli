// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeFeedbackPersistenceFailures(t *testing.T) {
	for _, mode := range []string{"final-save", "remove-save", "add-save", "cache-save"} {
		t.Run(mode, func(t *testing.T) {
			f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
			defer f.wait()
			e := employeeEvent{ConversationID: "c", MessageID: "m"}
			record := employeeFeedbackRecord{ConversationID: e.ConversationID, MessageID: e.MessageID}
			f.emotions["已完成"] = "new-id"
			var operations []string
			testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, _, operation string, _ employeeEvent, _ employeeEmotion) (map[string]any, error) {
				operations = append(operations, operation)
				return map[string]any{"emotionId": "new-id"}, nil
			})
			writes := 0
			boom := errors.New("private disk error")
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				writes++
				if mode == "final-save" && writes == 1 {
					return os.Rename(src, dst)
				}
				return boom
			})
			var err error
			switch mode {
			case "final-save":
				record.Applied = "已完成"
				record.Owned = []employeeEmotion{{ID: "new-id", Label: "已完成"}}
				err = f.apply(context.Background(), e, &record, "已完成")
				if writes != 2 {
					t.Fatalf("未验证收尾落盘：writes=%d", writes)
				}
			case "remove-save":
				record.Applied = "思考中"
				record.Owned = []employeeEmotion{{ID: "old-id", Label: "思考中"}}
				err = f.replace(context.Background(), e, &record, "已完成")
				if len(record.Owned) != 0 || record.Applied != "" {
					t.Fatal("撤旧成功后仍保留旧的已应用事实")
				}
			case "add-save":
				err = f.replace(context.Background(), e, &record, "已完成")
				if len(record.Owned) != 1 {
					t.Fatal("贴新前未记录准备投影")
				}
			case "cache-save":
				f.emotions = map[string]string{}
				_, err = f.emotionID(context.Background(), e, "已完成")
				if _, ok := f.emotions["已完成"]; ok {
					t.Fatal("未持久化的表情 ID 被缓存为成功")
				}
			}
			if !errors.Is(err, boom) {
				t.Fatalf("落盘失败被忽略：%v", err)
			}
			expected := map[string]string{"remove-save": "remove-text-emotion", "cache-save": "create-text-emotion"}[mode]
			if expected == "" && len(operations) != 0 || expected != "" && (len(operations) != 1 || operations[0] != expected) {
				t.Fatalf("落盘失败后继续调用外部表情接口：%v", operations)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackUnavailableRecords(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "unreadable", "cache-unreadable", "cache-null"} {
		t.Run(mode, func(t *testing.T) {
			f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
			defer f.wait()
			e := employeeEvent{ConversationID: "c", MessageID: "m"}
			calls := 0
			testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
				calls++
				return map[string]any{"emotionId": "new-id"}, nil
			})
			switch mode {
			case "missing":
				f.recover(e, "已完成")
			case "corrupt":
				if err := writeEmployeeJSON(f.path(e), "corrupt"); err != nil {
					t.Fatal(err)
				}
				f.recover(e, "已完成")
			case "unreadable":
				if err := os.MkdirAll(f.path(e), 0700); err != nil {
					t.Fatal(err)
				}
				f.set(e, "已完成", true)
				f.wait()
			case "cache-unreadable":
				if err := os.Mkdir(filepath.Join(f.dir, "feedback-emotions.json"), 0700); err != nil {
					t.Fatal(err)
				}
				if _, err := f.emotionID(context.Background(), e, "已完成"); err == nil {
					t.Fatal("无法读取缓存仍创建表情")
				}
			case "cache-null":
				if err := os.WriteFile(filepath.Join(f.dir, "feedback-emotions.json"), []byte("null"), 0600); err != nil {
					t.Fatal(err)
				}
				id, err := f.emotionID(context.Background(), e, "已完成")
				if err != nil || id != "new-id" || f.emotions["已完成"] != id {
					t.Fatalf("空缓存未正常初始化：%q %v", id, err)
				}
			}
			want := 0
			if mode == "cache-null" {
				want = 1
			}
			if calls != want {
				t.Fatalf("不完整记录触发外部操作：calls=%d want=%d", calls, want)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackCommandUnavailable(t *testing.T) {
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("private executable error") })
	_, err := runEmployeeFeedbackCall(context.Background(), "corp:employee", "create-text-emotion", employeeEvent{}, employeeEmotion{Label: "思考中"})
	if err == nil || err.Error() != "feedback_command_unavailable" {
		t.Fatalf("未安全报告命令不可用：%v", err)
	}
}

func TestCrossPlatformCoverageEmployeeUnconfirmedCompletion(t *testing.T) {
	for _, record := range []employeeTaskRecord{
		{Status: "completed_without_reply", Execution: "failure", Delivery: "not_required"},
		{Status: "needs_review", Delivery: "failure"},
	} {
		if got := employeeTaskLabel(record); got != "已中断，待核查" {
			t.Fatalf("未确认事实被标为完成：%+v => %s", record, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fwd := employeeFeedbackForwarder(func(context.Context, string, string) (string, error) { return "", errCodexCompletedWithoutReply })
	if _, err := forwardEmployeeTurn(ctx, fwd, "conversation", "question"); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消被当成无正文成功：%v", err)
	}
}
