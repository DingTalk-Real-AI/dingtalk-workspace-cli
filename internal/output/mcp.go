// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"encoding/json"
	"fmt"
)

var (
	marshalMCPResult   = json.Marshal
	unmarshalMCPResult = json.Unmarshal
)

// MCPResult is the protocol-neutral projection consumed by MCP transports.
// Both CLI and MCP adapters start from the same immutable CommandResult; MCP
// handlers must not rebuild a second business envelope.
type MCPResult struct {
	StructuredContent map[string]any `json:"structuredContent"`
	IsError           bool           `json:"isError,omitempty"`
}

// AdaptMCP projects the exact unified envelope into MCP structuredContent. Failure
// and partial failure set isError, while pending remains a successfully
// accepted request whose terminal operation state is carried in the envelope.
func AdaptMCP(result CommandResult) (*MCPResult, error) {
	env, err := EnvelopeFromResult(result)
	if err != nil {
		return nil, err
	}
	env = redactEnvelope(env)
	raw, err := marshalMCPResult(env)
	if err != nil {
		return nil, fmt.Errorf("output: marshal MCP result: %w", err)
	}
	var structured map[string]any
	if err := unmarshalMCPResult(raw, &structured); err != nil {
		return nil, fmt.Errorf("output: decode MCP structured result: %w", err)
	}
	return &MCPResult{
		StructuredContent: structured,
		IsError:           env.Outcome == OutcomeFailure || env.Outcome == OutcomePartialFailure,
	}, nil
}
