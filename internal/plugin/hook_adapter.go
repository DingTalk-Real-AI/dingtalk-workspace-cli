// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

const defaultHookTimeout = 30 * time.Second

// HookAdapter wraps a plugin hook entry as a pipeline.Handler.
type HookAdapter struct {
	pluginName string
	entry      HookEntry
	phase      pipeline.Phase
	timeout    time.Duration
}

// NewHookAdapter creates a pipeline handler from a plugin hook entry.
func NewHookAdapter(pluginName string, entry HookEntry) *HookAdapter {
	phase := parsePhase(entry.Phase)
	timeout := defaultHookTimeout
	if entry.Timeout > 0 {
		timeout = time.Duration(entry.Timeout) * time.Second
	}
	return &HookAdapter{
		pluginName: pluginName,
		entry:      entry,
		phase:      phase,
		timeout:    timeout,
	}
}

func (h *HookAdapter) Name() string {
	return fmt.Sprintf("plugin-hook:%s/%s", h.pluginName, h.entry.Phase)
}

func (h *HookAdapter) Phase() pipeline.Phase {
	return h.phase
}

func (h *HookAdapter) Handle(ctx *pipeline.Context) error {
	// Check matcher: if set, only run for matching commands.
	if h.entry.Matcher != "" {
		matched, err := filepath.Match(h.entry.Matcher, ctx.Command)
		if err != nil || !matched {
			return nil // skip silently
		}
	}

	// Serialize context to JSON for the hook's stdin.
	input, err := json.Marshal(map[string]any{
		"command": ctx.Command,
		"params":  ctx.Params,
		"args":    ctx.Args,
	})
	if err != nil {
		slog.Warn("plugin hook: failed to serialize context",
			"plugin", h.pluginName, "error", err)
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "sh", "-c", h.entry.Command)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.CombinedOutput()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code == 2 {
				// Exit 2 = abort pipeline.
				return fmt.Errorf("plugin hook %s/%s aborted: %s",
					h.pluginName, h.entry.Phase, strings.TrimSpace(string(output)))
			}
		}
		slog.Warn("plugin hook failed",
			"plugin", h.pluginName,
			"phase", h.entry.Phase,
			"error", err,
			"output", string(output),
		)
		return nil // non-fatal: log warning and continue
	}

	return nil
}

func parsePhase(s string) pipeline.Phase {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "pre-parse":
		return pipeline.PreParse
	case "post-parse":
		return pipeline.PostParse
	case "pre-request":
		return pipeline.PreRequest
	case "post-response":
		return pipeline.PostResponse
	default:
		return pipeline.PreRequest
	}
}
