// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

type smartMinutesFailWriter struct{}

func (smartMinutesFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture output failure")
}

func runMinutesCLIWithWriter(t *testing.T, caller *smartCoverageCaller, writer io.Writer, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(writer)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	executed, err := root.ExecuteC()
	if err != nil || !output.UsesUnifiedResult(executed) {
		return err
	}
	code, _, emitErr := output.EmitStoredResult(executed)
	if emitErr != nil {
		return emitErr
	}
	if code != 0 {
		return fmt.Errorf("command result exit code %d", code)
	}
	return nil
}

func TestCrossPlatformCoverageMinutesSmartContractFinalizer(t *testing.T) {
	value := shortcut.Shortcut{
		Description: "desc", Intent: "intent",
		Contract: corecmd.ContractDecl{Parameters: []contract.ParamDecl{
			{Name: "a", Description: "parameter"},
			{Name: "a", Description: "evidence present"},
			{Name: "other", Description: "other"},
		}},
		Flags: []shortcut.Flag{{Name: "a", Desc: "flag"}},
		Constraints: []shortcut.Constraint{
			{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"a"}},
			{Kind: shortcut.ConstraintCustom, Flags: []string{"a"}},
			{Kind: shortcut.ConstraintCustom, Flags: []string{"a"}, Description: "evidence"},
		},
	}
	got := finalizeMinutesSmartShortcut(value)
	if got.Contract.Selection.AgentSummary != "desc" || got.Contract.Selection.UseWhen[0] != "intent" || !strings.Contains(got.Flags[0].Desc, "evidence") || !strings.Contains(got.Contract.Parameters[0].Description, "evidence") || got.Contract.Parameters[1].Description != "evidence present" || got.Contract.Parameters[2].Description != "other" {
		t.Fatalf("finalized=%#v", got)
	}
}

func TestCrossPlatformCoverageMinutesLatestAndActionItemsBranchesE2E(t *testing.T) {
	listKey := "minutes/list_by_keyword_and_time_range"
	basicKey := "minutes/get_minutes_basic_info"
	todoKey := "minutes/list_minutes_todos"
	latestCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		wantError bool
	}{
		{name: "list call", failAt: map[string]int{listKey: 1}, wantError: true},
		{name: "list parse", responses: map[string][]string{listKey: {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "empty", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[]}}`}}, wantError: true},
		{name: "basic call", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}}, failAt: map[string]int{basicKey: 1}, wantError: true},
		{name: "basic parse", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, basicKey: {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "success", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, basicKey: {`{"success":true,"result":{"taskUuid":"u1","title":"ok"}}`}}},
	}
	for _, test := range latestCases {
		t.Run("latest "+test.name, func(t *testing.T) {
			payload, _, err := runMinutesCLI(t, &smartCoverageCaller{responses: test.responses, failAt: test.failAt}, "minutes", "+latest", "--keyword", "weekly")
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}
	latestOutput := &smartCoverageCaller{responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, basicKey: {`{"success":true,"result":{"taskUuid":"u1"}}`}}}
	if err := runMinutesCLIWithWriter(t, latestOutput, smartMinutesFailWriter{}, "minutes", "+latest"); err == nil {
		t.Fatal("latest output failure accepted")
	}
	if _, err := latestMinutesTaskUUID(map[string]any{}); err == nil {
		t.Fatal("latest malformed data accepted")
	}

	actionCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		args      []string
		wantError bool
	}{
		{name: "list call", failAt: map[string]int{listKey: 1}, wantError: true},
		{name: "list parse", responses: map[string][]string{listKey: {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "empty", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[]}}`}}, wantError: true},
		{name: "todo call", failAt: map[string]int{todoKey: 1}, args: []string{"--id", "u1"}, wantError: true},
		{name: "todo parse", responses: map[string][]string{todoKey: {`{"success":true,"result":{}}`}}, args: []string{"--id", "u1"}, wantError: true},
		{name: "success id", responses: map[string][]string{todoKey: {`{"success":true,"result":{"actions":[]}}`}}, args: []string{"--id", "u1"}},
		{name: "success latest", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, todoKey: {`{"success":true,"result":{"actions":[]}}`}}},
	}
	for _, test := range actionCases {
		t.Run("actions "+test.name, func(t *testing.T) {
			args := []string{"minutes", "+action-items"}
			args = append(args, test.args...)
			payload, _, err := runMinutesCLI(t, &smartCoverageCaller{responses: test.responses, failAt: test.failAt}, args...)
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}
	actionOutput := &smartCoverageCaller{responses: map[string][]string{todoKey: {`{"success":true,"result":{"actions":[]}}`}}}
	if err := runMinutesCLIWithWriter(t, actionOutput, smartMinutesFailWriter{}, "minutes", "+action-items", "--id", "u1"); err == nil {
		t.Fatal("actions output failure accepted")
	}
}

func TestCrossPlatformCoverageMinutesSearchAndTranscriptBranchesE2E(t *testing.T) {
	listKey := "minutes/list_by_keyword_and_time_range"
	transcriptKey := "minutes/get_minutes_transcription"
	searchCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
	}{
		{name: "call", failAt: map[string]int{listKey: 1}},
		{name: "project", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"title":"missing id"}]}}`}}},
	}
	for _, test := range searchCases {
		if _, _, err := runMinutesCLI(t, &smartCoverageCaller{responses: test.responses, failAt: test.failAt}, "minutes", "+minutes-search", "--query", "x"); err == nil {
			t.Fatalf("search %s failure accepted", test.name)
		}
	}
	searchOutput := &smartCoverageCaller{responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[]}}`}}}
	if err := runMinutesCLIWithWriter(t, searchOutput, smartMinutesFailWriter{}, "minutes", "+minutes-search", "--query", "x"); err == nil {
		t.Fatal("search output failure accepted")
	}

	if _, _, err := runMinutesCLI(t, &smartCoverageCaller{}, "minutes", "+transcript", "--id", "u1", "--page-limit", "0"); err == nil {
		t.Fatal("transcript page limit accepted")
	}
	transcriptCases := []struct {
		name      string
		responses map[string][]string
		failAt    map[string]int
		wantError bool
	}{
		{name: "list call", failAt: map[string]int{listKey: 1}, wantError: true},
		{name: "list parse", responses: map[string][]string{listKey: {`{"success":true,"result":{}}`}}, wantError: true},
		{name: "empty", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[]}}`}}, wantError: true},
		{name: "success", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, transcriptKey: {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`}}},
		{name: "partial", responses: map[string][]string{listKey: {`{"success":true,"result":{"itemList":[{"uuid":"u1","startTime":1}]}}`}, transcriptKey: {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`}}, failAt: map[string]int{transcriptKey: 2}, wantError: true},
	}
	for _, test := range transcriptCases {
		t.Run("transcript "+test.name, func(t *testing.T) {
			payload, output, err := runMinutesCLI(t, &smartCoverageCaller{responses: test.responses, failAt: test.failAt}, "minutes", "+transcript", "--keyword", "weekly", "--direction", "1")
			if (err != nil) != test.wantError {
				t.Fatalf("payload=%#v output=%q err=%v", payload, output, err)
			}
		})
	}
	transcriptOutput := &smartCoverageCaller{responses: map[string][]string{transcriptKey: {`{"success":true,"result":{"paragraphList":[],"hasNext":false}}`}}}
	if err := runMinutesCLIWithWriter(t, transcriptOutput, smartMinutesFailWriter{}, "minutes", "+transcript", "--id", "u1"); err == nil {
		t.Fatal("transcript output failure accepted")
	}
}

func TestCrossPlatformCoverageMinutesDetailRemainingBranchesE2E(t *testing.T) {
	invalid := [][]string{
		{"minutes", "+detail", "--id", "u1", "--page-limit", "0"},
		{"minutes", "+detail", "--id", "u1", "--ids", "u2"},
		{"minutes", "+detail"},
		{"minutes", "+detail", "--id", "u1", "--transcript-output", "file", "--output-dir", "../escape"},
	}
	for _, args := range invalid {
		if payload, output, err := runMinutesCLI(t, &smartCoverageCaller{}, args...); err == nil || payload != nil || output != "" {
			t.Fatalf("detail invalid accepted %v payload=%#v output=%q err=%v", args, payload, output, err)
		}
	}
	all := &smartCoverageCaller{responses: map[string][]string{
		"minutes/get_minutes_basic_info":    {`{"success":true,"result":{"taskUuid":"u1"}}`},
		"minutes/get_minutes_ai_summary":    {`{"success":true,"result":{"fullSummary":"summary"}}`},
		"minutes/get_minutes_keywords":      {`{"success":true,"result":{"keywords":[]}}`},
		"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`},
		"minutes/list_minutes_todos":        {`{"success":true,"result":{"actions":[]}}`},
	}}
	payload, _, err := runMinutesCLI(t, all, "minutes", "+detail", "--id", "u1")
	if err != nil || payload["complete"] != true {
		t.Fatalf("detail all payload=%#v err=%v", payload, err)
	}
	if err := runMinutesCLIWithWriter(t, all, smartMinutesFailWriter{}, "minutes", "+detail", "--id", "u1", "--artifacts", "basic"); err == nil {
		t.Fatal("detail single output failure accepted")
	}
	batchOutput := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1"}}`, `{"success":true,"result":{"taskUuid":"u2"}}`}}}
	if err := runMinutesCLIWithWriter(t, batchOutput, smartMinutesFailWriter{}, "minutes", "+detail", "--ids", "u1,u2", "--artifacts", "basic"); err == nil {
		t.Fatal("detail batch output failure accepted")
	}

	for _, artifact := range []string{"basic", "summary", "keywords", "todos"} {
		tool := minutesArtifactTools[artifact]
		caller := &smartCoverageCaller{failAt: map[string]int{"minutes/" + tool: 1}}
		payload, output, err := runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", artifact)
		if err == nil || output == "" || payload["complete"] != false {
			t.Fatalf("detail call %s payload=%#v err=%v", artifact, payload, err)
		}
		caller = &smartCoverageCaller{responses: map[string][]string{"minutes/" + tool: {`{"success":true,"result":{}}`}}}
		payload, output, err = runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", artifact)
		if err == nil || output == "" || payload["complete"] != false {
			t.Fatalf("detail parse %s payload=%#v err=%v", artifact, payload, err)
		}
	}

	transcript := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`}}}
	testseam.Swap(t, &smartMinutesMarshalIndent, func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") })
	payload, output, err := runMinutesCLI(t, transcript, "minutes", "+detail", "--id", "u1", "--artifacts", "transcript", "--transcript-output", "file", "--output-dir", "out")
	if err == nil || output == "" || payload["complete"] != false {
		t.Fatalf("detail marshal payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesDetailPublishFailure(t *testing.T) {
	testseam.Swap(t, &smartMinutesPublishBytes, func([]byte, localio.PublishBytesOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{}, errors.New("publish")
	})
	caller := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`}}}
	payload, output, err := runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", "transcript", "--transcript-output", "both", "--output-dir", "out")
	if err == nil || output == "" || payload["complete"] != false {
		t.Fatalf("detail publish payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageMinutesDetailFileSuccess(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	caller := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`}}}
	payload, _, err := runMinutesCLI(t, caller, "minutes", "+detail", "--id", "u1", "--artifacts", "transcript", "--transcript-output", "file", "--output-dir", "out")
	if err != nil || payload["complete"] != true {
		t.Fatalf("detail file payload=%#v err=%v", payload, err)
	}
	if _, statErr := os.Stat(filepath.Join(work, "out", "u1.json")); statErr != nil {
		t.Fatal(statErr)
	}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "detail"}
	cmd.Flags().String("cursor", "", "")
	cmd.Flags().Bool("single-page", false, "")
	cmd.Flags().Int("page-limit", 100, "")
	cmd.Flags().String("transcript-output", "inline", "")
	rt := shortcut.RuntimeContextForTest(cmd, MinutesDetail)
	if bundle, failures := readMinutesDetail(rt, "u1", []string{"unknown"}, "0"); len(failures) != 0 || bundle["complete"] != true {
		t.Fatalf("unknown detail bundle=%#v failures=%#v", bundle, failures)
	}
}

func TestCrossPlatformCoverageMinutesReplaceParsing(t *testing.T) {
	for _, raw := range [][]string{{"missing"}, {" =>x"}, {"a=>x", "a=>y"}} {
		if pairs, err := parseReplacePairs(raw); err == nil || pairs != nil {
			t.Fatalf("invalid pairs accepted: %#v pairs=%#v err=%v", raw, pairs, err)
		}
	}
	if pairs, err := parseReplacePairs([]string{" a => b "}); err != nil || len(pairs) != 1 || pairs[0].orig != "a" || pairs[0].repl != "b" {
		t.Fatalf("pairs=%#v err=%v", pairs, err)
	}

	for _, raw := range []string{"not-json", `[]`, `[{"replacement":"x"}]`} {
		if pairs, err := parseReplaceJSON(raw); err == nil || pairs != nil {
			t.Fatalf("invalid JSON accepted: %q pairs=%#v err=%v", raw, pairs, err)
		}
	}
	if pairs, err := parseReplaceJSON(`{"replacements":[{"originalText":" a ","replacedText":" b "}]}`); err != nil || len(pairs) != 1 || pairs[0] != (replacePair{orig: "a", repl: "b"}) {
		t.Fatalf("envelope pairs=%#v err=%v", pairs, err)
	}
	if pairs, err := parseReplaceJSON(`[{"source":"a","replacement":"b"}]`); err != nil || len(pairs) != 1 {
		t.Fatalf("direct pairs=%#v err=%v", pairs, err)
	}

	for _, pairs := range [][]replacePair{nil, {{orig: " "}}, {{orig: "a"}, {orig: "a"}}} {
		if got, err := validateReplacePairs(pairs); err == nil || got != nil {
			t.Fatalf("invalid validated: %#v got=%#v err=%v", pairs, got, err)
		}
	}
	valid := []replacePair{{orig: "a", repl: "b"}}
	if got, err := validateReplacePairs(valid); err != nil || len(got) != 1 {
		t.Fatalf("valid=%#v err=%v", got, err)
	}

	for _, test := range []struct {
		before, after string
		pair          replacePair
		beforeSource  int
		beforeTarget  int
		want          bool
	}{
		{before: "same", after: "same", pair: replacePair{orig: "a", repl: "b"}},
		{before: "a", after: "", pair: replacePair{orig: "a"}, beforeSource: 1, want: true},
		{before: "a", after: "a", pair: replacePair{orig: "a"}, beforeSource: 1},
		{before: "a", after: "aa", pair: replacePair{orig: "a", repl: "aa"}, beforeSource: 1, beforeTarget: 0, want: true},
		{before: "a", after: "b", pair: replacePair{orig: "a", repl: "b"}, beforeSource: 1, beforeTarget: 0, want: true},
		{before: "a", after: "c", pair: replacePair{orig: "a", repl: "b"}, beforeSource: 1, beforeTarget: 0},
	} {
		if got := replaceReadbackVerified(test.before, test.after, test.pair, test.beforeSource, test.beforeTarget); got != test.want {
			t.Fatalf("verified(%#v)=%v", test, got)
		}
	}
}

func TestCrossPlatformCoverageMinutesReplaceLoadAndTranscript(t *testing.T) {
	cmd := &cobra.Command{Use: "replace"}
	cmd.Flags().StringSlice("pair", []string{"bad"}, "")
	cmd.Flags().String("json", "", "")
	if pairs, err := loadReplacePairs(shortcut.RuntimeContextForTest(cmd, ReplaceBatch)); err == nil || pairs != nil {
		t.Fatalf("direct malformed pair accepted pairs=%#v err=%v", pairs, err)
	}
	if _, _, err := runMinutesCLI(t, &smartCoverageCaller{}, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>b", "--page-limit", "0"); err == nil {
		t.Fatal("replace page limit accepted")
	}
	if _, _, err := runMinutesCLI(t, &smartCoverageCaller{}, "minutes", "+replace-batch", "--id", "u1", "--json", "@missing", "--dry-run"); err == nil {
		t.Fatal("missing JSON file accepted")
	}
	if _, _, err := runMinutesCLI(t, &smartCoverageCaller{}, "minutes", "+replace-batch", "--id", "u1", "--json", "bad", "--dry-run"); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if _, _, err := runMinutesCLI(t, &smartCoverageCaller{}, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>b", "--json", `[{"source":"a","replacement":"c"}]`, "--dry-run"); err == nil {
		t.Fatal("cross-input duplicate accepted")
	}

	for _, test := range []struct {
		name      string
		responses []string
		failAt    int
	}{
		{name: "call", failAt: 1},
		{name: "empty", responses: []string{`{"success":true,"result":{"paragraphList":[],"hasNext":false}}`}},
		{name: "no text", responses: []string{`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":false}}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_transcription": test.responses}, failAt: map[string]int{"minutes/get_minutes_transcription": test.failAt}}
			helpers.InitDepsForTest(t, caller)
			cmd := &cobra.Command{Use: "replace"}
			rt := shortcut.RuntimeContextForTest(cmd, ReplaceBatch)
			if text, err := minutesReplaceTranscriptText(rt, "u1", 1); err == nil || text != "" {
				t.Fatalf("text=%q err=%v", text, err)
			}
		})
	}
	nested := &smartCoverageCaller{responses: map[string][]string{"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraph":"one","sentences":[{"text":"two"}]}],"hasNext":false}}`}}}
	helpers.InitDepsForTest(t, nested)
	if text, err := minutesReplaceTranscriptText(shortcut.RuntimeContextForTest(&cobra.Command{Use: "replace"}, ReplaceBatch), "u1", 1); err != nil || !strings.Contains(text, "one") || !strings.Contains(text, "two") {
		t.Fatalf("nested text=%q err=%v", text, err)
	}
}

func TestCrossPlatformCoverageMinutesReplaceExecutionBranchesE2E(t *testing.T) {
	transcriptKey := "minutes/get_minutes_transcription"
	writeKey := "minutes/replace_minutes_text"
	preRead := &smartCoverageCaller{failAt: map[string]int{transcriptKey: 1}}
	if _, _, err := runMinutesCLI(t, preRead, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes"); err == nil {
		t.Fatal("replace pre-read failure accepted")
	}
	missing := &smartCoverageCaller{responses: map[string][]string{transcriptKey: {`{"success":true,"result":{"paragraphList":[{"paragraph":"other"}],"hasNext":false}}`}}}
	if _, _, err := runMinutesCLI(t, missing, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes"); err == nil || missing.counts[writeKey] != 0 {
		t.Fatal("missing source write accepted")
	}
	readback := &smartCoverageCaller{
		responses: map[string][]string{
			transcriptKey: {`{"success":true,"result":{"paragraphList":[{"paragraph":"a"}],"hasNext":false}}`},
			writeKey:      {`{"success":true,"result":{}}`},
		},
		failAt: map[string]int{transcriptKey: 2},
	}
	payload, output, err := runMinutesCLI(t, readback, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes")
	if err == nil || output == "" || payload["failed"] != float64(1) {
		t.Fatalf("readback payload=%#v err=%v", payload, err)
	}
	success := func() *smartCoverageCaller {
		return &smartCoverageCaller{responses: map[string][]string{
			transcriptKey: {
				`{"success":true,"result":{"paragraphList":[{"paragraph":"a"}],"hasNext":false}}`,
				`{"success":true,"result":{"paragraphList":[{"paragraph":"A"}],"hasNext":false}}`,
			},
			writeKey: {`{"success":true,"result":{}}`},
		}}
	}
	payload, output, err = runMinutesCLI(t, success(), "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes")
	if err != nil || output == "" || payload["ok"] != true || payload["applied"] != float64(1) {
		t.Fatalf("success payload=%#v err=%v", payload, err)
	}
	if err := runMinutesCLIWithWriter(t, success(), smartMinutesFailWriter{}, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--yes"); err == nil {
		t.Fatal("replace output failure accepted")
	}
	if err := runMinutesCLIWithWriter(t, &smartCoverageCaller{}, smartMinutesFailWriter{}, "minutes", "+replace-batch", "--id", "u1", "--pair", "a=>A", "--dry-run"); err == nil {
		t.Fatal("replace dry-run output failure accepted")
	}
}
