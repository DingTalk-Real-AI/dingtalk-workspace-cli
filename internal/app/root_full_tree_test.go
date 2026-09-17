// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageProcessStartupAlwaysBuildsCompleteTree(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Protect(t, &os.Args)

	for _, args := range [][]string{
		{"dws", "--help"},
		{"dws", "--version"},
		{"dws", "schema", "calendar"},
		{"dws", "config", "get", "output"},
		{"dws", "calendar", "book", "list", "--dry-run"},
		{"dws", "__complete", "calendar", ""},
	} {
		os.Args = args
		root := newProcessRootCommandWithEngine(context.Background(), nil)
		for _, name := range []string{"calendar", "chat", "drive", "schema"} {
			command, _, err := root.Find([]string{name})
			if err != nil || command == nil || command == root {
				t.Fatalf("argv %v missing complete-tree command %q: command=%v err=%v", args, name, command, err)
			}
		}
	}
}
