// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type digitalEmployeeRunState struct {
	TransportReady bool      `json:"transportReady"`
	ExecutorReady  bool      `json:"executorReady"`
	SourceState    string    `json:"sourceState,omitempty"`
	RunID          string    `json:"runId"`
	Status         string    `json:"status"`
	PID            int       `json:"pid"`
	SupervisorPID  int       `json:"supervisorPid,omitempty"`
	AgentUUID      string    `json:"agentUuid"`
	Profile        string    `json:"dwsProfile"`
	Channel        string    `json:"channel"`
	LogPath        string    `json:"logPath,omitempty"`
	Code           string    `json:"code,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type employeeEvent struct {
	Type           string `json:"type"`
	EventID        string `json:"event_id"`
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_open_dingtalk_id"`
	Content        string `json:"content"`
}

type employeeTaskRecord struct {
	EventID        string    `json:"eventId"`
	MessageID      string    `json:"messageId"`
	ConversationID string    `json:"conversationId"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotencyKey"`
	ReplyID        string    `json:"replyId,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type employeeRetryState struct {
	Failures        int       `json:"failures"`
	UnknownFailures int       `json:"unknownFailures"`
	Terminal        bool      `json:"terminal"`
	NextAttempt     time.Time `json:"nextAttempt"`
}

// 所有错误只携带稳定分类，禁止把 MCP/Agent 的原始输出写到日志。
type employeeRunError struct {
	Code       string
	Retryable  *bool
	RetryAfter time.Duration
}

func (e *employeeRunError) Error() string { return e.Code }
func employeeTerminal(code string) error {
	no := false
	return &employeeRunError{Code: code, Retryable: &no}
}

var employeeReadyLine = regexp.MustCompile(`^\[event\] ready(?:\s|$)`)
var employeeExecCommand = exec.CommandContext

func readDigitalEmployeeState(dir string) (digitalEmployeeRunState, error) {
	var s digitalEmployeeRunState
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	return s, err
}

func employeeCommand(ctx context.Context, profile string, args ...string) (*exec.Cmd, error) {
	bin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	argv := append([]string{"--profile", profile}, args...)
	cmd := employeeExecCommand(ctx, bin, argv...)
	// 强制使用磁盘 Profile，不继承父进程的 OAuth 应用覆盖。
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "DWS_CLIENT_ID", "DWS_CLIENT_SECRET", "DWS_ACCESS_TOKEN", "DWS_TOKEN", "DWS_DUMP_RAW":
			continue
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "DWS_DUMP_RAW=0")
	return cmd, nil
}

// boundedBuffer 防止子进程输出耗尽内存；超限仍消费数据但让调用失败。
type employeeBoundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *employeeBoundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := digitalEmployeeStdinLimit - b.Len()
	if remaining < n {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func employeeMachineCall(ctx context.Context, profile string, payload any, args ...string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd, err := employeeCommand(ctx, profile, args...)
	if err != nil {
		return nil, employeeTerminal("dws_command_unavailable")
	}
	if payload != nil {
		data, e := json.Marshal(payload)
		if e != nil || len(data) > digitalEmployeeStdinLimit {
			return nil, employeeTerminal("invalid_payload")
		}
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr employeeBoundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if stdout.overflow || err != nil || json.Unmarshal(stdout.Bytes(), &envelope) != nil || !envelope.OK || envelope.Data == nil {
		return nil, &employeeRunError{Code: "dws_machine_failed"}
	}
	return envelope.Data, nil
}

type employeeRuntime struct {
	cfg    digitalEmployeeAdapterConfig
	dir    string
	fwd    forwarder
	mu     sync.Mutex
	queues map[string]chan employeeEvent
	wg     sync.WaitGroup
	fatal  chan error
	ctx    context.Context
}

func (r *employeeRuntime) audit(record employeeTaskRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return employeeTerminal("audit_unavailable")
	}
	file, err := os.OpenFile(filepath.Join(r.dir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return employeeTerminal("audit_unavailable")
	}
	defer file.Close()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		return employeeTerminal("audit_unavailable")
	}
	return nil
}

func (r *employeeRuntime) recordPath(e employeeEvent) string {
	id := sha256.Sum256([]byte(e.ConversationID + "\x00" + e.MessageID))
	return filepath.Join(r.dir, "tasks", fmt.Sprintf("%x.json", id[:]))
}

func employeeContains(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (r *employeeRuntime) accept(e employeeEvent) bool {
	if e.SenderID == r.cfg.SelfOpenDingTalkID {
		return false
	}
	if !validMachineString(e.EventID) || !validMachineString(e.MessageID) || !validMachineString(e.ConversationID) || !validMachineString(e.SenderID) || strings.TrimSpace(e.Content) == "" {
		return false
	}
	if !employeeContains(r.cfg.Options.AllowedUsers, e.SenderID) {
		return false
	}
	switch e.Type {
	case "user_im_message_receive_o2o_all", "user_im_message_receive_o2o":
		return true
	case "user_im_message_receive_group_all", "user_im_message_receive_group", "user_im_message_receive_at":
		return employeeContains(r.cfg.Options.AllowedGroups, e.ConversationID)
	default:
		return false
	}
}

func (r *employeeRuntime) enqueue(e employeeEvent) error {
	if !r.accept(e) {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.recordPath(e)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return employeeTerminal("ledger_unavailable")
	}
	key := sha256.Sum256([]byte(r.cfg.Binding.DWSProfile + "\x00" + e.ConversationID + "\x00" + e.MessageID))
	record := employeeTaskRecord{EventID: e.EventID, MessageID: e.MessageID, ConversationID: e.ConversationID, Status: "accepted", IdempotencyKey: fmt.Sprintf("%x", key[:16]), UpdatedAt: time.Now()}
	if err := r.audit(record); err != nil {
		return err
	}
	if err := writeEmployeeJSON(path, record); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	q := r.queues[e.ConversationID]
	if q == nil {
		if len(r.queues) >= 128 {
			return employeeTerminal("conversation_capacity")
		}
		q = make(chan employeeEvent, 32)
		r.queues[e.ConversationID] = q
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			for {
				select {
				case <-r.ctx.Done():
					return
				case event := <-q:
					if err := r.process(event); err != nil {
						select {
						case r.fatal <- err:
						default:
						}
						return
					}
				}
			}
		}()
	}
	select {
	case q <- e:
		return nil
	default:
		return employeeTerminal("queue_capacity")
	}
}

func (r *employeeRuntime) process(e employeeEvent) error {
	if r.ctx.Err() != nil {
		return nil
	}
	var record employeeTaskRecord
	raw, err := os.ReadFile(r.recordPath(e))
	if err != nil || json.Unmarshal(raw, &record) != nil {
		return employeeTerminal("ledger_unavailable")
	}
	answer, err := forwardEmployeeTurn(r.ctx, r.fwd, e.ConversationID, e.Content)
	if err != nil {
		record.Status = "agent_failed"
	} else if strings.TrimSpace(answer) == "" {
		record.Status = "empty_reply"
	} else {
		record.Status = "sending"
		if err = writeEmployeeJSON(r.recordPath(e), record); err != nil {
			return employeeTerminal("ledger_unavailable")
		}
		payload := digitalEmployeeReplyInput{BindingRevision: r.cfg.Binding.BindingRevision, SchemaVersion: 1, ProtocolVersion: 1, AgentUUID: r.cfg.Binding.AgentUUID, EventID: e.EventID, ConversationID: e.ConversationID, ReferenceMessageID: e.MessageID, Text: answer, IdempotencyKey: record.IdempotencyKey}
		result, sendErr := employeeMachineCall(r.ctx, r.cfg.Binding.DWSProfile, payload, "dingtalk-tag", "channel", "reply", "--channel", r.cfg.Binding.Channel, "--stdin", "--format", "json")
		record.Status = "needs_review"
		if sendErr == nil {
			record.ReplyID = jsonScalar(result["openMessageId"])
			if jsonScalar(result["deliveryStatus"]) == "delivered" && record.ReplyID != "" {
				record.Status = "delivered"
			}
		}
	}
	record.UpdatedAt = time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.audit(record); err != nil {
		return err
	}
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	return nil
}

func runDigitalEmployeeForeground(cmd *cobra.Command, cfg digitalEmployeeAdapterConfig) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := runEmployeeWorker(ctx, cfg, 0)
	if err != nil {
		return err
	}
	return writeDWSMachineEnvelope(cmd, map[string]any{"status": "stopped", "agentUuid": cfg.Binding.AgentUUID, "dwsProfile": cfg.Binding.DWSProfile, "channel": cfg.Binding.Channel, "restartRequired": false})
}

func runEmployeeWorker(parent context.Context, cfg digitalEmployeeAdapterConfig, supervisorPID int) (runErr error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	lock, err := auth.AcquireDualLock(ctx, filepath.Join(dir, "worker"))
	if err != nil {
		return employeeTerminal("worker_already_running")
	}
	defer lock.Release()
	if err := checkEmployeeLeaseGuard(cfg.Binding.DWSProfile); err != nil {
		return err
	}
	activation, err := auth.AcquireDualLock(ctx, filepath.Join(dir, "activation"))
	if err != nil {
		return err
	}
	defer activation.Release()
	binding, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding.DWSProfile)
	if err != nil || !sameEmployeeBinding(binding, cfg.Binding) || employeeBindingState(binding) != "bound" || employeeDesiredState(binding) != "running" {
		return employeeTerminal("binding_not_authorized")
	}
	state := digitalEmployeeRunState{Status: "starting", PID: os.Getpid(), SupervisorPID: supervisorPID, AgentUUID: cfg.Binding.AgentUUID, Profile: cfg.Binding.DWSProfile, Channel: cfg.Binding.Channel, LogPath: filepath.Join(dir, "runtime.log"), UpdatedAt: time.Now()}
	state.RunID = os.Getenv("DWS_EMPLOYEE_RUN_ID")
	if state.RunID == "" {
		state.RunID = uuid.NewString()
	}
	go employeeWatchStop(ctx, cancel, dir, state.RunID)
	persist := func() error { return persistEmployeeState(dir, state) }
	if err := persist(); err != nil {
		return employeeTerminal("state_unavailable")
	}
	activation.Release()
	defer func() {
		state.Status = "stopped"
		state.TransportReady, state.ExecutorReady = false, false
		if runErr != nil {
			state.Status = "blocked"
			state.Code = runErr.Error()
		}
		state.UpdatedAt = time.Now()
		_ = persist()
	}()
	fwd, err := digitalEmployeeNewForwarder(ctx, cfg)
	if err != nil {
		return employeeTerminal("agent_unavailable")
	}
	if closer, ok := fwd.(forwarderCloser); ok {
		defer closer.close()
	}
	if err := prepareEmployeeForwarder(ctx, fwd); err != nil {
		return employeeTerminal("agent_initialization_failed")
	}
	state.ExecutorReady = true
	if err := persist(); err != nil {
		return employeeTerminal("state_unavailable")
	}
	r := &employeeRuntime{cfg: cfg, dir: dir, fwd: fwd, queues: map[string]chan employeeEvent{}, fatal: make(chan error, 1), ctx: ctx}
	if err = r.audit(employeeTaskRecord{Status: "starting", UpdatedAt: time.Now()}); err != nil {
		return err
	}
	if err = r.recoverInterruptedTasks(); err != nil {
		return err
	}
	defer func() { cancel(); r.wg.Wait() }()
	args := []string{"event", "consume", "user_im_message_receive_o2o_all"}
	if len(cfg.Options.AllowedGroups) > 0 {
		args = append(args, "user_im_message_receive_group_all")
	}
	args = append(args, "--stream-source-id", "digital_employee", "--flatten", "--format", "ndjson")
	proc, err := employeeCommand(ctx, cfg.Binding.DWSProfile, args...)
	if err != nil {
		return employeeTerminal("consumer_unavailable")
	}
	// 使用 stdin EOF/SIGTERM 清理订阅，不让 CommandContext 立即 SIGKILL。
	stdin, err := proc.StdinPipe()
	if err != nil {
		return employeeTerminal("consumer_unavailable")
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		return employeeTerminal("consumer_unavailable")
	}
	stderr, err := proc.StderrPipe()
	if err != nil {
		return employeeTerminal("consumer_unavailable")
	}
	proc.Cancel = func() error { _ = stdin.Close(); return proc.Process.Signal(syscall.SIGTERM) }
	proc.WaitDelay = 5 * time.Second
	if err = proc.Start(); err != nil {
		return employeeTerminal("consumer_unavailable")
	}
	ready := make(chan struct{}, 1)
	transportStates := make(chan transport.StatusSource, 16)
	lines := make(chan []byte, 128)
	readErr := make(chan error, 2)
	var readWG sync.WaitGroup
	var failureMu sync.Mutex
	failure := &employeeRunError{Code: "consumer_exit"}
	readWG.Add(2)
	go func() {
		defer readWG.Done()
		var combined employeeBoundedBuffer
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), digitalEmployeeStdinLimit)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = combined.Write(append(append([]byte(nil), scanner.Bytes()...), '\n'))
			if employeeReadyLine.MatchString(line) {
				select {
				case ready <- struct{}{}:
				default:
				}
			} else if raw, ok := strings.CutPrefix(line, "[event] transport "); ok {
				var s transport.StatusSource
				if json.Unmarshal([]byte(raw), &s) == nil {
					select {
					case transportStates <- s:
					case <-ctx.Done():
						return
					}
				}
			} else {
				failureMu.Lock()
				employeeReadRetryHint(line, failure)
				failureMu.Unlock()
			}
		}
		if !combined.overflow {
			failureMu.Lock()
			employeeReadRetryHint(combined.String(), failure)
			failureMu.Unlock()
		}
		if scanner.Err() != nil {
			select {
			case readErr <- employeeTerminal("consumer_stderr_invalid"):
			default:
			}
		}
	}()
	go func() {
		defer readWG.Done()
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), digitalEmployeeStdinLimit)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		if scanner.Err() != nil {
			select {
			case readErr <- employeeTerminal("consumer_event_oversized"):
			default:
			}
		}
	}()
	done := make(chan struct{})
	go func() { readWG.Wait(); _ = proc.Wait(); close(done) }()
	defer func() {
		cancel()
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(7 * time.Second):
			_ = proc.Process.Kill()
		}
	}()
	timer := time.NewTimer(digitalEmployeeReadyTimeout)
	defer timer.Stop()
	updateTransport := func(s transport.StatusSource) error {
		if !s.Observed {
			return employeeTerminal("event_bus_upgrade_required")
		}
		state.SourceState = s.State
		state.TransportReady = s.Observed && (s.State == "connected" || s.State == "idle")
		state.UpdatedAt = time.Now()
		if err := persist(); err != nil {
			return employeeTerminal("state_unavailable")
		}
		return nil
	}
	ipcReady := false
	for !ipcReady || !state.TransportReady {
		select {
		case <-ready:
			ipcReady = true
		case s := <-transportStates:
			if err := updateTransport(s); err != nil {
				return err
			}
		case <-timer.C:
			return &employeeRunError{Code: "ready_timeout"}
		case <-ctx.Done():
			return nil
		case <-done:
			// CommandContext may close the consumer pipes before select observes
			// cancellation (especially on Windows). An explicit stop is not a crash.
			if ctx.Err() != nil {
				return nil
			}
			failureMu.Lock()
			defer failureMu.Unlock()
			return failure
		}
	}
	state.Status = "running"
	state.UpdatedAt = time.Now()
	if err := persist(); err != nil {
		return employeeTerminal("state_unavailable")
	}
	for {
		select {
		case s := <-transportStates:
			if err := updateTransport(s); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		case err := <-r.fatal:
			return err
		case err := <-readErr:
			return err
		case line, ok := <-lines:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				failureMu.Lock()
				defer failureMu.Unlock()
				return failure
			}
			var e employeeEvent
			if json.Unmarshal(line, &e) != nil {
				return employeeTerminal("invalid_event")
			}
			if err = r.enqueue(e); err != nil {
				return err
			}
		}
	}
}

func employeeReadRetryHint(line string, failure *employeeRunError) {
	var value any
	if json.Unmarshal([]byte(line), &value) != nil {
		return
	}
	var walk func(any)
	walk = func(v any) {
		switch obj := v.(type) {
		case map[string]any:
			if raw, ok := obj["next_retry_at"].(string); ok {
				if at, err := time.Parse(time.RFC3339, raw); err == nil && time.Until(at) > failure.RetryAfter {
					failure.RetryAfter = time.Until(at)
				}
			}
			if status, _ := obj["state"].(string); status == "in_flight" || status == "terminal_hold" {
				no := false
				failure.Retryable = &no
				failure.Code = "subscription_guard"
			}
			if b, ok := obj["retryable"].(bool); ok && (failure.Retryable == nil || *failure.Retryable) {
				failure.Retryable = &b
			}
			if sec, ok := obj["retry_after_seconds"].(float64); ok && sec > 0 {
				if delay := time.Duration(sec * float64(time.Second)); delay > failure.RetryAfter {
					failure.RetryAfter = delay
				}
			}
			for _, child := range obj {
				walk(child)
			}
		case []any:
			for _, child := range obj {
				walk(child)
			}
		}
	}
	walk(value)
}

func persistEmployeeState(dir string, state digitalEmployeeRunState) error {
	if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), state); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (r *employeeRuntime) recoverInterruptedTasks() error {
	taskDir := filepath.Join(r.dir, "tasks")
	info, err := os.Stat(taskDir)
	if os.IsNotExist(err) {
		return nil
	}
	// Windows ReadDir may classify an existing regular file as not found.
	// Only a genuinely absent ledger may be treated as a first startup.
	if err != nil || !info.IsDir() {
		return employeeTerminal("ledger_unavailable")
	}
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(r.dir, "tasks", entry.Name())
		data, e := os.ReadFile(path)
		if e != nil {
			return employeeTerminal("ledger_unavailable")
		}
		var record employeeTaskRecord
		if json.Unmarshal(data, &record) != nil {
			return employeeTerminal("ledger_unavailable")
		}
		if record.Status == "accepted" || record.Status == "sending" {
			record.Status = "needs_review"
			record.UpdatedAt = time.Now()
			if e = r.audit(record); e != nil {
				return e
			}
			if e = writeEmployeeJSON(path, record); e != nil {
				return employeeTerminal("ledger_unavailable")
			}
		}
	}
	return nil
}

func employeeNextRetry(s employeeRetryState, err error, now time.Time) (employeeRetryState, bool) {
	s.Failures++
	var failure *employeeRunError
	if !errors.As(err, &failure) {
		failure = &employeeRunError{Code: "worker_failed"}
	}
	if failure.Retryable != nil && !*failure.Retryable {
		s.Terminal = true
		return s, false
	}
	if failure.Retryable == nil {
		s.UnknownFailures++
	}
	if s.Failures > 2 || s.UnknownFailures > 1 {
		s.Terminal = true
		return s, false
	}
	delay := time.Duration(s.Failures) * time.Second
	if failure.RetryAfter > delay {
		delay = failure.RetryAfter
	}
	s.NextAttempt = now.Add(delay)
	return s, true
}
