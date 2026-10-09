// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func contractJSON(v any) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

// ExplainMany returns an optional lossless bundle and propagates store errors.
func (p *Protocol) ExplainMany(names []string, compact bool) (map[string]any, error) {
	store, err := BundledExplain()
	if err != nil {
		return nil, err
	}
	return store.LookupMany(names, compact)
}

func bundleContracts(contracts []any, compact bool) map[string]any {
	result := map[string]any{"kind": "bundle", "formatVersion": 1, "contracts": contracts}
	if !compact {
		return result
	}
	counts := map[string]int{}
	var count func(any)
	count = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			key := contractJSON(n)
			if len(key) >= 256 {
				counts[key]++
			}
			for _, c := range n {
				count(c)
			}
		case []any:
			for _, c := range n {
				count(c)
			}
		}
	}
	count(contracts)
	defs := map[string]any{}
	var pack func(any, bool) any
	pack = func(v any, replace bool) any {
		switch n := v.(type) {
		case map[string]any:
			key := contractJSON(n)
			if replace && counts[key] > 1 {
				hash := sha256.Sum256([]byte(key))
				id := fmt.Sprintf("%x", hash)[:16]
				if _, ok := defs[id]; !ok {
					defs[id] = pack(n, false)
				}
				return map[string]any{"$contractRef": id}
			}
			out := map[string]any{}
			for k, c := range n {
				out[k] = pack(c, true)
			}
			return out
		case []any:
			out := []any{}
			for _, c := range n {
				out = append(out, pack(c, true))
			}
			return out
		default:
			return v
		}
	}
	result["contracts"] = pack(contracts, true)
	result["definitions"] = defs
	return result
}
