// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxAgentJSONBytes = 256 * 1024

const (
	AgentUnknown   State = "unknown"
	AgentDegraded  State = "config_degraded"
	AgentInvalid   State = "parse_error"
	AgentOversized State = "oversized"
)

// AgentSnapshot is a process-scoped value, separate from the existing agentcode.
// The SDK object stays private; JSON returns a defensive copy for telemetry only.
type AgentSnapshot struct {
	State         State  `json:"state"`
	ErrorCode     int32  `json:"error_code"`
	ErrorCategory string `json:"error_category,omitempty"`
	AgentID       int64  `json:"agent_id"`
	OS            string `json:"os,omitempty"`
	ExecutionMode string `json:"execution_mode,omitempty"`
	raw           json.RawMessage
}

func (a AgentSnapshot) String() string {
	return fmt.Sprintf("agent collection state=%s code=%d", a.State, a.ErrorCode)
}
func (a AgentSnapshot) GoString() string      { return a.String() }
func (a AgentSnapshot) JSON() json.RawMessage { return append(json.RawMessage(nil), a.raw...) }
func (a AgentSnapshot) clone() AgentSnapshot  { a.raw = a.JSON(); return a }

var errAgentTooLarge = errors.New("agent result exceeds limit")

type agentCollector func(string, string) ([]byte, int32, error)
type agentProvider struct {
	once    sync.Once
	result  AgentSnapshot
	collect agentCollector
	locate  libraryLocator
	timeout time.Duration
}

var defaultAgentProvider = &agentProvider{collect: collectNativeAgent, locate: resolveLibraryPath, timeout: defaultTimeout}

// ResolveAgent collects full_info=0 in the original CLI process once. Timeout
// publishes a final snapshot; a late native result is released and discarded.
func ResolveAgent() AgentSnapshot { return defaultAgentProvider.resolve() }

func (p *agentProvider) resolve() AgentSnapshot {
	p.once.Do(func() {
		done := make(chan AgentSnapshot, 1)
		timer := time.NewTimer(p.timeout)
		defer timer.Stop()
		go func() {
			result := AgentSnapshot{State: StateError, ErrorCategory: "native_failed"}
			defer func() { _ = recover(); done <- result }()
			path, err := p.locate()
			if err != nil {
				result.State, result.ErrorCategory = StateUnavailable, classifyLocationError(err)
				return
			}
			raw, code, err := p.collect(path, filepath.Join(filepath.Dir(path), "skt.bin"))
			result = parseAgentResult(raw, code, err)
		}()
		select {
		case p.result = <-done:
		case <-timer.C:
			p.result = AgentSnapshot{State: StateTimeout, ErrorCategory: "collection_timeout"}
		}
	})
	return p.result.clone()
}

func parseAgentResult(raw []byte, code int32, collectErr error) AgentSnapshot {
	result := AgentSnapshot{State: StateError, ErrorCode: code}
	if errors.Is(collectErr, errAgentTooLarge) || len(raw) > MaxAgentJSONBytes {
		result.State, result.ErrorCategory = AgentOversized, "json_too_large"
		return result
	}
	if collectErr != nil {
		result.ErrorCategory = classifyStartError(collectErr)
		return result
	}
	if len(raw) == 0 {
		result.ErrorCategory = "empty_result"
		return result
	}
	var fields struct {
		Agent         *int64 `json:"agent"`
		MatchTier     string `json:"match_tier"`
		OS            string `json:"os"`
		ExecutionMode string `json:"execution_mode"`
	}
	if !utf8.Valid(raw) || json.Unmarshal(raw, &fields) != nil || fields.Agent == nil || *fields.Agent < 0 ||
		(fields.MatchTier != "high" && fields.MatchTier != "guess" && fields.MatchTier != "none") {
		result.State, result.ErrorCategory = AgentInvalid, "invalid_json"
		return result
	}
	result.raw = append(json.RawMessage(nil), raw...)
	switch code {
	case 0:
		result.State = StateReady
		if fields.MatchTier == "none" || *fields.Agent == 0 {
			result.State = AgentUnknown
		} else {
			result.AgentID = *fields.Agent
		}
	case 2002, 2003, 2016:
		result.State, result.ErrorCategory = AgentDegraded, "configuration_failed"
	default:
		result.ErrorCategory = "collection_failed"
		return result
	}
	result.OS, result.ExecutionMode = fields.OS, fields.ExecutionMode
	return result
}
