// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"time"
)

// aitableTaskMetrics counts CLI-to-MCP calls, not internal HSF attempts. All
// durations are inclusive components of elapsed time and must not be added to it.
type aitableTaskMetrics struct {
	Invocations   int   `json:"invocations"`
	RemoteCalls   int   `json:"remoteCalls"`
	ReadCalls     int   `json:"readCalls"`
	WriteCalls    int   `json:"writeCalls"`
	RequestMS     int64 `json:"requestMs"`
	BackoffMS     int64 `json:"backoffMs"`
	ResponseBytes int64 `json:"responseBytes"`
}

func (m *aitableTaskMetrics) recordCall(write bool, started time.Time, raw map[string]any) {
	m.RemoteCalls++
	if write {
		m.WriteCalls++
	} else {
		m.ReadCalls++
	}
	m.RequestMS += time.Since(started).Milliseconds()
	encoded, _ := json.Marshal(raw)
	m.ResponseBytes += int64(len(encoded))
}

// Legacy checkpoints contain calls but no invocation/backoff totals. Keep the
// available counters and mark their history partial rather than filling zeros.
func metricsFromPlanCalls(calls []buildPlanCall) aitableTaskMetrics {
	m := aitableTaskMetrics{}
	for _, call := range calls {
		m.RemoteCalls++
		if call.Phase == "write" {
			m.WriteCalls++
		} else {
			m.ReadCalls++
		}
		m.RequestMS += call.ElapsedMS
		m.ResponseBytes += int64(call.ResponseBytes)
	}
	return m
}
