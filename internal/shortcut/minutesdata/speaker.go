// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import "strings"

type SpeakerState string

const (
	SpeakerPending     SpeakerState = "pending"
	SpeakerReady       SpeakerState = "ready"
	SpeakerFailed      SpeakerState = "failed"
	SpeakerUnsupported SpeakerState = "unsupported_shape"
)

// SpeakerSummary separates transport acknowledgement from reviewed job evidence.
type SpeakerSummary struct {
	State  SpeakerState
	Status string
	TaskID string
	Reason string
	Result map[string]any
}

// ParseSpeakerSummary accepts the observed completed/Finished/content shape,
// not arbitrary nonempty objects or arrays. Creation status is never consulted.
func ParseSpeakerSummary(data map[string]any) SpeakerSummary {
	s := SpeakerSummary{State: SpeakerUnsupported, Reason: "unrecognized_result"}
	if err := validateEnvelope(data); err != nil {
		s.Reason = "invalid_envelope"
		return s
	}
	if v, exists := data["success"]; exists && v != true {
		s.Reason = "invalid_envelope"
		return s
	}
	r, ok := data["result"].(map[string]any)
	if !ok {
		return s
	}
	s.TaskID, _ = r["taskId"].(string)
	s.Status, _ = r["status"].(string)
	inner, _ := r["innerStatus"].(string)
	content, contentOK := r["content"].(string)
	message, messageOK := r["errorMsg"].(string)
	if success, ok := r["success"].(bool); ok && !success {
		s.State, s.Reason = SpeakerFailed, "explicit_failure"
		return s
	}
	if s.Status == "processing" {
		if inner == "Finished" || (messageOK && strings.TrimSpace(message) != "") {
			s.Reason = "conflicting_status"
			return s
		}
		s.State, s.Reason = SpeakerPending, "processing"
		return s
	}
	if s.Status == "completed" && inner == "Finished" && r["success"] == true &&
		contentOK && strings.TrimSpace(content) != "" && messageOK && message == "" && strings.TrimSpace(s.TaskID) != "" {
		s.State, s.Reason, s.Result = SpeakerReady, "completed_with_content", r
	}
	return s
}
