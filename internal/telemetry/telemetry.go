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

// Package telemetry hands completed CLI events to a short-lived sender process.
// The command process never initializes the AEM SDK or waits for a network request.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
)

const (
	workerArgument      = "--_dws-telemetry-worker=1"
	protocolVersion     = 2
	maxEventBytes       = 4096
	maxIPCBytes         = 2 * 1024 * 1024
	largeHandoffTimeout = 250 * time.Millisecond
	handoffTimeout      = 10 * time.Millisecond
	workerLifetime      = 6 * time.Second
	maxSenders          = 8
)

var (
	findExecutable = os.Executable
	openPipe       = os.Pipe
	startCommand   = (*exec.Cmd).Start
)

// Identity contains only the existing reviewed profile metadata projection.
type Identity struct {
	UserID   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	CorpID   string `json:"corpId,omitempty"`
}

// Event 描述原始 CLI 命令，不是发送进程自身的执行。基础事件不收集 argv、
// stdout 或工作目录；AgentInfo 单独保留 SDK 返回的进程和环境元数据，
// 由原始 CLI 采集，发送进程不重新识别。
type Event struct {
	Protocol          int        `json:"protocol"`
	Version           string     `json:"version"`
	Command           string     `json:"command"`
	Path              string     `json:"path"`
	ExitCode          int        `json:"exitCode"`
	DurationMillis    int64      `json:"durationMillis"`
	CompletedAtMillis int64      `json:"completedAtMillis"`
	ErrorSummary      string     `json:"errorSummary,omitempty"`
	Identity          Identity   `json:"identity"`
	AgentInfo         *AgentInfo `json:"agentInfo,omitempty"`
}

// AgentInfo preserves unknown SDK fields in a complete JSON object.
type AgentInfo struct {
	State         runtimecontext.State `json:"state"`
	ErrorCode     int32                `json:"errorCode"`
	ErrorCategory string               `json:"errorCategory,omitempty"`
	Result        json.RawMessage      `json:"result,omitempty"`
}

func AgentInfoFromSnapshot(snapshot runtimecontext.AgentSnapshot) *AgentInfo {
	return &AgentInfo{State: snapshot.State, ErrorCode: snapshot.ErrorCode,
		ErrorCategory: snapshot.ErrorCategory, Result: snapshot.JSON()}
}

func normalizeAgentInfo(info *AgentInfo) *AgentInfo {
	if info == nil {
		return nil
	}
	copy := *info
	if len(copy.Result) > runtimecontext.MaxAgentJSONBytes {
		copy.Result, copy.State, copy.ErrorCategory = nil, runtimecontext.AgentOversized, "json_too_large"
	} else if len(copy.Result) > 0 && (!utf8.Valid(copy.Result) || !json.Valid(copy.Result) || bytes.TrimSpace(copy.Result)[0] != '{') {
		copy.Result, copy.State, copy.ErrorCategory = nil, runtimecontext.AgentInvalid, "invalid_json"
	}
	copy.Result = append(json.RawMessage(nil), copy.Result...)
	return &copy
}

// OptedOut preserves the existing DO_NOT_TRACK semantics, including "0".
func OptedOut() bool { return strings.TrimSpace(os.Getenv("DO_NOT_TRACK")) != "" }

// Submit waits only for a bounded local handoff. A successful handoff is not an
// acknowledgement of delivery. Incomplete handoffs are discarded by the worker.
func Submit(event Event) {
	if OptedOut() {
		return
	}
	budget := handoffTimeout
	if event.AgentInfo != nil && len(event.AgentInfo.Result) > maxEventBytes {
		budget = largeHandoffTimeout
	}
	submit(event, budget, launchDetached)
}

func submit(event Event, budget time.Duration, launch func(context.Context, []byte) error) {
	defer func() { _ = recover() }()
	event.Protocol = protocolVersion
	event.AgentInfo = normalizeAgentInfo(event.AgentInfo)
	if !validEvent(event) {
		return
	}
	payload, err := encodeTelemetryJSON(event)
	if err != nil || len(payload) > maxIPCBytes {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	done := make(chan struct{}, 1)
	go func() {
		defer func() { _ = recover(); done <- struct{}{} }()
		_ = launch(ctx, payload)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Keep the bounded SDK object's representation from expanding during IPC
// (notably '<', '>' and '&' in process command lines). RawMessage still carries
// every unknown field and no partial JSON is emitted.
func encodeTelemetryJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func validEvent(e Event) bool {
	// Reject oversized profile metadata before JSON encoding on the exit path.
	// The reporting protocol accepts the CLI's portable 0..255 exit codes;
	// larger native Windows exit codes are intentionally discarded.
	baseValid := e.Protocol == protocolVersion && e.Command != "" && e.Path != "" &&
		e.DurationMillis >= 0 && e.DurationMillis <= int64((1<<63-1)/time.Millisecond) &&
		e.CompletedAtMillis > 0 && e.ExitCode >= 0 && e.ExitCode <= 255 &&
		len(e.Version)+len(e.Command)+len(e.Path)+len(e.ErrorSummary)+
			len(e.Identity.UserID)+len(e.Identity.UserName)+len(e.Identity.CorpID) <= maxEventBytes
	if !baseValid {
		return false
	}
	base := e
	base.AgentInfo = nil
	raw, err := encodeTelemetryJSON(base)
	if err != nil || len(raw) > maxEventBytes {
		return false
	}
	if e.AgentInfo == nil {
		return true
	}
	info := e.AgentInfo
	if len(info.ErrorCategory) > 128 || len(info.Result) > runtimecontext.MaxAgentJSONBytes {
		return false
	}
	switch info.State {
	case runtimecontext.StateReady, runtimecontext.AgentUnknown, runtimecontext.AgentDegraded,
		runtimecontext.AgentInvalid, runtimecontext.AgentOversized, runtimecontext.StateError,
		runtimecontext.StateTimeout, runtimecontext.StateUnavailable:
	default:
		return false
	}
	return len(info.Result) == 0 || (utf8.Valid(info.Result) && json.Valid(info.Result) && bytes.TrimSpace(info.Result)[0] == '{')
}

// launchDetached uses actual file handles rather than os/exec's stream-copy
// goroutines. Thus parent exit does not interrupt a successfully buffered event,
// and the worker cannot retain a caller's stdout/stderr pipe or controlling TTY.
func launchDetached(ctx context.Context, payload []byte) error {
	executable, err := findExecutable()
	if err != nil {
		return err
	}
	readEnd, writeEnd, err := openPipe()
	if err != nil {
		return err
	}
	defer readEnd.Close()
	defer writeEnd.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command(executable, workerArgument)
	cmd.Stdin = readEnd
	// nil stdout/stderr are connected to os.DevNull by os/exec.
	detach(cmd)
	if err := startCommand(cmd); err != nil {
		return err
	}
	_ = readEnd.Close()
	go func() { _ = cmd.Wait() }()
	stopCancel := context.AfterFunc(ctx, func() {
		_ = writeEnd.Close()
		// Cancellation may race with a completed handoff. Killing the sender
		// can lose that event; retaining the bounded, best-effort lifecycle wins.
		_ = cmd.Process.Kill()
	})
	defer stopCancel()
	if err := ctx.Err(); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_, err = writeEnd.Write(payload)
	if err != nil {
		_ = cmd.Process.Kill()
	}
	return err
}
