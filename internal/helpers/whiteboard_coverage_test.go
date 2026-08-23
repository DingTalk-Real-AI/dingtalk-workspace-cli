package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWhiteboardInjectedEncodingFailures(t *testing.T) {
	previousMarshal := whiteboardJSONMarshal
	whiteboardJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	if got := buildWhiteboardCardJSONML("b", "w"); got != "" {
		t.Fatalf("got %q", got)
	}
	whiteboardJSONMarshal = previousMarshal

	previousPrepare := prepareWhiteboardCard
	prepareWhiteboardCard = func(*cobra.Command, string) (string, error) { return "", errors.New("prepare") }
	t.Cleanup(func() { prepareWhiteboardCard = previousPrepare })
	caller := &whiteboardTestCaller{}
	installWhiteboardTestCaller(t, caller)
	cmd := newDocWhiteboardCommand()
	cmd.SetArgs([]string{"insert", "--node", "n", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "模板未通过") {
		t.Fatalf("err=%v", err)
	}

	previousCompact := compactWhiteboardJSON
	compactWhiteboardJSON = func(*bytes.Buffer, []byte) error { return errors.New("compact") }
	t.Cleanup(func() { compactWhiteboardJSON = previousCompact })
	if _, _, err := validateWhiteboardNodes(json.RawMessage(`[{"id":"n"}]`)); err == nil {
		t.Fatal("expected compact error")
	}
}

func TestDocWhiteboardInsertDryRun(t *testing.T) {
	caller := &whiteboardTestCaller{dry: true}
	installWhiteboardTestCaller(t, caller)
	cmd := newDocWhiteboardCommand()
	cmd.SetArgs([]string{"insert", "--node", "n"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func writeWhiteboardFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "whiteboard.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWhiteboardUpdateFileRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "invalid json", content: `{`},
		{name: "trailing value", content: `{}` + ` {}`},
		{name: "missing source", content: `{}`},
		{name: "schema version", content: `{"source":{"schemaVersion":"2.0","catalogVersion":"dml-v1","nodes":[]}}`},
		{name: "catalog version", content: `{"source":{"schemaVersion":"1.0","catalogVersion":"v2","nodes":[]}}`},
		{name: "nodes missing", content: `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1"}}`},
		{name: "nodes malformed", content: `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[}}`},
		{name: "node primitive", content: `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[1]}}`},
		{name: "append empty", content: `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`},
		{name: "unknown field", content: `{"unknown":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := loadWhiteboardUpdateFile(writeWhiteboardFixture(t, test.content)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	if _, _, err := loadWhiteboardUpdateFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing-file error")
	}
	if _, _, err := loadWhiteboardUpdateFile(t.TempDir()); err == nil {
		t.Fatal("expected directory read error")
	}
	if _, _, err := validateWhiteboardNodes(json.RawMessage(`[`)); err == nil {
		t.Fatal("expected malformed nodes array error")
	}
	input, nodes, err := loadWhiteboardUpdateFile(writeWhiteboardFixture(t,
		`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`))
	if err != nil || !input.Overwrite || nodes != "[]" {
		t.Fatalf("input=%#v nodes=%q err=%v", input, nodes, err)
	}
}

func TestWhiteboardOutputFiltersAndToolResponseErrors(t *testing.T) {
	for _, name := range []string{"jq", "fields"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String(name, "", "")
			if err := cmd.Flags().Set(name, ".result"); err != nil {
				t.Fatal(err)
			}
			if err := rejectWhiteboardOutputFilters(cmd); err == nil {
				t.Fatal("expected rejected output filter")
			}
		})
	}

	responses := []string{
		`{`,
		`{} {}`,
		`null`,
		`{"resultJson":"{"}`,
		`{"resultJson":"{} {}"}`,
	}
	for _, response := range responses {
		caller := &whiteboardTestCaller{format: "json", response: func(whiteboardTestCall, int) string { return response }}
		installWhiteboardTestCaller(t, caller)
		if err := callWhiteboardTool(&cobra.Command{}, whiteboardQueryTool, nil); err == nil {
			t.Fatalf("response %q should fail", response)
		}
	}

	caller := &whiteboardTestCaller{dry: true}
	installWhiteboardTestCaller(t, caller)
	if err := callWhiteboardTool(&cobra.Command{}, whiteboardQueryTool, map[string]any{"partId": "p"}); err != nil {
		t.Fatal(err)
	}
	caller = &whiteboardTestCaller{response: func(whiteboardTestCall, int) string { return "" }}
	installWhiteboardTestCaller(t, caller)
	if err := callWhiteboardTool(&cobra.Command{}, whiteboardQueryTool, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardDocumentQueryValidation(t *testing.T) {
	tests := []struct {
		name     string
		response string
		attrs    bool
	}{
		{name: "invalid response", response: `{`},
		{name: "missing block", response: `{"blocks":[]}`},
		{name: "non object block", response: `{"blocks":[1]}`},
		{name: "invalid jsonml", response: `{"blocks":[{"blockId":"b","jsonml":"{"}]}`},
		{name: "missing attrs", response: `{"blocks":[{"blockId":"b","jsonml":"[]"}]}`, attrs: true},
		{name: "attrs not object", response: `{"blocks":[{"blockId":"b","jsonml":"[\"card\",1]"}]}`, attrs: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &whiteboardTestCaller{response: func(whiteboardTestCall, int) string { return test.response }}
			installWhiteboardTestCaller(t, caller)
			var err error
			if test.attrs {
				_, err = queryWhiteboardCardAttrs(context.Background(), "n", "b")
			} else {
				_, err = queryWhiteboardCardNode(context.Background(), "n", "b")
			}
			if err == nil {
				t.Fatal("expected query validation error")
			}
		})
	}
	caller := &whiteboardTestCaller{err: func(whiteboardTestCall, int) error { return errors.New("boom") }}
	installWhiteboardTestCaller(t, caller)
	if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err == nil {
		t.Fatal("expected caller error")
	}
}

func TestWhiteboardCommandValidationBranches(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json"}
	installWhiteboardTestCaller(t, caller)
	for _, args := range [][]string{
		{"query", "--node", "n"},
		{"query", "--node", "n", "--part-id", "p", "--jq", "."},
		{"update", "--node", "n", "--part-id", "p"},
		{"update", "--node", "n", "--part-id", "p", "--fields", "result"},
	} {
		cmd := newWhiteboardCommand()
		cmd.PersistentFlags().String("jq", "", "")
		cmd.PersistentFlags().String("fields", "", "")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("args %v should fail", args)
		}
	}
}

func TestWhiteboardUpdateOverwriteAndSourceErrors(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json"}
	installWhiteboardTestCaller(t, caller)
	cmd := newWhiteboardCommand()
	cmd.SetArgs([]string{"update", "--node", "n", "--part-id", "p", "--source", writeWhiteboardFixture(t,
		`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`), "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if caller.calls[0].args["mode"] != "overwrite" {
		t.Fatalf("args=%#v", caller.calls[0].args)
	}

	cmd = newWhiteboardCommand()
	cmd.SetArgs([]string{"update", "--node", "n", "--part-id", "p", "--source", filepath.Join(t.TempDir(), "missing")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected source error")
	}
}

func TestDocMediaUploadValidationAndSuccess(t *testing.T) {
	caller := &whiteboardTestCaller{response: func(whiteboardTestCall, int) string {
		return `{"uploadUrl":"https://upload.example.test/token","resourceId":"r","resourceUrl":"https://resource.example.test/icon"}`
	}}
	installWhiteboardTestCaller(t, caller)
	previousPut := httpPutFile
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	t.Cleanup(func() { httpPutFile = previousPut })
	file := writeWhiteboardFixture(t, "svg")
	cmd := newDocCommand()
	cmd.SetArgs([]string{"media", "upload", "--node", "n", "--file", file, "--name", "icon", "--mime-type", "image/custom", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if caller.calls[0].args["fileName"] != "icon.json" || caller.calls[0].args["mimeType"] != "image/custom" {
		t.Fatalf("args=%#v", caller.calls[0].args)
	}

	for _, args := range [][]string{
		{"media", "upload", "--node", "n"},
		{"media", "upload", "--node", "n", "--file", filepath.Join(t.TempDir(), "missing")},
		{"media", "upload", "--node", "n", "--file", t.TempDir()},
	} {
		cmd = newDocCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("args %v should fail", args)
		}
	}
}

func TestDocMediaUploadRemainingBranches(t *testing.T) {
	file := writeWhiteboardFixture(t, "svg")

	caller := &whiteboardTestCaller{dry: true}
	installWhiteboardTestCaller(t, caller)
	cmd := newDocCommand()
	cmd.SetArgs([]string{"media", "upload", "--node", "n", "--file", file})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		caller   *whiteboardTestCaller
		response string
	}{
		{name: "caller error", caller: &whiteboardTestCaller{err: func(whiteboardTestCall, int) error { return errors.New("call") }}},
		{name: "missing resource url", caller: &whiteboardTestCaller{response: func(whiteboardTestCall, int) string {
			return `{"uploadUrl":"https://upload.example.test/token","resourceId":"r"}`
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			installWhiteboardTestCaller(t, test.caller)
			cmd := newDocCommand()
			cmd.SetArgs([]string{"media", "upload", "--node", "n", "--file", file, "--yes"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected upload error")
			}
		})
	}
}

func TestDocWhiteboardInsertCallerError(t *testing.T) {
	caller := &whiteboardTestCaller{err: func(call whiteboardTestCall, index int) error {
		if index == 0 {
			return errors.New("insert")
		}
		return nil
	}}
	installWhiteboardTestCaller(t, caller)
	cmd := newDocWhiteboardCommand()
	cmd.SetArgs([]string{"insert", "--node", "n", "--yes"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestExtractWhiteboardIDAndJSONEOF(t *testing.T) {
	if got := extractWhiteboardID(nil); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := extractWhiteboardID(map[string]any{"metadata": map[string]any{"id": 1}}); got != "" {
		t.Fatalf("got %q", got)
	}
	decoder := json.NewDecoder(strings.NewReader(`{} trailing`))
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if err := ensureWhiteboardJSONEOF(decoder); err == nil {
		t.Fatal("expected trailing token error")
	}
}
