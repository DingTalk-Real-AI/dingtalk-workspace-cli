// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	_ "embed"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

//go:embed testdata/shortcut_hallucination_badcases_20260728.json
var shortcutHallucinationBadcases20260728JSON []byte

type shortcutHallucinationReplayCorpus struct {
	SourceReport    string                               `json:"source_report"`
	SourceFile      string                               `json:"source_file"`
	SourceDWSCommit string                               `json:"source_dws_commit"`
	Model           string                               `json:"model"`
	ExpectedCount   int                                  `json:"expected_count"`
	Badcases        []shortcutHallucinationReplayBadcase `json:"badcases"`
}

type shortcutHallucinationReplayBadcase struct {
	ID               string `json:"id"`
	CaseID           string `json:"case_id"`
	Run              int    `json:"run"`
	CommandIndex     int    `json:"command_index"`
	Turn             int    `json:"turn"`
	SourcePath       string `json:"source_path"`
	ExpectedOutcome  string `json:"expected_outcome"`
	Raw              string `json:"raw"`
	OriginalExitCode int    `json:"original_exit_code"`
	IsHelp           bool   `json:"is_help"`
}

func TestCrossPlatformCoverageShortcutHallucinationBadcases20260728Replay(t *testing.T) {
	var corpus shortcutHallucinationReplayCorpus
	if err := json.Unmarshal(shortcutHallucinationBadcases20260728JSON, &corpus); err != nil {
		t.Fatalf("decode historical shortcut badcase corpus: %v", err)
	}
	if corpus.SourceReport != "param_hallucination_20260728_120943" ||
		corpus.SourceFile != "raw_dashscope_qwen3_7-max_20260728_120943.json" ||
		corpus.SourceDWSCommit != "a8e83e5" || corpus.Model != "dashscope/qwen3.7-max" {
		t.Fatalf("unexpected historical corpus provenance: %#v", corpus)
	}
	if corpus.ExpectedCount != 52 || len(corpus.Badcases) != corpus.ExpectedCount {
		t.Fatalf("historical shortcut badcases = %d, expected_count=%d; want 52", len(corpus.Badcases), corpus.ExpectedCount)
	}

	wantOutcomeCounts := map[string]int{
		"rewrite":   8,
		"ambiguous": 44,
	}
	wantSourceCounts := map[string]int{
		"chat +group-member-list": 1,
		"chat +group-send-text":   1,
		"chat +list-group-bots":   1,
		"chat +list-robot":        1,
		"chat +list-robots":       1,
		"chat +members":           3,
		"chat +message-list":      1,
		"chat +read-single":       6,
		"chat +rename-group":      1,
		"chat +send":              4,
		"chat +send-by-bot":       3,
		"chat +send-dm":           2,
		"chat +send-file":         15,
		"chat +send-image":        3,
		"chat +send-media":        2,
		"chat +send-message":      3,
		"chat +send-single":       1,
		"chat +send-text":         2,
		"chat +send-to":           1,
	}
	rewriteTargets := map[string]string{
		"chat +members":           "chat +group-members",
		"chat +group-member-list": "chat +group-members",
		"chat +list-group-bots":   "chat +chat-bots",
		"chat +list-robot":        "chat +chat-bots",
		"chat +list-robots":       "chat +chat-bots",
		"chat +rename-group":      "chat +chat-update",
	}

	seenIDs := make(map[string]bool, len(corpus.Badcases))
	gotOutcomeCounts := make(map[string]int)
	gotSourceCounts := make(map[string]int)
	for _, badcase := range corpus.Badcases {
		badcase := badcase
		t.Run(badcase.ID, func(t *testing.T) {
			if badcase.ID == "" || seenIDs[badcase.ID] {
				t.Fatalf("missing or duplicate historical badcase id %q", badcase.ID)
			}
			seenIDs[badcase.ID] = true
			gotOutcomeCounts[badcase.ExpectedOutcome]++
			gotSourceCounts[badcase.SourcePath]++

			args := historicalShortcutBadcaseArgv(t, badcase.Raw)
			if len(args) < 2 || strings.Join(args[:2], " ") != badcase.SourcePath {
				t.Fatalf("raw argv source = %v, fixture source_path=%q", args, badcase.SourcePath)
			}

			root := NewSchemaSourceRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(args)
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
			switch badcase.ExpectedOutcome {
			case "rewrite":
				assertHistoricalShortcutRewrite(t, badcase, rewriteTargets[badcase.SourcePath], ctx, err)
			case "ambiguous":
				assertHistoricalShortcutReason(t, badcase, ctx, err, "ambiguous_command_fallback")
				entry, ok := cli.LookupCommandPathFallback(badcase.SourcePath)
				if !ok || entry.Mode != cli.CommandPathFallbackAmbiguous || len(entry.Candidates) < 2 {
					t.Fatalf("ambiguous fallback table entry = %#v, %v", entry, ok)
				}
			case "unknown_shortcut":
				assertHistoricalShortcutReason(t, badcase, ctx, err, "unknown_shortcut")
				if entry, ok := cli.LookupCommandPathFallback(badcase.SourcePath); ok {
					t.Fatalf("contract-incomplete path unexpectedly entered fallback table: %#v", entry)
				}
			default:
				t.Fatalf("unsupported expected_outcome %q", badcase.ExpectedOutcome)
			}
		})
	}
	if !reflect.DeepEqual(gotOutcomeCounts, wantOutcomeCounts) {
		t.Errorf("historical outcome counts = %v, want %v", gotOutcomeCounts, wantOutcomeCounts)
	}
	if !reflect.DeepEqual(gotSourceCounts, wantSourceCounts) {
		t.Errorf("historical source counts = %v, want %v", gotSourceCounts, wantSourceCounts)
	}
}

func TestCrossPlatformCoverageShortcutHallucinationMerged20260720Replay(t *testing.T) {
	tests := []struct {
		id          string
		raw         string
		occurrences int
		outcome     string
		target      string
	}{
		{
			id:          "dws_im_v2_0013-search-group",
			raw:         `dws chat +search-group --name "dws测试群02" --format json`,
			occurrences: 1,
			outcome:     "official_alias",
			target:      "chat +chat-search",
		},
		{
			id:          "dws_oa_0017-list-processes",
			raw:         `dws oa +list-processes --format json --limit 50`,
			occurrences: 2,
			outcome:     "ambiguous",
		},
	}
	totalOccurrences := 0
	for _, test := range tests {
		test := test
		t.Run(test.id, func(t *testing.T) {
			totalOccurrences += test.occurrences
			args := historicalShortcutBadcaseArgv(t, test.raw)
			root := NewSchemaSourceRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(args)
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
			badcase := shortcutHallucinationReplayBadcase{
				ID:              test.id,
				SourcePath:      strings.Join(args[:2], " "),
				ExpectedOutcome: test.outcome,
				Raw:             test.raw,
			}
			switch test.outcome {
			case "rewrite":
				assertHistoricalShortcutRewrite(t, badcase, test.target, ctx, err)
			case "official_alias":
				assertHistoricalOfficialAlias(t, badcase, test.target, ctx, err)
			case "ambiguous":
				assertHistoricalShortcutReason(t, badcase, ctx, err, "ambiguous_command_fallback")
			default:
				t.Fatalf("unsupported merged expected outcome %q", test.outcome)
			}
		})
	}
	if totalOccurrences != 3 {
		t.Fatalf("merged shortcut hallucination occurrences = %d, want 3", totalOccurrences)
	}
}

func assertHistoricalOfficialAlias(
	t *testing.T,
	badcase shortcutHallucinationReplayBadcase,
	wantTarget string,
	ctx *pipeline.Context,
	err error,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("historical official alias %q returned error: %v", badcase.SourcePath, err)
	}
	if wantTarget == "" || ctx == nil || ctx.Command != "dws "+wantTarget {
		t.Fatalf("historical official alias context = %#v; want command dws %s", ctx, wantTarget)
	}
	for _, correction := range ctx.Corrections {
		if correction.Handler == "command-path-fallback" {
			t.Fatalf("historical official alias received fallback correction: %#v", ctx.Corrections)
		}
	}
	if entry, ok := cli.LookupCommandPathFallback(badcase.SourcePath); ok {
		t.Fatalf("official alias unexpectedly remains in fallback table: %#v", entry)
	}
}

func historicalShortcutBadcaseArgv(t *testing.T, raw string) []string {
	t.Helper()
	command := strings.TrimSpace(raw)
	for _, suffix := range []string{" 2>&1 | head -50", " 2>&1"} {
		command = strings.TrimSuffix(command, suffix)
	}
	argv, err := cli.ParseAgentExampleArgv(command)
	if err != nil {
		t.Fatalf("parse historical raw command %q without a shell: %v", raw, err)
	}
	if len(argv) < 3 || argv[0] != "dws" {
		t.Fatalf("historical raw command did not produce dws argv: %q => %v", raw, argv)
	}
	return append([]string(nil), argv[1:]...)
}

func assertHistoricalShortcutRewrite(
	t *testing.T,
	badcase shortcutHallucinationReplayBadcase,
	wantTarget string,
	ctx *pipeline.Context,
	err error,
) {
	t.Helper()
	if wantTarget == "" {
		t.Fatalf("historical rewrite %q has no reviewed target", badcase.SourcePath)
	}
	if ctx == nil || ctx.Command != "dws "+wantTarget {
		t.Fatalf("historical rewrite context = %#v, error=%v; want command dws %s", ctx, err, wantTarget)
	}
	found := false
	for _, correction := range ctx.Corrections {
		if correction.Handler == "command-path-fallback" && correction.Original == badcase.SourcePath && correction.Corrected == wantTarget {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("historical rewrite corrections = %#v; want %q -> %q", ctx.Corrections, badcase.SourcePath, wantTarget)
	}
	wantPrefix := strings.Fields(wantTarget)
	if len(ctx.Args) < len(wantPrefix) || !reflect.DeepEqual(ctx.Args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("historical rewrite argv = %v, want prefix %v", ctx.Args, wantPrefix)
	}
	if reason := structuredShortcutReplayReason(err); reason == "unknown_shortcut" || reason == "ambiguous_command_fallback" {
		t.Fatalf("historical rewrite returned command-resolution reason %q: %v", reason, err)
	}
}

func assertHistoricalShortcutReason(
	t *testing.T,
	badcase shortcutHallucinationReplayBadcase,
	ctx *pipeline.Context,
	err error,
	wantReason string,
) {
	t.Helper()
	if ctx == nil {
		t.Fatalf("historical %s badcase returned nil context: %v", wantReason, err)
	}
	if got := structuredShortcutReplayReason(err); got != wantReason {
		t.Fatalf("historical badcase %q reason = %q, error=%v; want %q (original exit=%d help=%v)", badcase.Raw, got, err, wantReason, badcase.OriginalExitCode, badcase.IsHelp)
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(err)), "unknown flag") {
		t.Fatalf("historical badcase %q regressed to unknown flag: %v", badcase.Raw, err)
	}
}

func structuredShortcutReplayReason(err error) string {
	if err == nil {
		return ""
	}
	var structured *apperrors.Error
	if stderrors.As(err, &structured) {
		return structured.Reason
	}
	return ""
}
