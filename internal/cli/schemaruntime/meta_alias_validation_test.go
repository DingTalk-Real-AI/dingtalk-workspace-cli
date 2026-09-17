// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"fmt"
	"math/rand"
	"testing"
)

// Keep the old expansion algorithm as an independent behavioral oracle. These
// cases include collisions, missing/extra aliases, damaged primaries and
// metadata disagreement; a faster validator must accept exactly the same set.
func TestCrossPlatformCoverageMetaAliasValidationMatchesExpansion(t *testing.T) {
	reference := func(lookup map[string]CommandMeta) bool {
		primary := make(map[string]CommandMeta)
		metas := []CommandMeta{}
		for _, meta := range lookup {
			path := meta.Identity.CLIPath
			if existing, ok := primary[path]; ok {
				if !equalCommandMeta(existing, meta) {
					return false
				}
				continue
			}
			primary[path] = meta
			metas = append(metas, meta)
		}
		return equalCommandMetaLookups(RegisterCommandMetaAliases(primary, metas), lookup)
	}
	check := func(lookup map[string]CommandMeta) {
		t.Helper()
		if got, want := validMetaAliasExpansion(lookup), reference(lookup); got != want {
			t.Fatalf("validation=%v expansion=%v lookup=%#v", got, want, lookup)
		}
	}
	check(nil)
	check(map[string]CommandMeta{})
	rng := rand.New(rand.NewSource(20260906))
	for iteration := 0; iteration < 200; iteration++ {
		primary := map[string]CommandMeta{}
		metas := []CommandMeta{}
		for i := 0; i < 12; i++ {
			path := fmt.Sprintf("sample command %02d", i)
			meta := CommandMeta{Identity: CommandIdentity{CLIPath: path, Canonical: fmt.Sprintf("sample.%d", i), ProductID: "sample"}}
			for j := 0; j < 4; j++ {
				alias := fmt.Sprintf("sample command %02d", rng.Intn(18))
				if rng.Intn(3) == 0 {
					alias = " " + alias + " "
				}
				meta.Identity.Aliases = append(meta.Identity.Aliases, alias)
			}
			primary[path] = meta
			metas = append(metas, meta)
		}
		expanded := RegisterCommandMetaAliases(primary, metas)
		check(expanded)
		for mutation := 0; mutation < 6; mutation++ {
			lookup := make(map[string]CommandMeta, len(expanded))
			for path, meta := range expanded {
				lookup[path] = meta
			}
			path := fmt.Sprintf("sample command %02d", rng.Intn(18))
			meta, found := lookup[path]
			switch mutation {
			case 0:
				delete(lookup, path)
			case 1:
				lookup["unrelated extra key"] = metas[0]
			case 2:
				if found {
					meta.Identity.Title = "changed"
					lookup[path] = meta
				}
			case 3:
				if found {
					meta.Identity.CLIPath = "missing primary"
					lookup[path] = meta
				}
			case 4:
				if found {
					meta.Identity.Aliases = nil
					lookup[path] = meta
				}
			case 5:
				if found {
					meta.Selection.Examples = []string{}
					lookup[path] = meta
				}
			}
			check(lookup)
		}
	}
}
