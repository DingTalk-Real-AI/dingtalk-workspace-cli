// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/launcher"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageLauncherVersionMatchesCoreBuildMetadata(t *testing.T) {
	testseam.Swap(t, &version, "v1.2.3")
	testseam.Swap(t, &gitCommit, strings.Repeat("a", 40))
	testseam.Swap(t, &buildTime, "2026-09-06T05:00:00Z")
	testseam.Swap(t, &os.Args, []string{"dws", "--version"})
	t.Setenv("DO_NOT_TRACK", "1")
	output, err := os.CreateTemp(t.TempDir(), "version-output-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	testseam.Swap(t, &os.Stdout, output)
	code := launcher.Main(launcher.Options{
		Version: RawVersion(), Commit: GitCommit(), BuildTime: BuildTime(), Edition: "open",
		CoreSHA256: strings.Repeat("a", 64),
	})
	if code != 0 {
		t.Fatalf("launcher --version exited %d", code)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "dws version "+Version()+"\n"; got != want {
		t.Fatalf("launcher version = %q, core version = %q", got, want)
	}
}
