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
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/userdef"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

// runtimeDefaultCorpID is the runtimeDefault placeholder key for the current
// logged-in enterprise corpId. It aliases helpers.RuntimeDefaultCorpID, the
// single source of truth, so the registration side here and the read side in
// helpers.resolveCurrentCorpID always share one placeholder key.
const runtimeDefaultCorpID = helpers.RuntimeDefaultCorpID

// corpIDRuntimeDefaultOnce guards a single $corpId registration per process:
// RegisterRuntimeDefault panics on duplicate ids, and newLegacyPublicCommands
// may run more than once within a process (e.g. across test cases).
var corpIDRuntimeDefaultOnce sync.Once

// resolveProfileMetadata is the indirection for ResolveProfileMetadataReadOnly
// used by resolveCorpIDRuntimeDefault. Tests swap it to inject specific
// ProfileMetadata without touching the filesystem.
var resolveProfileMetadata = authpkg.ResolveProfileMetadataReadOnly

// mountLegacyPublicCommands builds the product + shortcut command tree without
// mutating process-global MCP deps or dynamic server endpoints. Used by the
// Schema source root (declaration-only) path so assembly cannot clobber a live
// runtime's InitDeps caller or plugin endpoints.
func mountLegacyPublicCommands(runner executor.Runner, loadUserShortcuts bool) []*cobra.Command {
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

// newLegacyPublicCommands is the executable CLI path: inject static MCP
// endpoints, InitDeps, then mount the public command tree.
func newLegacyPublicCommands(runner executor.Runner, caller edition.ToolCaller, loadUserShortcuts bool) []*cobra.Command {
	injectStaticServers()
	// Register the $corpId runtimeDefault before the command tree is built so
	// helpers.resolveCurrentCorpID (delegation-auth options for the legacy
	// permission format) can resolve the current enterprise at RunE time.
	ensureCorpIDRuntimeDefault()
	helpers.InitDeps(caller)
	return mountLegacyPublicCommands(runner, loadUserShortcuts)
}

// ensureCorpIDRuntimeDefault registers the $corpId resolver exactly once. The
// resolver is lazy (invoked at RunE time) and read-only: it never refreshes
// credentials or mutates local auth state, mirroring ResolveTelemetryIdentity.
// An existence guard tolerates a pre-registration (e.g. from a test) so the
// duplicate-id panic in RegisterRuntimeDefault cannot fire.
func ensureCorpIDRuntimeDefault() {
	corpIDRuntimeDefaultOnce.Do(func() {
		if _, exists := helpers.RuntimeDefaultsSnapshot()[runtimeDefaultCorpID]; exists {
			return
		}
		helpers.RegisterRuntimeDefault(runtimeDefaultCorpID, resolveCorpIDRuntimeDefault)
	})
}

// resolveCorpIDRuntimeDefault reads the current default profile's corpId from
// the non-sensitive profiles.json metadata. Resolution is best-effort:
// missing, invalid, or unreadable auth data returns ("", false) so callers
// gracefully fall back (the legacy permission format then omits targetMembers).
func resolveCorpIDRuntimeDefault(_ context.Context) (string, bool) {
	profile, err := resolveProfileMetadata(defaultConfigDir(), authpkg.RuntimeProfile())
	if err != nil || profile == nil {
		return "", false
	}
	corpID := strings.TrimSpace(profile.CorpID)
	if corpID == "" {
		return "", false
	}
	return corpID, true
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
