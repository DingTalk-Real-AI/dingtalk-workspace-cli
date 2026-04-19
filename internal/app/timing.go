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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_PERF_DEBUG",
		Category:    configmeta.CategoryDebug,
		Description: "启用性能计时输出到 stderr",
		Example:     "1",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_PERF_REPORT",
		Category:    configmeta.CategoryDebug,
		Description: "JSON 性能报告输出路径 (auto=~/.dws/perf/latest.json)",
		Example:     "auto",
	})
}

const (
	// PerfDebugEnv is the environment variable to enable performance timing output.
	PerfDebugEnv = "DWS_PERF_DEBUG"

	// PerfReportEnv is the environment variable to enable JSON perf report output.
	// Set to "auto" to write to ~/.dws/perf/latest.json, or a custom file path.
	PerfReportEnv = "DWS_PERF_REPORT"

	perfReportDir  = "perf"
	perfReportFile = "latest.json"
)

// timingContextKey is the context key for TimingCollector.
type timingContextKey struct{}

// TimingEntry represents a single timing measurement.
type TimingEntry struct {
	Name      string
	Duration  time.Duration
	Timestamp time.Time
	Seq       int // insertion order
}

// TimingCollector collects timing measurements for a single command execution.
// It is safe for concurrent use.
type TimingCollector struct {
	mu      sync.Mutex
	start   time.Time
	entries []TimingEntry
	seq     int
}

// NewTimingCollector creates a new collector with the start time set to now.
func NewTimingCollector() *TimingCollector {
	return &TimingCollector{
		start:   time.Now(),
		entries: make([]TimingEntry, 0, 16),
	}
}

// Record adds a timing entry with the given name and duration.
func (tc *TimingCollector) Record(name string, d time.Duration) {
	if tc == nil {
		return
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.entries = append(tc.entries, TimingEntry{
		Name:      name,
		Duration:  d,
		Timestamp: time.Now(),
		Seq:       tc.seq,
	})
	tc.seq++
}

// StartTimer returns a function that, when called, records the elapsed time
// since StartTimer was called. This is convenient for defer usage:
//
//	defer tc.StartTimer("operation")()
func (tc *TimingCollector) StartTimer(name string) func() {
	if tc == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		tc.Record(name, time.Since(start))
	}
}

// Total returns the total elapsed time since the collector was created.
func (tc *TimingCollector) Total() time.Duration {
	if tc == nil {
		return 0
	}
	return time.Since(tc.start)
}

// Entries returns a copy of all recorded entries in insertion order.
func (tc *TimingCollector) Entries() []TimingEntry {
	if tc == nil {
		return nil
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([]TimingEntry, len(tc.entries))
	copy(result, tc.entries)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Seq < result[j].Seq
	})
	return result
}

// formatDuration returns a human-friendly duration string.
// Sub-µs → "0µs", sub-ms → microsecond precision (e.g. "142µs"), else → ms.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return "0µs"
	case d < time.Millisecond:
		return d.Truncate(time.Microsecond).String()
	default:
		return d.Truncate(time.Millisecond).String()
	}
}

// Print writes a summary of all timing entries to the given writer.
func (tc *TimingCollector) Print(w io.Writer) {
	if tc == nil || w == nil {
		return
	}
	entries := tc.Entries()
	total := tc.Total()
	if len(entries) == 0 {
		fmt.Fprintf(w, "\n[Perf] Total: %v (no detailed entries)\n", formatDuration(total))
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "[Perf] Execution breakdown:")
	for _, e := range entries {
		fmt.Fprintf(w, "  %-30s %v\n", e.Name, formatDuration(e.Duration))
	}
	fmt.Fprintf(w, "  %-30s %v\n", "──────────────────────────────", "──────────")
	fmt.Fprintf(w, "  %-30s %v\n", "Total", formatDuration(total))
}

// PrintIfEnabled prints timing info to stderr if DWS_PERF_DEBUG is set.
func (tc *TimingCollector) PrintIfEnabled() {
	if tc == nil {
		return
	}
	if os.Getenv(PerfDebugEnv) == "" {
		return
	}
	tc.Print(os.Stderr)
}

// WithTimingCollector returns a new context with the TimingCollector attached.
func WithTimingCollector(ctx context.Context, tc *TimingCollector) context.Context {
	return context.WithValue(ctx, timingContextKey{}, tc)
}

// TimingCollectorFromContext extracts the TimingCollector from context, or nil.
func TimingCollectorFromContext(ctx context.Context) *TimingCollector {
	if ctx == nil {
		return nil
	}
	tc, _ := ctx.Value(timingContextKey{}).(*TimingCollector)
	return tc
}

// RecordTiming is a convenience function to record timing to the collector in context.
func RecordTiming(ctx context.Context, name string, d time.Duration) {
	if tc := TimingCollectorFromContext(ctx); tc != nil {
		tc.Record(name, d)
	}
}

// StartTiming is a convenience function that returns a stop function for defer usage.
// Example:
//
//	defer StartTiming(ctx, "operation")()
func StartTiming(ctx context.Context, name string) func() {
	tc := TimingCollectorFromContext(ctx)
	if tc == nil {
		return func() {}
	}
	return tc.StartTimer(name)
}

// IsPerfDebugEnabled returns true if performance debug output is enabled.
func IsPerfDebugEnabled() bool {
	return os.Getenv(PerfDebugEnv) != ""
}

// ── Structured Performance Report ──────────────────────────────────────

// PerfPhase is a single phase in the performance report.
type PerfPhase struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Seq        int    `json:"seq"`
}

// PerfReport is the JSON-serialisable performance report.
type PerfReport struct {
	Kind       string      `json:"kind"`
	Version    string      `json:"version"`
	CLIVersion string      `json:"cli_version"`
	Command    string      `json:"command"`
	Timestamp  time.Time   `json:"timestamp"`
	TotalMs    int64       `json:"total_ms"`
	Phases     []PerfPhase `json:"phases"`
	Slowest    string      `json:"slowest"`
	OverheadMs int64       `json:"overhead_ms"`
}

// BuildReport constructs a PerfReport from the collected timing entries.
func (tc *TimingCollector) BuildReport(cliVersion, command string) PerfReport {
	entries := tc.Entries()
	total := tc.Total()
	totalMs := total.Milliseconds()

	phases := make([]PerfPhase, len(entries))
	var sumMs int64
	var slowestName string
	var slowestMs int64

	for i, e := range entries {
		ms := e.Duration.Milliseconds()
		phases[i] = PerfPhase{
			Name:       e.Name,
			DurationMs: ms,
			Seq:        e.Seq,
		}
		sumMs += ms
		if ms > slowestMs {
			slowestMs = ms
			slowestName = e.Name
		}
	}

	overhead := totalMs - sumMs
	if overhead < 0 {
		overhead = 0
	}

	return PerfReport{
		Kind:       "perf_report",
		Version:    "1",
		CLIVersion: cliVersion,
		Command:    command,
		Timestamp:  time.Now(),
		TotalMs:    totalMs,
		Phases:     phases,
		Slowest:    slowestName,
		OverheadMs: overhead,
	}
}

// WriteReportIfEnabled checks DWS_PERF_REPORT and writes a JSON report if set.
func (tc *TimingCollector) WriteReportIfEnabled(cliVersion, command string) {
	if tc == nil {
		return
	}
	dest := os.Getenv(PerfReportEnv)
	if dest == "" {
		return
	}

	report := tc.BuildReport(cliVersion, command)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}

	path := resolvePerfReportPath(dest)
	if path == "" {
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Rename(tmp, path)
}

// LoadLatestReport reads the default perf report file (~/.dws/perf/latest.json).
func LoadLatestReport() (*PerfReport, error) {
	path := defaultPerfReportPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report PerfReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// resolvePerfReportPath resolves the DWS_PERF_REPORT value to an absolute path.
func resolvePerfReportPath(dest string) string {
	if dest == "auto" {
		return defaultPerfReportPath()
	}
	return dest
}

func defaultPerfReportPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dws", perfReportDir, perfReportFile)
}

// sensitiveFlags are flag names whose values should be masked in commands.
var sensitiveFlags = map[string]bool{
	"--token":         true,
	"--client-secret": true,
	"--client-id":     true,
}

// SanitizeCommand redacts sensitive flag values from a command arg slice.
func SanitizeCommand(args []string) string {
	sanitized := make([]string, 0, len(args))
	skipNext := false
	for _, arg := range args {
		if skipNext {
			sanitized = append(sanitized, "***")
			skipNext = false
			continue
		}
		if idx := strings.IndexByte(arg, '='); idx > 0 {
			key := arg[:idx]
			if sensitiveFlags[key] {
				sanitized = append(sanitized, key+"=***")
				continue
			}
		}
		if sensitiveFlags[arg] {
			skipNext = true
		}
		sanitized = append(sanitized, arg)
	}
	return strings.Join(sanitized, " ")
}
