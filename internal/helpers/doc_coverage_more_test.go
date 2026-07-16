package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type docFailingReader struct{}

func (docFailingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestCrossPlatformCoverageDocVersionTraversalCoverage(t *testing.T) {
	for _, value := range []any{
		map[string]any{"hasMore": false, "nextCursor": "ignored"},
		map[string]any{"nextCursor": "next"}, map[string]any{"nextToken": "token"}, map[string]any{"cursor": "cursor"},
		map[string]any{"result": map[string]any{"nextCursor": "nested"}},
		map[string]any{"other": []any{map[string]any{"cursor": "array"}}},
		[]any{map[string]any{"nextCursor": "list"}}, "bad",
	} {
		_ = docVersionNextCursor(value)
	}
	for _, value := range []any{
		map[string]any{"version": float64(3)}, map[string]any{"version_number": "3"},
		map[string]any{"nested": []any{map[string]any{"revision": json.Number("3")}}},
		[]any{map[string]any{"docVersion": 3}}, "bad",
	} {
		_ = docVersionPayloadContains(value, 3)
	}
	for _, value := range []any{float64(3), float64(3.5), "3", "bad", json.Number("3"), json.Number("bad"), 3} {
		_ = docVersionNumberMatches(value, 3)
	}

	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nextCursor":"next","versions":[]}`},
		{text: `{"versions":[{"version":3}]}`},
	}}
	installScriptedCaller(t, caller)
	found, err := docVersionExists(context.Background(), "node", 3)
	if err != nil || !found {
		t.Fatalf("docVersionExists() = %v, %v", found, err)
	}
	caller.steps = []scriptedToolStep{{text: `{`}}
	caller.index = 0
	_, _ = docVersionExists(context.Background(), "node", 3)
	caller.steps = []scriptedToolStep{{err: errors.New("failed")}}
	caller.index = 0
	_, _ = docVersionExists(context.Background(), "node", 3)
}

func TestCrossPlatformCoverageDocUploadDownloadParsingCoverage(t *testing.T) {
	for _, raw := range []string{
		`{`, `{}`,
		`{"resourceUrl":"https://upload","uploadKey":"key","headers":{"x":"value","bad":1}}`,
		`{"result":{"resourceUrl":"https://upload","uploadKey":"key"}}`,
	} {
		_, _, _, _ = parseUploadInfo(raw)
	}
	for _, raw := range []string{
		`{`, `{}`,
		`{"resourceUrl":"https://download/file","headers":{"x":"value","bad":1}}`,
		`{"resourceUrl":["https://download/file"]}`,
		`{"downloadUrl":"https://download/file"}`,
		`{"result":{"resourceUrl":"https://download/file"}}`,
	} {
		_, _, _ = parseDownloadInfo(raw)
	}
	for _, raw := range []string{"", "https://example.test/", "https://example.test/file.txt?x=1", "https://example.test/a%2Fb.txt", "https://example.test/%"} {
		_ = inferFilename(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"uploadUrl":"https://upload","resourceId":"resource"}`, `{"result":{"uploadUrl":"https://upload","resourceId":"resource","resourceUrl":"https://image"}}`} {
		_, _, _, _ = parseAttachmentUploadInfo(raw)
	}
}

func TestCrossPlatformCoverageResolveDocContentCoverage(t *testing.T) {
	previous := deps
	InitDeps(&productExampleCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previous })
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "doc"}
		cmd.Flags().String("content-file", "", "")
		cmd.Flags().String("content-path", "", "")
		cmd.Flags().String("content", "", "")
		cmd.Flags().String("markdown", "", "")
		return cmd
	}
	file := filepath.Join(t.TempDir(), "content.md")
	if err := os.WriteFile(file, []byte("file content"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		flag, value string
		stdin       io.Reader
	}{
		{"content-file", file, nil}, {"content-file", filepath.Join(t.TempDir(), "missing"), nil},
		{"content", "-", strings.NewReader("stdin content")}, {"content", "-", docFailingReader{}},
		{"content", `line\nnext`, nil}, {"markdown", "-", strings.NewReader("markdown stdin")},
		{"markdown", "-", docFailingReader{}}, {"markdown", `line\tnext`, nil}, {"content", "plain", nil},
	} {
		cmd := newCommand()
		_ = cmd.Flags().Set(tc.flag, tc.value)
		if tc.stdin != nil {
			cmd.SetIn(tc.stdin)
		}
		_, _ = resolveContentFromFlags(cmd)
	}
	for _, value := range []string{"", "plain", `line\nnext`, `bad\q`} {
		_ = unescapeLiteralContent(value)
	}
}

func TestCrossPlatformCoverageRunDocReadJSONMLCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	cases := []struct {
		name     string
		step     scriptedToolStep
		output   string
		wantFail bool
	}{
		{"call-error", scriptedToolStep{err: errors.New("failed")}, "", true},
		{"invalid-response", scriptedToolStep{text: `{`}, "", true},
		{"missing-jsonml", scriptedToolStep{text: `{}`}, "", true},
		{"invalid-jsonml", scriptedToolStep{text: `{"jsonml":"{"}`}, "", true},
		{"revision-float", scriptedToolStep{text: `{"jsonml":"{}","revision":2}`}, "", false},
		{"revision-string", scriptedToolStep{text: `{"jsonml":"{}","revision":"3"}`}, "", false},
		{"revision-bad-string", scriptedToolStep{text: `{"jsonml":"{}","revision":"bad"}`}, "", false},
		{"write", scriptedToolStep{text: `{"jsonml":"{}"}`}, filepath.Join(t.TempDir(), "out.json"), false},
		{"write-error", scriptedToolStep{text: `{"jsonml":"{}"}`}, t.TempDir(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{tc.step}}
			installScriptedCaller(t, caller)
			err := runDocReadJsonML(&cobra.Command{}, "node", tc.output)
			if tc.wantFail && err == nil {
				t.Fatal("expected failure")
			}
		})
	}
}

func TestCrossPlatformCoverageDocDeprecationWrappersCoverage(t *testing.T) {
	previous := deps
	InitDeps(&productExampleCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previous })
	for _, wrap := range []func(*cobra.Command){
		func(cmd *cobra.Command) { wrapDocDeprecated(cmd, "drive target") },
		func(cmd *cobra.Command) { wrapDocDeprecatedToWiki(cmd, "wiki target") },
		func(cmd *cobra.Command) { wrapDocDeprecatedToTarget(cmd, "target") },
	} {
		for _, mounted := range []bool{true, false} {
			cmd := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
			wrap(cmd)
			if mounted {
				root := &cobra.Command{Use: "dws"}
				doc := &cobra.Command{Use: "doc"}
				root.AddCommand(doc)
				doc.AddCommand(cmd)
			}
			_ = cmd.RunE(cmd, nil)
		}
	}
}

func TestCrossPlatformCoverageRunDocUploadDownloadAndMediaCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	oldPut, oldGet := httpPutFile, httpGetFile
	t.Cleanup(func() { httpPutFile, httpGetFile = oldPut, oldGet })
	file := filepath.Join(t.TempDir(), "file.md")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, dry := range []bool{true, false} {
		caller := &scriptedToolCaller{dry: dry, steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {text: `{"ok":true}`},
		}}
		installScriptedCaller(t, caller)
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		root := newDocCommand()
		cmd, _, _ := root.Find([]string{"upload"})
		_ = cmd.Flags().Set("file", file)
		_ = cmd.Flags().Set("workspace", "workspace")
		_ = cmd.Flags().Set("name", "renamed")
		_ = cmd.Flags().Set("folder", "folder-node")
		_ = cmd.Flags().Set("convert", "true")
		_ = cmd.RunE(cmd, nil)
	}

	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"resourceUrl":"https://download/file.txt"}`}}}
	installScriptedCaller(t, caller)
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destination string) error {
		return os.WriteFile(destination, []byte("download"), 0o600)
	}
	root := newDocCommand()
	download, _, _ := root.Find([]string{"download"})
	_ = download.Flags().Set("node", "node")
	_ = download.Flags().Set("output", t.TempDir())
	_ = download.RunE(download, nil)

	for _, mime := range []string{"image/png", "text/markdown", "application/pdf"} {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"uploadUrl":"https://upload","resourceId":"resource","resourceUrl":"https://image"}`}, {text: `{"ok":true}`},
		}}
		installScriptedCaller(t, caller)
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		root := newDocCommand()
		media, _, _ := root.Find([]string{"media", "insert"})
		_ = media.Flags().Set("node", "node")
		_ = media.Flags().Set("file", file)
		_ = media.Flags().Set("mime-type", mime)
		_ = media.Flags().Set("name", "renamed")
		_ = media.Flags().Set("index", "1")
		_ = media.Flags().Set("where", "after")
		_ = media.Flags().Set("ref-block", "block")
		_ = media.RunE(media, nil)
	}
}
