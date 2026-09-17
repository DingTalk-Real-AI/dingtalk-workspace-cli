// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func waitFixtureTool(wait *contract.WaitSpec) ToolSpec {
	return ToolSpec{
		Identity: contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CLIName: "run", CanonicalPath: "sample.run", Path: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run", Source: "runtime",
		},
		Wait: wait,
	}
}

func pollWaitSpec() *contract.WaitSpec {
	return &contract.WaitSpec{
		Mode: contract.WaitModePoll, PollCommand: "sample get", StatusQuery: "data.status",
		Terminal:           map[string]contract.ResultOutcome{" done ": contract.ResultOutcomeSuccess, "failed": contract.ResultOutcomeFailure},
		PendingValues:      []string{" processing "},
		DefaultTimeoutSecs: 90,
	}
}

func TestCrossPlatformCoverageToolSpecWaitNormalizeValidatePayload(t *testing.T) {
	spec := waitFixtureTool(pollWaitSpec())
	normalized := spec.normalized()
	if normalized.Wait == nil {
		t.Fatal("normalized() dropped the wait declaration")
	}
	if normalized.Wait.Terminal["done"] != contract.ResultOutcomeSuccess {
		t.Fatalf("terminal status not trimmed into wire form: %#v", normalized.Wait.Terminal)
	}
	if normalized.Wait.PendingValues[0] != "processing" {
		t.Fatalf("pending values not trimmed: %#v", normalized.Wait.PendingValues)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	payload, err := spec.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	waitPayload, ok := payload["wait"]
	if !ok {
		t.Fatal("ToPayload() omitted the wait key for a declared wait")
	}
	raw, err := json.Marshal(waitPayload)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mode":"poll"`, `"poll_command":"sample get"`, `"status_query":"data.status"`, `"pending_values":["processing"]`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("wait payload missing %s: %s", want, raw)
		}
	}

	undeclared := waitFixtureTool(nil)
	if undeclaredPayload, err := undeclared.ToPayload(); err != nil {
		t.Fatal(err)
	} else if _, ok := undeclaredPayload["wait"]; ok {
		t.Fatal("ToPayload() emitted wait for a tool without a wait declaration")
	}
	compact := Compact(payload)
	if _, ok := compact["wait"]; !ok {
		t.Fatal("Compact() dropped the wait key from the Agent allowlist")
	}
}

func TestCrossPlatformCoverageToolSpecWaitValidateRejectsBrokenDeclarations(t *testing.T) {
	cases := map[string]*contract.WaitSpec{
		"unknown mode": {
			Mode: "teleport", PollCommand: "sample get", StatusQuery: "data.status",
			Terminal: map[string]contract.ResultOutcome{"done": contract.ResultOutcomeSuccess},
		},
		"terminal pending conflict": {
			Mode: contract.WaitModePoll, PollCommand: "sample get", StatusQuery: "data.status",
			Terminal:      map[string]contract.ResultOutcome{"processing": contract.ResultOutcomeSuccess},
			PendingValues: []string{"processing"},
		},
		"unknown terminal outcome": {
			Mode: contract.WaitModePoll, PollCommand: "sample get", StatusQuery: "data.status",
			Terminal: map[string]contract.ResultOutcome{"done": "exploded"},
		},
	}
	for name, wait := range cases {
		spec := waitFixtureTool(wait)
		if err := spec.Validate(); err == nil {
			t.Fatalf("%s: Validate() accepted a broken wait declaration", name)
		}
	}
}

func TestCrossPlatformCoverageSchemaCacheWaitCodec(t *testing.T) {
	wait := pollWaitSpec()
	proto, err := waitToProto(wait)
	if err != nil {
		t.Fatal(err)
	}
	decoded := waitFromProto(proto)
	if decoded == nil {
		t.Fatal("waitFromProto(nil-entry message) = nil")
	}
	// The codec is a faithful round trip: normalization belongs to
	// ToolSpec.normalized()/NormalizeWaitSpec, so raw (padded) statuses and
	// all scalar fields must survive byte-for-byte.
	if decoded.Terminal[" done "] != contract.ResultOutcomeSuccess || decoded.Terminal["failed"] != contract.ResultOutcomeFailure {
		t.Fatalf("terminal map round trip mismatch: %#v", decoded.Terminal)
	}
	if decoded.PendingValues[0] != " processing " {
		t.Fatalf("pending values round trip mismatch: %#v", decoded.PendingValues)
	}
	if decoded.DefaultTimeoutSecs != wait.DefaultTimeoutSecs || decoded.PollCommand != wait.PollCommand || decoded.StatusQuery != wait.StatusQuery || decoded.Mode != wait.Mode {
		t.Fatalf("wait round trip mismatch: %#v", decoded)
	}

	if waitFromProto(nil) != nil {
		t.Fatal("waitFromProto(nil) did not stay nil")
	}
	if toolsFromProto(nil) != nil {
		t.Fatal("toolsFromProto(nil) did not stay nil")
	}
	withoutWait := []ToolSpec{waitFixtureTool(nil)}
	encoded, err := toolsToProto(withoutWait)
	if err != nil {
		t.Fatal(err)
	}
	if roundTripped := toolsFromProto(encoded); roundTripped[0].Wait != nil {
		t.Fatalf("absent wait declaration materialized: %#v", roundTripped[0].Wait)
	}

	broken := *pollWaitSpec()
	broken.Terminal = map[string]contract.ResultOutcome{"done": "exploded"}
	if _, err := waitToProto(&broken); err == nil {
		t.Fatal("waitToProto accepted an unknown terminal outcome")
	}
}
