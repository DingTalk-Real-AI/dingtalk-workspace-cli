// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"os"
)

var aitableWorkingDirectory = os.Getwd
var downloadAITableAttachment = localio.Download
