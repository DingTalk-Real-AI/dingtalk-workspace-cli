// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package localename

import "testing"

func TestCrossPlatformCoverageResolveChineseAndDefault(t *testing.T) {
	for raw, want := range map[string]string{
		"zh":      "zh",
		" ZH-CN ": "zh",
		"zh_TW":   "zh",
		"en":      "en",
		"en-US":   "en",
		"":        "en",
		"fr":      "en",
	} {
		if got := Resolve(raw); got != want {
			t.Fatalf("Resolve(%q) = %q want %q", raw, got, want)
		}
	}
}
