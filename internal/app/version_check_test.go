// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func setupVersionCheckTest(t *testing.T) {
	t.Helper()
	t.Setenv("DWS_NO_UPDATE_CHECK", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &version, "1.0.62")
	previous := edition.Get()
	edition.Override(&edition.Hooks{})
	t.Cleanup(func() { edition.Override(previous) })
	t.Cleanup(CloseFileLogger)
}

func availableVersionCheck() upgrade.CheckResult {
	return upgrade.CheckResult{Current: "1.0.62", Latest: "1.0.63", Status: upgrade.CheckStatusUpdateAvailable}
}

func TestCrossPlatformCoverageVersionCheckKeyCommands(t *testing.T) {
	setupVersionCheckTest(t)
	checks := 0
	testseam.Swap(t, &checkCurrentVersion, func(_ context.Context, current string, opts upgrade.CheckOptions) upgrade.CheckResult {
		checks++
		if current != "1.0.62" || opts.Force || opts.ReadOnly {
			t.Fatalf("unexpected check: %q %+v", current, opts)
		}
		return availableVersionCheck()
	})
	for _, args := range [][]string{{"--version"}, {"version"}, {"version", "--format", "json"}} {
		root := NewRootCommand()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if args[0] == "--version" {
			if want := "dws version " + Version() + "\n"; stdout.String() != want {
				t.Fatalf("version stdout changed: %q, want %q", stdout.String(), want)
			}
		}
		if len(args) == 3 {
			var payload struct {
				Version string              `json:"version"`
				Check   upgrade.CheckResult `json:"version_check"`
				Notice  versionNotices      `json:"_notice"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload.Version != "1.0.62" || payload.Check.Status != upgrade.CheckStatusUpdateAvailable || payload.Notice.Update.Command != "dws upgrade" || stderr.Len() != 0 {
				t.Fatalf("JSON version: stdout=%s stderr=%s err=%v", &stdout, &stderr, err)
			}
		} else if !strings.Contains(stderr.String(), "dws upgrade") {
			t.Fatalf("missing diagnostic: %s", &stderr)
		}
	}
	if checks != 3 {
		t.Fatalf("checks=%d", checks)
	}
}

func TestCrossPlatformCoverageVersionCheckAuthStatus(t *testing.T) {
	setupVersionCheckTest(t)
	for _, readonly := range []bool{false, true} {
		for _, format := range []string{"json", "table"} {
			cmd := newAuthStatusCommand()
			cmd.PersistentFlags().String("format", format, "")
			_ = cmd.Flags().Set("readonly", map[bool]string{false: "false", true: "true"}[readonly])
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			testseam.Swap(t, &checkCurrentVersion, func(_ context.Context, _ string, opts upgrade.CheckOptions) upgrade.CheckResult {
				if opts.ReadOnly != readonly {
					t.Fatalf("readonly=%v, opts=%+v", readonly, opts)
				}
				return availableVersionCheck()
			})
			if err := writeAuthStatusResult(cmd, false, false, nil, nil); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				var got authStatusResponse
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !got.Success || got.Authenticated || got.Message != "未登录" || got.VersionCheck == nil || got.Notice == nil || got.Notice.Update.Latest != "1.0.63" || stderr.Len() != 0 {
					t.Fatalf("status JSON: %s / %s err=%v", &stdout, &stderr, err)
				}
			} else if !strings.Contains(stderr.String(), "1.0.63") || !strings.Contains(stdout.String(), "未登录") {
				t.Fatalf("status table: %s / %s", &stdout, &stderr)
			}
		}
	}
}

func TestCrossPlatformCoverageVersionCheckStatesAndSuppression(t *testing.T) {
	setupVersionCheckTest(t)
	cmd := &cobra.Command{Use: "version"}
	for _, result := range []upgrade.CheckResult{
		{Current: "1.0.63", Latest: "1.0.63", Status: upgrade.CheckStatusUpToDate, Cached: true, CheckedAt: "2026-10-08T00:00:00Z"},
		{Current: "1.0.63", Status: upgrade.CheckStatusUnknown},
		{Current: "dev", Status: upgrade.CheckStatusSkipped},
		{Current: "1.0.63-beta.1", Latest: "1.0.63-beta.2", Status: upgrade.CheckStatusUpdateAvailable, Track: upgrade.ReleaseTrackBeta},
	} {
		var out bytes.Buffer
		if err := writeVersionCheck(&out, result); err != nil {
			t.Fatal(err)
		}
		if result.Status == upgrade.CheckStatusUnknown && !strings.Contains(out.String(), "无法确认") {
			t.Fatal(out.String())
		}
		if result.Track == upgrade.ReleaseTrackBeta && !strings.Contains(out.String(), "dws upgrade --beta") {
			t.Fatal(out.String())
		}
		if result.Cached && !strings.Contains(out.String(), result.CheckedAt) {
			t.Fatal(out.String())
		}
		if result.Status != upgrade.CheckStatusUpdateAvailable && noticeForVersion(result, false) != nil {
			t.Fatal("spurious update notice")
		}
	}
	testseam.Swap(t, &checkCurrentVersion, func(context.Context, string, upgrade.CheckOptions) upgrade.CheckResult {
		t.Fatal("disabled checker called")
		return upgrade.CheckResult{}
	})
	t.Setenv("DWS_NO_UPDATE_CHECK", "1")
	if commandVersionCheck(cmd, false).Status != upgrade.CheckStatusSkipped {
		t.Fatal("opt-out ignored")
	}
	t.Setenv("DWS_NO_UPDATE_CHECK", "")
	edition.Override(&edition.Hooks{IsEmbedded: true})
	if commandVersionCheck(cmd, false).Status != upgrade.CheckStatusSkipped {
		t.Fatal("embedded host checked for CLI update")
	}
}

func TestCrossPlatformCoverageVersionCheckUnknownErrorOutputs(t *testing.T) {
	setupVersionCheckTest(t)
	for _, unified := range []bool{false, true} {
		for _, format := range []string{"json", "table", "ndjson"} {
			cmd := &cobra.Command{Use: "example"}
			cmd.Flags().String("format", format, "")
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			original := apperrors.NewValidation("unknown command", apperrors.WithHint("检查帮助"))
			err := apperrors.MarkUnknownInvocation(original)
			testseam.Swap(t, &checkCurrentVersion, func(_ context.Context, _ string, opts upgrade.CheckOptions) upgrade.CheckResult {
				if !opts.Force {
					t.Fatal("unknown invocation must refresh")
				}
				return availableVersionCheck()
			})
			notice := unknownInvocationNotice(cmd, err)
			if notice == nil || !strings.Contains(notice.Update.Message, "可能") {
				t.Fatal("missing qualified recovery")
			}
			if unified {
				code, emitErr := emitFailureWithVersionNotice(cmd, err, notice)
				if emitErr != nil || code != 3 {
					t.Fatalf("code=%d err=%v", code, emitErr)
				}
			} else if emitErr := printExecutionError(cmd, &stdout, &stderr, err, notice); emitErr != nil {
				t.Fatal(emitErr)
			}
			var typed *apperrors.Error
			if !stderrors.As(original, &typed) || typed.Hint != "检查帮助" || apperrors.ExitCode(err) != 3 {
				t.Fatal("error changed")
			}
			machine := format == "json" || unified && format == "ndjson"
			if machine {
				data := stderr.Bytes()
				if unified {
					data = stdout.Bytes()
				}
				var got map[string]any
				if err := json.Unmarshal(data, &got); err != nil || got["_notice"] == nil || got["error"] == nil {
					t.Fatalf("machine output %s err=%v", data, err)
				}
			} else if !strings.Contains(stderr.String(), "dws upgrade") {
				t.Fatal(stderr.String())
			}
		}
	}
	cmd := &cobra.Command{Use: "known"}
	testseam.Swap(t, &checkCurrentVersion, func(context.Context, string, upgrade.CheckOptions) upgrade.CheckResult {
		t.Fatal("business error triggered version check")
		return upgrade.CheckResult{}
	})
	if unknownInvocationNotice(cmd, apperrors.NewValidation("unknown command")) != nil {
		t.Fatal("matched business text")
	}
}

func TestCrossPlatformCoverageVersionCheckProcessRecovery(t *testing.T) {
	setupVersionCheckTest(t)
	for _, args := range [][]string{{"future-command"}, {"version", "--future-option"}} {
		testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
		testseam.Swap(t, &checkCurrentVersion, func(context.Context, string, upgrade.CheckOptions) upgrade.CheckResult {
			return availableVersionCheck()
		})
		capture, err := os.CreateTemp(t.TempDir(), "stderr")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = capture.Close() })
		testseam.Swap(t, &os.Stderr, capture)
		if code := Execute(); code != 3 {
			t.Fatalf("%v exit=%d", args, code)
		}
		if _, err := capture.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(capture)
		if err != nil {
			t.Fatal(err)
		}
		stderr := string(data)
		if !strings.Contains(stderr, `"_notice"`) || !strings.Contains(stderr, `"latest": "1.0.63"`) {
			t.Fatalf("%v stderr=%s", args, stderr)
		}
	}
}

// 失败写入不应触发二次 JSON，也不应执行网络检查。
func TestCrossPlatformCoverageVersionCheckWriteFailure(t *testing.T) {
	setupVersionCheckTest(t)
	errBroken := stderrors.New("broken output")
	w := versionCheckFailWriter{errBroken}
	if err := writeVersionCheck(w, availableVersionCheck()); !stderrors.Is(err, errBroken) {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "version"}
	cmd.Flags().String("format", "table", "")
	if err := printExecutionError(cmd, io.Discard, w, apperrors.NewValidation("bad"), noticeForVersion(availableVersionCheck(), false)); !stderrors.Is(err, errBroken) {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetOut(w)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--version"})
	testseam.Swap(t, &checkCurrentVersion, func(context.Context, string, upgrade.CheckOptions) upgrade.CheckResult {
		t.Fatal("checked after version output failed")
		return upgrade.CheckResult{}
	})
	if err := root.Execute(); !stderrors.Is(err, errBroken) {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageVersionCheckInvocationIntent(t *testing.T) {
	setupVersionCheckTest(t)
	root := &cobra.Command{Use: "dws"}
	auth := &cobra.Command{Use: "auth"}
	status := &cobra.Command{Use: "status"}
	status.Flags().Bool("readonly", false, "")
	status.Flags().String("profile", "", "")
	auth.AddCommand(status)
	root.AddCommand(auth)
	root.AddCommand(&cobra.Command{Use: "__complete"})
	root.AddCommand(&cobra.Command{Use: "completion"})
	root.AddCommand(&cobra.Command{Use: "help"})
	for _, tc := range []struct {
		args                 []string
		readonly, suppressed bool
	}{
		{[]string{"auth", "status", "--new-flag", "--readonly"}, true, false},
		{[]string{"auth", "status", "--readonly", "--new-flag"}, true, false},
		{[]string{"auth", "status", "--new-flag", "--readonly=false"}, false, false},
		{[]string{"auth", "status", "--new-flag", "--", "--readonly"}, false, false},
		{[]string{"auth", "status", "--profile", "--readonly", "--new-flag"}, false, false},
		{[]string{"auth", "status", "--new-flag", "--help"}, false, true},
		{[]string{"auth", "status", "--new-flag", "--help=false"}, false, false},
		{[]string{"auth", "status", "--profile", "--help", "--new-flag"}, false, false},
		{[]string{"auth", "status", "--new-flag", "--readonly=broken"}, true, true},
		{[]string{"__complete", "--new-flag"}, false, true},
		{[]string{"completion", "--new-flag"}, false, true},
		{[]string{"help", "--new-flag"}, false, true},
	} {
		root.SetContext(context.WithValue(context.Background(), versionCheckArgsKey{}, tc.args))
		called := false
		testseam.Swap(t, &checkCurrentVersion, func(_ context.Context, _ string, opts upgrade.CheckOptions) upgrade.CheckResult {
			called = true
			if opts.ReadOnly != tc.readonly {
				t.Fatalf("%v readonly=%v, want %v", tc.args, opts.ReadOnly, tc.readonly)
			}
			return availableVersionCheck()
		})
		got := unknownInvocationNotice(root, apperrors.MarkUnknownInvocation(apperrors.NewValidation("unknown flag")))
		if called == tc.suppressed || (got == nil) != tc.suppressed {
			t.Fatalf("%v called=%v notice=%v", tc.args, called, got)
		}
	}
}

type versionCheckFailWriter struct{ err error }

func (w versionCheckFailWriter) Write([]byte) (int, error) { return 0, w.err }
