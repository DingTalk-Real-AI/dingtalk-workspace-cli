// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type minutesE2ECaller struct {
	responses  map[string][]string
	failAt     map[string]int
	counts     map[string]int
	arguments  map[string][]map[string]any
	beforeFail map[string]func()
	failErrors map[string]error
}

func (c *minutesE2ECaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	if c.arguments == nil {
		c.arguments = map[string][]map[string]any{}
	}
	key := product + "/" + tool
	c.counts[key]++
	c.arguments[key] = append(c.arguments[key], args)
	if c.failAt[key] == c.counts[key] {
		if hook := c.beforeFail[key]; hook != nil {
			hook()
		}
		if err := c.failErrors[key]; err != nil {
			return nil, err
		}
		return nil, errors.New("fixture failure")
	}
	responses := c.responses[key]
	text := `{"success":true,"result":{}}`
	if len(responses) > 0 {
		index := c.counts[key] - 1
		if index >= len(responses) {
			index = len(responses) - 1
		}
		text = responses[index]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (c *minutesE2ECaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func (*minutesE2ECaller) Format() string { return "json" }
func (*minutesE2ECaller) DryRun() bool   { return false }
func (*minutesE2ECaller) Fields() string { return "" }
func (*minutesE2ECaller) JQ() string     { return "" }

func runMinutesAlignmentCLI(t *testing.T, caller *minutesE2ECaller, args ...string) (map[string]any, string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().String("profile", "", "")
	root.AddCommand(shortcut.Commands()...)
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	executed, err := root.ExecuteC()
	if err == nil && output.UsesUnifiedResult(executed) {
		code, _, emitErr := output.EmitStoredResult(executed)
		if emitErr != nil {
			err = emitErr
		} else if code != 0 {
			err = fmt.Errorf("command result exit code %d", code)
		}
	}
	if stdout.Len() == 0 {
		return nil, "", err
	}
	var envelope map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), decodeErr)
	}
	if payload, ok := envelope["data"].(map[string]any); ok {
		return payload, stdout.String(), err
	}
	return envelope, stdout.String(), err
}

func minutesPaginationFromOutput(t *testing.T, raw string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode pagination envelope %q: %v", raw, err)
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("missing meta in envelope: %#v", envelope)
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("missing meta.pagination in envelope: %#v", envelope)
	}
	return pagination
}

func TestCrossPlatformCoverageMinutesSearchPaginatesFiltersAndRejectsUnknownE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {
			`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"周会 A","startTime":1},{"uuid":"u2","title":"其他","startTime":2}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"周会 A","startTime":1},{"uuid":"u3","title":"周会 B","startTime":3}],"hasNext":false}}`,
		},
	}}
	payload, raw, err := runMinutesAlignmentCLI(t, caller, "minutes", "+search", "--query", "周会", "--scope", "mine", "--page-all")
	if err != nil || payload["count"] != float64(2) || payload["scannedCount"] != float64(3) || payload["pages"] != float64(2) || payload["complete"] != true {
		t.Fatalf("search payload=%#v err=%v", payload, err)
	}
	pagination := minutesPaginationFromOutput(t, raw)
	if pagination["endpoint_exhausted"] != true || pagination["pages"] != float64(2) || pagination["items"] != float64(2) {
		t.Fatalf("search pagination=%#v", pagination)
	}
	if calls := caller.arguments["minutes/list_by_keyword_and_time_range"]; len(calls) != 2 || calls[1]["nextToken"] != "n2" {
		t.Fatalf("search calls=%#v", calls)
	}

	for _, test := range []struct {
		scope     string
		belonging string
		complete  bool
	}{
		{scope: "mine", belonging: "created", complete: true},
		{scope: "shared", belonging: "shared", complete: true},
		{scope: "all", belonging: "noLimit", complete: false},
	} {
		t.Run("scope "+test.scope, func(t *testing.T) {
			scoped := &minutesE2ECaller{responses: map[string][]string{
				"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":false}}`},
			}}
			payload, _, err := runMinutesAlignmentCLI(t, scoped, "minutes", "+search", "--query", "needle", "--scope", test.scope, "--limit", "1")
			if err != nil || payload["count"] != float64(0) || payload["complete"] != test.complete {
				t.Fatalf("scope %s payload=%#v err=%v", test.scope, payload, err)
			}
			calls := scoped.arguments["minutes/list_by_keyword_and_time_range"]
			want := map[string]any{"belongingConditionId": test.belonging, "keyword": "needle", "maxResults": 1}
			if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
				t.Fatalf("scope %s calls=%#v, want exactly %#v", test.scope, calls, want)
			}
		})
	}

	unknown := &minutesE2ECaller{responses: map[string][]string{"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{}}`}}}
	if payload, output, err := runMinutesAlignmentCLI(t, unknown, "minutes", "+search", "--query", "周会"); err == nil || payload != nil || output != "" {
		t.Fatalf("unknown list accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesDownloadEmptyMediaIsPartialFailureE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{}}`}}}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+download", "--id", "u1", "--url-only")
	if err == nil || output == "" || payload["ok"] != false || payload["failed"] != float64(1) || payload["succeeded"] != float64(0) {
		t.Fatalf("empty media accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesUpdateRequiresVerifiedReadbackE2E(t *testing.T) {
	success := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"旧标题"}}`, `{"success":true,"result":{"taskUuid":"u1","title":"新标题"}}`},
		"minutes/update_minutes_title":   {`{"success":true,"result":{"updated":true}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, success, "minutes", "+update", "--id", "u1", "--title", "新标题", "--yes")
	if err != nil || payload["changed"] != true || payload["verified"] != true {
		t.Fatalf("verified update payload=%#v err=%v", payload, err)
	}

	mismatch := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"旧标题"}}`, `{"success":true,"result":{"taskUuid":"u1","title":"仍是旧标题"}}`},
		"minutes/update_minutes_title":   {`{"success":true,"result":{"updated":true}}`},
	}}
	if payload, output, err := runMinutesAlignmentCLI(t, mismatch, "minutes", "+update", "--id", "u1", "--title", "新标题", "--yes"); err == nil || payload != nil || output != "" {
		t.Fatalf("readback mismatch accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesSummaryProtectsImagesAndVerifiesE2E(t *testing.T) {
	missingImage := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"旧内容\n![图](https://example.invalid/a.png)"}}`},
	}}
	if payload, output, err := runMinutesAlignmentCLI(t, missingImage, "minutes", "+summary", "--id", "u1", "--content", "新内容", "--yes"); err == nil || payload != nil || output != "" || missingImage.counts["minutes/update_minutes_summary"] != 0 {
		t.Fatalf("missing image accepted: payload=%#v output=%q err=%v calls=%#v", payload, output, err, missingImage.counts)
	}

	content := "新内容\n![图](https://example.invalid/a.png)"
	success := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"旧内容\n![图](https://example.invalid/a.png)"}}`, `{"success":true,"result":{"fullSummary":"新内容\n![图](https://example.invalid/a.png)"}}`},
		"minutes/update_minutes_summary": {`{"success":true,"result":{"updated":true}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, success, "minutes", "+summary", "--id", "u1", "--content", content, "--yes")
	if err != nil || payload["changed"] != true || payload["verified"] != true || payload["preservedImages"] != true {
		t.Fatalf("summary payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesSpeakerReplacePaginatesAndVerifiesE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","speakerNick":"甲"}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p2","speakerNick":"甲"}],"hasNext":false}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","speakerNick":"乙"}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"paragraphList":[{"paragraphId":"p2","speakerNick":"乙"}],"hasNext":false}}`,
		},
		"minutes/replace_speaker": {`{"success":true,"result":{"updated":true}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+speaker-replace", "--id", "u1", "--from", "甲", "--to", "乙", "--yes")
	if err != nil || payload["verified"] != true || payload["affectedParagraphs"] != float64(2) || caller.counts["minutes/get_minutes_transcription"] != 4 {
		t.Fatalf("speaker payload=%#v err=%v calls=%#v", payload, err, caller.counts)
	}
}

func TestCrossPlatformCoverageMinutesUploadAndPermissionDryRunDoNotWriteE2E(t *testing.T) {
	work := t.TempDir()
	file := filepath.Join(work, "source.wav")
	if err := os.WriteFile(file, []byte("non-empty-audio-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &minutesE2ECaller{}
	upload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+upload", "--file", file, "--title", "E2E", "--dry-run")
	if err != nil || upload["dryRun"] != true || upload["executed"] != false || len(caller.counts) != 0 {
		t.Fatalf("upload dry-run=%#v err=%v calls=%#v", upload, err, caller.counts)
	}
	permission, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+apply-permission", "--id", "u1", "--permission", "edit", "--dry-run")
	if err != nil || permission["policyId"] != float64(2) || permission["executed"] != false || len(caller.counts) != 0 {
		t.Fatalf("permission dry-run=%#v err=%v calls=%#v", permission, err, caller.counts)
	}
}

func TestCrossPlatformCoverageMinutesShortcutConfirmationPolicyE2E(t *testing.T) {
	for _, value := range []shortcut.Shortcut{Upload, UploadAndAnalyze, UploadAndNotify, Mindmap, SpeakerInsights, PrepareASR, SyncASR} {
		if value.Safety.Confirmation != "user_required" {
			t.Errorf("%s confirmation=%q, want user_required", value.Command, value.Safety.Confirmation)
		}
	}

	file := filepath.Join(t.TempDir(), "notify.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "upload", args: []string{"minutes", "+upload", "--file", file}},
		{name: "upload and analyze", args: []string{"minutes", "+upload-and-analyze", "--file", file}},
		{name: "upload notify", args: []string{"minutes", "+upload-and-notify", "--file", file}},
		{name: "sync asr", args: []string{"minutes", "+sync-asr", "--words", "DWS"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &minutesE2ECaller{}
			_, _, err := runMinutesAlignmentCLI(t, caller, test.args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
				t.Fatalf("unconfirmed error=%#v", err)
			}
			if len(caller.counts) != 0 {
				t.Fatalf("remote calls before confirmation=%#v", caller.counts)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesUploadUnknownCompletionPreservesSessionE2E(t *testing.T) {
	work := t.TempDir()
	file := filepath.Join(work, "response-loss.wav")
	if err := os.WriteFile(file, []byte("non-empty-audio-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &minutesPutFile, func(context.Context, string, string, int64) (localio.UploadResult, error) {
		return localio.UploadResult{SizeBytes: 23, Attempts: 1}, nil
	})

	serverCompleted := false
	caller := &minutesE2ECaller{
		responses: map[string][]string{
			"minutes/create_upload_session": {`{"success":true,"result":{"sessionId":"session-redacted","presignedUrl":"https://upload.example.invalid/object"}}`},
		},
		failAt: map[string]int{"minutes/complete_upload_session": 1},
		beforeFail: map[string]func(){
			"minutes/complete_upload_session": func() { serverCompleted = true },
		},
	}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+upload", "--file", file, "--yes")
	if err == nil || payload != nil || output != "" || !serverCompleted {
		t.Fatalf("response-loss outcome = payload:%#v output:%q err:%v completed:%v", payload, output, err, serverCompleted)
	}
	if caller.counts["minutes/cancel_upload_session"] != 0 {
		t.Fatalf("unknown remote completion was cancelled: calls=%#v", caller.counts)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "minutes_upload_completion_unknown" || typed.Retryable || typed.FailureStage != "complete" {
		t.Fatalf("unknown completion error = %#v", err)
	}
	if typed.Details["sessionId"] != "session-redacted" || typed.Details["cancelled"] != false || typed.Details["remoteEffect"] != "unknown" {
		t.Fatalf("unknown completion recovery details = %#v", typed.Details)
	}
}

func TestCrossPlatformCoverageMinutesMindmapExplicitStatusControlsExitE2E(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     string
		wantErr    bool
		wantResult bool
	}{
		{name: "success", status: `{"success":true,"result":{"taskStatus":1,"mindGraph":"ready"}}`, wantResult: true},
		{name: "platform failed", status: `{"success":true,"result":{"taskStatus":2}}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &minutesE2ECaller{responses: map[string][]string{
				"minutes/create_mind_graph":       {`{"success":true,"result":{}}`},
				"minutes/query_mind_graph_status": {test.status},
			}}
			payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+mindmap", "--id", "u1", "--timeout", "2", "--interval", "1", "--yes")
			if output == "" || (err != nil) != test.wantErr || payload["complete"] != test.wantResult {
				t.Fatalf("mindmap payload=%#v output=%q err=%v", payload, output, err)
			}
			if caller.counts["minutes/create_mind_graph"] != 1 {
				t.Fatalf("mindmap create repeated: %#v", caller.counts)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesSpeakerInsightsRequiresTaskAndResultE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/create_speaker_summary": {`{"success":true,"result":{"taskId":"job-1","status":"processing"}}`},
		"minutes/get_speaker_summary": {
			`{"success":true,"result":{"status":"processing","taskId":"job-1"}}`,
			`{"success":true,"result":{"status":"completed","innerStatus":"Finished","success":true,"content":"总结","errorMsg":"","taskId":"job-1"}}`,
		},
	}}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+speaker-insights", "--id", "u1", "--timeout", "3", "--interval", "1", "--yes")
	if err != nil || output == "" || payload["complete"] != true || payload["taskId"] != "job-1" || payload["attempts"] != float64(2) {
		t.Fatalf("speaker insights payload=%#v output=%q err=%v", payload, output, err)
	}

	missingTask := &minutesE2ECaller{responses: map[string][]string{"minutes/create_speaker_summary": {`{"success":true,"result":{"status":"processing"}}`}}}
	if payload, output, err := runMinutesAlignmentCLI(t, missingTask, "minutes", "+speaker-insights", "--id", "u1", "--timeout", "2", "--interval", "1", "--yes"); err == nil || output == "" || payload["complete"] != false {
		t.Fatalf("missing task accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesPrepareASRDiffWritesAndReadbackE2E(t *testing.T) {
	migrated := &minutesE2ECaller{}
	if payload, output, err := runMinutesAlignmentCLI(t, migrated, "minutes", "+prepare-asr", "--words", "DWS", "--sync", "--yes"); err == nil || !strings.Contains(err.Error(), "--sync 已迁移") || payload != nil || output != "" {
		t.Fatalf("legacy --sync was not rejected by validation: payload=%#v output=%q err=%v", payload, output, err)
	}
	if len(migrated.counts) != 0 {
		t.Fatalf("legacy --sync reached MCP before migration error: calls=%#v", migrated.counts)
	}

	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_my_hotwords": {
			`{"success":true,"result":{"hotWordList":["已有"],"currentCount":1}}`,
			`{"success":true,"result":{"hotWordList":["已有","DWS"],"currentCount":2}}`,
		},
		"minutes/add_personal_hot_word": {`{"success":true,"result":{}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+prepare-asr", "--words", "已有,DWS", "--yes")
	if err != nil || payload["complete"] != true || payload["verified"] != true || caller.counts["minutes/add_personal_hot_word"] != 1 || caller.counts["minutes/delete_personal_hotword"] != 0 {
		t.Fatalf("prepare asr payload=%#v err=%v calls=%#v", payload, err, caller.counts)
	}

	unknown := &minutesE2ECaller{responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{}}`}}}
	if payload, output, err := runMinutesAlignmentCLI(t, unknown, "minutes", "+prepare-asr", "--words", "DWS", "--yes"); err == nil || payload != nil || output != "" {
		t.Fatalf("unknown hotword list accepted: payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesRecordWrapUpStopSuccessArtifactFailureIsPartialE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/" + listeningNoteCmdTool: {`{"success":true,"result":{"cmd":"end","uuid":"u1"}}`},
		"minutes/get_minutes_ai_summary":  {`{"success":true,"result":{}}`},
	}}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+record-wrap-up", "--id", "u1", "--artifacts", "summary", "--wait-timeout", "1", "--poll-interval", "1", "--yes")
	if err == nil || output == "" || payload["complete"] != false || payload["taskUuid"] != "u1" || payload["recovery"] == nil || caller.counts["minutes/"+listeningNoteCmdTool] != 1 {
		t.Fatalf("wrap-up partial payload=%#v output=%q err=%v calls=%#v", payload, output, err, caller.counts)
	}
}

func TestCrossPlatformCoverageMinutesExportPackPublishesOnlyCompleteArtifactsE2E(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"E2E"}}`},
		"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"完整纪要"}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic,summary")
	if err != nil || payload["complete"] != true || payload["published"] != true {
		t.Fatalf("export payload=%#v err=%v", payload, err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(work, "pack", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(manifestRaw, []byte("https://")) || !bytes.Contains(manifestRaw, []byte(`"complete": true`)) {
		t.Fatalf("manifest contains secret URL or is incomplete: %s", manifestRaw)
	}
}

func TestCrossPlatformCoverageMinutesSharePartialWriteIsNonZeroE2E(t *testing.T) {
	caller := &minutesE2ECaller{
		responses: map[string][]string{"minutes/add_member_permission": {`{"success":true,"result":{}}`, `{"success":false,"errorMsg":"denied"}`}},
	}
	payload, output, err := runMinutesAlignmentCLI(t, caller, "minutes", "+share", "--id", "u1", "--member-uids", "m1,m2,m3", "--permission", "view", "--yes")
	if err == nil || output == "" || payload["complete"] != false || payload["succeeded"] != float64(1) || payload["failed"] != float64(1) || len(payload["unattempted"].([]any)) != 1 {
		t.Fatalf("share partial payload=%#v output=%q err=%v", payload, output, err)
	}
}

func TestCrossPlatformCoverageMinutesUploadAndAnalyzeDryRunDoesNotWriteE2E(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(file, []byte("non-empty-audio-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &minutesE2ECaller{}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+upload-and-analyze", "--file", file, "--artifacts", "summary,transcript", "--mindmap", "--dry-run")
	if err != nil || payload["dryRun"] != true || payload["executed"] != false || len(caller.counts) != 0 {
		t.Fatalf("upload-and-analyze dry-run=%#v err=%v calls=%#v", payload, err, caller.counts)
	}
}

func TestCrossPlatformCoverageMinutesArtifactWaitDoesNotTreatEmptyAnalysisAsReadyE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[],"hasNext":false}}`},
	}}
	// Exercise the same readiness collector used after upload/record stop. An
	// explicit [] is a valid transport shape, but cannot prove asynchronous ASR
	// has finished when the workflow promised transcript analysis.
	helpers.InitDepsForTest(t, caller)
	rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "+export-pack"}, ExportPack)
	bundle, failures := collectMinutesArtifactsOnce(rt, "u1", []string{"transcript"}, 10)
	if len(failures) != 1 || bundle["transcript"] != nil {
		t.Fatalf("empty transcript accepted: bundle=%#v failures=%#v", bundle, failures)
	}
}

func TestCrossPlatformCoverageMinutesExportSummarySignedLinks(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"plain text", "plain text"},
		{"[link](https://example.test/a)", "[link](https://example.test/a)"},
		{"[link](https://example.test/a?page=2#part)", "[link](https://example.test/a?page=2#part)"},
		{"![图](https://example.test/a.png?OSSAccessKeyId=secret&Expires=1&Signature=secret)", "![图]([signed-url-removed])"},
		{"<img src=\"https://example.test/a?foo=1&amp;Signature=secret\">", "<img src=\"[signed-url-removed]\">"},
		{"https://example.test/a?%53ignature=secret", exportRemovedURL},
		{"https://example.test/a?X-Amz-Credential=secret", exportRemovedURL},
		{"https://example.test/a?X-Oss-Signature=secret", exportRemovedURL},
		{"https://example.test/a?bad%zz=value", exportRemovedURL},
	} {
		if got := (&exportRedactions{}).text(tc.input); got != tc.want {
			t.Errorf("sanitizeExportSummary(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageMinutesExportSummarySanitizedOnDisk(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"纪要\n![图](https://example.test/a.png?OSSAccessKeyId=secret&Signature=secret)\n[文档](https://example.test/doc?id=1)"}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "summary")
	if err != nil || payload["published"] != true {
		t.Fatalf("payload=%v err=%v", payload, err)
	}
	data, err := os.ReadFile(filepath.Join(work, "pack", "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "纪要\n![图]([signed-url-removed])\n[文档](https://example.test/doc?id=1)" {
		t.Fatalf("unexpected archive content: %s", data)
	}
}

func TestCrossPlatformCoverageMinutesExportNestedCredentials(t *testing.T) {
	original := map[string]any{"id": int64(9007199254740993), "nested": []map[string]any{{"text": `{"url":"https:\/\/example.test\/a?Signature=CANARY"}`, "Signature": "CANARY"}}, "plain": "https://example.test/doc?id=7"}
	clean, count, err := sanitizeExportArtifact(original)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	raw, _ := json.Marshal(clean)
	if strings.Contains(string(raw), "CANARY") || !strings.Contains(string(raw), "9007199254740993") || containsExportCredentials(clean) {
		t.Fatalf("unsafe/changed result: %s", raw)
	}
	if original["nested"].([]map[string]any)[0]["Signature"] != "CANARY" {
		t.Fatal("mutated original payload")
	}
	if _, _, err := sanitizeExportArtifact(make(chan int)); err == nil {
		t.Fatal("unencodable value accepted")
	}
}

func TestCrossPlatformCoverageMinutesExportScanRejectsResidual(t *testing.T) {
	for _, body := range []string{`{"nested":["https://example.test/a?Signature=CANARY"]}`, `{"Signature":"CANARY"}`, `{"https://example.test/?Signature=CANARY":0}`, `{broken`} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "basic.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := scanExportCredentials(dir, []string{"basic"}, false); err == nil || strings.Contains(err.Error(), "CANARY") {
			t.Fatalf("scan error=%v", err)
		}
	}
}

func TestCrossPlatformCoverageMinutesExportAllTextAndNoPublishOnLeak(t *testing.T) {
	for _, inject := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "injected"}[inject], func(t *testing.T) {
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			work := t.TempDir()
			if err := os.Chdir(work); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })
			if inject {
				write := minutesWriteFile
				testseam.Swap(t, &minutesWriteFile, func(path string, raw []byte, mode os.FileMode) error {
					if filepath.Base(path) == "basic.json" {
						raw = []byte(`{"url":"https://example.test/?Signature=CANARY"}`)
					}
					return write(path, raw, mode)
				})
			}
			caller := &minutesE2ECaller{responses: map[string][]string{
				"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"https://example.test/?Signature=CANARY"}}`},
				"minutes/list_minutes_todos":     {`{"success":true,"result":{"actions":["https://example.test/?OSSAccessKeyId=CANARY&Signature=CANARY"]}}`},
			}}
			payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic,todos")
			if inject {
				if err == nil || payload["published"] != false {
					t.Fatalf("leak published: %v %v", payload, err)
				}
				if _, err := os.Stat("pack"); !os.IsNotExist(err) {
					t.Fatal("target exists on failure")
				}
				entries, _ := os.ReadDir(work)
				if len(entries) != 0 {
					t.Fatal("temporary export not cleaned")
				}
				return
			}
			if err != nil || payload["sanitized"] != true || payload["redactionCount"].(float64) < 2 {
				t.Fatalf("payload=%v err=%v", payload, err)
			}
			entries, _ := os.ReadDir("pack")
			for _, entry := range entries {
				raw, err := os.ReadFile(filepath.Join("pack", entry.Name()))
				if err != nil || strings.Contains(string(raw), "CANARY") {
					t.Fatal("secret in archive")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesExportScanIOFailures(t *testing.T) {
	if err := scanExportCredentials(filepath.Join(t.TempDir(), "missing"), nil, false); err == nil {
		t.Fatal("missing export directory accepted")
	}
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "basic.json")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := scanExportCredentials(dir, []string{"basic"}, false); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink scan=%v", err)
		}
	})
	for _, kind := range []string{"relative-path", "read", "unexpected", "trailing-json"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			body := `{}`
			if kind == "trailing-json" {
				body = `{} {}`
			}
			if err := os.WriteFile(filepath.Join(dir, "basic.json"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			artifacts := []string{"basic"}
			switch kind {
			case "relative-path":
				testseam.Swap(t, &minutesRel, func(string, string) (string, error) { return "", errors.New("path failure") })
			case "read":
				testseam.Swap(t, &minutesReadFile, func(string) ([]byte, error) { return nil, errors.New("read failure") })
			case "unexpected":
				artifacts = nil
			}
			if err := scanExportCredentials(dir, artifacts, false); err == nil {
				t.Fatalf("%s failure accepted", kind)
			}
		})
	}
	var nested any = "body"
	for i := 0; i < 10001; i++ {
		nested = []any{nested}
	}
	if _, _, err := sanitizeExportArtifact(nested); err == nil || !strings.Contains(err.Error(), "decoded") {
		t.Fatalf("over-depth JSON accepted: %v", err)
	}
}

func TestCrossPlatformCoverageMinutesExportSanitizerFailureDoesNotPublish(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	sentinel := errors.New("sanitizer rejected artifact")
	testseam.Swap(t, &minutesSanitizeExportArtifact, func(any) (any, int, error) { return nil, 0, sentinel })
	caller := &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"example"}}`}}}
	_, _, err = runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic")
	if !errors.Is(err, sentinel) {
		t.Fatalf("sanitizer error=%v", err)
	}
	entries, err := os.ReadDir(work)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed export left files: %v %v", entries, err)
	}
}

func TestCrossPlatformCoverageMinutesListDisplayFinalData(t *testing.T) {
	for _, command := range []string{"+list-mine", "+list-shared", "+list-all", "+search"} {
		c := &minutesE2ECaller{responses: map[string][]string{
			"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[{"taskUuid":"u1","title":"周会","orgName":"source org","flashUserInfo":{"name":"display user","extra":"must-not-copy"}}],"hasNext":false}}`},
		}}
		args := []string{"minutes", command, "--page-all"}
		wantCalls := 1
		if command == "+search" {
			args = append(args, "--query", "周会", "--scope", "all")
		}
		if command == "+search" || command == "+list-all" {
			wantCalls = 2
		}
		p, _, err := runMinutesAlignmentCLI(t, c, args...)
		if err != nil {
			t.Fatal(err)
		}
		rows := p["minutes"].([]any)
		if len(rows) != 1 {
			t.Fatalf("rows=%#v", rows)
		}
		row := rows[0].(map[string]any)
		if row["orgName"] != "source org" || !reflect.DeepEqual(row["flashUserInfo"], map[string]any{"name": "display user"}) || row["creator"] != nil {
			t.Fatalf("%s row=%#v", command, row)
		}
		if len(c.counts) != 1 || c.counts["minutes/list_by_keyword_and_time_range"] != wantCalls {
			t.Fatalf("unexpected extra calls=%v", c.counts)
		}
	}
}

func TestCrossPlatformCoverageMinutesStaffIDPermissionPlanParity(t *testing.T) {
	args := []string{"minutes", "+share", "--ids", "u1,u2", "--member-staff-ids", "007,008", "--permission", "view", "--sub-resources", "Summary", "--cover=false", "--failure-policy", "continue"}
	preview := &minutesE2ECaller{}
	plan, _, err := runMinutesAlignmentCLI(t, preview, append(append([]string{}, args...), "--dry-run")...)
	if err != nil || len(preview.counts) != 0 || plan["executed"] != false || !reflect.DeepEqual(plan["members"], []any{"007", "008"}) {
		t.Fatalf("plan=%#v calls=%v err=%v", plan, preview.counts, err)
	}
	options := plan["options"].(map[string]any)
	wantOptions := map[string]any{"policyId": float64(4), "roleSubResourceIds": []any{"Summary"}, "coverPermission": "false"}
	if !reflect.DeepEqual(options, wantOptions) || plan["failurePolicy"] != "continue" {
		t.Fatalf("plan options=%#v", plan)
	}
	live := &minutesE2ECaller{}
	result, _, err := runMinutesAlignmentCLI(t, live, append(append([]string{}, args...), "--yes")...)
	if err != nil {
		t.Fatal(err)
	}
	calls := live.arguments["minutes/add_member_permission"]
	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	for i, member := range []string{"007", "008"} {
		raw, err := json.Marshal(calls[i])
		if err != nil {
			t.Fatal(err)
		}
		var actual map[string]any
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"uuids": []any{"u1", "u2"}, "memberStaffIds": []any{member}}
		for key, value := range wantOptions {
			want[key] = value
		}
		if !reflect.DeepEqual(actual, want) {
			t.Fatalf("call=%#v want=%#v", actual, want)
		}
		row := result["results"].([]any)[i].(map[string]any)
		if row["memberStaffId"] != member || row["memberUid"] != nil || row["complete"] != true {
			t.Fatalf("receipt=%#v", row)
		}
	}
}

func TestCrossPlatformCoverageMinutesPermissionPlanScope(t *testing.T) {
	for _, cover := range []string{"", "--cover", "--cover=false"} {
		args := []string{"minutes", "+share", "--ids", "u1,u2", "--member-uids", "m1,m2", "--permission", "view", "--sub-resources", "Summary", "--failure-policy", "continue"}
		if cover != "" {
			args = append(args, cover)
		}
		previewCaller := &minutesE2ECaller{}
		p, _, err := runMinutesAlignmentCLI(t, previewCaller, append(append([]string{}, args...), "--dry-run")...)
		if err != nil || len(previewCaller.counts) != 0 || p["executed"] != false || p["permission"] != "view" || p["failurePolicy"] != "continue" {
			t.Fatalf("plan=%#v err=%v", p, err)
		}
		options := p["options"].(map[string]any)
		if options["policyId"] != float64(4) || !reflect.DeepEqual(options["roleSubResourceIds"], []any{"Summary"}) {
			t.Fatalf("options=%#v", options)
		}
		expectedCover := map[string]any{"--cover": "true", "--cover=false": "false"}[cover]
		if options["coverPermission"] != expectedCover {
			t.Fatalf("cover=%#v want=%#v", options["coverPermission"], expectedCover)
		}
		live := &minutesE2ECaller{}
		if _, _, err := runMinutesAlignmentCLI(t, live, append(append([]string{}, args...), "--yes")...); err != nil {
			t.Fatal(err)
		}
		calls := live.arguments["minutes/add_member_permission"]
		if len(calls) != 2 {
			t.Fatalf("calls=%v", calls)
		}
		for _, call := range calls {
			for key, expected := range options {
				raw, err := json.Marshal(call[key])
				if err != nil {
					t.Fatal(err)
				}
				var actual any
				if err := json.Unmarshal(raw, &actual); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("%s actual=%#v plan=%#v", key, actual, expected)
				}
			}
		}
	}
	c := &minutesE2ECaller{}
	p, _, err := runMinutesAlignmentCLI(t, c, "minutes", "+unshare", "--ids", "u1,u2", "--member-uids", "m1,m2", "--failure-policy", "continue", "--dry-run")
	if err != nil || len(c.counts) != 0 || p["failurePolicy"] != "continue" || p["options"] != nil || p["permission"] != nil || p["memberCount"] != float64(2) {
		t.Fatalf("unshare=%#v err=%v", p, err)
	}
	c = &minutesE2ECaller{}
	p, _, err = runMinutesAlignmentCLI(t, c, "minutes", "+share", "--id", "u1", "--member-uids", "m1", "--permission", "view", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	options := p["options"].(map[string]any)
	if options["coverPermission"] != nil || options["roleSubResourceIds"] != nil || len(c.counts) != 0 {
		t.Fatal("invented unset options or remote call")
	}
}

const speakerReadyFixture = `{"success":true,"result":{"status":"completed","innerStatus":"Finished","success":true,"content":"Synthetic speaker summary","errorMsg":"","taskId":"job"}}`

func TestCrossPlatformCoverageMinutesSpeakerTerminalStates(t *testing.T) {
	for _, tc := range []struct {
		name, response, state string
		wantErr               bool
	}{
		{"ready", speakerReadyFixture, "ready", false},
		{"pending", `{"success":true,"result":{"status":"processing","taskId":"job"}}`, "pending", true},
		{"unknown", `{"success":true,"result":{"anything":"nonempty"}}`, "unsupported_shape", true},
		{"failed", `{"success":true,"result":{"success":false,"errorMsg":"failed"}}`, "failed", true},
		{"mismatch", `{"success":true,"result":{"status":"processing","taskId":"other"}}`, "unsupported_shape", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &minutesE2ECaller{responses: map[string][]string{"minutes/get_speaker_summary": {tc.response}}}
			p, _, err := runMinutesAlignmentCLI(t, c, "minutes", "+speaker-insights", "--id", "u1", "--resume", "--task-id", "job", "--timeout", "1", "--interval", "1", "--yes")
			if (err != nil) != tc.wantErr || p["state"] != tc.state || p["complete"] != (tc.state == "ready") {
				t.Fatalf("payload=%#v err=%v", p, err)
			}
			if c.counts["minutes/create_speaker_summary"] != 0 || c.counts["minutes/get_speaker_summary"] != 1 {
				t.Fatalf("calls=%v", c.counts)
			}
			if tc.wantErr && (p["taskId"] != "job" || p["recovery"] == nil) {
				t.Fatalf("lost recovery: %#v", p)
			}
		})
	}
	apiErr := apperrors.NewAPI("downstream query empty", apperrors.WithReason("business_error"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "000"}))
	if !speakerSummaryPending(apiErr) {
		t.Fatal("observed structured error not recognized")
	}
	otherErr := apperrors.NewAPI("permission denied processing", apperrors.WithReason("business_error"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "000"}))
	if speakerSummaryPending(otherErr) {
		t.Fatal("unrelated error retried")
	}
	c := &minutesE2ECaller{}
	if _, _, err := runMinutesAlignmentCLI(t, c, "minutes", "+speaker-insights", "--id", "u1", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(c.counts) != 0 {
		t.Fatal("dry run made remote calls")
	}
}

func TestCrossPlatformCoverageMinutesSpeakerUnavailableRecovery(t *testing.T) {
	unavailable := apperrors.NewAPI("downstream query empty", apperrors.WithReason("business_error"), apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "000"}))
	c := &minutesE2ECaller{
		responses:  map[string][]string{"minutes/get_speaker_summary": {speakerReadyFixture}},
		failAt:     map[string]int{"minutes/get_speaker_summary": 1},
		failErrors: map[string]error{"minutes/get_speaker_summary": unavailable},
	}
	p, _, err := runMinutesAlignmentCLI(t, c, "minutes", "+speaker-insights", "--id", "u1", "--resume", "--task-id", "job", "--timeout", "3", "--interval", "1", "--yes")
	if err != nil || p["state"] != "ready" || p["attempts"] != float64(2) || c.counts["minutes/create_speaker_summary"] != 0 {
		t.Fatalf("payload=%#v calls=%v err=%v", p, c.counts, err)
	}
	c = &minutesE2ECaller{responses: map[string][]string{"minutes/get_speaker_summary": {`{"success":true,"result":{"status":"processing","taskId":"job"}}`}}}
	p, _, err = runMinutesAlignmentCLI(t, c, "minutes", "+speaker-insights", "--id", "u1", "--resume", "--task-id", "job", "--timeout", "1", "--interval", "1", "--profile", "corp:user", "--yes")
	if err == nil || p["retryable"] != true {
		t.Fatalf("timeout = %#v, %v", p, err)
	}
	recovery := p["recovery"].(map[string]any)
	want := []any{"dws", "minutes", "+speaker-insights", "--id", "u1", "--resume", "--task-id", "job", "--timeout", "1", "--interval", "1", "--profile", "corp:user"}
	if !reflect.DeepEqual(recovery["nextCommand"], want) {
		t.Fatalf("recovery=%#v", recovery)
	}
	c = &minutesE2ECaller{}
	if _, _, err = runMinutesAlignmentCLI(t, c, "minutes", "+speaker-insights", "--id", "u1"); err == nil || len(c.counts) != 0 {
		t.Fatal("confirmation gate crossed")
	}
}

func TestCrossPlatformCoverageMinutesUploadPlanOptions(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"+upload", "+upload-and-notify", "+upload-and-analyze"} {
		caller := &minutesE2ECaller{}
		p, _, err := runMinutesAlignmentCLI(t, caller, "minutes", command, "--file", file, "--title", " meeting ", "--input-language", "zh", "--template-id", "template-A", "--complete-timeout", "120", "--poll-interval", "4", "--dry-run")
		if err != nil || len(caller.counts) != 0 || p["executed"] != false {
			t.Fatalf("plan=%#v err=%v", p, err)
		}
		if command == "+upload-and-analyze" {
			p = p["upload"].(map[string]any)
		}
		options := p["options"].(map[string]any)
		if options["inputLanguage"] != "zh" || options["templateId"] != "template-A" || p["title"] != "meeting" || p["completeTimeoutSeconds"] != float64(120) || p["pollIntervalSeconds"] != float64(4) {
			t.Fatalf("lost options=%#v", p)
		}
		if command == "+upload-and-notify" && options["enableMessageCard"] != true {
			t.Fatal("lost notification setting")
		}
	}
	c := &minutesE2ECaller{}
	p, _, err := runMinutesAlignmentCLI(t, c, "minutes", "+upload", "--file", file, "--dry-run")
	if err != nil || len(c.counts) != 0 || len(p["options"].(map[string]any)) != 0 || p["completeTimeoutSeconds"] != float64(90) || p["pollIntervalSeconds"] != float64(2) {
		t.Fatalf("defaults=%#v err=%v", p, err)
	}
	for _, args := range [][]string{
		{"minutes", "+upload", "--file", file, "--complete-timeout", "0", "--dry-run"},
		{"minutes", "+upload", "--file", filepath.Join(t.TempDir(), "missing.wav"), "--dry-run"},
	} {
		c := &minutesE2ECaller{}
		if _, _, err := runMinutesAlignmentCLI(t, c, args...); err == nil || len(c.counts) != 0 {
			t.Fatal("invalid plan did not fail locally")
		}
	}
}
