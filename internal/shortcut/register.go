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

package shortcut

import (
	"sort"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/spf13/cobra"
)

// allShortcuts is the registry of built-in shortcuts. Service packages append to
// it via Register from their init(), keeping this package free of import cycles
// on the concrete command definitions.
var (
	shortcutRegistryMu        sync.RWMutex
	allShortcuts              []Shortcut
	shortcutsByService        = make(map[string][]Shortcut)
	builtInShortcutsByService = make(map[string][]Shortcut)
)

// Register adds one or more shortcuts to the built-in registry. Call from a
// service package's init().
func Register(shortcuts ...Shortcut) {
	shortcutRegistryMu.Lock()
	defer shortcutRegistryMu.Unlock()
	for i := range shortcuts {
		shortcuts[i] = applyPublicCatalog(shortcuts[i])
		registered := shortcuts[i]
		shortcutsByService[registered.Service] = append(shortcutsByService[registered.Service], registered)
		if !registered.UserDefined {
			builtInShortcutsByService[registered.Service] = append(builtInShortcutsByService[registered.Service], registered)
		}
	}
	allShortcuts = append(allShortcuts, shortcuts...)
}

// Commands compiles all registered shortcuts into a slice of top-level cobra
// commands, one per service, each carrying its `+command` leaves. The result is
// merged into the root command tree by the host application.
func Commands() []*cobra.Command {
	shortcutRegistryMu.RLock()
	defer shortcutRegistryMu.RUnlock()
	return build(allShortcuts)
}

// CommandsForService compiles only one top-level service from the same
// registered shortcut declarations used by Commands.
func CommandsForService(service string) []*cobra.Command {
	shortcutRegistryMu.RLock()
	defer shortcutRegistryMu.RUnlock()
	return build(shortcutsByService[service])
}

func HasService(service string) bool {
	shortcutRegistryMu.RLock()
	_, ok := shortcutsByService[service]
	shortcutRegistryMu.RUnlock()
	return ok
}

// BuiltInCommands compiles only distribution-owned shortcuts.
func BuiltInCommands() []*cobra.Command {
	shortcutRegistryMu.RLock()
	defer shortcutRegistryMu.RUnlock()
	return build(filterService(allShortcuts, "", true))
}

// BuiltInCommandsForService is the declaration-only counterpart of
// CommandsForService.
func BuiltInCommandsForService(service string) []*cobra.Command {
	shortcutRegistryMu.RLock()
	defer shortcutRegistryMu.RUnlock()
	return build(builtInShortcutsByService[service])
}

func shortcutsSnapshot() []Shortcut {
	shortcutRegistryMu.RLock()
	defer shortcutRegistryMu.RUnlock()
	return append([]Shortcut(nil), allShortcuts...)
}

func filterService(shortcuts []Shortcut, service string, builtInOnly bool) []Shortcut {
	filtered := make([]Shortcut, 0, len(shortcuts))
	for _, registered := range shortcuts {
		if builtInOnly && registered.UserDefined {
			continue
		}
		if service != "" && registered.Service != service {
			continue
		}
		filtered = append(filtered, registered)
	}
	return filtered
}

// All returns the registered shortcuts. Primarily for coverage tests that need
// each shortcut's declared flags (types/enums/required) to synthesize inputs.
func All() []Shortcut {
	return shortcutsSnapshot()
}

// build groups shortcuts by service and mounts each as a leaf under its service
// parent command.
func build(shortcuts []Shortcut) []*cobra.Command {
	byService := make(map[string]*cobra.Command)
	var order []string

	for _, s := range shortcuts {
		parent, ok := byService[s.Service]
		if !ok {
			parent = cobracmd.NewGroupCommand(s.Service, s.Service+" shortcuts")
			byService[s.Service] = parent
			order = append(order, s.Service)
		}
		parent.AddCommand(mount(s))
	}

	sort.Strings(order)
	out := make([]*cobra.Command, 0, len(order))
	for _, svc := range order {
		out = append(out, byService[svc])
	}
	return out
}
