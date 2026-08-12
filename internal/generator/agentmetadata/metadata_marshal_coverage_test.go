// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMetadataMarshalPresentLists(t *testing.T) {
	product := ProductMetadata{
		AgentSummary:     "summary",
		UseWhen:          []string{" use ", ""},
		AvoidWhen:        nil,
		useWhenPresent:   true,
		avoidWhenPresent: true,
	}
	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"use_when"`) || !strings.Contains(string(raw), `"avoid_when"`) {
		t.Fatalf("product marshal = %s", raw)
	}

	tool := ToolMetadata{
		AgentSummary:     "tool",
		UseWhen:          []string{"a"},
		AvoidWhen:        []string{"b"},
		useWhenPresent:   true,
		avoidWhenPresent: true,
	}
	raw, err = json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"use_when"`) || !strings.Contains(string(raw), `"avoid_when"`) {
		t.Fatalf("tool marshal = %s", raw)
	}

	absent := ProductMetadata{AgentSummary: "x"}
	raw, err = json.Marshal(absent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"use_when"`) {
		t.Fatalf("absent lists must omit use_when: %s", raw)
	}
}
