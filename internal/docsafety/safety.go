// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package docsafety defines the shared safety model for Doc shortcuts and
// their equivalent leaf commands. Keeping these constructors in one package
// prevents the two command surfaces from assigning different confirmation
// semantics to the same operation.
package docsafety

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"

// RecoverableWrite covers ordinary document mutations whose result can be
// corrected or restored through a follow-up edit, version history, or another
// idempotent style update. These operations must not block on confirmation.
func RecoverableWrite(idempotency string) contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "write",
		Risk:         "medium",
		Confirmation: "not_required",
		Idempotency:  idempotency,
	}
}

// SensitiveWrite covers access-control changes and outward communication.
// Even when technically reversible, these operations can expose information
// or notify another person and therefore retain an explicit confirmation gate.
func SensitiveWrite(idempotency string) contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "write",
		Risk:         "medium",
		Confirmation: "user_required",
		Idempotency:  idempotency,
	}
}

// ProtectedDelete covers deletion or revocation operations that must retain an
// explicit confirmation boundary, including lifecycle deletes, comment
// deletes, and collaborator revocation.
func ProtectedDelete(idempotency string) contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "destructive",
		Risk:         "high",
		Confirmation: "user_required",
		Idempotency:  idempotency,
	}
}
