// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 表情是独立的展示投影，不参与业务或正文发送重试。每条消息串行更新，
// 期间的新状态合并为最新期望，避免较慢的“思考中”覆盖已经发送的回复。
type employeeFeedback struct {
	ctx          context.Context
	cancel       context.CancelFunc
	profile, dir string
	mu           sync.Mutex
	items        map[string]*employeeFeedbackItem
	wg           sync.WaitGroup
	emotionsMu   sync.Mutex
	emotions     map[string]string
}

type employeeEmotion struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type employeeFeedbackRecord struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Desired        string `json:"desired"`
	// Owned 同时包含接口结果未知的表情；恢复时仅清理本员工实际尝试添加的 ID。
	Owned     []employeeEmotion `json:"owned,omitempty"`
	Applied   string            `json:"applied,omitempty"`
	Status    string            `json:"status"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type employeeFeedbackItem struct {
	mu       sync.Mutex
	e        employeeEvent
	label    string
	terminal bool
	wake     chan struct{}
}

func newEmployeeFeedback(parent context.Context, profile, dir string) *employeeFeedback {
	ctx, cancel := context.WithCancel(parent)
	return &employeeFeedback{ctx: ctx, cancel: cancel, profile: profile, dir: dir, items: map[string]*employeeFeedbackItem{}, emotions: map[string]string{}}
}

func (f *employeeFeedback) wait() {
	f.cancel()
	f.wg.Wait()
}

func (f *employeeFeedback) set(e employeeEvent, label string, terminal bool) {
	if f == nil || !validMachineString(e.ConversationID) || !validMachineString(e.MessageID) {
		return
	}
	key := e.ConversationID + "\x00" + e.MessageID
	f.mu.Lock()
	defer f.mu.Unlock()
	item := f.items[key]
	if item == nil {
		item = &employeeFeedbackItem{e: e, wake: make(chan struct{}, 1)}
		f.items[key] = item
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer func() {
				f.mu.Lock()
				delete(f.items, key)
				f.mu.Unlock()
			}()
			f.run(item)
		}()
	}
	item.mu.Lock()
	// 已提交终态的同一轮不能被迟到的非终态覆盖。
	if !item.terminal {
		item.label, item.terminal = label, terminal
	}
	item.mu.Unlock()
	select {
	case item.wake <- struct{}{}:
	default:
	}
}

// 只对本版本留下的未收敛投影做恢复；不回补历史消息，不重发正文或重跑 Agent。
func (f *employeeFeedback) recover(e employeeEvent, label string) {
	if f == nil {
		return
	}
	raw, err := os.ReadFile(f.path(e))
	var record employeeFeedbackRecord
	if err != nil || json.Unmarshal(raw, &record) != nil {
		return
	}
	if record.Status != "applied" || record.Applied != label {
		f.set(e, label, true)
	}
}

func (f *employeeFeedback) path(e employeeEvent) string {
	r := employeeRuntime{dir: f.dir}
	return filepath.Join(f.dir, "feedback", filepath.Base(r.recordPath(e)))
}

func (f *employeeFeedback) run(item *employeeFeedbackItem) {
	record := employeeFeedbackRecord{ConversationID: item.e.ConversationID, MessageID: item.e.MessageID}
	if raw, err := os.ReadFile(f.path(item.e)); err == nil {
		if json.Unmarshal(raw, &record) != nil || record.ConversationID != item.e.ConversationID || record.MessageID != item.e.MessageID {
			return // 归属不明时不撤除任何表情。
		}
	} else if !os.IsNotExist(err) {
		return
	}
	for {
		select {
		case <-item.wake:
		case <-f.ctx.Done():
		}
		item.mu.Lock()
		label, terminal := item.label, item.terminal
		item.mu.Unlock()
		stopping := f.ctx.Err() != nil
		if stopping && !terminal {
			label, terminal = "已中断，待核查", true
		}
		ctx := f.ctx
		if stopping {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := f.apply(ctx, item.e, &record, label)
		cancel()
		if err != nil {
			// 只记录稳定分类，不输出上游错误、正文或凭据。
			fmt.Fprintln(os.Stderr, "[employee] message_feedback_failed")
		}
		if terminal || stopping {
			return
		}
	}
}

func (f *employeeFeedback) save(e employeeEvent, record *employeeFeedbackRecord) error {
	record.UpdatedAt = time.Now()
	return writeEmployeeJSON(f.path(e), record)
}

func (f *employeeFeedback) apply(ctx context.Context, e employeeEvent, record *employeeFeedbackRecord, label string) error {
	record.Desired, record.Status = label, "updating"
	if err := f.save(e, record); err != nil {
		return err
	}
	operationErr := f.replace(ctx, e, record, label)
	record.Status = "applied"
	if operationErr != nil {
		record.Status = "failed"
	}
	if err := f.save(e, record); err != nil {
		return err
	}
	return operationErr
}

func (f *employeeFeedback) replace(ctx context.Context, e employeeEvent, record *employeeFeedbackRecord, label string) error {
	if record.Applied == label && len(record.Owned) == 1 {
		return nil
	}
	for len(record.Owned) > 0 {
		old := record.Owned[0]
		if _, err := employeeFeedbackCall(ctx, f.profile, "remove-text-emotion", e, old); err != nil {
			return err
		}
		record.Owned, record.Applied = record.Owned[1:], ""
		if err := f.save(e, record); err != nil {
			return err
		}
	}
	id, err := f.emotionID(ctx, e, label)
	if err != nil {
		return err
	}
	emotion := employeeEmotion{ID: id, Label: label}
	// 发送前记录 ID，进程在接口返回前退出也能在下次启动尝试清理。
	record.Owned = []employeeEmotion{emotion}
	if err := f.save(e, record); err != nil {
		return err
	}
	if _, err := employeeFeedbackCall(ctx, f.profile, "add-text-emotion", e, emotion); err != nil {
		return err
	}
	record.Applied = label
	return nil
}

// 每个员工只创建并保存有限个状态定义；重启后复用真实返回的 ID。
func (f *employeeFeedback) emotionID(ctx context.Context, e employeeEvent, label string) (string, error) {
	f.emotionsMu.Lock()
	defer f.emotionsMu.Unlock()
	path := filepath.Join(f.dir, "feedback-emotions.json")
	if len(f.emotions) == 0 {
		if raw, err := os.ReadFile(path); err == nil {
			var cached map[string]string
			if json.Unmarshal(raw, &cached) != nil {
				return "", fmt.Errorf("emotion_cache_invalid")
			}
			for _, id := range cached {
				if !validMachineString(id) {
					return "", fmt.Errorf("emotion_cache_invalid")
				}
			}
			f.emotions = cached
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("emotion_cache_unavailable")
		}
		if f.emotions == nil {
			f.emotions = map[string]string{}
		}
	}
	if id := f.emotions[label]; validMachineString(id) {
		return id, nil
	}
	created, err := employeeFeedbackCall(ctx, f.profile, "create-text-emotion", e, employeeEmotion{Label: label})
	if err != nil {
		return "", err
	}
	id := findJSONScalar(created, "emotionId")
	if !validMachineString(id) {
		return "", fmt.Errorf("emotion_id_missing")
	}
	f.emotions[label] = id
	if err := writeEmployeeJSON(path, f.emotions); err != nil {
		delete(f.emotions, label)
		return "", err
	}
	return id, nil
}

var employeeFeedbackCall = runEmployeeFeedbackCall

// 复用员工 Profile 和现有 IM 文字表情命令，不使用机器人凭据或固定 emotionId。
func runEmployeeFeedbackCall(ctx context.Context, profile, operation string, e employeeEvent, emotion employeeEmotion) (map[string]any, error) {
	args := []string{"chat", "message", operation, "--emotion-name", emotion.Label, "--text", emotion.Label, "--background-id", "im_bg_1", "--format", "json"}
	if operation != "create-text-emotion" {
		args = append(args, "--conversation-id", e.ConversationID, "--message-id", e.MessageID, "--emotion-id", emotion.ID)
	}
	cmd, err := employeeCommand(ctx, profile, args...)
	if err != nil {
		return nil, fmt.Errorf("feedback_command_unavailable")
	}
	var stdout, stderr employeeBoundedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || stdout.overflow {
		return nil, fmt.Errorf("feedback_command_failed")
	}
	return decodeEmployeeFeedbackResult(stdout.Bytes())
}

func decodeEmployeeFeedbackResult(raw []byte) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil || result == nil {
		return nil, fmt.Errorf("feedback_result_invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("feedback_result_invalid")
	}
	if ok, present := result["ok"]; present {
		if ok != true {
			return nil, fmt.Errorf("feedback_rejected")
		}
		result = businessDataMap(result)
		if success, present := result["success"]; present && success != true {
			return nil, fmt.Errorf("feedback_rejected")
		}
		return result, nil
	}
	if ok, present := result["success"]; present && ok == true {
		return result, nil
	}
	return nil, fmt.Errorf("feedback_result_unconfirmed")
}

func employeeTaskLabel(record employeeTaskRecord) string {
	switch record.Status {
	case "delivered":
		return "已完成"
	case "completed_without_reply":
		if record.Execution == "success" && record.Delivery == "not_required" {
			return "收到"
		}
		return "已中断，待核查"
	case "agent_failed", "empty_reply":
		return "处理失败"
	case "cancelled":
		return "已取消"
	case "needs_review":
		if record.Delivery == "unknown" || record.Delivery == "pending" {
			return "发送待确认"
		}
		return "已中断，待核查"
	default:
		return "已中断，待核查"
	}
}
