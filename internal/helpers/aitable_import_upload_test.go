// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

func runAitableImportUploadCommand(t *testing.T, args ...string) (*aitableTestCaller, error) {
	t.Helper()
	caller := &aitableTestCaller{}
	installAitableDeps(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard

	cmd := newAitableCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"import", "upload"}, args...))
	return caller, cmd.ExecuteContext(context.Background())
}

func TestAitableImportUploadRequiresPositiveFileSize(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing", args: nil},
		{name: "zero", args: []string{"--file-size", "0"}},
		{name: "negative", args: []string{"--file-size", "-1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"--base-id", "base-smoke", "--file-name", "data.xlsx"}
			caller, err := runAitableImportUploadCommand(t, append(args, test.args...)...)
			if err == nil {
				t.Fatal("expected --file-size validation error")
			}
			if !strings.Contains(err.Error(), "--file-size") {
				t.Fatalf("error = %q, want --file-size validation", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid file size dispatched %d MCP call(s): %#v", len(caller.calls), caller.calls)
			}
		})
	}
}

func TestAitableImportUploadPassesFileSizeToMCP(t *testing.T) {
	caller, err := runAitableImportUploadCommand(t,
		"--base-id", "base-smoke",
		"--file-name", "data.xlsx",
		"--file-size", "204800",
	)
	if err != nil {
		t.Fatalf("aitable import upload returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "aitable" || call.tool != "prepare_import_upload" {
		t.Fatalf("tool call = %s/%s, want aitable/prepare_import_upload", call.server, call.tool)
	}
	want := map[string]any{
		"baseId":   "base-smoke",
		"fileName": "data.xlsx",
		"fileSize": int64(204800),
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("tool args = %#v, want %#v", call.args, want)
	}
}
