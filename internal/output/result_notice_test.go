// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageResultNoticeSnapshot(t *testing.T) {
	notice := map[string]any{"update": map[string]any{"latest": "1.0.63", "command": "dws upgrade"}}
	result := FailureWithExitCode(&ErrorInfo{Message: "original failure"}, 3, WithNotice(notice))
	notice["update"].(map[string]any)["latest"] = "mutated"
	first := result.envelope()
	first.Notice.(map[string]any)["update"].(map[string]any)["command"] = "mutated"
	payload, err := json.Marshal(result.envelope())
	if err != nil || !strings.Contains(string(payload), `"latest":"1.0.63"`) || !strings.Contains(string(payload), `"command":"dws upgrade"`) || strings.Contains(string(payload), "mutated") {
		t.Fatalf("notice did not retain its immutable snapshot: %s, %v", payload, err)
	}
	if result.Outcome() != OutcomeFailure || result.ExitCode() != 3 || !strings.Contains(string(payload), "original failure") {
		t.Fatalf("notice changed the original error: %s", payload)
	}
}
