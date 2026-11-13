// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import "fmt"

// verifyPlanRecordCandidates requires each accepted ID exactly once, then finds
// a one-to-one value match. Receipt IDs are unordered; same-valued records remain
// distinct, and overlapping partial cell assertions may require reassignment.
func verifyPlanRecordCandidates(actual, declared []map[string]any, candidateIDs []string) error {
	allowed := map[string]bool{}
	for _, id := range candidateIDs {
		allowed[id] = true
	}
	byID := map[string]map[string]any{}
	for _, row := range actual {
		id := stringValue(row, "recordId")
		if !allowed[id] {
			continue
		}
		if byID[id] != nil {
			return fmt.Errorf("候选记录 ID 回读重复，不能标记 verified")
		}
		byID[id] = row
	}
	if len(byID) != len(candidateIDs) {
		return fmt.Errorf("候选记录 ID 尚未完整回读，不能凭同值旧记录确认创建")
	}
	owner := make([]int, len(candidateIDs))
	for i := range owner {
		owner[i] = -1
	}
	var match func(int, []bool) bool
	match = func(input int, seen []bool) bool {
		want := declared[input]
		cells, _ := want["cells"].(map[string]any)
		for i, id := range candidateIDs {
			if seen[i] || (stringValue(want, "recordId") != "" && stringValue(want, "recordId") != id) {
				continue
			}
			values, _ := byID[id]["cells"].(map[string]any)
			if !mapContains(values, cells) {
				continue
			}
			seen[i] = true
			if owner[i] == -1 || match(owner[i], seen) {
				owner[i] = input
				return true
			}
		}
		return false
	}
	for i, record := range declared {
		cells, _ := record["cells"].(map[string]any)
		if len(cells) == 0 {
			return fmt.Errorf("空记录没有可验证业务键，请单独通过记录对账核验")
		}
		if !match(i, make([]bool, len(candidateIDs))) {
			return fmt.Errorf("记录值或真实 ID 无法一一匹配，不能标记 verified")
		}
	}
	return nil
}
