// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesParseCanonicalAndDigest(t *testing.T) {
	direct := []byte(`{"nodes":[{"type":"shape","id":"n1","x":1.50}],"catalogVersion":"dml-v1","schemaVersion":"1.0"}`)
	wrapper := []byte(`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"x":1.50,"id":"n1","type":"shape"}]}}`)
	left, err := Parse(direct)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	leftDigest, err := DigestSource(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := DigestSource(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest || !ValidDigest(leftDigest) || !EqualDigest(strings.ToUpper(leftDigest), rightDigest) {
		t.Fatalf("digests left=%q right=%q", leftDigest, rightDigest)
	}
	canonical, err := CanonicalJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"shape","x":1.50}]}`
	if string(canonical) != want {
		t.Fatalf("canonical=%s want=%s", canonical, want)
	}
	appendDigest, err := DigestUpdate(false, left.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	overwriteDigest, err := DigestUpdate(true, left.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	if appendDigest == overwriteDigest {
		t.Fatal("update digest did not bind overwrite intent")
	}
}

func TestCrossPlatformCoverageOpenNodesParseRejectsInvalidSources(t *testing.T) {
	tests := []string{
		`null`, `{`, `{} {}`, `{}`,
		`{"schemaVersion":"2.0","catalogVersion":"dml-v1","nodes":[]}`,
		`{"schemaVersion":"1.0","catalogVersion":"bad","nodes":[]}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":null}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[1]}`,
		`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[],"unknown":true}`,
		`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]},"unknown":true}`,
	}
	for _, source := range tests {
		if _, err := Parse([]byte(source)); err == nil {
			t.Fatalf("Parse(%s) unexpectedly succeeded", source)
		}
	}
	if ValidDigest("sha256:bad") {
		t.Fatal("short digest accepted")
	}
}
