// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

// ValidateOpenNodesPresentationOrder keeps the OpenNodes V1 frame-only
// presentation order contract consistent across native and Shortcut writes.
func ValidateOpenNodesPresentationOrder(nodes []map[string]any) error {
	if err := opennodes.ValidatePresentationOrder(nodes); err != nil {
		return apperrors.NewValidation(err.Error(), apperrors.WithCause(err))
	}
	return nil
}
