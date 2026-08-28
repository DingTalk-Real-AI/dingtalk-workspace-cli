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
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMinutesWorkflowValidationAndDefaults(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid := [][]string{
		{"minutes", "+record-wrap-up", "--id", "u1", "--wait-timeout", "0"},
		{"minutes", "+upload-and-analyze", "--file", file, "--complete-timeout", "0"},
		{"minutes", "+mindmap", "--id", "u1", "--timeout", "0"},
		{"minutes", "+speaker-insights", "--id", "u1", "--interval", "0"},
		{"minutes", "+export-pack", "--id", "u1", "--output", "pack", "--page-limit", "0"},
		{"minutes", "+share", "--id", "u1", "--member-uids", strings.Repeat("m,", 51), "--permission", "view"},
	}
	for _, args := range invalid {
		if payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, args...); err == nil || payload != nil || output != "" {
			t.Fatalf("invalid workflow accepted: %v payload=%#v output=%q err=%v", args, payload, output, err)
		}
	}

	caller := &minutesE2ECaller{}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "workflow"}
	cmd.Flags().StringSlice("artifacts", nil, "")
	rt := shortcut.RuntimeContextForTest(cmd, RecordWrapUp)
	selected := selectedWorkflowArtifacts(rt)
	selected[0] = "mutated"
	if minutesWorkflowArtifacts[0] == "mutated" || len(selected) != len(minutesWorkflowArtifacts) {
		t.Fatalf("default artifacts alias global: %#v", selected)
	}
}

func TestCrossPlatformCoverageMinutesRecordWrapUpBranchesE2E(t *testing.T) {
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+record-wrap-up", "--id", "u1", "--dry-run"); err == nil {
		t.Fatal("record dry-run output failure accepted")
	}
	stopFailure := &minutesE2ECaller{failAt: map[string]int{"minutes/" + listeningNoteCmdTool: 1}}
	if _, _, err := runMinutesAlignmentCLI(t, stopFailure, "minutes", "+record-wrap-up", "--id", "u1", "--yes"); err == nil {
		t.Fatal("record stop failure accepted")
	}
	badStop := &minutesE2ECaller{responses: map[string][]string{"minutes/" + listeningNoteCmdTool: {`{"success":true,"result":{"cmd":"pause","uuid":"u1"}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, badStop, "minutes", "+record-wrap-up", "--id", "u1", "--yes"); err == nil {
		t.Fatal("record mismatched acknowledgement accepted")
	}
	success := &minutesE2ECaller{responses: map[string][]string{
		"minutes/" + listeningNoteCmdTool: {`{"success":true,"result":{"cmd":"end","uuid":"u1"}}`},
		"minutes/get_minutes_basic_info":  {`{"success":true,"result":{"taskUuid":"u1","title":"done"}}`},
	}}
	payload, output, err := runMinutesAlignmentCLI(t, success, "minutes", "+record-wrap-up", "--id", "u1", "--artifacts", "basic", "--yes")
	if err != nil || output == "" || payload["complete"] != true {
		t.Fatalf("record success payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesUploadAndAnalyzeBranchesE2E(t *testing.T) {
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+upload-and-analyze", "--resume-id", "u1", "--dry-run"); err == nil {
		t.Fatal("resume dry-run output failure accepted")
	}
	if _, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+upload-and-analyze", "--file", filepath.Join(t.TempDir(), "missing"), "--yes"); err == nil {
		t.Fatal("upload-and-analyze invalid file accepted")
	}
	if _, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+upload-and-analyze", "--file", filepath.Join(t.TempDir(), "missing"), "--dry-run"); err == nil {
		t.Fatal("upload-and-analyze invalid dry-run file accepted")
	}

	resume := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info":  {`{"success":true,"result":{"taskUuid":"u1","title":"done"}}`},
		"minutes/create_mind_graph":       {`{"success":true,"result":{}}`},
		"minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":1,"mindGraph":"ready"}}`},
		"minutes/create_speaker_summary":  {`{"success":true,"result":{"taskId":"job","status":"processing"}}`},
		"minutes/get_speaker_summary":     {`{"success":true,"result":{"summaries":[{"speaker":"a","summary":"b"}]}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, resume, "minutes", "+upload-and-analyze", "--resume-id", "u1", "--artifacts", "basic", "--mindmap", "--speaker-insights", "--yes")
	if err != nil || payload["complete"] != true || payload["taskUuid"] != "u1" {
		t.Fatalf("resume payload=%#v err=%v", payload, err)
	}

	partial := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info":  {`{"success":true,"result":{"taskUuid":"u1","title":"done"}}`},
		"minutes/create_mind_graph":       {`{"success":true,"result":{}}`},
		"minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":2}}`},
		"minutes/create_speaker_summary":  {`{"success":true,"result":{"taskId":"job","status":"processing"}}`},
		"minutes/get_speaker_summary":     {`{"success":false,"errorMsg":"denied"}`},
	}}
	payload, output, err := runMinutesAlignmentCLI(t, partial, "minutes", "+upload-and-analyze", "--resume-id", "u1", "--artifacts", "basic", "--mindmap", "--speaker-insights", "--yes")
	if err == nil || output == "" || payload["complete"] != false || payload["recovery"] == nil {
		t.Fatalf("partial payload=%#v err=%v", payload, err)
	}

	file := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &minutesPutFile, func(context.Context, string, string, int64) (localio.UploadResult, error) {
		return localio.UploadResult{SizeBytes: 5, Attempts: 1}, nil
	})
	upload := &minutesE2ECaller{responses: map[string][]string{
		"minutes/create_upload_session":   {`{"success":true,"result":{"sessionId":"s1","presignedUrl":"https://example.invalid/u"}}`},
		"minutes/complete_upload_session": {`{"success":true,"result":{"taskUuid":"u2"}}`},
		"minutes/get_minutes_basic_info": {
			`{"success":true,"result":{"taskUuid":"u2","title":"uploaded"}}`,
			`{"success":true,"result":{"taskUuid":"u2","title":"uploaded"}}`,
		},
	}}
	payload, _, err = runMinutesAlignmentCLI(t, upload, "minutes", "+upload-and-analyze", "--file", file, "--artifacts", "basic", "--enable-message-card", "--yes")
	if err != nil || payload["taskUuid"] != "u2" || payload["complete"] != true {
		t.Fatalf("fresh upload payload=%#v err=%v", payload, err)
	}
	createArgs := upload.arguments["minutes/create_upload_session"][0]
	if option, _ := createArgs["minutesOption"].(map[string]any); option["enableMessageCard"] != true {
		t.Fatalf("legacy upload-and-analyze message flag args=%#v", createArgs)
	}
}

func TestCrossPlatformCoverageMinutesMindmapBranchesE2E(t *testing.T) {
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+mindmap", "--id", "u1", "--dry-run"); err == nil {
		t.Fatal("mindmap dry-run output failure accepted")
	}
	cases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		args      []string
	}{
		{name: "create call", failAt: map[string]int{"minutes/create_mind_graph": 1}},
		{name: "poll call", responses: map[string][]string{"minutes/create_mind_graph": {`{}`}}, failAt: map[string]int{"minutes/query_mind_graph_status": 1}},
		{name: "poll parse", responses: map[string][]string{"minutes/create_mind_graph": {`{}`}, "minutes/query_mind_graph_status": {`{"success":true,"result":{}}`}}},
		{name: "timeout", responses: map[string][]string{"minutes/create_mind_graph": {`{}`}, "minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":0}}`}}},
		{name: "resume", responses: map[string][]string{"minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":1}}`}}, args: []string{"--resume"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"minutes", "+mindmap", "--id", "u1", "--timeout", "1", "--interval", "1", "--yes"}
			args = append(args, test.args...)
			payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, args...)
			if test.name == "resume" {
				if err != nil || payload["complete"] != true {
					t.Fatalf("resume payload=%#v err=%v", payload, err)
				}
			} else if err == nil || output == "" {
				t.Fatalf("failure accepted payload=%#v err=%v", payload, err)
			}
		})
	}
	outputFail := &minutesE2ECaller{responses: map[string][]string{
		"minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":1}}`},
	}}
	if err := runMinutesAlignmentCLIWithWriter(t, outputFail, minutesFailWriter{}, "minutes", "+mindmap", "--id", "u1", "--resume", "--yes"); err == nil {
		t.Fatal("mindmap output failure accepted")
	}

	caller := &minutesE2ECaller{responses: map[string][]string{"minutes/query_mind_graph_status": {`{"success":true,"result":{"taskStatus":0}}`}}}
	helpers.InitDepsForTest(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{Use: "mindmap"}
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, Mindmap)
	if _, err := runMinutesMindmap(rt, "u1", 2*time.Hour, time.Hour, false); err == nil {
		t.Fatal("mindmap cancelled wait accepted")
	}
}

func TestCrossPlatformCoverageMinutesSpeakerInsightsBranchesE2E(t *testing.T) {
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+speaker-insights", "--id", "u1", "--dry-run"); err == nil {
		t.Fatal("speaker dry-run output failure accepted")
	}
	cases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		args      []string
	}{
		{name: "create call", failAt: map[string]int{"minutes/create_speaker_summary": 1}},
		{name: "create parse", responses: map[string][]string{"minutes/create_speaker_summary": {`{"success":true,"result":{}}`}}},
		{name: "poll nonpending", responses: map[string][]string{"minutes/create_speaker_summary": {`{"success":true,"result":{"taskId":"job","status":"processing"}}`}, "minutes/get_speaker_summary": {`{"success":false,"errorMsg":"denied"}`}}},
		{name: "poll parse", responses: map[string][]string{"minutes/create_speaker_summary": {`{"success":true,"result":{"taskId":"job","status":"processing"}}`}, "minutes/get_speaker_summary": {`{"success":true,"result":{}}`}}},
		{name: "timeout", responses: map[string][]string{"minutes/create_speaker_summary": {`{"success":true,"result":{"taskId":"job","status":"processing"}}`}, "minutes/get_speaker_summary": {`{"success":false,"errorMsg":"processing"}`}}},
		{name: "resume", responses: map[string][]string{"minutes/get_speaker_summary": {`{"success":true,"result":{"summaries":[{"speaker":"a","summary":"b"}]}}`}}, args: []string{"--resume", "--task-id", "job"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"minutes", "+speaker-insights", "--id", "u1", "--timeout", "1", "--interval", "1", "--yes"}
			args = append(args, test.args...)
			payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, args...)
			if test.name == "resume" {
				if err != nil || payload["complete"] != true || payload["taskId"] != "job" {
					t.Fatalf("resume payload=%#v err=%v", payload, err)
				}
			} else if err == nil || output == "" {
				t.Fatalf("failure accepted payload=%#v err=%v", payload, err)
			}
		})
	}
	outputFail := &minutesE2ECaller{responses: map[string][]string{"minutes/get_speaker_summary": {`{"success":true,"result":{"summaries":[{"speaker":"a","summary":"b"}]}}`}}}
	if err := runMinutesAlignmentCLIWithWriter(t, outputFail, minutesFailWriter{}, "minutes", "+speaker-insights", "--id", "u1", "--resume", "--yes"); err == nil {
		t.Fatal("speaker output failure accepted")
	}

	for _, message := range []string{"query empty", "processing", "not ready", "result is empty", "business error: code 000", "暂无"} {
		if !speakerSummaryPending(errors.New(message)) {
			t.Fatalf("pending message rejected: %q", message)
		}
	}
	if speakerSummaryPending(nil) || speakerSummaryPending(errors.New("denied")) {
		t.Fatal("non-pending speaker error accepted")
	}

	caller := &minutesE2ECaller{responses: map[string][]string{"minutes/get_speaker_summary": {`{"success":false,"errorMsg":"processing"}`}}}
	helpers.InitDepsForTest(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{Use: "speaker"}
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, SpeakerInsights)
	if _, err := runMinutesSpeakerInsights(rt, "u1", 2*time.Hour, time.Hour, false, "job"); err == nil {
		t.Fatal("speaker cancelled wait accepted")
	}
}

func TestCrossPlatformCoverageMinutesPrepareASRBranchesE2E(t *testing.T) {
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+prepare-asr", "--words", "a", "--dry-run"); err == nil {
		t.Fatal("ASR dry-run output failure accepted")
	}
	readFailure := &minutesE2ECaller{failAt: map[string]int{"minutes/list_my_hotwords": 1}}
	if _, _, err := runMinutesAlignmentCLI(t, readFailure, "minutes", "+prepare-asr", "--words", "a", "--yes"); err == nil {
		t.Fatal("ASR current read failure accepted")
	}
	parseFailure := &minutesE2ECaller{responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{}}`}}}
	if _, _, err := runMinutesAlignmentCLI(t, parseFailure, "minutes", "+prepare-asr", "--words", "a", "--yes"); err == nil {
		t.Fatal("ASR current parse failure accepted")
	}

	for _, test := range []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
	}{
		{name: "add call", responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":[]}}`}}, failAt: map[string]int{"minutes/add_personal_hot_word": 1}},
		{name: "add ack", responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":[]}}`}, "minutes/add_personal_hot_word": {`{"result":{}}`}}},
		{name: "delete call", responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":["old"]}}`}}, failAt: map[string]int{"minutes/delete_personal_hotword": 1}},
		{name: "delete ack", responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":["old"]}}`}, "minutes/delete_personal_hotword": {`{"result":{}}`}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"minutes", "+prepare-asr", "--words", "a", "--yes"}
			if strings.HasPrefix(test.name, "delete") {
				args = []string{"minutes", "+sync-asr", "--words", "a", "--yes"}
				test.responses["minutes/add_personal_hot_word"] = []string{`{"success":true,"result":{}}`}
			}
			payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: test.responses, failAt: test.failAt}, args...)
			if err == nil || output == "" || payload["complete"] != false {
				t.Fatalf("ASR failure accepted payload=%#v err=%v", payload, err)
			}
		})
	}

	verifyCall := &minutesE2ECaller{
		responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":["a"]}}`}},
		failAt:    map[string]int{"minutes/list_my_hotwords": 2},
	}
	payload, output, err := runMinutesAlignmentCLI(t, verifyCall, "minutes", "+prepare-asr", "--words", "a", "--yes")
	if err == nil || output == "" || payload["complete"] != false {
		t.Fatalf("ASR verify call payload=%#v err=%v", payload, err)
	}
	verifyCallOutput := &minutesE2ECaller{
		responses: map[string][]string{"minutes/list_my_hotwords": {`{"success":true,"result":{"hotWordList":["a"]}}`}},
		failAt:    map[string]int{"minutes/list_my_hotwords": 2},
	}
	if err := runMinutesAlignmentCLIWithWriter(t, verifyCallOutput, minutesFailWriter{}, "minutes", "+prepare-asr", "--words", "a", "--yes"); err == nil {
		t.Fatal("ASR verify read output failure accepted")
	}
	verifyParse := &minutesE2ECaller{responses: map[string][]string{"minutes/list_my_hotwords": {
		`{"success":true,"result":{"hotWordList":["a"]}}`, `{"success":true,"result":{}}`,
	}}}
	if _, _, err := runMinutesAlignmentCLI(t, verifyParse, "minutes", "+prepare-asr", "--words", "a", "--yes"); err == nil {
		t.Fatal("ASR verify parse failure accepted")
	}
	verifyMismatch := &minutesE2ECaller{responses: map[string][]string{"minutes/list_my_hotwords": {
		`{"success":true,"result":{"hotWordList":["a","old"]}}`, `{"success":true,"result":{"hotWordList":["old"]}}`,
	}, "minutes/delete_personal_hotword": {`{"success":true,"result":{}}`}}}
	payload, output, err = runMinutesAlignmentCLI(t, verifyMismatch, "minutes", "+sync-asr", "--words", "a", "--yes")
	if err == nil || output == "" || payload["verified"] != false {
		t.Fatalf("ASR mismatch payload=%#v err=%v", payload, err)
	}
	syncSuccess := &minutesE2ECaller{responses: map[string][]string{
		"minutes/list_my_hotwords":        {`{"success":true,"result":{"hotWordList":["old"]}}`, `{"success":true,"result":{"hotWordList":["a"]}}`},
		"minutes/add_personal_hot_word":   {`{"success":true,"result":{}}`},
		"minutes/delete_personal_hotword": {`{"success":true,"result":{}}`},
	}}
	payload, _, err = runMinutesAlignmentCLI(t, syncSuccess, "minutes", "+sync-asr", "--words", "a", "--yes")
	if err != nil || payload["complete"] != true {
		t.Fatalf("ASR sync payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesPermissionLedgerBranchesE2E(t *testing.T) {
	many := make([]string, 51)
	for index := range many {
		many[index] = fmt.Sprintf("m%d", index)
	}
	if payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{}, "minutes", "+share", "--id", "u1", "--member-uids", strings.Join(many, ","), "--permission", "view", "--yes"); err == nil || payload != nil || output != "" {
		t.Fatalf("too many members accepted payload=%#v err=%v", payload, err)
	}
	if err := runMinutesAlignmentCLIWithWriter(t, &minutesE2ECaller{}, minutesFailWriter{}, "minutes", "+unshare", "--id", "u1", "--member-uids", "m1", "--dry-run"); err == nil {
		t.Fatal("unshare dry-run output failure accepted")
	}
	unshare := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info":   {`{"success":true,"result":{"taskUuid":"u1"}}`},
		"minutes/remove_member_permission": {`{"success":true,"result":{"resultMap":{"u1":["m1"]}}}`},
	}}
	payload, _, err := runMinutesAlignmentCLI(t, unshare, "minutes", "+unshare", "--id", "u1", "--member-uids", "m1", "--yes")
	if err != nil || payload["complete"] != true || payload["succeeded"] != float64(1) {
		t.Fatalf("unshare payload=%#v err=%v", payload, err)
	}
	share := &minutesE2ECaller{responses: map[string][]string{"minutes/add_member_permission": {`{"success":true,"result":{}}`}}}
	payload, _, err = runMinutesAlignmentCLI(t, share, "minutes", "+share", "--id", "u1", "--member-uids", "m1", "--permission", "edit", "--cover", "--sub-resources", "Summary,Note", "--yes")
	if err != nil || payload["complete"] != true {
		t.Fatalf("share payload=%#v err=%v", payload, err)
	}
	args := share.arguments["minutes/add_member_permission"][0]
	if args["coverPermission"] != "true" || len(args["roleSubResourceIds"].([]string)) != 2 {
		t.Fatalf("share args=%#v", args)
	}
	continueFailure := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1"}}`},
		"minutes/remove_member_permission": {
			`{"result":{}}`,
			`{"success":true,"result":{"resultMap":{"u1":["m2"]}}}`,
		},
	}}
	payload, output, err := runMinutesAlignmentCLI(t, continueFailure, "minutes", "+unshare", "--id", "u1", "--member-uids", "m1,m2", "--failure-policy", "continue", "--yes")
	if err == nil || output == "" || payload["failed"] != float64(1) || payload["succeeded"] != float64(1) {
		t.Fatalf("continue ledger payload=%#v err=%v", payload, err)
	}

	missing := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info": {`{"success":true,"result":{}}`},
	}}
	payload, output, err = runMinutesAlignmentCLI(t, missing, "minutes", "+unshare", "--id", "missing", "--member-uids", "m1", "--yes")
	if err == nil || payload != nil || output != "" || missing.counts["minutes/remove_member_permission"] != 0 {
		t.Fatalf("missing minutes reached unshare: payload=%#v output=%q err=%v calls=%#v", payload, output, err, missing.counts)
	}

	preflightFailure := &minutesE2ECaller{failAt: map[string]int{"minutes/get_minutes_basic_info": 1}}
	payload, output, err = runMinutesAlignmentCLI(t, preflightFailure, "minutes", "+unshare", "--id", "unavailable", "--member-uids", "m1", "--yes")
	if err == nil || payload != nil || output != "" || preflightFailure.counts["minutes/remove_member_permission"] != 0 {
		t.Fatalf("failed preflight reached unshare: payload=%#v output=%q err=%v calls=%#v", payload, output, err, preflightFailure.counts)
	}
}

func TestCrossPlatformCoverageMinutesArtifactCollectorBranches(t *testing.T) {
	success := &minutesE2ECaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info":    {`{"success":true,"result":{"taskUuid":"u1","title":"ok"}}`},
		"minutes/get_minutes_ai_summary":    {`{"success":true,"result":{"fullSummary":"summary"}}`},
		"minutes/get_minutes_keywords":      {`{"success":true,"result":{"keywords":[]}}`},
		"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`},
		"minutes/list_minutes_todos":        {`{"success":true,"result":{"actions":[]}}`},
	}}
	helpers.InitDepsForTest(t, success)
	rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "collect"}, ExportPack)
	bundle, failures := collectMinutesArtifactsOnce(rt, "u1", append(minutesWorkflowArtifacts, "unknown"), 10)
	if len(bundle) != 5 || len(failures) != 1 {
		t.Fatalf("collector bundle=%#v failures=%#v", bundle, failures)
	}

	for _, artifact := range []string{"basic", "summary", "keywords", "transcript", "todos"} {
		t.Run("call "+artifact, func(t *testing.T) {
			tool := map[string]string{
				"basic": "get_minutes_basic_info", "summary": "get_minutes_ai_summary", "keywords": "get_minutes_keywords",
				"transcript": "get_minutes_transcription", "todos": "list_minutes_todos",
			}[artifact]
			caller := &minutesE2ECaller{failAt: map[string]int{"minutes/" + tool: 1}}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "collect"}, ExportPack)
			got, failed := collectMinutesArtifactsOnce(rt, "u1", []string{artifact}, 1)
			if len(got) != 0 || len(failed) != 1 {
				t.Fatalf("got=%#v failed=%#v", got, failed)
			}
		})
		t.Run("parse "+artifact, func(t *testing.T) {
			tool := map[string]string{
				"basic": "get_minutes_basic_info", "summary": "get_minutes_ai_summary", "keywords": "get_minutes_keywords",
				"transcript": "get_minutes_transcription", "todos": "list_minutes_todos",
			}[artifact]
			caller := &minutesE2ECaller{responses: map[string][]string{"minutes/" + tool: {`{"success":true,"result":{}}`}}}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "collect"}, ExportPack)
			got, failed := collectMinutesArtifactsOnce(rt, "u1", []string{artifact}, 1)
			if len(got) != 0 || len(failed) != 1 {
				t.Fatalf("got=%#v failed=%#v", got, failed)
			}
		})
	}
	emptySummary := &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":""}}`}}}
	helpers.InitDepsForTest(t, emptySummary)
	rt = shortcut.RuntimeContextForTest(&cobra.Command{Use: "collect"}, ExportPack)
	if _, failures = collectMinutesArtifactsOnce(rt, "u1", []string{"summary"}, 1); len(failures) != 1 {
		t.Fatalf("empty summary failures=%#v", failures)
	}
}

func TestCrossPlatformCoverageMinutesArtifactWaitAndOutput(t *testing.T) {
	success := &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1"}}`}}}
	helpers.InitDepsForTest(t, success)
	rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "wait"}, ExportPack)
	if _, failures, attempts := waitMinutesArtifacts(rt, "u1", []string{"basic"}, 1, time.Second, 0); len(failures) != 0 || attempts != 1 {
		t.Fatalf("success failures=%#v attempts=%d", failures, attempts)
	}
	failing := &minutesE2ECaller{failAt: map[string]int{"minutes/get_minutes_basic_info": 1}}
	helpers.InitDepsForTest(t, failing)
	rt = shortcut.RuntimeContextForTest(&cobra.Command{Use: "wait"}, ExportPack)
	if _, failures, attempts := waitMinutesArtifacts(rt, "u1", []string{"basic"}, 1, 0, 0); len(failures) != 1 || attempts != 1 {
		t.Fatalf("timeout failures=%#v attempts=%d", failures, attempts)
	}
	cancelled := &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{}}`}}}
	helpers.InitDepsForTest(t, cancelled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{Use: "wait"}
	cmd.SetContext(ctx)
	rt = shortcut.RuntimeContextForTest(cmd, ExportPack)
	if _, failures, _ := waitMinutesArtifacts(rt, "u1", []string{"basic"}, 1, 2*time.Hour, time.Hour); len(failures) != 2 || failures[1]["artifact"] != "wait" {
		t.Fatalf("cancel failures=%#v", failures)
	}
	cmd = &cobra.Command{Use: "wait"}
	rt = shortcut.RuntimeContextForTest(cmd, ExportPack)
	if err := waitMinutesInterval(rt, 0); err != nil {
		t.Fatalf("timer wait with unset command context: %v", err)
	}

	cmd.SetOut(minutesFailWriter{})
	if err := outputWorkflowResult(rt, map[string]any{"complete": true}, false, "", ""); err == nil {
		t.Fatal("workflow output failure accepted")
	}
}

func TestCrossPlatformCoverageMinutesExportPackBranchesE2E(t *testing.T) {
	withCWD := func(t *testing.T) string {
		t.Helper()
		old, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		work := t.TempDir()
		if err := os.Chdir(work); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(old) })
		return work
	}
	basicCaller := func() *minutesE2ECaller {
		return &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1","title":"ok"}}`}}}
	}

	t.Run("collect failure", func(t *testing.T) {
		withCWD(t)
		payload, output, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{}}`}}}, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic")
		if err == nil || output == "" || payload["published"] != false {
			t.Fatalf("collect payload=%#v err=%v", payload, err)
		}
	})
	t.Run("target exists", func(t *testing.T) {
		work := withCWD(t)
		if err := os.Mkdir(filepath.Join(work, "pack"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("existing export accepted")
		}
	})
	t.Run("mkdir temp", func(t *testing.T) {
		withCWD(t)
		testseam.Swap(t, &minutesMkdirTemp, func(string, string) (string, error) { return "", errors.New("mkdir temp") })
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("mkdir temp failure accepted")
		}
	})
	t.Run("summary write", func(t *testing.T) {
		withCWD(t)
		testseam.Swap(t, &minutesWriteFile, func(string, []byte, os.FileMode) error { return errors.New("write") })
		caller := &minutesE2ECaller{responses: map[string][]string{"minutes/get_minutes_ai_summary": {`{"success":true,"result":{"fullSummary":"summary"}}`}}}
		if _, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "summary"); err == nil {
			t.Fatal("summary write failure accepted")
		}
	})
	t.Run("json write", func(t *testing.T) {
		withCWD(t)
		testseam.Swap(t, &minutesWriteFile, func(string, []byte, os.FileMode) error { return errors.New("write") })
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("JSON write failure accepted")
		}
	})
	t.Run("stat", func(t *testing.T) {
		withCWD(t)
		testseam.Swap(t, &minutesStat, func(string) (os.FileInfo, error) { return nil, errors.New("stat") })
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("stat failure accepted")
		}
	})
	for _, test := range []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		download  func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error)
		wantError bool
	}{
		{name: "media call", failAt: map[string]int{"minutes/query_minutes_audio_url": 1}, wantError: true},
		{name: "media parse", responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "media download", responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{"downloadUrl":"https://example.invalid/a.mp3"}}`}}, download: func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, errors.New("download")
		}, wantError: true},
		{name: "media success", responses: map[string][]string{"minutes/query_minutes_audio_url": {`{"success":true,"result":{"downloadUrl":"https://example.invalid/a.mp3"}}`}}, download: func(_ context.Context, _ string, options localio.DownloadOptions) (localio.DownloadResult, error) {
			path := filepath.Join(options.BaseDir, "media", "u1.mp3")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return localio.DownloadResult{}, err
			}
			if err := os.WriteFile(path, []byte("media"), 0o600); err != nil {
				return localio.DownloadResult{}, err
			}
			return localio.DownloadResult{RelativePath: "media/u1.mp3", SizeBytes: 5}, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withCWD(t)
			if test.download != nil {
				testseam.Swap(t, &minutesDownload, test.download)
			}
			responses := map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1"}}`}}
			for key, values := range test.responses {
				responses[key] = values
			}
			payload, _, err := runMinutesAlignmentCLI(t, &minutesE2ECaller{responses: responses, failAt: test.failAt}, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic", "--include-media")
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			if !test.wantError && payload["published"] != true {
				t.Fatalf("payload=%#v", payload)
			}
		})
	}
	t.Run("manifest write", func(t *testing.T) {
		withCWD(t)
		realWrite := minutesWriteFile
		testseam.Swap(t, &minutesWriteFile, func(path string, data []byte, mode os.FileMode) error {
			if filepath.Base(path) == "manifest.json" {
				return errors.New("manifest")
			}
			return realWrite(path, data, mode)
		})
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("manifest failure accepted")
		}
	})
	t.Run("rename", func(t *testing.T) {
		withCWD(t)
		testseam.Swap(t, &minutesRename, func(string, string) error { return errors.New("rename") })
		if _, _, err := runMinutesAlignmentCLI(t, basicCaller(), "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("rename failure accepted")
		}
	})
	t.Run("output", func(t *testing.T) {
		withCWD(t)
		if err := runMinutesAlignmentCLIWithWriter(t, basicCaller(), minutesFailWriter{}, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic"); err == nil {
			t.Fatal("export output failure accepted")
		}
	})
}

func TestCrossPlatformCoverageMinutesExportPathAndJSONFaults(t *testing.T) {
	if _, _, _, err := prepareExportTarget(filepath.Join("..", "escape")); err == nil {
		t.Fatal("unsafe target accepted")
	}
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "getwd", run: func(t *testing.T) {
			testseam.Swap(t, &minutesGetwd, func() (string, error) { return "", errors.New("getwd") })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("getwd failure accepted")
			}
		}},
		{name: "eval", run: func(t *testing.T) {
			testseam.Swap(t, &minutesEvalSymlinks, func(string) (string, error) { return "", errors.New("eval") })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("eval failure accepted")
			}
		}},
		{name: "rel", run: func(t *testing.T) {
			testseam.Swap(t, &minutesRel, func(string, string) (string, error) { return "", errors.New("rel") })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("rel failure accepted")
			}
		}},
		{name: "escape", run: func(t *testing.T) {
			testseam.Swap(t, &minutesRel, func(string, string) (string, error) { return filepath.Join("..", "escape"), nil })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("rel escape accepted")
			}
		}},
		{name: "target lstat", run: func(t *testing.T) {
			testseam.Swap(t, &minutesLstat, func(string) (os.FileInfo, error) { return nil, errors.New("lstat") })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("lstat failure accepted")
			}
		}},
		{name: "parent", run: func(t *testing.T) {
			testseam.Swap(t, &minutesRel, func(string, string) (string, error) { return filepath.Join("parent", "pack"), nil })
			testseam.Swap(t, &minutesLstat, func(string) (os.FileInfo, error) { return nil, errors.New("parent") })
			if _, _, _, err := prepareExportTarget("pack"); err == nil {
				t.Fatal("unsafe parent accepted")
			}
		}},
	} {
		t.Run(test.name, test.run)
	}

	base := t.TempDir()
	if err := ensureExportParent(base, "."); err != nil {
		t.Fatal(err)
	}
	t.Run("mkdir", func(t *testing.T) {
		testseam.Swap(t, &minutesLstat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
		testseam.Swap(t, &minutesMkdir, func(string, os.FileMode) error { return errors.New("mkdir") })
		if err := ensureExportParent(base, "a"); err == nil {
			t.Fatal("mkdir failure accepted")
		}
	})
	t.Run("lstat", func(t *testing.T) {
		testseam.Swap(t, &minutesLstat, func(string) (os.FileInfo, error) { return nil, errors.New("lstat") })
		if err := ensureExportParent(base, "a"); err == nil {
			t.Fatal("lstat failure accepted")
		}
	})
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(base, "file")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ensureExportParent(base, "file/child"); err == nil {
			t.Fatal("file parent accepted")
		}
	})
	t.Run("create", func(t *testing.T) {
		if err := ensureExportParent(base, "created/nested"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("marshal", func(t *testing.T) {
		testseam.Swap(t, &minutesMarshalIndent, func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") })
		if err := writeJSONFile(filepath.Join(t.TempDir(), "x.json"), map[string]any{}); err == nil {
			t.Fatal("marshal failure accepted")
		}
	})
	t.Run("write", func(t *testing.T) {
		testseam.Swap(t, &minutesWriteFile, func(string, []byte, os.FileMode) error { return errors.New("write") })
		if err := writeJSONFile(filepath.Join(t.TempDir(), "x.json"), map[string]any{}); err == nil {
			t.Fatal("write failure accepted")
		}
	})
	path := filepath.Join(t.TempDir(), "ok.json")
	if err := writeJSONFile(path, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.HasSuffix(raw, []byte("\n")) || !json.Valid(raw) {
		t.Fatalf("JSON output=%q err=%v", raw, err)
	}
}
