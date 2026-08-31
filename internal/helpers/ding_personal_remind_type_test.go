// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import "testing"

func TestCrossPlatformCoverageDingPersonalRemindTypeMatchesMCPEnum(t *testing.T) {
	for input, want := range map[string]string{"app": "APP", "sms": "SMS", "call": "PHONE", " APP ": "APP"} {
		got, err := dingPersonalRemindType(input)
		if err != nil || got != want {
			t.Errorf("dingPersonalRemindType(%q)=(%q,%v), want %q", input, got, err, want)
		}
	}
	if got, err := dingPersonalRemindType("push"); err == nil || got != "" {
		t.Fatalf("unsupported remind type=(%q,%v)", got, err)
	}
}
