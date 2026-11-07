// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageNormalizeCLIPathMatchesFieldsReference(t *testing.T) {
	cases := []string{
		"",
		"dws",
		"dws ",
		" dws",
		"dws calendar",
		"dws calendar event list",
		"calendar event list",
		"dwsdrive copy",
		"dws  calendar",
		"calendar  event  list",
		"dws\tcalendar",
		"dws calendar\tlist",
		"dws\ncalendar list",
		"dws calendar\u00a0list",
		"dws 日程 列表",
		"dws calendar\x00list",
		"dws calendar ",
		"   dws calendar list   ",
		"dws dws calendar",
		"d",
		"dw",
		"dwsx",
	}
	for _, path := range cases {
		want := strings.Join(dropDWSRoot(strings.Fields(strings.TrimSpace(path))), " ")
		if got := NormalizeCLIPath(path); got != want {
			t.Errorf("NormalizeCLIPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCrossPlatformCoverageNormalizeCLIPathReturnsSubstringOfInput(t *testing.T) {
	path := "dws calendar event list"
	got := NormalizeCLIPath(path)
	if want := "calendar event list"; got != want {
		t.Fatalf("NormalizeCLIPath(%q) = %q, want %q", path, got, want)
	}
	if !strings.Contains(path, got) {
		t.Fatalf("normalized %q is not a substring of %q", got, path)
	}
}

func dropDWSRoot(parts []string) []string {
	if len(parts) > 0 && parts[0] == "dws" {
		return parts[1:]
	}
	return parts
}
