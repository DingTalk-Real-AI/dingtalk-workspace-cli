package app

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// captureRuntimeFailure previously persisted a recovery snapshot for
// `dws recovery`. The recovery package is gone; keep a no-op seam so runner
// failure paths stay stable while the visible Deprecated shim remains.
func captureRuntimeFailure(_ executor.Invocation, _, _ error) {}
