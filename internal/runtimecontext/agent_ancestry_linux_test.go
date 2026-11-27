// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build linux

package runtimecontext

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageLinuxAncestryHelpers(t *testing.T) {
	if _, ok := linuxLookupProcess(-1); ok {
		t.Fatal("expected invalid pid lookup to fail")
	}
	testseam.Swap(t, &linuxReadProcStat, func(string) ([]byte, error) { return []byte("malformed"), nil })
	if _, ok := linuxLookupProcess(123); ok {
		t.Fatal("expected malformed stat lookup to fail")
	}
	for _, bad := range [][]byte{
		[]byte("invalid"),
		[]byte("1 (cmd) S 2"),
		[]byte("1 (cmd) S bad 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
		[]byte("1 (cmd) S 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 bad"),
	} {
		if _, _, _, ok := parseLinuxProcStat(bad); ok {
			t.Fatalf("accepted invalid stat %q", bad)
		}
	}
	comm, ppid, start, ok := parseLinuxProcStat([]byte("42 (my (host)) S 10 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 987654"))
	if !ok || comm != "my (host)" || ppid != 10 || start != 987654 {
		t.Fatalf("parsed = %q %d %d %v", comm, ppid, start, ok)
	}
}
