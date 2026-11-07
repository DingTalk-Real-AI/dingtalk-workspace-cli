// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package personal

import "testing"

func TestCrossPlatformCoverageTimestampRejectsMalformedString(t *testing.T) {
	var value timestampText
	if err := value.UnmarshalJSON([]byte(`"unterminated`)); err == nil {
		t.Fatal("malformed timestamp accepted")
	}
}
