// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package logging

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageNestedEmployeeArgumentsAreRedacted(t *testing.T) {
	input := map[string]any{"nested": [2]any{map[string]string{"dwsAuthCode": "never-log-code", "name": "employee"}, nil}, "configString": "never-log-config", "safe": map[int]string{1: "value"}}
	redactMapValues(input)
	raw, err := json.Marshal(input)
	if err != nil || strings.Contains(string(raw), "never-log") || !strings.Contains(string(raw), "employee") {
		t.Fatalf("redaction=%s err=%v", raw, err)
	}
	if !reflect.DeepEqual(input["safe"], map[int]string{1: "value"}) {
		t.Fatal("non-string keyed map changed")
	}
	if got := SanitizeArguments(map[string]any{"unsupported": make(chan int)}, 1000); got != "{}" {
		t.Fatalf("marshal failure=%s", got)
	}
}
