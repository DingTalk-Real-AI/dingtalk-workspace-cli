// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	filelock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type aiWaitCheckpoint struct {
	Version            int                `json:"version"`
	Kind               string             `json:"kind"`
	Identity           string             `json:"identity"`
	BaseID             string             `json:"baseId"`
	TableID            string             `json:"tableId"`
	Targets            []map[string]any   `json:"targets"`
	Progress           aiTaskEvidence     `json:"progress"`
	Metrics            aitableTaskMetrics `json:"metrics"`
	MetricsPartial     bool               `json:"metricsPartial"`
	InvocationComplete bool               `json:"invocationComplete"`
	ElapsedMS          int64              `json:"elapsedMs"`
}

// aiTaskEvidence persists per-row capabilities separately from aggregate flags.
// Mixed providers and an incomplete later read must not erase earlier evidence.
type aiTaskEvidence struct {
	Body    map[string]any             `json:"body"`
	Read    map[string]map[string]bool `json:"read"`
	Exact   map[string]map[string]bool `json:"exact"`
	Queried map[string]map[string]bool `json:"queried"`
}

func (p *aiTaskPoll) evidence() aiTaskEvidence {
	return aiTaskEvidence{Body: p.body, Read: p.read, Exact: p.exact, Queried: p.queried}
}

func restoreAITaskPoll(base, table string, targets []map[string]any, saved aiTaskEvidence) (*aiTaskPoll, error) {
	raw, err := json.Marshal(targets)
	if err != nil {
		return nil, err
	}
	if _, err := parseAITaskTargets(string(raw)); err != nil {
		return nil, err
	}
	if saved.Body["baseId"] != base || saved.Body["tableId"] != table {
		return nil, fmt.Errorf("AI 检查点目标不匹配")
	}
	if err := validateAITaskStatusReceipt(saved.Body, targets); err != nil {
		return nil, fmt.Errorf("AI 检查点行证据无效: %w", err)
	}
	p := newAITaskPoll(base, table, targets)
	tasks, _ := findNamedObjectList(saved.Body, "tasks")
	for _, task := range tasks {
		field := stringValue(task, "fieldId")
		rows, _ := findNamedObjectList(task, "rows")
		for _, row := range rows {
			id := stringValue(row, "recordId")
			for key, value := range row {
				p.rows[field][id][key] = value
			}
			p.read[field][id] = saved.Read[field][id]
			p.exact[field][id] = saved.Exact[field][id]
			p.queried[field][id] = saved.Queried[field][id]
		}
	}
	// Preserve the prior query budget diagnosis until refresh proves every
	// original row was read; reconstructing rows alone must not erase it.
	p.body["queryBudgetExhausted"] = saved.Body["queryBudgetExhausted"] == true
	p.refresh()
	return p, nil
}

// openAIWaitCheckpoint binds to the complete original target list. Existing
// files require explicit resume; holding the OS lock covers the entire wait.
func openAIWaitCheckpoint(rt *shortcut.RuntimeContext, targets []map[string]any) (*aiWaitCheckpoint, func() error, func(), error) {
	state := &aiWaitCheckpoint{Version: 1, Kind: "ai_wait", BaseID: rt.Str("base-id"), TableID: rt.Str("table-id"), Targets: targets}
	path := rt.Str("checkpoint")
	if path == "" {
		if rt.Bool("resume") {
			return nil, nil, nil, fmt.Errorf("resume 需要 checkpoint")
		}
		return state, func() error { return nil }, func() {}, nil
	}
	identity, err := buildPlanIdentity(rt)
	if err != nil {
		return nil, nil, nil, err
	}
	state.Identity = identity
	path, err = aitableTaskAbsPath(path)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, nil, err
	}
	lock, err := filelock.TryAcquire(path + ".lock")
	if err != nil {
		return nil, nil, nil, err
	}
	closeLock := func() { _ = lock.Close() }
	if rt.Bool("resume") {
		// Decode into a separate object. Unmarshal can reuse slices/maps and
		// otherwise overwrite the caller's targets before identity comparison.
		var loaded aiWaitCheckpoint
		readErr := readAitableTaskFile(path, &loaded)
		want, _ := json.Marshal(targets)
		got, _ := json.Marshal(loaded.Targets)
		if readErr != nil || loaded.Version != 1 || loaded.Kind != "ai_wait" || loaded.Identity != identity || loaded.BaseID != rt.Str("base-id") || loaded.TableID != rt.Str("table-id") || string(want) != string(got) {
			closeLock()
			return nil, nil, nil, fmt.Errorf("AI 检查点不可读或计划、身份、目标不匹配")
		}
		*state = loaded
		state.MetricsPartial = state.MetricsPartial || !state.InvocationComplete
	} else {
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			closeLock()
			return nil, nil, nil, fmt.Errorf("新等待拒绝覆盖已有检查点: %w", createErr)
		}
		_ = f.Close()
	}
	save := func() error {
		raw, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		return aitableTaskWriteJSON(path, append(raw, '\n'))
	}
	return state, save, closeLock, nil
}
