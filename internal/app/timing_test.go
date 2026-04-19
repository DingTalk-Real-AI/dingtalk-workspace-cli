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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimingCollector_Basic(t *testing.T) {
	tc := NewTimingCollector()
	if tc == nil {
		t.Fatal("NewTimingCollector returned nil")
	}

	// Record some timings
	tc.Record("op1", 10*time.Millisecond)
	tc.Record("op2", 20*time.Millisecond)

	entries := tc.Entries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Check ordering
	if entries[0].Name != "op1" {
		t.Errorf("expected first entry to be 'op1', got %q", entries[0].Name)
	}
	if entries[1].Name != "op2" {
		t.Errorf("expected second entry to be 'op2', got %q", entries[1].Name)
	}
}

func TestTimingCollector_StartTimer(t *testing.T) {
	tc := NewTimingCollector()

	stop := tc.StartTimer("timed_op")
	time.Sleep(5 * time.Millisecond)
	stop()

	entries := tc.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "timed_op" {
		t.Errorf("expected entry name 'timed_op', got %q", entries[0].Name)
	}
	if entries[0].Duration < 5*time.Millisecond {
		t.Errorf("expected duration >= 5ms, got %v", entries[0].Duration)
	}
}

func TestTimingCollector_NilSafe(t *testing.T) {
	var tc *TimingCollector

	// Should not panic on nil collector
	tc.Record("op", 10*time.Millisecond)
	stop := tc.StartTimer("op")
	stop()
	_ = tc.Total()
	_ = tc.Entries()
	tc.Print(nil)
	tc.PrintIfEnabled()
}

func TestTimingCollector_Print(t *testing.T) {
	tc := NewTimingCollector()
	tc.Record("auth_token", 44*time.Millisecond)
	tc.Record("mcp_call", 150*time.Millisecond)

	var buf bytes.Buffer
	tc.Print(&buf)

	output := buf.String()
	if !strings.Contains(output, "[Perf]") {
		t.Error("output should contain [Perf] header")
	}
	if !strings.Contains(output, "auth_token") {
		t.Error("output should contain 'auth_token'")
	}
	if !strings.Contains(output, "mcp_call") {
		t.Error("output should contain 'mcp_call'")
	}
	if !strings.Contains(output, "Total") {
		t.Error("output should contain 'Total'")
	}
}

func TestTimingCollector_PrintIfEnabled(t *testing.T) {
	// Set environment variable
	os.Setenv(PerfDebugEnv, "1")
	defer os.Unsetenv(PerfDebugEnv)

	tc := NewTimingCollector()
	tc.Record("test_op", 10*time.Millisecond)

	// This should not panic and should print to stderr
	tc.PrintIfEnabled()
}

func TestTimingCollector_ContextIntegration(t *testing.T) {
	tc := NewTimingCollector()
	ctx := WithTimingCollector(context.Background(), tc)

	// Retrieve from context
	retrieved := TimingCollectorFromContext(ctx)
	if retrieved != tc {
		t.Error("TimingCollectorFromContext should return the same collector")
	}

	// Use convenience functions
	RecordTiming(ctx, "ctx_op", 30*time.Millisecond)
	stop := StartTiming(ctx, "ctx_timed")
	time.Sleep(2 * time.Millisecond)
	stop()

	entries := tc.Entries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestTimingCollectorFromContext_NilContext(t *testing.T) {
	//lint:ignore SA1012 Testing explicit nil-context guard in TimingCollectorFromContext.
	tc := TimingCollectorFromContext(nil)
	if tc != nil {
		t.Error("TimingCollectorFromContext(nil) should return nil")
	}
}

func TestTimingCollectorFromContext_NoCollector(t *testing.T) {
	tc := TimingCollectorFromContext(context.Background())
	if tc != nil {
		t.Error("TimingCollectorFromContext with no collector should return nil")
	}
}

func TestStartTiming_NoCollector(t *testing.T) {
	ctx := context.Background()
	stop := StartTiming(ctx, "no_collector")
	// Should not panic
	stop()
}

func TestIsPerfDebugEnabled(t *testing.T) {
	// Clear the env var first
	os.Unsetenv(PerfDebugEnv)

	if IsPerfDebugEnabled() {
		t.Error("IsPerfDebugEnabled should return false when env var is not set")
	}

	os.Setenv(PerfDebugEnv, "1")
	defer os.Unsetenv(PerfDebugEnv)

	if !IsPerfDebugEnabled() {
		t.Error("IsPerfDebugEnabled should return true when env var is set")
	}
}

// ── PerfReport tests ────────────────────────────────────────────────────

func TestBuildReport(t *testing.T) {
	tc := NewTimingCollector()
	tc.Record("cmd_init", 45*time.Millisecond)
	tc.Record("auth_keychain", 72*time.Millisecond)
	tc.Record("mcp_call", 620*time.Millisecond)

	report := tc.BuildReport("v1.0.8", "dws aitable list-records")

	if report.Kind != "perf_report" {
		t.Errorf("expected kind 'perf_report', got %q", report.Kind)
	}
	if report.Version != "1" {
		t.Errorf("expected version '1', got %q", report.Version)
	}
	if report.CLIVersion != "v1.0.8" {
		t.Errorf("expected cli_version 'v1.0.8', got %q", report.CLIVersion)
	}
	if report.Command != "dws aitable list-records" {
		t.Errorf("expected command 'dws aitable list-records', got %q", report.Command)
	}
	if len(report.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(report.Phases))
	}
	if report.Phases[0].Name != "cmd_init" || report.Phases[0].DurationMs != 45 {
		t.Errorf("unexpected first phase: %+v", report.Phases[0])
	}
	if report.Slowest != "mcp_call" {
		t.Errorf("expected slowest 'mcp_call', got %q", report.Slowest)
	}
	if report.TotalMs < 0 {
		t.Errorf("total_ms should be >= 0, got %d", report.TotalMs)
	}
	if report.OverheadMs < 0 {
		t.Errorf("overhead_ms should be >= 0, got %d", report.OverheadMs)
	}
}

func TestBuildReportEmpty(t *testing.T) {
	tc := NewTimingCollector()
	report := tc.BuildReport("dev", "dws version")

	if len(report.Phases) != 0 {
		t.Errorf("expected 0 phases, got %d", len(report.Phases))
	}
	if report.Slowest != "" {
		t.Errorf("expected empty slowest, got %q", report.Slowest)
	}
}

func TestBuildReportJSON(t *testing.T) {
	tc := NewTimingCollector()
	tc.Record("cmd_init", 10*time.Millisecond)

	report := tc.BuildReport("v1.0.0", "dws version")
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	requiredKeys := []string{"kind", "version", "cli_version", "command", "timestamp", "total_ms", "phases", "slowest", "overhead_ms"}
	for _, key := range requiredKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}
}

func TestWriteReportIfEnabled(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")

	t.Setenv(PerfReportEnv, reportPath)

	tc := NewTimingCollector()
	tc.Record("cmd_init", 50*time.Millisecond)
	tc.Record("mcp_call", 200*time.Millisecond)

	tc.WriteReportIfEnabled("v1.0.0", "dws version")

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report file not written: %v", err)
	}

	var report PerfReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON in report: %v", err)
	}
	if report.Kind != "perf_report" {
		t.Errorf("expected kind 'perf_report', got %q", report.Kind)
	}
	if len(report.Phases) != 2 {
		t.Errorf("expected 2 phases, got %d", len(report.Phases))
	}
}

func TestWriteReportIfEnabled_Auto(t *testing.T) {
	tmpHome := t.TempDir()
	expected := filepath.Join(tmpHome, ".dws", "perf", "latest.json")

	// Temporarily override HOME for defaultPerfReportPath
	t.Setenv("HOME", tmpHome)
	t.Setenv(PerfReportEnv, "auto")

	tc := NewTimingCollector()
	tc.Record("cmd_init", 10*time.Millisecond)
	tc.WriteReportIfEnabled("v1.0.0", "dws version")

	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected report at %s: %v", expected, err)
	}
}

func TestWriteReportIfEnabled_Disabled(t *testing.T) {
	t.Setenv(PerfReportEnv, "")

	tc := NewTimingCollector()
	tc.Record("op", 10*time.Millisecond)
	tc.WriteReportIfEnabled("v1.0.0", "dws version")
	// No file should be written; no error expected
}

func TestWriteReportIfEnabled_NilCollector(t *testing.T) {
	t.Setenv(PerfReportEnv, "/tmp/should-not-exist.json")
	var tc *TimingCollector
	tc.WriteReportIfEnabled("v1.0.0", "dws version")
}

func TestLoadLatestReport(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	perfDir := filepath.Join(tmpHome, ".dws", "perf")
	if err := os.MkdirAll(perfDir, 0o700); err != nil {
		t.Fatal(err)
	}

	report := PerfReport{
		Kind:       "perf_report",
		Version:    "1",
		CLIVersion: "v1.0.0",
		Command:    "dws version",
		TotalMs:    100,
		Phases:     []PerfPhase{{Name: "cmd_init", DurationMs: 50, Seq: 0}},
		Slowest:    "cmd_init",
		OverheadMs: 50,
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(perfDir, "latest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadLatestReport()
	if err != nil {
		t.Fatalf("LoadLatestReport failed: %v", err)
	}
	if loaded.CLIVersion != "v1.0.0" {
		t.Errorf("expected cli_version 'v1.0.0', got %q", loaded.CLIVersion)
	}
	if len(loaded.Phases) != 1 {
		t.Errorf("expected 1 phase, got %d", len(loaded.Phases))
	}
}

func TestLoadLatestReport_NotFound(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	_, err := LoadLatestReport()
	if err == nil {
		t.Error("expected error when report file does not exist")
	}
}

func TestSanitizeCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no sensitive flags",
			args: []string{"dws", "aitable", "list-records"},
			want: "dws aitable list-records",
		},
		{
			name: "token with space-separated value",
			args: []string{"dws", "--token", "secret123", "version"},
			want: "dws --token *** version",
		},
		{
			name: "token with equals sign",
			args: []string{"dws", "--token=secret123", "version"},
			want: "dws --token=*** version",
		},
		{
			name: "client-secret space-separated",
			args: []string{"dws", "--client-secret", "mysecret", "--client-id", "myid", "auth"},
			want: "dws --client-secret *** --client-id *** auth",
		},
		{
			name: "client-id with equals",
			args: []string{"dws", "--client-id=abc123"},
			want: "dws --client-id=***",
		},
		{
			name: "empty args",
			args: []string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeCommand(tt.args)
			if got != tt.want {
				t.Errorf("SanitizeCommand(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestResolvePerfReportPath_Auto(t *testing.T) {
	p := resolvePerfReportPath("auto")
	if p == "" {
		t.Skip("HOME not available")
	}
	if !strings.HasSuffix(p, filepath.Join("perf", "latest.json")) {
		t.Errorf("expected path ending in perf/latest.json, got %q", p)
	}
}

func TestResolvePerfReportPath_Custom(t *testing.T) {
	p := resolvePerfReportPath("/tmp/my-report.json")
	if p != "/tmp/my-report.json" {
		t.Errorf("expected '/tmp/my-report.json', got %q", p)
	}
}

func TestPrintPerfReportSummary(t *testing.T) {
	report := &PerfReport{
		Command:    "dws version",
		Timestamp:  time.Now(),
		TotalMs:    300,
		Phases:     []PerfPhase{{Name: "cmd_init", DurationMs: 50, Seq: 0}, {Name: "mcp_call", DurationMs: 200, Seq: 1}},
		Slowest:    "mcp_call",
		OverheadMs: 50,
	}

	var buf bytes.Buffer
	printPerfReportSummary(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "cmd_init") {
		t.Error("output should contain 'cmd_init'")
	}
	if !strings.Contains(out, "mcp_call") {
		t.Error("output should contain 'mcp_call'")
	}
	if !strings.Contains(out, "← 最慢") {
		t.Error("output should contain '← 最慢' marker")
	}
	if !strings.Contains(out, "总耗时") {
		t.Error("output should contain '总耗时'")
	}
	if !strings.Contains(out, "框架开销") {
		t.Error("output should contain '框架开销'")
	}
}
