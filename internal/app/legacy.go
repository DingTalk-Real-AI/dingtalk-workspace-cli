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

package app

import (
	"log/slog"
	"sort"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/userdef"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func newLegacyPublicCommands(runner executor.Runner, caller edition.ToolCaller, loadUserShortcuts bool) []*cobra.Command {
	injectStaticServers()
	helpers.InitDeps(caller)
	commands := helpers.NewPublicCommands(runner)
	// Load user-defined shortcuts (~/.dws/shortcuts/*.yaml) BEFORE compiling the
	// command tree, so distilled high-frequency operations mount alongside the
	// built-ins. Conflicts with built-ins are skipped inside Load.
	if loadUserShortcuts {
		if _, err := userdef.Load(); err != nil {
			slog.Warn("shortcut: failed to load user-defined shortcuts", "error", err)
		}
	}
	// Built-in + user shortcuts (`dws <service> +<command>`) share the same
	// command tree; mergeTopLevelCommands folds each shortcut's service parent
	// into the matching helper command so the `+leaf` sits alongside existing
	// subcommands.
	if loadUserShortcuts {
		commands = append(commands, builtin.Commands()...)
	} else {
		commands = append(commands, builtin.BaseCommands()...)
	}
	return mergeTopLevelCommands(commands)
}

func injectStaticServers() {
	hooks := edition.Get()
	var servers []edition.ServerInfo

	if fn := hooks.StaticServers; fn != nil {
		servers = append(servers, fn()...)
	}
	if fn := hooks.SupplementServers; fn != nil {
		servers = append(servers, fn()...)
	}

	if len(servers) == 0 {
		return
	}

	descriptors := make([]mcptypes.ServerDescriptor, 0, len(servers))
	for _, s := range servers {
		descriptors = append(descriptors, mcptypes.ServerDescriptor{
			Key:         s.ID,
			DisplayName: s.Name,
			Endpoint:    s.Endpoint,
			CLI: mcptypes.CLIOverlay{
				ID:       s.ID,
				Command:  s.ID,
				Prefixes: s.Prefixes,
			},
		})
	}
	SetDynamicServers(descriptors)
}

func newLegacyHiddenCommands(_ executor.Runner) []*cobra.Command {
	return nil
}

func mergeTopLevelCommands(commands []*cobra.Command) []*cobra.Command {
	byName := make(map[string]*cobra.Command, len(commands))
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		name := cmd.Name()
		if name == "" {
			continue
		}
		if existing, ok := byName[name]; ok {
			cobracmd.MergeCommandTree(existing, cmd)
			continue
		}
		byName[name] = cmd
	}

	out := make([]*cobra.Command, 0, len(byName))
	for _, cmd := range byName {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Use < out[j].Use
	})
	return out
}
