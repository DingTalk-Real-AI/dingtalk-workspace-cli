// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestRuntimeFlagContractCoversPflagFamilies(t *testing.T) {
	flags := pflag.NewFlagSet("schema-types", pflag.ContinueOnError)
	flags.Uint("uint", 0, "")
	flags.Count("count", "")
	flags.Float64("ratio", 0, "")
	flags.IntSlice("ids", nil, "")
	flags.DurationSlice("windows", nil, "")
	flags.Duration("timeout", 0, "")
	flags.StringToString("labels", nil, "")

	tests := []struct {
		name        string
		wantType    string
		wantDefault string
	}{
		{name: "uint", wantType: "integer"},
		{name: "count", wantType: "integer"},
		{name: "ratio", wantType: "number"},
		{name: "ids", wantType: "array"},
		{name: "windows", wantType: "array"},
		{name: "timeout", wantType: "string"},
		{name: "labels", wantType: "string"},
	}
	for _, test := range tests {
		flag := flags.Lookup(test.name)
		if got := runtimeFlagCLIType(flag); got != test.wantType {
			t.Errorf("--%s type = %q, want %q", test.name, got, test.wantType)
		}
		if got := runtimeFlagDefault(flag); got != test.wantDefault {
			t.Errorf("--%s default = %q, want %q", test.name, got, test.wantDefault)
		}
	}

	flags.Duration("nonzero-timeout", 5*time.Second, "")
	if got := runtimeFlagDefault(flags.Lookup("nonzero-timeout")); got != "5s" {
		t.Fatalf("non-zero duration default = %q, want 5s", got)
	}
}
