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
	"strconv"

	"github.com/spf13/cobra"
)

// allShortcuts is the registry of built-in shortcuts. Service packages append to
// it via Register from their init(), keeping this package free of import cycles
// on the concrete command definitions.
var allShortcuts []Shortcut

// Register adds one or more shortcuts to the built-in registry. Call from a
// service package's init().
func Register(shortcuts ...Shortcut) {
	for i := range shortcuts {
		shortcuts[i] = applyPublicCatalog(shortcuts[i])
	}
	allShortcuts = append(allShortcuts, shortcuts...)
}

// Commands compiles all registered shortcuts into a slice of top-level cobra
// commands, one per service, each carrying its `+command` leaves. The result is
// merged into the root command tree by the host application.
func Commands() []*cobra.Command {
	return build(allShortcuts)
}

// All returns the registered shortcuts. Primarily for coverage tests that need
// each shortcut's declared flags (types/enums/required) to synthesize inputs.
func All() []Shortcut {
	out := make([]Shortcut, len(allShortcuts))
	copy(out, allShortcuts)
	return out
}

// build groups shortcuts by service and mounts each as a leaf under its service
// parent command.
func build(shortcuts []Shortcut) []*cobra.Command {
	byService := make(map[string]*cobra.Command)
	var order []string

	for _, s := range shortcuts {
		parent, ok := byService[s.Service]
		if !ok {
			parent = &cobra.Command{
				Use:   s.Service,
				Short: s.Service + " shortcuts",
			}
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

// atoiDefault parses s as an int, returning 0 when empty or malformed.
func atoiDefault(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// contains reports whether v is present in list.
func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
