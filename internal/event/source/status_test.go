// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package source

import (
	"testing"
	"time"
)

func TestCrossPlatformCoverageSourceStatusPreservesObservedTimestamps(t *testing.T) {
	for _, withTimes := range []bool{false, true} {
		snapshot := Snapshot{State: StateConnected, StateSource: SourceInferred, ReconnectCount: 3}
		if withTimes {
			snapshot.LastEventAt = time.Unix(100, 0)
			snapshot.LastReconnectAt = time.Unix(90, 0)
		}
		got := transportStatus(snapshot)
		if !got.Observed || got.State != "connected" || got.Source != "inferred" || got.ReconnectCount != 3 {
			t.Fatalf("status=%+v", got)
		}
		if withTimes && (got.LastEventAtMS != 100000 || got.LastReconnectMS != 90000) {
			t.Fatalf("timestamps=%+v", got)
		}
		if !withTimes && (got.LastEventAtMS != 0 || got.LastReconnectMS != 0) {
			t.Fatalf("zero timestamps changed: %+v", got)
		}
	}
	personal := (&PersonalSource{machine: NewMachine()}).SourceStatus()
	dingtalk := (&DingtalkSource{machine: NewMachine()}).SourceStatus()
	if personal != dingtalk || !personal.Observed || personal.State != "disconnected" {
		t.Fatalf("personal=%+v dingtalk=%+v", personal, dingtalk)
	}
}
