// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type recordReadbackMismatchError struct {
	fieldID string
}

func (e *recordReadbackMismatchError) Error() string {
	return fmt.Sprintf("read-back mismatch for field %s: requested value is unconfirmed", e.fieldID)
}

// A mismatch is an observation, not proof that a submitted write failed.
// Never copy upstream error prose or attachment values into recovery metadata.
func addRecordReadbackDiagnostics(target map[string]any, err error) {
	var mismatch *recordReadbackMismatchError
	if errors.As(err, &mismatch) {
		copyRecordReadbackDiagnostics(target, "mismatch", mismatch.fieldID)
	} else if isPendingRecordReadback(err) {
		copyRecordReadbackDiagnostics(target, "pending", "")
	} else {
		copyRecordReadbackDiagnostics(target, "unavailable", "")
	}
}

func copyRecordReadbackDiagnostics(target map[string]any, kind, fieldID string) {
	switch kind {
	case "pending", "mismatch", "unavailable":
		target["readback"] = kind
	default:
		return
	}
	if kind == "mismatch" && strings.TrimSpace(fieldID) != "" && len(fieldID) <= 256 && strings.IndexFunc(fieldID, unicode.IsControl) < 0 {
		target["mismatchedFieldId"] = fieldID
	}
}
