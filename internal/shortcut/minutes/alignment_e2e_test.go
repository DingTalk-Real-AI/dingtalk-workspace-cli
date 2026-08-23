// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
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
	root.AddCommand(shortcut.Commands()...)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	if stdout.Len() == 0 {
		return nil, "", err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), decodeErr)
	}
	return payload, stdout.String(), err
}

func TestCrossPlatformCoverageMinutesSearchPaginatesFiltersAndRejectsUnknownE2E(t *testing.T) {
	caller := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_by_keyword_and_time_range": {
			`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"周会 A","startTime":1},{"uuid":"u2","title":"其他","startTime":2}],"hasNext":true,"nextToken":"n2"}}`,
			`{"success":true,"result":{"itemList":[{"uuid":"u1","title":"周会 A","startTime":1},{"uuid":"u3","title":"周会 B","startTime":3}],"hasNext":false}}`,
		},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+search", "--query", "周会", "--page-all")
	if err != nil || payload["count"] != float64(2) || payload["scannedCount"] != float64(3) || payload["pages"] != float64(2) || payload["complete"] != true {
		t.Fatalf("search payload=%#v err=%v", payload, err)
	}
	if calls := caller.arguments["minutes/list_by_keyword_and_time_range"]; len(calls) != 2 || calls[1]["nextToken"] != "n2" {
		t.Fatalf("search calls=%#v", calls)
	}

	for _, test := range []struct {
		scope     string
		belonging string
	}{
		{scope: "mine", belonging: "createdByMe"},
		{scope: "shared", belonging: "sharedToMe"},
		{scope: "all", belonging: "noLimit"},
	} {
		t.Run("scope "+test.scope, func(t *testing.T) {
			scoped := &minutesE2ECaller{responses: map[string][]string{
				"minutes/list_by_keyword_and_time_range": {`{"success":true,"result":{"itemList":[],"hasNext":false}}`},
			}}
			payload, _, err := runMinutesAlignmentCLI(t, scoped, "minutes", "+search", "--query", "needle", "--scope", test.scope, "--limit", "1")
			if err != nil || payload["count"] != float64(0) || payload["complete"] != true {
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
			`{"success":false,"errorMsg":"downstream query empty"}`,
			`{"success":true,"result":{"summaries":[{"speaker":"甲","summary":"结论"}]}}`,
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
