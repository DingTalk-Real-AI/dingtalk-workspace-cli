// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

// CardFlowStatus is one reviewed state accepted by DingTalk's streaming-card
// update transport.
type CardFlowStatus struct {
	Value int
	Name  string
}

// CardWorkflowContract describes the real card subset exposed by the current
// Runtime. It intentionally does not claim Lark card JSON/component compilation
// or callback consumption, neither of which exists in the lower interface.
type CardWorkflowContract struct {
	Version           string
	Targets           []string
	ContentTypes      []string
	FlowStatuses      []CardFlowStatus
	CallbackSupported bool
}

var currentCardWorkflowContract = CardWorkflowContract{
	Version:      "im.streaming-card.v1",
	Targets:      []string{"group", "direct-user", "direct-open-dingtalk-id"},
	ContentTypes: []string{"streaming-text"},
	FlowStatuses: []CardFlowStatus{
		{Value: 1, Name: "processing"},
		{Value: 2, Name: "typing"},
		{Value: 3, Name: "completed"},
		{Value: 4, Name: "executing"},
		{Value: 5, Name: "error"},
	},
	CallbackSupported: false,
}

// CurrentCardWorkflowContract returns a defensive copy for policy and docs.
func CurrentCardWorkflowContract() CardWorkflowContract {
	contract := currentCardWorkflowContract
	contract.Targets = append([]string(nil), contract.Targets...)
	contract.ContentTypes = append([]string(nil), contract.ContentTypes...)
	contract.FlowStatuses = append([]CardFlowStatus(nil), contract.FlowStatuses...)
	return contract
}

func validCardFlowStatus(value int) bool {
	for _, status := range currentCardWorkflowContract.FlowStatuses {
		if status.Value == value {
			return true
		}
	}
	return false
}

// IMCapabilityBoundary makes unsupported Lark-parity requests explicit and
// testable instead of leaving them to prose or transport guessing.
type IMCapabilityBoundary struct {
	Capability  string
	Supported   bool
	Alternative string
}

var currentIMCapabilityBoundaries = []IMCapabilityBoundary{
	{Capability: "thread-write", Supported: false, Alternative: "quote reply with +messages-reply; thread reading with +thread-replies"},
	{Capability: "bot-rich-media", Supported: false, Alternative: "bot text/markdown, or current-user file/image send"},
	{Capability: "card-action-callback", Supported: false, Alternative: "streaming text card create/update only"},
	{Capability: "resource-resume", Supported: false, Alternative: "atomic whole-file download with explicit retry"},
	{Capability: "group-member-full-pagination", Supported: true, Alternative: "+chat-members-list or +group-members"},
	{Capability: "group-owner-selection", Supported: true, Alternative: "+chat-create owner flags"},
}

// CurrentIMCapabilityBoundaries returns the reviewed positive/negative matrix.
func CurrentIMCapabilityBoundaries() []IMCapabilityBoundary {
	return append([]IMCapabilityBoundary(nil), currentIMCapabilityBoundaries...)
}
