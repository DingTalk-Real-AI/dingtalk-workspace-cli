// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package delivery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/state"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestStoreRoundTripAndExtractBizID(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	want := Record{Handle: "card-123", BizID: "biz-1", Profile: ProfileScope{Selector: "corp:user", CorpID: "corp", UserID: "user", Environment: "im:test"}, ConversationID: "cid", Surface: state.Surface{SurfaceID: "surface"}, FlowStatus: "PROCESSING", Revision: 1}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(want.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if got.BizID != want.BizID || got.Surface.SurfaceID != "surface" || got.Surface.Version != "v1.0" {
		t.Fatalf("got %+v", got)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if got := ExtractBizID(map[string]any{"result": map[string]any{"bizId": "nested"}}); got != "nested" {
		t.Fatalf("bizId=%q", got)
	}
	if got := ExtractCardInstanceID(map[string]any{"result": map[string]any{"cardInstanceId": json.Number("506703919635")}}); got != "506703919635" {
		t.Fatalf("cardInstanceId=%q", got)
	}
	if got := ExtractCardInstanceID(map[string]any{"result": map[string]any{"cardInstanceId": float64(12.5)}}); got != "" {
		t.Fatalf("nonintegral cardInstanceId=%q", got)
	}
}

func TestStoreScopeLockAndRevisionCAS(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	scope := ProfileScope{Selector: "corp:user", CorpID: "corp", UserID: "user", Environment: "im:test"}
	record := Record{Handle: "card-locked", BizID: "biz", Profile: scope, ConversationID: "cid", Surface: state.Surface{SurfaceID: "surface"}, FlowStatus: "PROCESSING", Revision: 1}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := record.ValidateUpdateScope(scope); err != nil {
		t.Fatal(err)
	}
	if err := record.ValidateUpdateScope(ProfileScope{Selector: "other", Environment: "im:test"}); err == nil {
		t.Fatal("expected cross-profile scope rejection")
	}
	legacy := record
	legacy.Profile = ProfileScope{Selector: "corp:user", Environment: "im:test"}
	if err := legacy.ValidateUpdateScope(scope); err == nil {
		t.Fatal("expected legacy selector-only scope rejection")
	}
	if err := store.WithHandleLock(record.Handle, func() error {
		err := store.WithHandleLock(record.Handle, func() error { return nil })
		if err == nil || !strings.Contains(err.Error(), "busy") {
			t.Fatalf("nested lock error=%v", err)
		}
		record.Revision = 2
		return store.SaveIfRevision(record, 1)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIfRevision(record, 1); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale CAS error=%v", err)
	}
}

func TestWithHandleLockReclaimsDeadOwnerAndPreservesLiveOwner(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(store.Dir, "card-stale.lock")
	writeLock := func(pid int) {
		t.Helper()
		raw, err := json.Marshal(handleLockRecord{PID: pid, AcquiredAt: time.Now().UTC().Add(-time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lockPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	testseam.Swap(t, &handleLockProcessAlive, func(pid int) (bool, error) { return false, nil })
	writeLock(424242)
	called := false
	if err := store.WithHandleLock("card-stale", func() error { called = true; return nil }); err != nil {
		t.Fatalf("recover stale lock: %v", err)
	}
	if !called {
		t.Fatal("callback was not called")
	}

	testseam.Swap(t, &handleLockProcessAlive, func(pid int) (bool, error) { return true, nil })
	writeLock(424243)
	if err := store.WithHandleLock("card-stale", func() error { return nil }); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("live-owner lock error=%v", err)
	}
}
