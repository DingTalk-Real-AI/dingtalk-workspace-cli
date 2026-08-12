// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

type minutesFailWriter struct{}

func (minutesFailWriter) Write([]byte) (int, error) { return 0, errors.New("fixture output failure") }

func runMinutesAlignmentCLIWithWriter(t *testing.T, caller *minutesE2ECaller, writer io.Writer, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	root.SetOut(writer)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageMinutesAlignmentValidationAndHelpers(t *testing.T) {
	caller := &minutesE2ECaller{}
	manyIDs := make([]string, 51)
	for index := range manyIDs {
		manyIDs[index] = fmt.Sprintf("u%d", index)
	}
	invalid := [][]string{
		{"minutes", "+search", "--query", "x", "--limit", "0"},
		{"minutes", "+search", "--query", "x", "--start", "bad"},
		{"minutes", "+search", "--query", "x", "--end", "bad"},
		{"minutes", "+search", "--query", "x", "--start", "2026-08-02T00:00:00Z", "--end", "2026-08-01T00:00:00Z"},
		{"minutes", "+download", "--id", "u1", "--ids", "u2"},
		{"minutes", "+download", "--ids", strings.Join(manyIDs, ",")},
		{"minutes", "+download", "--ids", "u1,u2", "--output", "one"},
		{"minutes", "+download", "--id", "u1", "--output", "../escape"},
		{"minutes", "+upload", "--file", "missing", "--complete-timeout", "0"},
		{"minutes", "+speaker-replace", "--id", "u1", "--from", "same", "--to", " same "},
		{"minutes", "+speaker-replace", "--id", "u1", "--from", "a", "--to", "b", "--page-limit", "0"},
	}
	for _, args := range invalid {
		if payload, output, err := runMinutesAlignmentCLI(t, caller, args...); err == nil || payload != nil || output != "" {
			t.Fatalf("invalid argv accepted: %v payload=%#v output=%q err=%v", args, payload, output, err)
		}
	}

	if start, end, err := minutesTimeRange("2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z"); err != nil || start == 0 || end <= start {
		t.Fatalf("valid range = %d %d %v", start, end, err)
	}
	if got := mediaExtension("https://example.invalid/a.MP3?token=x"); got != ".mp3" {
		t.Fatalf("extension = %q", got)
	}
	if got := mediaExtension("://bad"); got != "" {
		t.Fatalf("bad extension = %q", got)
	}
	if got := mediaExtension("https://example.invalid/a.12345678901"); got != "" {
		t.Fatalf("long extension = %q", got)
	}
	counts := speakerCounts([]map[string]any{{
		"speakerNick": " A ", "nested": []any{map[string]any{"nickName": "A", "subSpeakerNickname": "B"}},
	}})
	if counts["A"] != 1 || counts["B"] != 1 {
		t.Fatalf("speaker counts = %#v", counts)
	}

	decl := corecmd.ContractDecl{Parameters: []contract.ParamDecl{
		{Name: "a", Description: "parameter"},
		{Name: "a", Description: "already evidence"},
		{Name: "other", Description: "other"},
	}}
	value := shortcut.Shortcut{Description: "desc", Intent: "intent", Contract: decl,
		Flags: []shortcut.Flag{{Name: "a", Desc: "flag"}},
		Constraints: []shortcut.Constraint{
			{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"a"}},
			{Kind: shortcut.ConstraintCustom, Flags: []string{"a"}},
		},
	}
	finalized := finalizeMinutesShortcuts(value)[0]
	if finalized.Contract.Selection.AgentSummary != "desc" || strings.Contains(finalized.Flags[0].Desc, "约束") {
		t.Fatalf("finalized = %#v", finalized)
	}
	withEvidence := value
	withEvidence.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"a"}, Description: "evidence"}}
	withEvidence.Flags[0].Desc = "flag evidence"
	withEvidence.Contract.Parameters[1].Description = "already evidence"
	finalized = finalizeMinutesShortcuts(withEvidence)[0]
	if !strings.Contains(finalized.Contract.Parameters[0].Description, "约束：evidence") || finalized.Contract.Parameters[1].Description != "already evidence" || finalized.Contract.Parameters[2].Description != "other" {
		t.Fatalf("parameter evidence = %#v", finalized.Contract.Parameters)
	}
	payload := minutesDryRunPayload("plan", "op", nil)
	if payload["operation"] != "op" || payload["executed"] != false {
		t.Fatalf("dry-run payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMinutesSearchFailureAndCursorBranchesE2E(t *testing.T) {
	cases := []struct {
		name      string
		caller    *minutesE2ECaller
		args      []string
		wantError bool
		wantNext  string
	}{
		{name: "call", caller: &minutesE2ECaller{failAt: map[string]int{"minutes/list_by_keyword_and_time_range": 1}}, args: []string{"minutes", "+search", "--query", "x"}, wantError: true},
		{name: "parse", caller: &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{}}`}}}, args: []string{"minutes", "+search", "--query", "x"}, wantError: true},
		{name: "project", caller: &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"title":"x"}],"hasNext":false}}`}}}, args: []string{"minutes", "+search", "--query", "x"}, wantError: true},
		{name: "single page", caller: &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"x"}],"hasNext":true,"nextToken":"n2"}}`}}}, args: []string{"minutes", "+search", "--start", "2026-08-01T00:00:00Z", "--end", "2026-08-02T00:00:00Z", "--cursor", "n1"}, wantNext: "n2"},
		{name: "limit", caller: &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":true,"nextToken":"n2"}}`}}}, args: []string{"minutes", "+search", "--query", "x", "--page-all", "--page-limit", "1"}, wantError: true},
		{name: "cycle", caller: &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":true,"nextToken":"n1"}}`}}}, args: []string{"minutes", "+search", "--query", "x", "--page-all", "--cursor", "n1"}, wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			payload, _, err := runMinutesAlignmentCLI(t, test.caller, test.args...)
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			if test.wantNext != "" && payload["nextToken"] != test.wantNext {
				t.Fatalf("payload=%#v", payload)
			}
		})
	}
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":false}}`},
	}}, minutesFailWriter{}, "minutes", "+search", "--query", "x"); err == nil {
		t.Fatal("output failure accepted")
	}
	// Execute is defensive even when invoked without the Shortcut validator.
	cmd := &cobra.Command{Use: "search"}
	cmd.Flags().String("start", "bad", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().String("scope", "all", "")
	cmd.Flags().String("cursor", "", "")
	cmd.Flags().String("query", "x", "")
	cmd.Flags().Int("limit", 20, "")
	cmd.Flags().Int("page-limit", 100, "")
	cmd.Flags().Bool("page-all", false, "")
	if err := executeMinutesSearch(shortcut.RuntimeContextForTest(cmd, Search)); err == nil {
		t.Fatal("direct invalid time accepted")
	}
}

func TestCrossPlatformCoverageMinutesDownloadBranchesE2E(t *testing.T) {
	testseam.Swap(t, &minutesDownload, func(_ context.Context, rawURL string, options localio.DownloadOptions) (localio.DownloadResult, error) {
		if strings.Contains(rawURL, "fail") {
			return localio.DownloadResult{}, errors.New("fixture download failure")
		}
		return localio.DownloadResult{RelativePath: filepath.ToSlash(filepath.Join(options.Output, options.PreferredName)), SizeBytes: 12}, nil
	})
	caller := &minutesE2ECaller{
		responses: map[string][]string{"minutes/query_minutes_audio_url": {
			`{"success":true,"result":{"downloadUrl":"https://example.invalid/u1.mp3"}}`,
			`{"success":true,"result":{"downloadUrl":"https://example.invalid/fail.mp4"}}`,
		}},
		failAt: map[string]int{"minutes/query_minutes_audio_url": 3},
	}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+download", "--ids", "u1,u2,u3", "--output-dir", "media")
	if err == nil || output == "" || payload["succeeded"] != float64(1) || payload["failed"] != float64(2) {
		t.Fatalf("download payload=%#v err=%v", payload, err)
	}
	urlOnly := &minutesE2ECaller{responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{"downloadUrl":"https://example.invalid/u1.mp3"}}`}}}
	if payload, _, err = runMinutesAlignmentCLI(t, urlOnly, "minutes", "+download", "--id", "u1", "--url-only"); err != nil || payload["succeeded"] != float64(1) {
		t.Fatalf("url-only payload=%#v err=%v", payload, err)
	}
	if err := runMinutesAlignmentCLIWithWriter(t, urlOnly, minutesFailWriter{}, "minutes", "+download", "--id", "u1", "--url-only"); err == nil {
		t.Fatal("download output failure accepted")
	}
	single := &minutesE2ECaller{responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{"downloadUrl":"https://example.invalid/u1.mp3"}}`}}}
	if payload, _, err = runMinutesAlignmentCLI(t, single, "minutes", "+download", "--id", "u1", "--output", "single.mp3"); err != nil || payload["succeeded"] != float64(1) {
		t.Fatalf("single payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesUploadFailureAndSuccessBranchesE2E(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+upload", "--file", t.TempDir(), "--yes"); err == nil || payload != nil || output != "" {
		t.Fatalf("directory upload accepted: %#v %q %v", payload, output, err)
	}

	createFail := &minutesE2ECaller{failAt: map[string]int{"minutes/create_upload_session": 1}}
	if _, _, err := runMinutesAlignmentCLI(t, createFail, "minutes", "+upload", "--file", file, "--yes"); err == nil {
		t.Fatal("create failure accepted")
	}
	badSession := &minutesE2ECaller{responses: map[string][]string{"minutes/create_upload_session": {`{"success":true,"result":{}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, badSession, "minutes", "+upload", "--file", file, "--yes"); err == nil {
		t.Fatal("bad session accepted")
	}

	for _, test := range []struct {
		name         string
		cancelRaw    string
		cancelFailAt int
	}{
		{name: "cancel success", cancelRaw: `{"success":true,"result":{}}`},
		{name: "cancel call failure", cancelFailAt: 1},
		{name: "cancel acknowledgement failure", cancelRaw: `{"result":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			testseam.Swap(t, &minutesPutFile, func(context.Context, string, string, int64) (localio.UploadResult, error) {
				return localio.UploadResult{}, errors.New("put failed")
			})
			caller := &minutesE2ECaller{
				responses: map[string][]string{
					"minutes/create_upload_session": {`{"success":true,"result":{"sessionId":"s1","presignedUrl":"https://example.invalid/u"}}`},
					"minutes/cancel_upload_session": {test.cancelRaw},
				},
				failAt: map[string]int{"minutes/cancel_upload_session": test.cancelFailAt},
			}
			if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+upload", "--file", file, "--yes"); err == nil {
				t.Fatal("put failure accepted")
			}
		})
	}

	testseam.Swap(t, &minutesPutFile, func(context.Context, string, string, int64) (localio.UploadResult, error) {
		return localio.UploadResult{SizeBytes: 5, Attempts: 2}, nil
	})
	base := map[string][]string{
		"minutes/create_upload_session":   {`{"success":true,"result":{"sessionId":"s1","presignedUrl":"https://example.invalid/u"}}`},
		"minutes/complete_upload_session": {`{"success":true,"result":{"taskUuid":"u1"}}`},
		"minutes/get_minutes_basic_info":  {`{"success":true,"result":{"taskUuid":"u1","title":"ok"}}`},
	}
	success := &minutesE2ECaller{responses: base}
	payload, _, err := runMinutesAlignmentCLI(t, success, "minutes", "+upload", "--file", file, "--title", "T", "--template-id", "tpl", "--input-language", "zh", "--enable-message-card", "--yes")
	if err != nil || payload["complete"] != true || payload["verified"] != true {
		t.Fatalf("upload payload=%#v err=%v", payload, err)
	}

	cases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
	}{
		{name: "invalid completion", responses: map[string][]string{
			"minutes/create_upload_session":   {`{"success":true,"result":{"sessionId":"s1","presignedUrl":"https://example.invalid/u"}}`},
			"minutes/complete_upload_session": {`{"success":true,"result":{}}`},
		}},
		{name: "verify call", responses: base, failAt: map[string]int{"minutes/get_minutes_basic_info": 1}},
		{name: "verify parse", responses: map[string][]string{
			"minutes/create_upload_session":   {`{"success":true,"result":{"sessionId":"s1","presignedUrl":"https://example.invalid/u"}}`},
			"minutes/complete_upload_session": {`{"success":true,"result":{"taskUuid":"u1"}}`},
			"minutes/get_minutes_basic_info":  {`{"success":true,"result":{}}`},
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			caller := &minutesE2ECaller{responses: test.responses, failAt: test.failAt}
			if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+upload", "--file", file, "--yes"); err == nil {
				t.Fatal("upload failure accepted")
			}
		})
	}
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+upload", "--file", file, "--dry-run"); err == nil {
		t.Fatal("upload output failure accepted")
	}
}

func TestCrossPlatformCoverageMinutesUploadErrorConstructors(t *testing.T) {
	if err := minutesUploadFailure("put", "s", false, nil, nil); err == nil {
		t.Fatal("nil-cause failure missing")
	}
	if err := minutesUploadCompletionUnknown("s", 1, nil); err == nil {
		t.Fatal("nil-cause unknown completion missing")
	}
}

func TestCrossPlatformCoverageMinutesUpdatePermissionSummarySpeakerFailuresE2E(t *testing.T) {
	// Dry-run branches must not access MCP and must still propagate output errors.
	dryCases := [][]string{
		{"minutes", "+update", "--id", "u1", "--title", "new", "--dry-run"},
		{"minutes", "+summary", "--id", "u1", "--content", "new", "--dry-run"},
		{"minutes", "+speaker-replace", "--id", "u1", "--from", "a", "--to", "b", "--dry-run"},
	}
	for _, args := range dryCases {
		if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, args...); err == nil {
			t.Fatalf("output failure accepted: %v", args)
		}
	}

	updateCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		wantError bool
	}{
		{name: "before call", failAt: map[string]int{"minutes/get_minutes_basic_info": 1}, wantError: true},
		{name: "before parse", responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "unchanged", responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"new"}}`}}},
		{name: "write", responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"old"}}`}}, failAt: map[string]int{"minutes/update_minutes_title": 1}, wantError: true},
		{name: "after call", responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"old"}}`}}, failAt: map[string]int{"minutes/get_minutes_basic_info": 2}, wantError: true},
		{name: "after parse", responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"old"}}`, `{"success":true,"result":{}}`}, "minutes/update_minutes_title": {`{"success":true,"result":{}}`}}, wantError: true},
	}
	for _, test := range updateCases {
		t.Run("update "+test.name, func(t *testing.T) {
			payload, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, "minutes", "+update", "--id", "u1", "--title", "new", "--yes")
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	permissionCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		writer    io.Writer
	}{
		{name: "call", failAt: map[string]int{"minutes/apply_minutes_permission": 1}},
		{name: "ack", responses: map[string][]string{"minutes/apply_minutes_permission": {`{"result":{}}`}}},
		{name: "success output", responses: map[string][]string{"minutes/apply_minutes_permission": {`{"success":true,"result":{}}`}}, writer: minutesFailWriter{}},
	}
	for _, test := range permissionCases {
		t.Run("permission "+test.name, func(t *testing.T) {
			writer := test.writer
			if writer == nil {
				writer = &bytes.Buffer{}
			}
			if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, writer, "minutes", "+apply-permission", "--id", "u1", "--permission", "view", "--yes"); err == nil {
				t.Fatal("permission failure accepted")
			}
		})
	}
	permissionSuccess := &minutesE2ECaller{responses: map[string][]string{"minutes/apply_minutes_permission": {`{"success":true,"result":{}}`}}}
	if payload, _, err := runMinutesAlignmentCLI(t, permissionSuccess, "minutes", "+apply-permission", "--id", "u1", "--permission", "view", "--yes"); err != nil || payload["requested"] != true {
		t.Fatalf("permission payload=%#v err=%v", payload, err)
	}

	if _, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+summary", "--id", "u1", "--content", "@missing", "--yes"); err == nil {
		t.Fatal("missing input accepted")
	}
	if _, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+summary", "--id", "u1", "--content", "   ", "--yes"); err == nil {
		t.Fatal("empty summary accepted")
	}
	cmd := &cobra.Command{Use: "summary"}
	cmd.Flags().String("id", "u1", "")
	cmd.Flags().String("content", "   ", "")
	if err := executeMinutesSummary(shortcut.RuntimeContextForTest(cmd, Summary)); err == nil {
		t.Fatal("direct empty summary accepted")
	}
	summaryCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		wantError bool
	}{
		{name: "before call", failAt: map[string]int{"minutes/get_minutes_ai_summary": 1}, wantError: true},
		{name: "before parse", responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "unchanged", responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"new"}}`}}},
		{name: "write", responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"old"}}`}}, failAt: map[string]int{"minutes/update_minutes_summary": 1}, wantError: true},
		{name: "readback call", responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"old"}}`}}, failAt: map[string]int{"minutes/get_minutes_ai_summary": 2}, wantError: true},
		{name: "readback parse", responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"old"}}`, `{"success":true,"result":{}}`}, "minutes/update_minutes_summary": {`{"success":true,"result":{}}`}}, wantError: true},
	}
	for _, test := range summaryCases {
		t.Run("summary "+test.name, func(t *testing.T) {
			payload, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, "minutes", "+summary", "--id", "u1", "--content", "new", "--yes")
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	speakerCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
	}{
		{name: "before", failAt: map[string]int{"minutes/get_minutes_transcription": 1}},
		{name: "missing", responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"speakerNick":"x"}],"hasNext":false}}`}}},
		{name: "write", responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"speakerNick":"a"}],"hasNext":false}}`}}, failAt: map[string]int{"minutes/replace_speaker": 1}},
		{name: "after", responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"speakerNick":"a"}],"hasNext":false}}`}}, failAt: map[string]int{"minutes/get_minutes_transcription": 2}},
		{name: "verify", responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"speakerNick":"a"}],"hasNext":false}}`, `{"success":true,"result":{"paragraphList":[{"speakerNick":"a"}],"hasNext":false}}`}, "minutes/replace_speaker": {`{"success":true,"result":{}}`}}},
	}
	for _, test := range speakerCases {
		t.Run("speaker "+test.name, func(t *testing.T) {
			if _, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, "minutes", "+speaker-replace", "--id", "u1", "--from", "a", "--to", "b", "--target-uid", "uid", "--yes"); err == nil {
				t.Fatal("speaker failure accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesCompleteUploadRetryAndContext(t *testing.T) {
	// Use the real runtime/caller path so retry classification is tested, while
	// zero intervals keep the fault-injection cases deterministic and fast.
	for _, test := range []struct {
		name      string
		responses []string
		failAt    int
		failError error
		cancel    bool
		timeout   time.Duration
		interval  time.Duration
		wantCalls int
		wantError bool
	}{
		{name: "success", responses: []string{`{"success":true,"result":{"taskUuid":"u1"}}`}, timeout: time.Second, wantCalls: 1},
		{name: "retry success", responses: []string{`{"success":true,"result":{"taskUuid":"u1"}}`}, failAt: 1, failError: errors.New("still uploading"), timeout: time.Second, wantCalls: 2},
		{name: "nonretryable", failAt: 1, failError: errors.New("denied"), timeout: time.Second, wantCalls: 1, wantError: true},
		{name: "timeout", failAt: 1, failError: errors.New("wait and retry"), wantCalls: 1, wantError: true},
		{name: "context", failAt: 1, failError: errors.New("正在上传"), cancel: true, timeout: 2 * time.Hour, interval: time.Hour, wantCalls: 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := "minutes/complete_upload_session"
			caller := &minutesE2ECaller{
				responses:  map[string][]string{key: test.responses},
				failAt:     map[string]int{key: test.failAt},
				failErrors: map[string]error{key: test.failError},
			}
			helpers.InitDepsForTest(t, caller)
			ctx := context.Background()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			cmd := &cobra.Command{Use: "test"}
			cmd.SetContext(ctx)
			rt := shortcut.RuntimeContextForTest(cmd, Upload)
			_, attempts, err := completeMinutesUpload(rt, "s1", test.timeout, test.interval)
			if (err != nil) != test.wantError || attempts != test.wantCalls || caller.counts[key] != test.wantCalls {
				t.Fatalf("attempts=%d calls=%d err=%v", attempts, caller.counts[key], err)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesLegacyListsAndRecordCommandsE2E(t *testing.T) {
	listSuccess := &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"weekly"}],"hasNext":false}}`}}}
	for _, command := range []string{"+list-mine", "+list-shared", "+list-all"} {
		payload, _, err := runMinutesAlignmentCLI(t, listSuccess, "minutes", command, "--query", "weekly", "--limit", "5", "--cursor", "n1")
		if err != nil || payload["count"] != float64(1) {
			t.Fatalf("%s payload=%#v err=%v", command, payload, err)
		}
	}
	if calls := listSuccess.arguments["minutes/list_by_keyword_and_time_range"]; len(calls) != 3 || calls[0]["maxResults"] != 5 || calls[0]["keyword"] != "weekly" || calls[0]["nextToken"] != "n1" {
		t.Fatalf("list args=%#v", calls)
	}
	listCall := &minutesE2ECaller{failAt: map[string]int{"minutes/list_by_keyword_and_time_range": 1}}
	if _, _, err := runMinutesAlignmentCLI(t, listCall, "minutes", "+list-all"); err == nil {
		t.Fatal("list call failure accepted")
	}
	listParse := &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, listParse, "minutes", "+list-all"); err == nil {
		t.Fatal("list parse failure accepted")
	}
	listProject := &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"title":"missing id"}]}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, listProject, "minutes", "+list-all"); err == nil {
		t.Fatal("list projection failure accepted")
	}
	if err := runMinutesAlignmentCLIWithWriter(t, listSuccess, minutesFailWriter{}, "minutes", "+list-all"); err == nil {
		t.Fatal("list output failure accepted")
	}

	for _, test := range []struct {
		command string
		cmd     string
		id      string
	}{
		{command: "+record-start", cmd: "create"},
		{command: "+record-pause", cmd: "pause", id: "u1"},
		{command: "+record-resume", cmd: "resume", id: "u1"},
		{command: "+record-stop", cmd: "end", id: "u1"},
	} {
		t.Run(test.command, func(t *testing.T) {
			result := fmt.Sprintf(`{"success":true,"result":{"cmd":%q,"uuid":%q}}`, test.cmd, test.id)
			caller := &minutesE2ECaller{responses: map[string][]string{"minutes/" + listeningNoteCmdTool: {result}}}
			args := []string{"minutes", test.command, "--session-id", "session", "--yes"}
			if test.id != "" {
				args = append(args, "--id", test.id)
			}
			payload, _, err := runMinutesAlignmentCLI(t, caller, args...)
			if err != nil || payload["accepted"] != true || payload["command"] != test.cmd {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			if caller.arguments["minutes/"+listeningNoteCmdTool][0]["sessionId"] != "session" {
				t.Fatalf("args=%#v", caller.arguments)
			}
		})
	}
	recordCall := &minutesE2ECaller{failAt: map[string]int{"minutes/" + listeningNoteCmdTool: 1}}
	if _, _, err := runMinutesAlignmentCLI(t, recordCall, "minutes", "+record-start", "--yes"); err == nil {
		t.Fatal("record call failure accepted")
	}
	recordParse := &minutesE2ECaller{responses: map[string][]string{"minutes/" + listeningNoteCmdTool: {`{"success":true,"result":{"cmd":"pause","uuid":"u1"}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, recordParse, "minutes", "+record-stop", "--id", "u1", "--yes"); err == nil {
		t.Fatal("record result mismatch accepted")
	}
	recordOutput := &minutesE2ECaller{responses: map[string][]string{"minutes/" + listeningNoteCmdTool: {`{"success":true,"result":{"cmd":"end","uuid":"u1"}}`}}}
	if err := runMinutesAlignmentCLIWithWriter(t, recordOutput, minutesFailWriter{}, "minutes", "+record-stop", "--id", "u1", "--yes"); err == nil {
		t.Fatal("record output failure accepted")
	}
}
