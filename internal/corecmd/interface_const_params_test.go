// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package corecmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageInterfaceBoolConstParamsRegistryClonesAndDeletes(t *testing.T) {
	attachInterfaceBoolConstParams(nil, map[string]any{"ignored": true})
	if got := InterfaceBoolConstParams(nil); got != nil {
		t.Fatalf("nil command evidence = %#v, want nil", got)
	}

	cmd := &cobra.Command{Use: "send"}
	declared := map[string]any{"convThreadEnabled": true, "precheckOnly": false}
	attachInterfaceBoolConstParams(cmd, declared)
	declared["convThreadEnabled"] = false

	want := map[string]bool{"convThreadEnabled": true, "precheckOnly": false}
	got := InterfaceBoolConstParams(cmd)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registered bool ConstParams = %#v, want %#v", got, want)
	}
	got["convThreadEnabled"] = false
	if again := InterfaceBoolConstParams(cmd); !reflect.DeepEqual(again, want) {
		t.Fatalf("reader leaked mutable registry state: %#v", again)
	}

	attachInterfaceBoolConstParams(cmd, map[string]any{"convThreadEnabled": true, "retryLimit": 3})
	if got := InterfaceBoolConstParams(cmd); got != nil {
		t.Fatalf("mixed ConstParams retained evidence: %#v", got)
	}

	attachInterfaceBoolConstParams(cmd, map[string]any{"convThreadEnabled": true})
	attachInterfaceBoolConstParams(cmd, nil)
	if got := InterfaceBoolConstParams(cmd); got != nil {
		t.Fatalf("empty ConstParams retained evidence: %#v", got)
	}
}
