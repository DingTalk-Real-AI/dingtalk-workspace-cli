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

package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Read loads all usage records from the log. Returns an empty slice when the log
// does not exist yet. Malformed lines are skipped.
func Read() ([]Record, error) {
	f, err := os.Open(LogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var recs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(line), &r) == nil {
			recs = append(recs, r)
		}
	}
	return recs, sc.Err()
}

// Group aggregates records that share the same (product, tool, arg_keys) shape —
// the unit a custom shortcut would capture.
type Group struct {
	Product  string   `json:"product"`
	Tool     string   `json:"tool"`
	ArgKeys  []string `json:"arg_keys"`
	Count    int      `json:"count"`
	OKCount  int      `json:"ok_count"`
	LastSeen string   `json:"last_seen"`
	// FixedArgs are argument keys whose sampled value was identical across every
	// occurrence — strong candidates to bake into a custom shortcut as constants.
	FixedArgs map[string]string `json:"fixed_args,omitempty"`
}

// Aggregate groups records and computes per-group counts and fixed-value args,
// sorted by descending count.
func Aggregate(recs []Record) []Group {
	type acc struct {
		g         Group
		valueSets map[string]map[string]struct{}
		samples   map[string]int
	}
	byKey := map[string]*acc{}
	for _, r := range recs {
		key := r.Product + "\x00" + r.Tool + "\x00" + strings.Join(r.ArgKeys, ",")
		a := byKey[key]
		if a == nil {
			a = &acc{
				g:         Group{Product: r.Product, Tool: r.Tool, ArgKeys: r.ArgKeys},
				valueSets: map[string]map[string]struct{}{},
				samples:   map[string]int{},
			}
			byKey[key] = a
		}
		a.g.Count++
		if r.OK {
			a.g.OKCount++
		}
		if r.TS > a.g.LastSeen {
			a.g.LastSeen = r.TS
		}
		for k, v := range r.SampleArgs {
			a.samples[k]++
			if a.valueSets[k] == nil {
				a.valueSets[k] = map[string]struct{}{}
			}
			a.valueSets[k][v] = struct{}{}
		}
	}

	out := make([]Group, 0, len(byKey))
	for _, a := range byKey {
		fixed := map[string]string{}
		for k, set := range a.valueSets {
			// A value is "fixed" only if it appeared in every occurrence and never varied.
			if a.samples[k] == a.g.Count && len(set) == 1 {
				for v := range set {
					fixed[k] = v
				}
			}
		}
		if len(fixed) > 0 {
			a.g.FixedArgs = fixed
		}
		out = append(out, a.g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// Purge deletes the usage log. Returns nil when there is nothing to delete.
func Purge() error {
	err := os.Remove(LogPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
