// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package edition

import (
	"reflect"
	"slices"
	"testing"
)

func TestCrossPlatformCoverageOutboundCallSupplementEndpoint(t *testing.T) {
	want := ServerInfo{
		ID: "outbound-call", Name: "AI 语音外呼",
		Endpoint: "https://mcp-gw.dingtalk.com/server/1b382233a46b7208b9adeb2396bcaac5ffc344b97502f11dea0a3eaa13853e5f",
		Prefixes: []string{"outbound-call"},
	}
	count := 0
	for _, server := range openSupplementServers() {
		if server.ID == want.ID {
			count++
			if !reflect.DeepEqual(server, want) {
				t.Fatalf("outbound-call endpoint = %#v, want %#v", server, want)
			}
			server.Prefixes[0] = "changed"
		}
	}
	if count != 1 {
		t.Fatalf("outbound-call registered %d times", count)
	}
	for _, server := range openSupplementServers() {
		if server.ID == want.ID && !reflect.DeepEqual(server, want) {
			t.Fatal("supplement endpoint shares mutable prefix storage")
		}
	}
	if slices.Contains(openVisibleProducts(), want.ID) {
		t.Fatal("outbound-call must use explicit helper registration")
	}
}
