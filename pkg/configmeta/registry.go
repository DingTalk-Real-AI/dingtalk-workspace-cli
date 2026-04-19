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

// Package configmeta provides a central registry of all environment-variable
// based configuration items used by the DWS CLI.  Each package registers its
// own items via init(), and the "dws config list" command reads the registry
// to present a unified view to the developer.
package configmeta

import (
	"os"
	"sort"
	"strings"
	"sync"
)

// Category groups related configuration items for display purposes.
type Category string

const (
	CategoryCore     Category = "core"
	CategoryAuth     Category = "auth"
	CategoryNetwork  Category = "network"
	CategorySecurity Category = "security"
	CategoryRuntime  Category = "runtime"
	CategoryDebug    Category = "debug"
	CategoryExternal Category = "external"
)

// categoryOrder defines the display order for categories.
var categoryOrder = map[Category]int{
	CategoryCore:     0,
	CategoryAuth:     1,
	CategoryNetwork:  2,
	CategorySecurity: 3,
	CategoryRuntime:  4,
	CategoryDebug:    5,
	CategoryExternal: 6,
}

// ConfigItem describes a single environment-variable configuration item.
type ConfigItem struct {
	Name         string   // Environment variable name, e.g. "DWS_CONFIG_DIR"
	Category     Category // Logical grouping
	Description  string   // Short human-readable description
	DefaultValue string   // Description of the default value
	Example      string   // Example value for documentation
	Sensitive    bool     // If true, actual value is masked in output
	Hidden       bool     // If true, omitted from default list output
}

var (
	mu    sync.RWMutex
	items []ConfigItem
)

// Register adds a configuration item to the global registry.
// Duplicate names are silently ignored (first registration wins).
func Register(item ConfigItem) {
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range items {
		if existing.Name == item.Name {
			return
		}
	}
	items = append(items, item)
}

// All returns every registered configuration item sorted by category
// (display order) then by name.
func All() []ConfigItem {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ConfigItem, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool {
		ci, cj := categoryOrder[out[i].Category], categoryOrder[out[j].Category]
		if ci != cj {
			return ci < cj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ByCategory returns registered items that match the given category.
func ByCategory(cat Category) []ConfigItem {
	all := All()
	var out []ConfigItem
	for _, item := range all {
		if item.Category == cat {
			out = append(out, item)
		}
	}
	return out
}

// Resolve returns the current value of the named environment variable.
// For sensitive items the value is masked.  Returns ("", false) when the
// variable is not set.
func Resolve(name string) (string, bool) {
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}

	mu.RLock()
	defer mu.RUnlock()
	for _, item := range items {
		if item.Name == name && item.Sensitive {
			return maskValue(val), true
		}
	}
	return val, true
}

// Categories returns all known category values in display order.
func Categories() []Category {
	cats := make([]Category, 0, len(categoryOrder))
	for c := range categoryOrder {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool {
		return categoryOrder[cats[i]] < categoryOrder[cats[j]]
	})
	return cats
}

func maskValue(v string) string {
	if len(v) == 0 {
		return ""
	}
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

// Reset clears the registry. Intended for testing only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	items = nil
}
