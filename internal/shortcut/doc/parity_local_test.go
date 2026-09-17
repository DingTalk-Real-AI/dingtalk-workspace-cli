// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocClipboardDecodersAndNativeCommandErrors(t *testing.T) {
	var pngOut bytes.Buffer
	if err := png.Encode(&pngOut, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	body := pngOut.Bytes()
	testseam.Swap(t, &docClipboardRun, func(_ context.Context, name string, args []string) ([]byte, error) {
		switch name {
		case "osascript":
			return []byte("«data PNGf" + hex.EncodeToString(body) + "»"), nil
		case "powershell":
			return []byte(base64.StdEncoding.EncodeToString(body)), nil
		default:
			return body, nil
		}
	})
	testseam.Swap(t, &docClipboardLookPath, func(string) (string, error) { return "/usr/bin/wl-paste", nil })
	for _, platform := range []string{"darwin", "windows", "linux"} {
		got, err := readDocClipboardImageForOS(context.Background(), platform)
		if err != nil || !bytes.Equal(got, body) {
			t.Fatal(platform, err)
		}
	}
	if _, err := readDocClipboardImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &docClipboardLookPath, func(string) (string, error) { return "", os.ErrNotExist })
	if got, err := readDocClipboardImageForOS(context.Background(), "linux"); err != nil || !bytes.Equal(got, body) {
		t.Fatal(err)
	}
	if _, err := readDocClipboardImageForOS(context.Background(), "unsupported"); err == nil {
		t.Fatal("unknown platform accepted")
	}
	for _, tc := range []struct {
		platform string
		body     []byte
		err      error
	}{
		{"darwin", nil, errors.New("command failed")}, {"darwin", []byte("not PNGf"), nil}, {"darwin", []byte("«data PNGfzz»"), nil},
		{"darwin", []byte("«data PNGf»"), nil}, {"darwin", []byte("«data PNGf6162»"), nil},
		{"windows", []byte("invalid!"), nil}, {"linux", bytes.Repeat([]byte("x"), docClipboardLimit+1), nil},
	} {
		testseam.Swap(t, &docClipboardRun, func(context.Context, string, []string) ([]byte, error) { return tc.body, tc.err })
		if _, err := readDocClipboardImageForOS(context.Background(), tc.platform); err == nil {
			t.Fatal("invalid clipboard accepted")
		}
	}
	if _, err := runDocClipboardCommand(context.Background(), "dws-fixture-does-not-exist-129863", nil); err == nil {
		t.Fatal("missing program")
	}
	if data, err := runDocClipboardCommand(context.Background(), "go", []string{"version"}); err != nil || len(data) == 0 {
		t.Fatal("bounded command runner", err)
	}
	var out limitedClipboardBuffer
	if n, err := out.Write([]byte("ok")); err != nil || n != 2 {
		t.Fatal(n, err)
	}
}

type parityClipboardFile struct {
	*os.File
	writeErr, closeErr bool
}

func (f parityClipboardFile) Write(b []byte) (int, error) {
	if f.writeErr {
		return 0, errors.New("disk full")
	}
	return f.File.Write(b)
}
func (f parityClipboardFile) Close() error {
	err := f.File.Close()
	if f.closeErr {
		return errors.New("close failed")
	}
	return err
}

type parityRejectValue struct{}

func (parityRejectValue) String() string   { return "" }
func (parityRejectValue) Type() string     { return "string" }
func (parityRejectValue) Set(string) error { return errors.New("flag storage failure") }
func TestCrossPlatformCoverageDocClipboardTempErrorsCleanUp(t *testing.T) {
	t.Chdir(t.TempDir())
	testseam.Swap(t, &docClipboardImage, func(context.Context) ([]byte, error) { return []byte("fixture"), nil })
	for _, kind := range []string{"create", "write", "close", "flag", "success"} {
		t.Run(kind, func(t *testing.T) {
			testseam.Swap(t, &docClipboardCreateTemp, func(dir, pattern string) (docClipboardFile, error) {
				if kind == "create" {
					return nil, errors.New("read-only filesystem")
				}
				f, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return parityClipboardFile{f, kind == "write", kind == "close"}, nil
			})
			s := MediaInsert
			s.Execute = func(rt *shortcut.RuntimeContext) error {
				if kind == "flag" {
					rt.Command().Flags().Lookup("file").Value = parityRejectValue{}
				}
				return withDocClipboard(rt, func() error {
					if _, err := os.Stat(rt.Str("file")); err != nil {
						t.Fatal(err)
					}
					return nil
				})
			}
			err := runDocCoverage(t, s, &docCoverageCaller{}, "--node", "n", "--from-clipboard", "--yes")
			if (err == nil) != (kind == "success") {
				t.Fatal(kind, err)
			}
			paths, _ := filepath.Glob(".dws-clipboard-*")
			if len(paths) > 0 {
				t.Fatal("temporary file leak", paths)
			}
		})
	}
}

func TestCrossPlatformCoverageDocScriptReadIOFailuresAndPassAssessment(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, kind := range []string{"cwd", "mkdir", "write"} {
		t.Run(kind, func(t *testing.T) {
			switch kind {
			case "cwd":
				testseam.Swap(t, &docGetwd, func() (string, error) { return "", errors.New("cwd removed") })
			case "mkdir":
				testseam.Swap(t, &docMkdirTemp, func(string, string) (string, error) { return "", errors.New("disk full") })
			case "write":
				testseam.Swap(t, &docMkdirTemp, func(string, string) (string, error) { return "missing-parent/draft", nil })
			}
			if err := runDocCoverage(t, Script, &docCoverageCaller{}, "--command", "init-draft"); err == nil {
				t.Fatal("draft IO error accepted")
			}
		})
	}
	for _, tc := range []struct {
		args []string
		data map[string]any
		fail int
		want bool
	}{
		{[]string{"--command", "parse", "--node", "n"}, map[string]any{"nodeId": "other", "markdown": "body"}, 0, false},
		{[]string{"--command", "parse", "--node", "n"}, map[string]any{}, 0, false},
		{[]string{"--command", "parse", "--node", "n"}, nil, 1, false},
		{[]string{"--command", "parse", "--node", "n"}, map[string]any{"nodeId": "n", "markdown": "# Heading\n\nBody"}, 0, true},
		{[]string{"--command", "parse", "--content", "@missing"}, nil, 0, false},
		{[]string{"--command", "parse", "--content", strings.Repeat("x", docMarkdownVerifyMax+1)}, nil, 0, false},
		{[]string{"--command", "parse", "--content", "{}", "--doc-format", "jsonml"}, nil, 0, false},
		{[]string{"--command", "parse", "--content", "# Heading\n\nA body.", "--required-blocks", "heading", "--min-words", "2", "--max-words", "20"}, nil, 0, true},
		{[]string{"--command", "parse", "--content", "body", "--min-words", "30"}, nil, 0, false},
		{[]string{"--command", "parse", "--node", ""}, nil, 0, false},
		{[]string{"--command", "init-draft", "--dry-run"}, nil, 0, true},
	} {
		ctx, _ := output.WithResultStore(context.Background())
		c := &docCoverageCaller{ctx: ctx, failAt: tc.fail, responses: map[string][]map[string]any{"get_document_content": {tc.data}}}
		err := runDocCoverage(t, Script, c, tc.args...)
		if (err == nil) != tc.want {
			t.Fatal(tc.args[0:2], err)
		}
	}
	if docDefaultTitle("```\n# Code\n```\n# Real", "markdown") != "Real" {
		t.Fatal("code block used as title")
	}
	if len([]rune(docDefaultTitle("# "+strings.Repeat("中", 100), "markdown"))) != 80 {
		t.Fatal("title bound")
	}
	if _, err := docScriptProfile(`[]`, "jsonml"); err == nil {
		t.Fatal("non-node JSON accepted")
	}
}

func TestCrossPlatformCoverageDocSelectionErrorsAndAliasStorage(t *testing.T) {
	for _, raw := range []string{`["root",{},["p"]]`, `["root",{},[3,{}]]`, `["root",{},["p",true]]`, `["root",{},7]`} {
		if _, err := docScriptProfile(raw, "jsonml"); err == nil {
			t.Fatal("invalid structure passed", raw)
		}
	}
	if headingLevel(nil) != 0 {
		t.Fatal("non-node heading")
	}
	if _, err := selectDocChapter(map[string]any{}, "a", 0, 0); err == nil {
		t.Fatal("missing full body")
	}
	if _, err := projectKeywordBlocks(map[string]any{}, "x", false, 0, 0); err == nil {
		t.Fatal("missing full body")
	}
	for _, args := range [][]string{
		{"--node", "n", "--context-unit", "blocks"},
		{"--node", "n", "--scope", "chapter"},
		{"--node", "n", "--scope", "keyword", "--keyword", strings.Repeat("x", 4097), "--regex", "--context-unit", "blocks"},
		{"--node", "n", "--scope", "keyword", "--keyword", "[", "--regex", "--context-unit", "blocks"},
	} {
		if err := runDocCoverage(t, Fetch, &docCoverageCaller{}, args...); err == nil {
			t.Fatal("selection error accepted")
		}
	}
	for _, scope := range []string{"chapter", "keyword"} {
		for _, good := range []bool{true, false} {
			data := map[string]any{}
			if good {
				data = map[string]any{"jsonml": `["root",{},["h1",{"uuid":"a"},"Heading"],["p",{"uuid":"b"},"known body"]]`}
			}
			c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {data}}}
			args := []string{"--node", "n", "--scope", scope, "--start-block-id", "a"}
			if scope == "keyword" {
				args = append(args, "--keyword", "known", "--regex", "--context-unit", "blocks")
			}
			err := runDocCoverage(t, Fetch, c, args...)
			if (err == nil) != good {
				t.Fatal(scope, err)
			}
		}
	}
	data := map[string]any{"jsonml": `["root",{},["p",{"uuid":"a"},"Known body"]]`}
	if result, err := projectKeywordBlocks(data, "unknown|known", false, 0, 0); err != nil || result["count"] != 1 {
		t.Fatal(result, err)
	}
	if len(findDocCommentItems(map[string]any{"data": map[string]any{"commentList": []any{1}}})) != 1 {
		t.Fatal("nested comment list")
	}
	c := &docCoverageCaller{failAt: 2}
	if err := runDocCoverage(t, Fetch, c, "--node", "n", "--include-comments"); err == nil {
		t.Fatal("failed comments accepted")
	}
	// Every page is valid, but a bounded list must not report completion when more remain.
	pages := []map[string]any{}
	for i := 0; i < 20; i++ {
		pages = append(pages, map[string]any{"commentList": []any{map[string]any{"commentKey": strings.Repeat("c", i+1)}}, "hasMore": true, "nextToken": strings.Repeat("t", i+1)})
	}
	c = &docCoverageCaller{responses: map[string][]map[string]any{"list_comments": pages}}
	if err := runDocCoverage(t, Fetch, c, "--node", "n", "--include-comments"); err == nil {
		t.Fatal("bounded comments claimed complete")
	}
	s := Script
	s.Flags = append(s.Flags, shortcut.Flag{Name: "doc", Type: shortcut.FlagString})
	_ = withDocParityAliases(s)
	s = Script
	s.Flags = nil
	_ = withDocParityAliases(s)
	s = Create
	s.Execute = func(*shortcut.RuntimeContext) error { return nil }
	if err := runDocCoverage(t, withDocParityAliases(s), &docCoverageCaller{}, "--title", "title"); err != nil {
		t.Fatal(err)
	}
	// A failed underlying flag setter must propagate, not dispatch with an old value.
	s = withDocParityAliases(Create)
	validate := s.Validate
	s.Validate = func(rt *shortcut.RuntimeContext) error {
		rt.Command().Flags().Lookup("name").Value = parityRejectValue{}
		return validate(rt)
	}
	if err := runDocCoverage(t, s, &docCoverageCaller{}, "--title", "title"); err == nil {
		t.Fatal("alias setter failure swallowed")
	}
	for _, value := range []string{"invalid-date", "2026-01-01"} {
		s = Search
		s.Validate = func(rt *shortcut.RuntimeContext) error {
			if value != "invalid-date" {
				rt.Command().Flags().Lookup("created-from").Value = parityRejectValue{}
			}
			return validateDocSearch(rt)
		}
		if err := runDocCoverage(t, s, &docCoverageCaller{}, "--query", "q", "--created-after", value); err == nil {
			t.Fatal("time setter/parse failure")
		}
	}
	s = MediaInsert
	s.Constraints = nil
	s.Validate = nil
	s.Execute = func(rt *shortcut.RuntimeContext) error { return validateDocImageInput(rt) }
	for _, args := range [][]string{{"--node", "n", "--from-clipboard", "--file", "x", "--yes"}, {"--node", "n", "--yes"}} {
		if err := runDocCoverage(t, s, &docCoverageCaller{}, args...); err == nil {
			t.Fatal("bad image input")
		}
	}
	if err := runDocCoverage(t, Script, &docCoverageCaller{}, "--command", "parse", "--node", "https://example.com/not-a-document"); err == nil {
		t.Fatal("unresolved document")
	}
}

func TestCrossPlatformCoverageDocSourceRoleAndScriptURLResolve(t *testing.T) {
	args := []string{"--node", "n", "--command", "block_copy_insert_after", "--src-block-ids", "a,b", "--after-block-id", "ref", "--dry-run"}
	if err := runDocCoverage(t, Update, &docCoverageCaller{dryRun: true}, args...); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--node", "n", "--command", "append", "--content", "x", "--src-block-ids", "a"},
		{"--node", "n", "--command", "block_copy_insert_after", "--src-block-ids", "", "--after-block-id", "ref"},
		{"--node", "n", "--command", "block_copy_insert_after", "--src-block-ids", "a", "--block-id", "b", "--after-block-id", "ref"},
	} {
		if err := runDocCoverage(t, Update, &docCoverageCaller{}, args...); err == nil {
			t.Fatal("source role conflict")
		}
	}
	s := Update
	validate := s.Validate
	s.Validate = func(rt *shortcut.RuntimeContext) error {
		rt.Command().Flags().Lookup("block-id").Value = parityRejectValue{}
		return validate(rt)
	}
	if err := runDocCoverage(t, s, &docCoverageCaller{dryRun: true}, args...); err == nil {
		t.Fatal("source setter failure swallowed")
	}
	for _, fail := range []bool{false, true} {
		ctx, _ := output.WithResultStore(context.Background())
		c := &docCoverageCaller{ctx: ctx, responses: map[string][]map[string]any{"get_document_info": {{"nodeId": "n"}}, "get_document_content": {{"nodeId": "n", "markdown": "Body"}}}}
		if fail {
			c.failAt = 1
		}
		err := runDocCoverage(t, Script, c, "--command", "parse", "--node", "https://alidocs.dingtalk.com/i/nodes/n")
		if (err == nil) == fail {
			t.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageDocScriptCountsWordsAcrossInlineStylesAndCode(t *testing.T) {
	for _, tc := range []struct {
		body, format string
		words        int
	}{
		{"Hel**lo** world", "markdown", 2},
		{`["root",{},["p",{},"Hel",["b",{},"lo"]," world"]]`, "jsonml", 2},
		{"```\nalpha beta\n```", "markdown", 2},
		{"    alpha beta\n", "markdown", 2},
		{"alpha\nbeta", "markdown", 2},
	} {
		profile, err := docScriptProfile(tc.body, tc.format)
		if err != nil || profile["word_count"] != tc.words {
			t.Fatal(tc.body, profile, err)
		}
	}
}
