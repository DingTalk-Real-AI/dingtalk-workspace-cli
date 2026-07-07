// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"0.2.7", "0.3.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"v1.0.6", "1.0.6", 0},
		{"v1.0.5", "v1.0.6", -1},
		{"1.0.6-beta", "1.0.6", -1},
		{"1.0.6", "1.0.6-beta", 1},
		{"1.0.6-beta.2", "1.0.6-beta.10", -1},
		{"1.0.6-beta.10", "1.0.6-beta.2", 1},
		{"1.0.6-alpha", "1.0.6-beta", -1},
		{"1.0.5-rc1", "1.0.6-beta", -1},
		{"2.0.0", "1.99.99", 1},
		{"0.0.1", "0.0.0", 1},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNeedsUpgrade(t *testing.T) {
	tests := []struct {
		current, remote string
		want            bool
	}{
		{"v1.0.6", "1.0.6", false},
		{"v1.0.5", "1.0.6", true},
		{"v1.0.7", "1.0.6", false},
		{"v1.0.6", "1.0.7", true},
		{"v1.0.6-beta.1", "1.0.6", true},
		{"v1.0.6", "1.0.6-beta.1", false},
	}

	for _, tt := range tests {
		got := NeedsUpgrade(tt.current, tt.remote)
		if got != tt.want {
			t.Errorf("NeedsUpgrade(%q, %q) = %v, want %v", tt.current, tt.remote, got, tt.want)
		}
	}
}
