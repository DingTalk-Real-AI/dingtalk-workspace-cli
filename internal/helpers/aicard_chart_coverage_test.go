// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/chartgen"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type aicardChartTestWriter struct {
	writeErr error
	closeErr error
}

func (w *aicardChartTestWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(data), nil
}

func (w *aicardChartTestWriter) Close() error { return w.closeErr }

func requireAicardChartCommandError(t *testing.T, result map[string]any, code int, err error, want string) {
	t.Helper()
	if err != nil || code == 0 {
		t.Fatalf("code=%d err=%v result=%#v", code, err, result)
	}
	failure, _ := result["error"].(map[string]any)
	message, _ := failure["message"].(string)
	if !strings.Contains(message, want) {
		t.Fatalf("message=%q, want %q; result=%#v", message, want, result)
	}
}

func TestCrossPlatformCoverageAicardChartListExplainAndExamples(t *testing.T) {
	t.Run("unknown result schema", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("chartResultSchema accepted an unknown command")
			}
		}()
		_ = chartResultSchema("unknown")
	})
	result, code, err := executeAicard(t, &aicardCaller{}, "chart", "list")
	if err != nil || code != 0 || len(result["data"].(map[string]any)["presets"].([]any)) != 43 {
		t.Fatalf("list: code=%d err=%v result=%#v", code, err, result)
	}
	for _, id := range []string{"G01", "G13", "G17", "G19", "G22", "G24", "G26", "G29", "G31", "G34"} {
		result, code, err = executeAicard(t, &aicardCaller{}, "chart", "explain", id)
		if err != nil || code != 0 || result["data"].(map[string]any)["chartSpecExample"] == nil {
			t.Fatalf("explain %s: code=%d err=%v result=%#v", id, code, err, result)
		}
	}
	result, code, err = executeAicard(t, &aicardCaller{}, "chart", "explain", "missing")
	requireAicardChartCommandError(t, result, code, err, "unknown chart preset")
	t.Run("list registry error", func(t *testing.T) {
		testseam.Swap(t, &aicardChartList, func() ([]chartgen.Preset, error) { return nil, errors.New("registry injected") })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "list")
		requireAicardChartCommandError(t, result, code, err, "registry injected")
	})
	if chartExample(chartgen.Preset{Family: "unknown"}) != nil {
		t.Fatal("unknown chart family produced an example")
	}
}

func TestCrossPlatformCoverageAicardChartBuildInputAndInjectedFailures(t *testing.T) {
	valid := aicardTestFile(t, `{"specVersion":"1.0","presetId":"G26","title":"Score","data":{"kind":"measure","name":"Score","value":5,"min":0,"max":10}}`)
	t.Run("stdin", func(t *testing.T) {
		testseam.Swap(t, &aicardChartStdin, io.Reader(strings.NewReader(`{"specVersion":"1.0","presetId":"G26","title":"Score","data":{"kind":"measure","name":"Score","value":5,"min":0,"max":10}}`)))
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", "-", "--dry-run")
		if err != nil || code != 0 || result["ok"] != true {
			t.Fatalf("stdin: code=%d err=%v result=%#v", code, err, result)
		}
	})
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing file", []string{"chart", "build", "--dry-run"}, "--file is required"},
		{"missing out", []string{"chart", "build", "--file", valid}, "--out is required"},
		{"stdout", []string{"chart", "build", "--file", valid, "--out", "-"}, "cannot be -"},
		{"open", []string{"chart", "build", "--file", valid + ".missing", "--dry-run"}, "missing"},
		{"invalid", []string{"chart", "build", "--file", aicardTestFile(t, `{}`), "--dry-run"}, "ChartSpec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, code, err := executeAicard(t, &aicardCaller{}, tc.args...)
			requireAicardChartCommandError(t, result, code, err, tc.want)
		})
	}

	t.Run("read", func(t *testing.T) {
		testseam.Swap(t, &aicardChartReadAll, func(io.Reader) ([]byte, error) { return nil, errors.New("read injected") })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "read injected")
	})
	t.Run("protocol", func(t *testing.T) {
		testseam.Swap(t, &aicardLoadProtocol, func() (*a2ui.Protocol, error) { return nil, errors.New("protocol injected") })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "protocol injected")
	})
	t.Run("marshal create", func(t *testing.T) {
		testseam.Swap(t, &aicardChartMarshalIndent, func(any, string, string) ([]byte, error) { return nil, errors.New("marshal injected") })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "marshal injected")
	})
	t.Run("lint", func(t *testing.T) {
		testseam.Swap(t, &aicardLint, func(*a2ui.Protocol, []byte, bool, bool) (a2ui.Report, error) {
			return a2ui.Report{}, errors.New("lint injected")
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "lint injected")
	})
	t.Run("lint report", func(t *testing.T) {
		testseam.Swap(t, &aicardLint, func(*a2ui.Protocol, []byte, bool, bool) (a2ui.Report, error) { return a2ui.Report{Valid: false}, nil })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		if err != nil || code == 0 || result["ok"] != false {
			t.Fatalf("lint report: code=%d err=%v result=%#v", code, err, result)
		}
	})
	t.Run("preflight error", func(t *testing.T) {
		testseam.Swap(t, &aicardPreflight, func(*a2ui.Protocol, []string, string) (map[string]any, error) {
			return nil, errors.New("preflight injected")
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "preflight injected")
	})
	t.Run("preflight invalid", func(t *testing.T) {
		testseam.Swap(t, &aicardPreflight, func(*a2ui.Protocol, []string, string) (map[string]any, error) {
			return map[string]any{"valid": false}, nil
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--dry-run")
		requireAicardChartCommandError(t, result, code, err, "offline preflight")
	})
}

func TestCrossPlatformCoverageAicardChartOutputFailureBoundaries(t *testing.T) {
	boom := errors.New("chart output injected")
	t.Run("open", func(t *testing.T) {
		testseam.Swap(t, &aicardChartOpenNew, func(string) (chartWriteCloser, error) { return nil, boom })
		if err := chartWriteNew("ignored", []byte("x")); !errors.Is(err, boom) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("write", func(t *testing.T) {
		removed := ""
		testseam.Swap(t, &aicardChartOpenNew, func(string) (chartWriteCloser, error) { return &aicardChartTestWriter{writeErr: boom}, nil })
		testseam.Swap(t, &aicardChartRemove, func(path string) error { removed = path; return nil })
		if err := chartWriteNew("write-path", []byte("x")); !errors.Is(err, boom) || removed != "write-path" {
			t.Fatalf("error=%v removed=%q", err, removed)
		}
	})
	t.Run("close", func(t *testing.T) {
		removed := ""
		testseam.Swap(t, &aicardChartOpenNew, func(string) (chartWriteCloser, error) { return &aicardChartTestWriter{closeErr: boom}, nil })
		testseam.Swap(t, &aicardChartRemove, func(path string) error { removed = path; return nil })
		if err := chartWriteNew("close-path", []byte("x")); !errors.Is(err, boom) || removed != "close-path" {
			t.Fatalf("error=%v removed=%q", err, removed)
		}
	})

	valid := aicardTestFile(t, `{"specVersion":"1.0","presetId":"G26","title":"Score","data":{"kind":"measure","name":"Score","value":5,"min":0,"max":10}}`)
	t.Run("stat", func(t *testing.T) {
		testseam.Swap(t, &aicardChartStat, func(string) (os.FileInfo, error) { return nil, boom })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", filepath.Join(t.TempDir(), "out.json"))
		requireAicardChartCommandError(t, result, code, err, "chart output injected")
	})
	t.Run("artifact exists", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &aicardChartStat, func(string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, os.ErrNotExist
			}
			return nil, nil
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", filepath.Join(t.TempDir(), "out.json"))
		requireAicardChartCommandError(t, result, code, err, "Artifact already exists")
	})
	t.Run("artifact stat", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &aicardChartStat, func(string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, os.ErrNotExist
			}
			return nil, boom
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", filepath.Join(t.TempDir(), "out.json"))
		requireAicardChartCommandError(t, result, code, err, "chart output injected")
	})
	t.Run("artifact marshal", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &aicardChartMarshalIndent, func(value any, prefix, indent string) ([]byte, error) {
			calls++
			if calls == 2 {
				return nil, boom
			}
			return json.MarshalIndent(value, prefix, indent)
		})
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", filepath.Join(t.TempDir(), "out.json"))
		requireAicardChartCommandError(t, result, code, err, "chart output injected")
	})
	t.Run("artifact write cleanup", func(t *testing.T) {
		calls := 0
		removed := ""
		testseam.Swap(t, &aicardChartOpenNew, func(string) (chartWriteCloser, error) {
			calls++
			if calls == 2 {
				return nil, boom
			}
			return &aicardChartTestWriter{}, nil
		})
		testseam.Swap(t, &aicardChartRemove, func(path string) error { removed = path; return nil })
		out := filepath.Join(t.TempDir(), "out.json")
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", out)
		requireAicardChartCommandError(t, result, code, err, "chart output injected")
		if removed != out {
			t.Fatalf("removed=%q, want %q", removed, out)
		}
	})
	t.Run("card write", func(t *testing.T) {
		testseam.Swap(t, &aicardChartOpenNew, func(string) (chartWriteCloser, error) { return nil, boom })
		result, code, err := executeAicard(t, &aicardCaller{}, "chart", "build", "--file", valid, "--out", filepath.Join(t.TempDir(), "out.json"))
		requireAicardChartCommandError(t, result, code, err, "chart output injected")
	})
}
