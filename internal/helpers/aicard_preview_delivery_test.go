// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package helpers

import "testing"

func TestCrossPlatformCoverageAicardPreviewDeliveryOptions(t *testing.T) {
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, tc := range []struct {
		name    string
		flags   []string
		summary string
		forward bool
	}{
		{name: "default", summary: "Card preview"},
		{name: "shareable", flags: []string{"--summary", "Weekly report", "--support-forward"}, summary: "Weekly report", forward: true},
		{name: "explicit false", flags: []string{"--summary", "Task result", "--support-forward=false"}, summary: "Task result"},
	} {
		for _, dryRun := range []bool{true, false} {
			mode := "send"
			if dryRun {
				mode = "dry run"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				caller := &aicardCaller{response: `{"success":true,"result":{"bizId":"preview-card"}}`}
				args := append([]string{"preview", "--file", file}, tc.flags...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				result, code, err := executeAicard(t, caller, args...)
				if err != nil || code != 0 {
					t.Fatalf("result=%v code=%d err=%v", result, code, err)
				}
				data := result["data"].(map[string]any)
				if data["summary"] != tc.summary || data["supportForward"] != tc.forward {
					t.Fatalf("delivery options=%v", data)
				}
				if dryRun {
					if len(caller.calls) != 0 {
						t.Fatalf("dry run called the service: %v", caller.calls)
					}
				} else if caller.sent["summary"] != tc.summary || caller.sent["supportForward"] != tc.forward {
					t.Fatalf("send differs from the reported options: %v", caller.sent)
				}
			})
		}
	}
	t.Run("invalid forwarding never calls the service", func(t *testing.T) {
		caller := &aicardCaller{}
		_, _, err := executeAicard(t, caller, "preview", "--file", file, "--support-forward=invalid")
		if err == nil || len(caller.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, caller.calls)
		}
	})
}
