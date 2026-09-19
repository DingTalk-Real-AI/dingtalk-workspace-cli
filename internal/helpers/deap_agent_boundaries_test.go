// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeJSONValidationBoundaries(t *testing.T) {
	for _, raw := range []string{"{", "null", "[]", `{"mainProgramType":1}`, `{"mainProgramType":" "}`, `{"unexpected":1}`, `{"responseMode":false}`, `{"responseMode":"other"}`, `{"employeeNo":"` + strings.Repeat("字", 65) + `"}`, `{"positionName":"` + strings.Repeat("字", 129) + `"}`} {
		if _, err := deapAgentProfileJSON(raw); err == nil {
			t.Fatalf("accepted invalid profile %s", raw)
		}
	}
	for _, raw := range []string{"{", "null", "{}"} {
		if _, err := deapAgentJSONArray(raw); err == nil {
			t.Fatalf("accepted non-array %s", raw)
		}
	}
	if value, err := deapAgentJSONArray(`[1,"two"]`); err != nil || len(value.([]any)) != 2 {
		t.Fatalf("array=%v err=%v", value, err)
	}
	if err := writeEmployeeJSON(filepath.Join(t.TempDir(), "config"), make(chan int)); err == nil {
		t.Fatal("encoded unsupported value")
	}
	var empty *deapAgentSkillStageError
	if empty.Unwrap() != nil {
		t.Fatal("nil error has cause")
	}
	cause := errors.New("cause")
	err := &deapAgentSkillStageError{Stage: "upload", Err: cause}
	if !errors.Is(err, cause) {
		t.Fatal("stage error lost cause")
	}
	for _, tc := range []struct {
		input any
		want  string
	}{{json.Number("42"), "42"}, {float64(42), "42"}} {
		if got := jsonScalar(tc.input); got != tc.want {
			t.Fatalf("scalar=%q", got)
		}
	}
}

func TestCrossPlatformCoverageEmployeeAdapterInvalidOptions(t *testing.T) {
	for _, flags := range []map[string]string{
		{"channel": "codex", "local-lease": "true"},
		{"channel": "dsh", "local-lease": "true", "daemon": "true"},
		{"channel": "dsh", "local-lease": "true"},
		{"local-worker": "true", "local-supervise": "true"},
		{"local-worker": "true", "daemon": "true"},
		{"channel": "custom"},
		{"channel": "codex", "agent-cmd": "echo"},
		{"channel": "codex", "agent-timeout": "-1"},
		{"channel": "codex", "agent-workdir": filepath.Join(t.TempDir(), "missing")},
	} {
		cmd := newDeapConnectCommand()
		for name, value := range flags {
			if err := cmd.Flags().Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
		err := validateDigitalEmployeeAdapter(cmd)
		validLease := len(flags) == 2 && flags["local-lease"] == "true" && flags["channel"] == "dsh"
		if (err == nil) != validLease {
			t.Fatalf("options=%v err=%v", flags, err)
		}
	}
	if _, err := digitalEmployeeAdapterFor("unsupported"); err == nil {
		t.Fatal("unknown adapter accepted")
	}
	if _, err := digitalEmployeeAdapterFor("codex"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := digitalEmployeeNewForwarder(ctx, digitalEmployeeAdapterConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeBindingCorruptionAndInvalidIdentity(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:user", OperatorOpenDingTalkID: "operator", Channel: "codex"}
	if err := saveDigitalEmployeeBinding(dir, b); err != nil {
		t.Fatal(err)
	}
	other := b
	other.AgentUUID = "different"
	if err := saveDigitalEmployeeBinding(dir, other); err == nil {
		t.Fatal("overwrote another employee")
	}
	bad := b
	bad.DWSProfile = "corp:invalid"
	bad.SchemaVersion = 2
	if err := saveDigitalEmployeeBinding(dir, bad); err == nil {
		t.Fatal("invalid schema saved")
	}
	for _, raw := range []string{`{"unknown":true}`, `{"schemaVersion":2}`} {
		if err := os.WriteFile(digitalEmployeeBindingPath(dir, b.DWSProfile), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadDigitalEmployeeBinding(dir, b.DWSProfile); err == nil {
			t.Fatal("invalid binding loaded")
		}
		if err := checkDigitalEmployeeBinding(dir, b.DWSProfile, b.AgentUUID, b.Channel); err == nil {
			t.Fatal("corrupt binding allowed")
		}
	}
	previous := auth.RuntimeProfile()
	auth.SetRuntimeProfile("")
	t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
	cmd := newDeapConnectCommand()
	if err := validateEmployeeMachineBinding(cmd, b.AgentUUID); err == nil {
		t.Fatal("implicit profile accepted")
	}
	auth.SetRuntimeProfile(b.DWSProfile)
	if err := validateEmployeeMachineBinding(cmd, b.AgentUUID); err == nil {
		t.Fatal("corrupt machine binding accepted")
	}
}

func TestCrossPlatformCoverageEmployeeLocalConfigurationMustMatchBinding(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	profile := "corp:user"
	if _, err := loadDigitalEmployeeConfig(profile); err == nil {
		t.Fatal("missing config accepted")
	}
	path := filepath.Join(digitalEmployeeRuntimeDir(profile), "adapter.json")
	for _, raw := range []string{"{", `{}`, `{"selfOpenDingTalkId":"self"}`} {
		if err := AtomicWriteJSON(path, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		if _, err := loadDigitalEmployeeConfig(profile); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("channel", "custom")
	if _, err := prepareDigitalEmployeeLocal(cmd, digitalEmployeeBinding{}, "token"); err == nil {
		t.Fatal("missing custom command accepted")
	}
	_ = cmd.Flags().Set("channel", "codex")
	opts, err := prepareDigitalEmployeeLocal(cmd, digitalEmployeeBinding{OperatorOpenDingTalkID: "owner"}, "token")
	if err != nil || len(opts.AllowedUsers) != 1 || opts.AllowedUsers[0] != "owner" {
		t.Fatalf("options=%+v err=%v", opts, err)
	}
	_ = cmd.Flags().Set("allowed-users", "extra")
	caller := &digitalEmployeeProtocolCaller{tokenResponses: map[string][]string{"contact/search_contact_by_key_word": {`{"result":[{"userId":"extra","openDingTalkId":"extra-open"}]}`}}}
	InitDepsForTest(t, caller)
	opts, err = prepareDigitalEmployeeLocal(cmd, digitalEmployeeBinding{OperatorOpenDingTalkID: "owner"}, "token")
	if err != nil || len(opts.AllowedUsers) != 2 || opts.AllowedUsers[1] != "extra-open" {
		t.Fatalf("options=%+v err=%v", opts, err)
	}
	if _, err = prepareDigitalEmployeeLocal(cmd, digitalEmployeeBinding{}, "token"); err == nil {
		t.Fatal("unresolved allowlist accepted")
	}
}

type employeeBoundaryForwarder struct {
	answer string
	err    error
}

func (*employeeBoundaryForwarder) label() string { return "boundary" }
func (f *employeeBoundaryForwarder) forward(context.Context, string, string) (string, error) {
	return f.answer, f.err
}

type employeeResetForwarder struct {
	employeeBoundaryForwarder
	reset string
}

func (f *employeeResetForwarder) resetSession(id string) { f.reset = id }

type employeeClearForwarder struct {
	employeeResetForwarder
	clear    string
	clearErr error
}

func (f *employeeClearForwarder) clearSession(_ context.Context, id string) error {
	f.clear = id
	return f.clearErr
}

func TestCrossPlatformCoverageEmployeeTurnControls(t *testing.T) {
	ctx := context.Background()
	f := &employeeBoundaryForwarder{answer: "answer"}
	if err := prepareEmployeeForwarder(ctx, f); err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{nil, {"dws-nonexistent-test-executable"}} {
		if err := prepareEmployeeForwarder(ctx, &execForwarder{argv: argv}); err == nil {
			t.Fatal("invalid executable accepted")
		}
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareEmployeeForwarder(ctx, &execForwarder{argv: []string{bin}}); err != nil {
		t.Fatal(err)
	}
	if got, err := forwardEmployeeTurn(ctx, f, "conversation", "question"); err != nil || got != "answer" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := forwardEmployeeTurn(ctx, f, "conversation", "/new"); err != nil || !strings.Contains(got, "不支持") {
		t.Fatalf("%q %v", got, err)
	}
	reset := &employeeResetForwarder{}
	if _, err := forwardEmployeeTurn(ctx, reset, "conversation", "/new"); err != nil || reset.reset != "conversation" {
		t.Fatal("session not reset", err)
	}
	clear := &employeeClearForwarder{}
	if _, err := forwardEmployeeTurn(ctx, clear, "conversation", "/clear"); err != nil || clear.clear != "conversation" {
		t.Fatal("session not cleared", err)
	}
	clear.clearErr = errors.New("private upstream error")
	if _, err := forwardEmployeeTurn(ctx, clear, "conversation", "/clear"); err == nil || err.Error() != "session_clear_failed" {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestCrossPlatformCoverageEmployeeInterruptedLedgerRecovery(t *testing.T) {
	dir := t.TempDir()
	r := &employeeRuntime{dir: dir}
	if err := r.recoverInterruptedTasks(); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(dir, "tasks")
	if err := os.WriteFile(tasks, []byte("not directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverInterruptedTasks(); err == nil {
		t.Fatal("non-directory ledger accepted")
	}
	if err := os.Remove(tasks); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tasks, "ignored"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasks, "ignored.txt"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"accepted", "sending", "delivered"} {
		if err := writeEmployeeJSON(filepath.Join(tasks, status+".json"), employeeTaskRecord{Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.recoverInterruptedTasks(); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"accepted", "sending", "delivered"} {
		data, err := os.ReadFile(filepath.Join(tasks, status+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var got employeeTaskRecord
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		want := "needs_review"
		if status == "delivered" {
			want = status
		}
		if got.Status != want {
			t.Fatalf("%s became %s", status, got.Status)
		}
	}
	if err := os.WriteFile(filepath.Join(tasks, "corrupt.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverInterruptedTasks(); err == nil {
		t.Fatal("corrupt ledger accepted")
	}
	if err := r.audit(employeeTaskRecord{UpdatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("unencodable audit accepted")
	}
}

func TestCrossPlatformCoverageEmployeeControlOutputBounded(t *testing.T) {
	var buf bytes.Buffer
	w := employeeControlWriter{buf: &buf, limit: 3}
	if n, err := w.Write([]byte("abc")); err != nil || n != 3 || buf.String() != "abc" {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, err := w.Write([]byte("x")); err == nil || n != 0 {
		t.Fatal("overflow accepted")
	}
	w = employeeControlWriter{limit: 3}
	if n, err := w.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("discard=%d %v", n, err)
	}
}
