// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mcpwire

import (
	"errors"
	"testing"
)

func TestCrossPlatformCoverageMCPWirePrimaryDocAbsentIsExact(t *testing.T) {
	absent := errors.New(`[MCP_TOOL_ERROR] {"data":{},"error":{"code":"-1","message":"no record","retryable":true,"type":"SYSTEM_ERROR"}}`)
	if !IsPrimaryDocAbsent(absent) {
		t.Fatal("reviewed primary-doc absence shape was not recognized")
	}
	for _, err := range []error{
		errors.New(`SYSTEM_ERROR no record`),
		errors.New(`[MCP_TOOL_ERROR] {"error":{"code":"-2","message":"no record","type":"SYSTEM_ERROR"}}`),
		errors.New(`[MCP_TOOL_ERROR] {"error":{"code":"-1","message":"permission denied","type":"SYSTEM_ERROR"}}`),
	} {
		if IsPrimaryDocAbsent(err) {
			t.Fatalf("non-reviewed error was classified as absence: %v", err)
		}
	}
}

func TestCrossPlatformCoverageMCPWireRetryability(t *testing.T) {
	retryable := false
	details, ok := ParseError(errors.New(`[MCP_TOOL_ERROR] {"error":{"code":"DASHBOARD_NOT_FOUND","message":"missing","retryable":false,"type":"INPUT_ERROR"}}`))
	if !ok || details.Code != "DASHBOARD_NOT_FOUND" || details.Retryable == nil || *details.Retryable != retryable {
		t.Fatalf("details = %#v, %v", details, ok)
	}
	for _, err := range []error{
		errors.New(`[MCP_TOOL_ERROR] {broken}`),
		errors.New(`[MCP_TOOL_ERROR] {"error":{}}`),
	} {
		if details, ok := ParseError(err); ok {
			t.Fatalf("malformed or empty envelope parsed as %#v", details)
		}
	}
}
