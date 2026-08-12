// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package docsafety

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestDocSafetyModel(t *testing.T) {
	tests := []struct {
		name         string
		spec         contract.SafetySpec
		effect       string
		risk         string
		confirmation string
	}{
		{name: "recoverable", spec: RecoverableWrite("unknown"), effect: "write", risk: "medium", confirmation: "not_required"},
		{name: "sensitive", spec: SensitiveWrite("unknown"), effect: "write", risk: "medium", confirmation: "user_required"},
		{name: "protected delete", spec: ProtectedDelete("unknown"), effect: "destructive", risk: "high", confirmation: "user_required"},
	}

	for _, test := range tests {
		if test.spec.Effect != test.effect || test.spec.Risk != test.risk ||
			test.spec.Confirmation != test.confirmation || test.spec.Idempotency != "unknown" {
			t.Errorf("%s = %s/%s/%s/%s, want %s/%s/%s/unknown",
				test.name, test.spec.Effect, test.spec.Risk, test.spec.Confirmation, test.spec.Idempotency,
				test.effect, test.risk, test.confirmation)
		}
	}
}
