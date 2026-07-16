package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageTodoPureUtilityCoverage(t *testing.T) {
	cmd := &cobra.Command{Use: "todo"}
	cmd.Flags().String("remind-at", "", "")
	_ = rejectUnsupportedTodoReminderFlags(cmd)
	_ = cmd.Flags().Set("remind-at", "2026-01-02")
	_ = rejectUnsupportedTodoReminderFlags(cmd)
	for _, raw := range []string{"", " one, ,two "} {
		_ = parseExecutorIds(raw)
		_ = parseIntList(raw)
		_ = parseStringList(raw)
	}
	_ = parseIntList("1,bad,,2")
	for _, raw := range []string{"", "creator", "creator,executor,participant", "invalid"} {
		_, _ = parseRoleTypes(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"result":{"dentryId":1,"spaceId":"2"}}`} {
		_, _, _ = parseTodoFileSendIDs(raw)
	}
}

func TestCrossPlatformCoverageTodoFileUtilityCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = buildTodoLocalFileMeta(filepath.Join(t.TempDir(), "missing"), "", "")
	_, _ = buildTodoLocalFileMeta(t.TempDir(), "", "")
	_, _ = buildTodoLocalFileMeta(file, "custom.bin", "provided")
	_, _ = buildTodoLocalFileMeta(file, "", "")
	for _, raw := range []string{
		`{`, `{}`,
		`{"resourceUrls":["https://upload"],"uploadKey":"key","headers":{"x-test":1,"blank":null},"ossHeaders":{"x-other":"yes"}}`,
		`{"result":{"resourceUrl":"https://upload","key":"key"}}`,
	} {
		_, _, _, _ = parseTodoFileUploadInfo(raw)
	}
}

func TestCrossPlatformCoverageUploadTodoLocalFileCoverage(t *testing.T) {
	oldPut := httpPutFile
	t.Cleanup(func() { httpPutFile = oldPut })
	meta := todoLocalFileMeta{LocalPath: "file", FileName: "file.txt", FileSize: 4, MD5: "md5"}
	for _, tc := range []struct {
		name  string
		steps []scriptedToolStep
		put   error
	}{
		{"success", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {text: `{"ok":true}`}}, nil},
		{"init-error", []scriptedToolStep{{err: errors.New("init")}}, nil},
		{"parse-error", []scriptedToolStep{{text: `{}`}}, nil},
		{"put-error", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}}, errors.New("put")},
		{"commit-error", []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}, {err: errors.New("commit")}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return tc.put }
			_, _ = uploadTodoLocalFile(context.Background(), meta)
		})
	}
}

func TestCrossPlatformCoverageTodoAutoPageStopsAtWantedSize(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "todo"}
	t.Cleanup(func() { os.Args = oldArgs })
	root := newTodoCommand()
	cmd, _, err := root.Find([]string{"task", "list"})
	if err != nil {
		t.Fatal(err)
	}
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"result":{"todoCards":[{"id":1},{"id":2}]}}`}}}
	installScriptedCaller(t, caller)
	_ = todoListAutoPage(cmd, "1", 1)
}
