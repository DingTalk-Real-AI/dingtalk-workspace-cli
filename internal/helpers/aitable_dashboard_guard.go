// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// validateAitablePersistentMetadata rejects runtime-only layout metadata from
// persistent Dashboard/Chart payloads. These values are read evidence, not
// writable business configuration.
func validateAitablePersistentMetadata(flag string, value map[string]any) error {
	if err := aitableprotocol.ValidateDashboardPersistentMetadata(flag, value); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return nil
}
