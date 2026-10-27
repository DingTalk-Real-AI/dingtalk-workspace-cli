// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

// aiTaskPoll retains evidence for the original fixed targets while each MCP
// call queries at most one field and 50 records. A slow/failed later chunk must
// not erase earlier rows or make every retry start at the first 50 records.
type aiTaskPoll struct {
	body    map[string]any
	rows    map[string]map[string]map[string]any
	read    map[string]map[string]bool
	exact   map[string]map[string]bool
	queried map[string]map[string]bool
}

// newAITaskPoll creates explicit unknown rows for targets not queried yet.
func newAITaskPoll(base, table string, targets []map[string]any) *aiTaskPoll {
	p := &aiTaskPoll{body: map[string]any{"baseId": base, "tableId": table, "resultReadableVerified": false},
		rows: map[string]map[string]map[string]any{}, read: map[string]map[string]bool{}, exact: map[string]map[string]bool{}, queried: map[string]map[string]bool{}}
	tasks := make([]any, 0, len(targets))
	for _, target := range targets {
		field := stringValue(target, "fieldId")
		p.rows[field], p.read[field], p.exact[field] = map[string]map[string]any{}, map[string]bool{}, map[string]bool{}
		p.queried[field] = map[string]bool{}
		rows := []any{}
		for _, value := range target["recordIds"].([]any) {
			id := value.(string)
			row := map[string]any{"taskId": target["taskId"], "fieldId": field, "recordId": id, "state": "unknown", "terminal": false}
			p.rows[field][id] = row
			rows = append(rows, row)
		}
		tasks = append(tasks, map[string]any{"taskId": target["taskId"], "fieldId": field, "recordIds": target["recordIds"], "rows": rows, "terminalVerified": false})
	}
	p.body["tasks"] = tasks
	p.refresh()
	return p
}

// batches visits every pending row once per round, with untouched rows first.
// Queried tracks this round only: after a complete round it resets without
// erasing row evidence. This also makes repeated 0s checkpoint resumes fair.
func (p *aiTaskPoll) batches(targets []map[string]any) [][]map[string]any {
	unread := false
	for _, target := range targets {
		field := stringValue(target, "fieldId")
		for _, value := range target["recordIds"].([]any) {
			id := value.(string)
			unread = unread || (p.rows[field][id]["terminal"] != true && !p.queried[field][id])
		}
	}
	if !unread {
		for _, queried := range p.queried {
			clear(queried)
		}
	}
	var batches [][]map[string]any
	// The phase is outside the field loop so a long-running first field does
	// not starve another field whose rows have not been visited this round.
	for _, read := range []bool{false, true} {
		for _, target := range targets {
			field := stringValue(target, "fieldId")
			ids := []any{}
			for _, value := range target["recordIds"].([]any) {
				id := value.(string)
				if p.rows[field][id]["terminal"] != true && p.queried[field][id] == read {
					ids = append(ids, id)
				}
			}
			for offset := 0; offset < len(ids); offset += 50 {
				batch := map[string]any{"taskId": target["taskId"], "fieldId": field,
					"recordIds": append([]any(nil), ids[offset:min(offset+50, len(ids))]...)}
				batches = append(batches, []map[string]any{batch})
			}
		}
	}
	return batches
}

// queriedField checks values, since restored checkpoints also contain explicit
// false entries for untouched rows; map length is not evidence of a query.
func (p *aiTaskPoll) queriedField(field string) bool {
	for _, queried := range p.queried[field] {
		if queried {
			return true
		}
	}
	return false
}

// merge accepts a validated response only, preserving known rows when a later
// query can only report unknown. Capability flags never erase per-row evidence.
func (p *aiTaskPoll) merge(body map[string]any) {
	tasks, _ := findNamedObjectList(body, "tasks")
	for _, task := range tasks {
		field := stringValue(task, "fieldId")
		rows, _ := findNamedObjectList(task, "rows")
		for _, row := range rows {
			id := stringValue(row, "recordId")
			p.queried[field][id] = true
			p.read[field][id] = body["queryComplete"] != false
			p.exact[field][id] = body["successTerminalVerifiable"] == true
			old := p.rows[field][id]
			if row["state"] != "unknown" || old["state"] == "unknown" {
				for key := range old {
					delete(old, key)
				}
				for key, value := range row {
					old[key] = value
				}
			}
		}
	}
	p.body["queryBudgetExhausted"] = p.body["queryBudgetExhausted"] == true || body["queryBudgetExhausted"] == true
	p.refresh()
}

// refresh derives aggregate facts from all original targets, not just the last
// 50-row response. Query completeness does not establish a task terminal state.
func (p *aiTaskPoll) refresh() {
	terminal, complete, exact, version := true, true, true, false
	tasks, _ := findNamedObjectList(p.body, "tasks")
	for _, task := range tasks {
		field := stringValue(task, "fieldId")
		finished := true
		for id, row := range p.rows[field] {
			finished = finished && row["terminal"] == true
			complete = complete && p.read[field][id]
			exact = exact && p.exact[field][id]
			version = version || row["resultVersion"] != nil
		}
		task["terminalVerified"] = finished
		terminal = terminal && finished
	}
	p.body["terminalVerified"], p.body["successTerminalVerifiable"] = terminal, exact
	p.body["queryComplete"], p.body["queryRetryable"] = complete, !complete
	p.body["resultVersionAvailable"] = version
	if complete || p.body["queryBudgetExhausted"] == nil {
		p.body["queryBudgetExhausted"] = false
	}
}
