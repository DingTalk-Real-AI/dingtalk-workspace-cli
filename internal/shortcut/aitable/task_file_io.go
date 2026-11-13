// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/atomicfile"
)

// Keep path resolution and durable publication injectable so failure tests can
// prove that a checkpoint error stops dispatch on every supported platform.
var aitableTaskAbsPath = filepath.Abs
var aitableTaskWriteJSON = atomicfile.WriteJSON
