package helpers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDocMediaFilesystemAndPositionCompatibilityCoverage(t *testing.T) {
	oldGetwd, oldEval, oldStat := docMediaGetwd, docMediaEvalSymlinks, docMediaStat
	t.Cleanup(func() { docMediaGetwd, docMediaEvalSymlinks, docMediaStat = oldGetwd, oldEval, oldStat })
	if err := ValidateDocMediaInsertCommand(nil); err == nil {
		t.Fatal("nil command accepted")
	}
	docMediaGetwd = func() (string, error) { return "", errors.New("getwd") }
	if _, _, err := resolveDocMediaInputPath("file"); err == nil {
		t.Fatal("getwd failure ignored")
	}
	docMediaGetwd = func() (string, error) { return "/workspace", nil }
	docMediaEvalSymlinks = func(path string) (string, error) {
		if path == "/workspace" {
			return "", errors.New("base")
		}
		return path, nil
	}
	if _, _, err := resolveDocMediaInputPath("file"); err == nil {
		t.Fatal("base symlink failure ignored")
	}
	docMediaEvalSymlinks = func(path string) (string, error) {
		if path == "/workspace" {
			return path, nil
		}
		return "/outside/file", nil
	}
	if _, _, err := resolveDocMediaInputPath("file"); err == nil {
		t.Fatal("workspace escape accepted")
	}
	docMediaEvalSymlinks = func(path string) (string, error) { return path, nil }
	docMediaStat = func(string) (fs.FileInfo, error) { return nil, errors.New("stat") }
	if _, _, err := resolveDocMediaInputPath("file"); err == nil {
		t.Fatal("stat failure ignored")
	}

	cmd := &cobra.Command{Use: "insert"}
	cmd.Flags().Int("index", 0, "")
	cmd.Flags().String("where", "", "")
	cmd.Flags().String("ref-block", "", "")
	_ = cmd.Flags().Set("where", "sideways")
	_ = cmd.Flags().Set("ref-block", "ref")
	if position, err := readDocMediaInsertPosition(cmd); err != nil || position.Mode != "relative" {
		t.Fatalf("main-compatible relative position = %#v, %v", position, err)
	}
	for _, tc := range []struct {
		set      map[string]string
		wantMode string
	}{
		{set: map[string]string{"ref-block": "ref"}, wantMode: "partial_relative"},
		{set: map[string]string{"index": "1", "where": "after", "ref-block": "ref"}, wantMode: "mixed"},
	} {
		cmd := &cobra.Command{Use: "insert"}
		cmd.Flags().Int("index", 0, "")
		cmd.Flags().String("where", "", "")
		cmd.Flags().String("ref-block", "", "")
		for name, value := range tc.set {
			_ = cmd.Flags().Set(name, value)
		}
		if position, err := readDocMediaInsertPosition(cmd); err != nil || position.Mode != tc.wantMode {
			t.Fatalf("position %#v = %#v, %v; want mode %s", tc.set, position, err, tc.wantMode)
		}
	}
}

func TestDocMediaRunEInputResolverFailureCoverage(t *testing.T) {
	oldResolve := docMediaResolveInsertInput
	t.Cleanup(func() { docMediaResolveInsertInput = oldResolve })
	docMediaResolveInsertInput = func(*cobra.Command) (string, fs.FileInfo, docMediaInsertPosition, error) {
		return "", nil, docMediaInsertPosition{}, errors.New("resolve")
	}
	root := newDocCommand()
	cmd, _, _ := root.Find([]string{"media", "insert"})
	_ = cmd.Flags().Set("node", "node")
	if err := runMediaInsert(cmd, nil); err == nil {
		t.Fatal("resolver failure ignored")
	}
}

func TestDocMediaShapeHelperCoverage(t *testing.T) {
	for _, tc := range []struct {
		value any
		keys  []string
	}{
		{map[string]any{"result": map[string]any{"id": " nested "}}, []string{"id"}},
		{[]any{map[string]any{"id": "array"}}, []string{"id"}},
		{map[string]any{"bad": true}, []string{"id"}},
	} {
		_ = docNestedString(tc.value, tc.keys...)
	}
	_ = docResponseString("", "id")
	_ = docResponseString("{", "id")

	for _, value := range []any{
		map[string]any{"items": []any{"item"}},
		map[string]any{"elements": []any{"element"}},
		map[string]any{"blockList": []any{"block"}},
		map[string]any{"result": map[string]any{"blocks": []any{"nested"}}},
		map[string]any{"data": map[string]any{"blocks": []any{}}},
		map[string]any{"content": []any{"plain"}},
		[]any{"root", []any{"p", map[string]any{"uuid": "p"}}},
		[]any{"root", map[string]any{}, []any{"p", map[string]any{"uuid": "p"}}},
		[]any{"p", map[string]any{"uuid": "p"}},
		"bad",
	} {
		_ = docTopLevelBlocks(value)
	}
	for _, value := range []any{
		map[string]any{"jsonml": `[`},
		map[string]any{"data": map[string]any{"jsonml": `["root",{}]`}},
		map[string]any{"content": "plain"},
		"plain",
	} {
		_ = docJSONMLValue(value)
	}
	for _, value := range []any{
		[]any{"p"}, []any{"p", "attrs"}, []any{"p", map[string]any{"uuid": " u "}},
		map[string]any{"elementId": " e "}, map[string]any{"element": map[string]any{"id": "nested"}},
		map[string]any{"block": map[string]any{"uuid": "block"}}, true,
	} {
		_ = docDirectBlockID(value)
	}
	for _, value := range []any{
		[]any{"img", map[string]any{"uuid": "image"}},
		[]any{"p", map[string]any{}, []any{"attachment", map[string]any{"uuid": "attachment"}}},
		map[string]any{"image": map[string]any{"id": "image-map"}},
		map[string]any{"child": map[string]any{"attachment": map[string]any{"id": "attachment-map"}}},
		map[string]any{"child": []any{"img", map[string]any{"uuid": "image-child"}}},
	} {
		_ = docNestedMediaElementID(value, "inline_image")
		_ = docNestedMediaElementID(value, "attachment")
	}
	for _, tc := range []struct {
		value any
		kind  string
	}{
		{map[string]any{"blockType": "attachment"}, "attachment"},
		{map[string]any{"elementType": "image"}, "inline_image"},
		{map[string]any{"attachment": map[string]any{}}, "attachment"},
		{map[string]any{"image": map[string]any{}}, "inline_image"},
		{map[string]any{"child": []any{"attachment", map[string]any{}}}, "attachment"},
		{[]any{"image", map[string]any{}}, "inline_image"},
		{[]any{"attachment", map[string]any{}}, "attachment"},
		{[]any{"p", map[string]any{}, []any{"img", map[string]any{}}}, "inline_image"},
	} {
		if !docMediaKindMatches(tc.value, tc.kind) {
			t.Fatalf("kind mismatch for %#v", tc.value)
		}
	}
	positions := []docMediaInsertPosition{
		{Mode: "index", Index: 1}, {Mode: "relative", RefBlock: "ref", Where: "before"},
		{Mode: "relative", RefBlock: "ref", Where: "after"}, {Mode: "end"},
	}
	for _, position := range positions {
		_, _ = verifyDocMediaPosition([]string{"before", "inserted", "ref"}, "inserted", position)
		_ = describeDocMediaPosition(position)
	}
	_, _ = verifyDocMediaPosition([]string{"ref"}, "missing", positions[0])
	_, _ = verifyDocMediaPosition([]string{"inserted"}, "inserted", docMediaInsertPosition{Mode: "relative", RefBlock: "missing"})
	for _, value := range []any{
		map[string]any{"resourceId": "rid"}, map[string]any{"nested": map[string]any{"src": "url"}},
		[]any{map[string]any{"url": "url"}}, "bad",
	} {
		_ = docMediaIdentityMatches(value, "rid", "url")
	}
	_ = docMediaMatches(map[string]any{"blocks": []any{
		map[string]any{"id": "b", "attachment": map[string]any{"resourceId": "rid"}},
		map[string]any{"attachment": map[string]any{"resourceId": "rid"}},
	}}, "rid", "", "attachment")
	_ = containsDocMediaBlockID([]docMediaMatch{{BlockID: "a"}}, "missing")
}

func TestDocMediaInsertVerificationBranchCoverage(t *testing.T) {
	oldPut := httpPutFile
	t.Cleanup(func() { httpPutFile = oldPut })
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	installImmediateTiming(t)
	workspace := t.TempDir()
	t.Chdir(workspace)
	if err := os.WriteFile("file.txt", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("image.png", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, caller *scriptedToolCaller, file string, extra ...string) error {
		t.Helper()
		installScriptedCaller(t, caller)
		root := newDocCommand()
		cmd, _, _ := root.Find([]string{"media", "insert"})
		_ = cmd.Flags().Set("node", "node")
		_ = cmd.Flags().Set("file", file)
		for i := 0; i+1 < len(extra); i += 2 {
			_ = cmd.Flags().Set(extra[i], extra[i+1])
		}
		cmd.SetIn(strings.NewReader("yes\n"))
		return cmd.RunE(cmd, nil)
	}

	if err := run(t, &scriptedToolCaller{}, "file.txt", "name", "../bad"); err == nil {
		t.Fatal("path-like display name accepted")
	}
	if err := run(t, &scriptedToolCaller{}, "file.txt", "mime-type", "not a mime"); err == nil {
		t.Fatal("invalid MIME accepted")
	}
	if err := run(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}}}, "image.png"); err == nil {
		t.Fatal("image without resource URL accepted")
	}
	if err := run(t, &scriptedToolCaller{dry: true}, "file.txt", "index", "0"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, &scriptedToolCaller{dry: true}, "file.txt", "where", "after", "ref-block", "ref"); err != nil {
		t.Fatal(err)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	root := newDocCommand()
	missingFile, _, _ := root.Find([]string{"media", "insert"})
	_ = missingFile.Flags().Set("node", "node")
	if err := missingFile.RunE(missingFile, nil); err == nil {
		t.Fatal("missing file accepted")
	}
	root = newDocCommand()
	missingNode, _, _ := root.Find([]string{"media", "insert"})
	_ = missingNode.Flags().Set("file", "file.txt")
	if err := missingNode.RunE(missingNode, nil); err == nil {
		t.Fatal("missing node accepted")
	}
	root = newDocCommand()
	invalidPosition, _, _ := root.Find([]string{"media", "insert"})
	_ = invalidPosition.Flags().Set("node", "node")
	_ = invalidPosition.Flags().Set("file", "file.txt")
	_ = invalidPosition.Flags().Set("where", "after")
	if err := invalidPosition.RunE(invalidPosition, nil); err != nil {
		t.Fatalf("direct RunE rejected main-compatible incomplete relative position: %v", err)
	}

	oldEval := docMediaEvalSymlinks
	calls := 0
	docMediaEvalSymlinks = func(path string) (string, error) {
		calls++
		if calls > 4 {
			return "", errors.New("second position resolution")
		}
		return oldEval(path)
	}
	_ = run(t, &scriptedToolCaller{}, "file.txt")
	docMediaEvalSymlinks = oldEval

	attachment := `{"blocks":[{"id":"media","attachment":{"resourceId":"rid"}}]}`
	cases := []struct {
		name  string
		steps []scriptedToolStep
		extra []string
	}{
		{"full read error", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {err: errors.New("read")}}, nil},
		{"full read invalid json", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: `{`}}, nil},
		{"duplicate media", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: `{"blocks":[{"id":"a","attachment":{"resourceId":"rid"}},{"id":"b","attachment":{"resourceId":"rid"}}]}`}}, nil},
		{"position mismatch", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: attachment}}, []string{"index", "1"}},
		{"targeted read error", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: attachment}, {err: errors.New("target")}}, nil},
		{"targeted invalid json", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: attachment}, {text: `{`}}, nil},
		{"targeted identity mismatch", []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`}, {text: attachment}, {text: `{"blocks":[{"id":"media","attachment":{"resourceId":"other"}}]}`}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(t, &scriptedToolCaller{steps: tc.steps}, "file.txt", tc.extra...); err == nil {
				t.Fatal("expected verification failure")
			}
		})
	}

	// Cover automatic extension preservation and a successful explicit end-position receipt.
	if err := run(t, &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"uploadUrl":"https://upload","resourceId":"rid"}`}, {text: `{"blockId":"media"}`},
		{text: attachment}, {text: attachment},
	}}, "file.txt", "name", "renamed"); err != nil {
		t.Fatal(err)
	}
}
