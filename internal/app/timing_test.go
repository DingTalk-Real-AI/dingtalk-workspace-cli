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
	"os"
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
	if !strings.Contains(output, "[Timing]") {
		t.Error("output should contain [Timing] header")
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
	os.Setenv(PerfTimingEnv, "1")
	defer os.Unsetenv(PerfTimingEnv)

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

func TestIsPerfTimingEnabled(t *testing.T) {
	// Clear the env var first
	os.Unsetenv(PerfTimingEnv)

	if IsPerfTimingEnabled() {
		t.Error("IsPerfTimingEnabled should return false when env var is not set")
	}

	os.Setenv(PerfTimingEnv, "1")
	defer os.Unsetenv(PerfTimingEnv)

	if !IsPerfTimingEnabled() {
		t.Error("IsPerfTimingEnabled should return true when env var is set")
	}
}
