// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func executePR868Command(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"dws", root.Name()}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func newMinutesCommandWithYesForTest() *cobra.Command {
	cmd := newMinutesCommand()
	cmd.PersistentFlags().Bool("yes", false, "confirm test write")
	return cmd
}

func TestCrossPlatformCoverageMinutesNewSurfaces(t *testing.T) {
	t.Run("permission add requires explicit policy", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newMinutesCommand(), "permission", "add", "--ids", "task-1", "--member-uids", "user-1")
		if err == nil || !strings.Contains(err.Error(), "--policy") {
			t.Fatalf("permission add without --policy error = %v", err)
		}
		if caller.calls != 0 {
			t.Fatalf("permission add called MCP %d times before required policy validation", caller.calls)
		}
	})
	t.Run("hot-word delete dry-run", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executePR868Command(t, newMinutesCommand(), "hot-word", "delete", "--words", "钉钉,OKR"); err != nil {
			t.Fatalf("hot-word delete dry-run: %v", err)
		}
	})
	t.Run("hot-word delete missing words", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executePR868Command(t, newMinutesCommand(), "hot-word", "delete"); err == nil {
			t.Fatal("expected missing --words error")
		}
	})
	t.Run("hot-word delete executes", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMinutesCommandWithYesForTest(), "hot-word", "delete", "--words", "钉钉", "--yes"); err != nil {
			t.Fatalf("hot-word delete: %v", err)
		}
		if caller.tool != "delete_personal_hotword" {
			t.Fatalf("tool=%q", caller.tool)
		}
	})

	t.Run("permission apply dry-run", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executePR868Command(t, newMinutesCommand(), "permission", "apply", "--id", "task-1", "--policy", "4"); err != nil {
			t.Fatalf("permission apply dry-run: %v", err)
		}
	})
	t.Run("permission apply alias uuid", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMinutesCommandWithYesForTest(), "permission", "apply", "--uuid", "task-2", "--policy", "2", "--yes"); err != nil {
			t.Fatalf("permission apply alias: %v", err)
		}
		if caller.tool != "apply_minutes_permission" {
			t.Fatalf("tool=%q", caller.tool)
		}
		if caller.args["taskUuid"] != "task-2" || caller.args["policyId"] != float64(2) {
			t.Fatalf("args=%#v", caller.args)
		}
	})
	t.Run("permission apply invalid policy", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		if err := executePR868Command(t, newMinutesCommand(), "permission", "apply", "--id", "task", "--policy", "1"); err == nil {
			t.Fatal("expected invalid policy")
		}
		if err := executePR868Command(t, newMinutesCommand(), "permission", "apply", "--id", "task", "--policy", "x"); err == nil {
			t.Fatal("expected non-numeric policy")
		}
	})
	t.Run("permission apply missing flags", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		if err := executePR868Command(t, newMinutesCommand(), "permission", "apply", "--policy", "4"); err == nil {
			t.Fatal("expected missing id")
		}
		if err := executePR868Command(t, newMinutesCommand(), "permission", "apply", "--id", "task"); err == nil {
			t.Fatal("expected missing policy")
		}
	})
	t.Run("permission apply policy flag is int", func(t *testing.T) {
		// 数值参数声明为 int 类型 flag；必填校验走 cmd.Flags().Changed，
		// 不能用 validateRequiredFlags（它把 int 零值当成未传）。
		cmd, _, err := newMinutesCommand().Find([]string{"permission", "apply"})
		if err != nil {
			t.Fatalf("find permission apply: %v", err)
		}
		flag := cmd.Flags().Lookup("policy")
		if flag == nil {
			t.Fatal("flag --policy not found")
		}
		if flag.Value.Type() != "int" {
			t.Fatalf("flag --policy type = %q, want %q", flag.Value.Type(), "int")
		}
	})

	t.Run("audio-memo list default", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"items":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list"); err != nil {
			t.Fatalf("audio-memo list: %v", err)
		}
		if caller.tool != "list_audio_memos" {
			t.Fatalf("tool=%q", caller.tool)
		}
		if caller.args["pageSize"] != float64(200) {
			t.Fatalf("pageSize=%#v", caller.args["pageSize"])
		}
	})
	t.Run("audio-memo list with range and cursor", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"items":[]}`}}}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newMinutesCommand(),
			"audio-memo", "list",
			"--max", "10",
			"--cursor", "1740000000000",
			"--start", "2026-01-01T00:00:00+08:00",
			"--end", "2026-07-21T23:59:59+08:00",
		)
		if err != nil {
			t.Fatalf("audio-memo list ranged: %v", err)
		}
		if caller.args["cursor"] != float64(1740000000000) {
			t.Fatalf("cursor=%#v", caller.args["cursor"])
		}
		start, _ := caller.args["startTime"].(string)
		end, _ := caller.args["endTime"].(string)
		if !strings.Contains(start, "2026-01-01") || !strings.Contains(end, "2026-07-21") {
			t.Fatalf("start/end=%q/%q", start, end)
		}
	})
	t.Run("audio-memo list validation", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list", "--max", "0"); err == nil {
			t.Fatal("expected max validation")
		}
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list", "--max", "1001"); err == nil {
			t.Fatal("expected max upper bound")
		}
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list", "--cursor", "-1"); err == nil {
			t.Fatal("expected cursor validation")
		}
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list",
			"--start", "2026-07-21T00:00:00+08:00", "--end", "2026-01-01T00:00:00+08:00"); err == nil {
			t.Fatal("expected reversed range error")
		}
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list", "--start", "bad"); err == nil {
			t.Fatal("expected bad start")
		}
		if err := executePR868Command(t, newMinutesCommand(), "audio-memo", "list", "--end", "bad"); err == nil {
			t.Fatal("expected bad end")
		}
	})
}

func TestCrossPlatformCoverageMinutesAtomicConfirmationPolicy(t *testing.T) {
	protected := []struct {
		name string
		args []string
	}{
		{name: "upload notify", args: []string{"upload", "create-and-notify", "--file-name", "meeting.mp4", "--file-size", "10"}},
	}
	for _, test := range protected {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
			installScriptedCaller(t, caller)
			err := executePR868Command(t, newMinutesCommand(), test.args...)
			if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
				t.Fatalf("unconfirmed error = %v", err)
			}
			if caller.calls != 0 {
				t.Fatalf("remote calls before confirmation = %d", caller.calls)
			}
		})
	}

	notRequired := []struct {
		name string
		args []string
	}{
		{name: "update title", args: []string{"update", "title", "--id", "u1", "--title", "新标题"}},
		{name: "record start", args: []string{"record", "start"}},
		{name: "record pause", args: []string{"record", "pause", "--id", "u1"}},
		{name: "record resume", args: []string{"record", "resume", "--id", "u1"}},
		{name: "record stop", args: []string{"record", "stop", "--id", "u1"}},
		{name: "update summary", args: []string{"update", "summary", "--id", "u1", "--content", "新纪要"}},
		{name: "speaker replace", args: []string{"speaker", "replace", "--id", "u1", "--from", "甲", "--to", "乙"}},
		{name: "hot-word delete", args: []string{"hot-word", "delete", "--words", "旧词"}},
		{name: "replace text", args: []string{"replace-text", "--id", "u1", "--search", "旧", "--replace", "新"}},
		{name: "permission add", args: []string{"permission", "add", "--ids", "u1", "--member-uids", "m1", "--policy", "4"}},
		{name: "permission remove", args: []string{"permission", "remove", "--ids", "u1", "--member-uids", "m1"}},
		{name: "permission apply", args: []string{"permission", "apply", "--id", "u1", "--policy", "4"}},
		{name: "mind graph create", args: []string{"mind-graph", "create", "--id", "u1"}},
		{name: "speaker summary create", args: []string{"speaker", "summary", "create", "--ids", "u1"}},
		{name: "hot-word add", args: []string{"hot-word", "add", "--words", "新词"}},
		{name: "upload create", args: []string{"upload", "create", "--file-name", "meeting.mp4", "--file-size", "10"}},
		{name: "upload complete", args: []string{"upload", "complete", "--session-id", "s1"}},
		{name: "upload cancel", args: []string{"upload", "cancel", "--session-id", "s1"}},
	}
	for _, test := range notRequired {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
			installScriptedCaller(t, caller)
			if err := executePR868Command(t, newMinutesCommand(), test.args...); err != nil {
				t.Fatalf("not-required command error = %v", err)
			}
			if caller.calls != 1 {
				t.Fatalf("remote calls = %d, want 1", caller.calls)
			}
		})
	}

	t.Run("legacy message flag stops before write", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
		installScriptedCaller(t, caller)
		err := executePR868Command(t, newMinutesCommand(), "upload", "create", "--file-name", "meeting.mp4", "--file-size", "10", "--enable-message-card")
		if err == nil || !strings.Contains(err.Error(), "create-and-notify") || caller.calls != 0 {
			t.Fatalf("legacy flag err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("notifying upload forces message card after confirmation", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMinutesCommandWithYesForTest(), "upload", "create-and-notify", "--file-name", "meeting.mp4", "--file-size", "10", "--yes"); err != nil {
			t.Fatalf("confirmed notifying upload: %v", err)
		}
		option, _ := caller.args["minutesOption"].(map[string]any)
		if option["enableMessageCard"] != true {
			t.Fatalf("notifying upload args=%#v", caller.args)
		}
	})
}

func TestCrossPlatformCoverageDocExportGetTaskIDAlias(t *testing.T) {
	// Existing primary --job-id must remain usable.
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"SUCCESS","downloadUrl":"https://x"}`}}}
	installScriptedCaller(t, caller)
	if err := executePR868Command(t, newDocCommand(), "export", "get", "--job-id", "job-legacy"); err != nil {
		t.Fatalf("export get --job-id: %v", err)
	}
	if caller.tool != "query_export_job" || caller.args["jobId"] != "job-legacy" {
		t.Fatalf("tool/args=%q %#v", caller.tool, caller.args)
	}

	// Add-only synonym --task-id.
	caller2 := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}}
	installScriptedCaller(t, caller2)
	if err := executePR868Command(t, newDocCommand(), "export", "get", "--task-id", "job-123"); err != nil {
		t.Fatalf("export get --task-id: %v", err)
	}
	if caller2.args["jobId"] != "job-123" {
		t.Fatalf("task-id args=%#v", caller2.args)
	}
}

func TestCrossPlatformCoverageDriveAliasAndDownloadVersion(t *testing.T) {
	t.Run("permission list max-results", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newDriveCommand(), "permission", "list", "--node", "n1", "--max-results", "10"); err != nil {
			t.Fatalf("permission list: %v", err)
		}
		if caller.args["pageSize"] != 10 {
			t.Fatalf("pageSize=%#v", caller.args["pageSize"])
		}
	})
	t.Run("permission list next-token pagination", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":[]}`}}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newDriveCommand(), "permission", "list", "--node", "n1", "--next-token", "50"); err != nil {
			t.Fatalf("permission list --next-token: %v", err)
		}
		if caller.args["nextToken"] != "50" {
			t.Fatalf("nextToken=%#v", caller.args["nextToken"])
		}
	})
	t.Run("cover file-id alias", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executePR868Command(t, newDriveCommand(), "cover", "--file-id", "n1"); err != nil {
			t.Fatalf("cover --file-id: %v", err)
		}
	})
	t.Run("revert doc-id alias dry-run", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executePR868Command(t, newDriveCommand(), "revert", "--doc-id", "n1", "--version", "3"); err != nil {
			t.Fatalf("revert --doc-id: %v", err)
		}
	})
	t.Run("star add url alias", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executePR868Command(t, newDriveCommand(), "star", "add", "--url", "https://example/n1"); err != nil {
			t.Fatalf("star add --url: %v", err)
		}
	})
	t.Run("download --version routes", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		out := filepath.Join(t.TempDir(), "out.pdf")
		if err := executePR868Command(t, newDriveCommand(), "download", "--node", "n1", "--version", "3", "--output", out); err != nil {
			t.Fatalf("download --version: %v", err)
		}
	})
}

func TestCrossPlatformCoverageMailCalendarEventFolderID(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
	err := executePR868Command(t, newMailCommand(),
		"calendar-event", "list",
		"--email", "a@b.com",
		"--folder-id", "cal-1",
		"--start", "2026-07-01T00:00:00Z",
		"--end", "2026-07-31T23:59:59Z",
	)
	if err != nil {
		t.Fatalf("calendar-event list --folder-id: %v", err)
	}
}

func TestCrossPlatformCoverageUnifiedDiffEngine(t *testing.T) {
	if got := UnifiedDiff("a", []byte("same\n"), "b", []byte("same\n"), 3); got != nil {
		t.Fatalf("identical should be nil, got %q", got)
	}
	old := []byte("one\ntwo\nthree\n")
	neu := []byte("one\nTWO\nthree\nfour\n")
	diff := UnifiedDiff("old.txt", old, "new.txt", neu, 2)
	if len(diff) == 0 || !strings.Contains(string(diff), "@@") {
		t.Fatalf("expected hunk diff, got %q", diff)
	}
	_ = UnifiedDiff("o", []byte("a\nb\n"), "n", []byte("a\nc\n"), 0)
	_ = UnifiedDiff("o", []byte("{\n\n}\n"), "n", []byte("{\n x\n}\n"), 3)
	_ = UnifiedDiff("o", []byte("alpha\n"), "n", []byte("beta\n"), 1)
}

func TestCrossPlatformCoverageDriveLatestHelpers(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().Int("latest", 0, "")
	cmd.Flags().String("order-by", "", "")
	cmd.Flags().String("order", "", "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Int("max", 0, "")
	cmd.Flags().String("cursor", "", "")
	cmd.Flags().String("next-token", "", "")

	if err := validateDriveListLatest(cmd, 0); err == nil {
		t.Fatal("expected latest lower bound")
	}
	if err := validateDriveListLatest(cmd, 51); err == nil {
		t.Fatal("expected latest upper bound")
	}
	_ = cmd.ParseFlags([]string{"--order-by=name"})
	if err := validateDriveListLatest(cmd, 3); err == nil {
		t.Fatal("expected exclusive order-by")
	}

	cmd2 := &cobra.Command{Use: "list"}
	cmd2.Flags().Int("latest", 0, "")
	cmd2.Flags().String("order-by", "", "")
	cmd2.Flags().String("order", "", "")
	cmd2.Flags().Int("limit", 0, "")
	cmd2.Flags().Int("max", 0, "")
	cmd2.Flags().String("cursor", "", "")
	cmd2.Flags().String("next-token", "", "")
	_ = cmd2.ParseFlags([]string{"--limit=10"})
	if err := validateDriveListLatest(cmd2, 3); err == nil {
		t.Fatal("expected exclusive limit")
	}

	cmd3 := &cobra.Command{Use: "list"}
	cmd3.Flags().Int("latest", 0, "")
	cmd3.Flags().String("order-by", "", "")
	cmd3.Flags().String("order", "", "")
	cmd3.Flags().Int("limit", 0, "")
	cmd3.Flags().Int("max", 0, "")
	cmd3.Flags().String("cursor", "", "")
	cmd3.Flags().String("next-token", "", "")
	_ = cmd3.ParseFlags([]string{"--cursor=tok"})
	if err := validateDriveListLatest(cmd3, 3); err == nil {
		t.Fatal("expected exclusive cursor")
	}
	if err := validateDriveListLatest(cmd3, 3); err == nil {
		// already failed above
	}
	_ = validateDriveListLatest(&cobra.Command{Use: "x"}, 3) // no exclusive flags

	items := []map[string]any{
		{"name": "b.txt", "sortTime": int64(1), "rel_path": "b", "fileId": "2", "type": "file"},
		{"name": "a.txt", "sortTime": int64(2), "rel_path": "a", "fileId": "1", "type": "file"},
		{"name": "dir", "sortTime": int64(9), "type": "folder", "dentryType": "folder"},
	}
	got := applyDriveListLatest(items, 1, false)
	if len(got) != 1 {
		t.Fatalf("latest len=%d", len(got))
	}
	stripDriveDepthDecorations(got)
	if _, ok := got[0]["sortTime"]; ok {
		t.Fatal("sortTime should be stripped")
	}

	if ms, ok := driveItemModifiedMillis(map[string]any{"modifiedTime": float64(123)}); !ok || ms != 123 {
		t.Fatalf("float millis=%v %v", ms, ok)
	}
	if ms, ok := toMillis("2026-01-02T03:04:05Z"); !ok || ms <= 0 {
		t.Fatalf("rfc3339 millis=%v %v", ms, ok)
	}
	if _, ok := toMillis(""); ok {
		t.Fatal("empty string should fail")
	}
	if _, ok := toMillis(float64(-1)); ok {
		t.Fatal("negative float should fail")
	}
	if ms, ok := toMillis("42"); !ok || ms != 42 {
		t.Fatalf("int string=%v %v", ms, ok)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
	listCmd := &cobra.Command{Use: "list"}
	listCmd.SetContext(context.Background())
	if err := runDriveListLatest(listCmd, map[string]any{"spaceId": "s"}, "folder", 2, "*.md", true); err != nil {
		t.Fatalf("dry-run latest: %v", err)
	}

	caller := &scriptedToolCaller{steps: []scriptedToolStep{{
		text: `{"items":[{"name":"a.md","fileId":"1","type":"file"},{"name":"skip.bin","fileId":"2","type":"file"},{"name":"dir","type":"FOLDER"}],"nextToken":""}`,
	}}}
	installScriptedCaller(t, caller)
	oldArgs := os.Args
	os.Args = []string{"dws", "drive", "list"}
	t.Cleanup(func() { os.Args = oldArgs })
	listCmd2 := &cobra.Command{Use: "list"}
	listCmd2.SetContext(context.Background())
	if err := runDriveListLatest(listCmd2, nil, "", 5, "*.md", false); err != nil {
		t.Fatalf("latest scan: %v", err)
	}
}

func executeWhiteboardCommand(t *testing.T, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"dws", "doc", "whiteboard"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	root := newDocWhiteboardCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageWhiteboardInsertPaths(t *testing.T) {
	t.Run("dry-run plan", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
		if err := executeWhiteboardCommand(t, "insert", "--node", "doc-1"); err != nil {
			t.Fatalf("whiteboard insert dry-run: %v", err)
		}
	})
	t.Run("insert with placement and verify", func(t *testing.T) {
		caller := &pr868FlexibleCaller{whiteboardID: "wb-persisted"}
		testseam.Protect(t, &deps)
		InitDeps(caller)
		deps.Out.w = io.Discard
		deps.Out.errW = io.Discard
		testseam.Swap(t, &whiteboardSleep, func(time.Duration) {})
		if err := executeWhiteboardCommand(t,
			"insert", "--node", "doc-1", "--ref-block", "ref", "--where", "before", "--index", "2", "--yes"); err != nil {
			t.Fatalf("whiteboard insert: %v", err)
		}
	})
	t.Run("soft success when verify empty", func(t *testing.T) {
		// Empty blocks are eventual-consistency pending, not hard query failure.
		caller := &pr868FlexibleCaller{emptyVerify: true}
		testseam.Protect(t, &deps)
		InitDeps(caller)
		deps.Out.w = io.Discard
		deps.Out.errW = io.Discard
		testseam.Swap(t, &whiteboardSleep, func(time.Duration) {})
		if err := executeWhiteboardCommand(t, "insert", "--node", "doc-1", "--yes"); err != nil {
			t.Fatalf("pending-block soft success: %v", err)
		}
	})
	t.Run("helpers", func(t *testing.T) {
		oldArgs := os.Args
		os.Args = []string{"dws", "doc", "whiteboard", "insert"}
		t.Cleanup(func() { os.Args = oldArgs })

		if buildWhiteboardCardJSONML("b", "w") == "" {
			t.Fatal("empty jsonml")
		}
		if extractWhiteboardID(nil) != "" {
			t.Fatal("nil attrs")
		}
		if extractWhiteboardID(map[string]any{"metadata": map[string]any{"id": "x"}}) != "x" {
			t.Fatal("extract id")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: context.Canceled}}})
		if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected query error")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{`}}})
		if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected parse error")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"blocks":[{"blockId":"other","jsonml":"[]"}]}`}}})
		if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected missing block")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"blocks":[{"blockId":"b","jsonml":"not-json"}]}`}}})
		if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected jsonml parse error")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"blocks":[{"blockId":"b","jsonml":"[\"card\"]"}]}}`}}})
		if _, err := queryWhiteboardCardAttrs(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected missing attrs")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"blocks":[{"blockId":"b","jsonml":"[\"card\",null]"}]}`}}})
		if _, err := queryWhiteboardCardAttrs(context.Background(), "n", "b"); err == nil {
			t.Fatal("expected nil attrs")
		}
	})
}

type pr868FlexibleCaller struct {
	whiteboardID string
	emptyVerify  bool
	format       string
	dry          bool
}

func (c *pr868FlexibleCaller) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	if tool == "insert_document_block" {
		return textToolResult(`{}`), nil
	}
	if tool == "list_document_blocks" {
		if c.emptyVerify {
			return textToolResult(`{"blocks":[]}`), nil
		}
		blockID, _ := args["blockId"].(string)
		payload := `{"blocks":[{"blockId":"` + blockID + `","jsonml":"[\"card\",{\"metadata\":{\"type\":\"hetu/draw\",\"id\":\"` + c.whiteboardID + `\"}},[\"span\",{},[\"span\",{},\"\"]]]"}]}`
		return textToolResult(payload), nil
	}
	return textToolResult(`{}`), nil
}
func (c *pr868FlexibleCaller) Format() string { return c.format }
func (c *pr868FlexibleCaller) DryRun() bool   { return c.dry }
func (*pr868FlexibleCaller) Fields() string   { return "" }
func (*pr868FlexibleCaller) JQ() string       { return "" }

func TestCrossPlatformCoverageMarkdownDiffHelpers(t *testing.T) {
	if formatFileSize(100) == "" || formatFileSize(2048) == "" || formatFileSize(2*1024*1024) == "" {
		t.Fatal("formatFileSize")
	}
	small := filepath.Join(t.TempDir(), "ok.md")
	if err := os.WriteFile(small, []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkFileSize(small); err != nil {
		t.Fatalf("checkFileSize small: %v", err)
	}
	if err := checkFileSize(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing file")
	}

	diff, add, _, hunks, changed := computeUnifiedDiff("a\n", "a\nb\n", 2)
	if !changed || add < 1 || hunks < 1 || diff == "" {
		t.Fatalf("computeUnifiedDiff=%v %d %d %q", changed, add, hunks, diff)
	}
	if _, _, _, _, ch := computeUnifiedDiff("x\n", "x\n", 2); ch {
		t.Fatal("identical should be unchanged")
	}

	right := filepath.Join(t.TempDir(), "right.md")
	if err := os.WriteFile(right, []byte("hi\nthere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if describeDingTalkDocType("adoc") == "" {
		t.Fatal("describeDingTalkDocType")
	}
	_ = right
}
