// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cli

import "sync"

var (
	commandPathFallbackIndexOnce sync.Once
	commandPathFallbackIndex     map[string]CommandPathFallback
)

func buildCommandPathFallbackIndex() {
	entries := loadGeneratedCommandPathFallbacks()
	commandPathFallbackIndex = make(map[string]CommandPathFallback, len(entries))
	for _, entry := range entries {
		commandPathFallbackIndex[entry.From] = entry
	}
}

// LookupCommandPathFallback performs an exact O(1) lookup of a reviewed
// recovery-only path. Normalization only removes a leading dws and folds
// whitespace; it does not apply prefix, typo, or semantic matching.
func LookupCommandPathFallback(rawPath string) (CommandPathFallback, bool) {
	commandPathFallbackIndexOnce.Do(buildCommandPathFallbackIndex)
	entry, ok := commandPathFallbackIndex[normalizeSchemaCLIPath(rawPath)]
	if !ok {
		return CommandPathFallback{}, false
	}
	entry.Candidates = append([]string(nil), entry.Candidates...)
	return entry, true
}
