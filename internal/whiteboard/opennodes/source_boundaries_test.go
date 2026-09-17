// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesDecoderAndDigestFailures(t *testing.T) {
	if _, err := Parse([]byte(`{"source":null}`)); err == nil {
		t.Fatal("null source accepted")
	}
	for _, raw := range []string{"", `null`, `{}`, `[] []`, `[] invalid`} {
		if _, err := decodeNodeArray([]byte(raw)); err == nil {
			t.Fatalf("invalid array accepted: %s", raw)
		}
	}
	if _, err := CanonicalJSON(nil); err == nil {
		t.Fatal("nil canonical source accepted")
	}
	if _, err := DigestSource(nil); err == nil {
		t.Fatal("nil digest source accepted")
	}
	if _, err := DigestUpdate(false, []map[string]any{{"invalid": make(chan int)}}); err == nil {
		t.Fatal("unencodable update accepted")
	}
	if err := requireEOF(json.NewDecoder(strings.NewReader("invalid"))); err == nil {
		t.Fatal("malformed trailing data accepted")
	}
}
