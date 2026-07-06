package helpers

import (
	"context"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type captureRunner struct {
	last executor.Invocation
}

func (r *captureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}
