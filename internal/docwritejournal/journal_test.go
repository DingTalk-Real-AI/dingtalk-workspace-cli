// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package docwritejournal

import (
	"context"
	"testing"
	"time"
)

func TestJournalRecordLookupAndTTL(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldNow := now
	clock := time.Unix(1000, 0)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = oldNow })

	entry := Entry{Fingerprint: "fp", NodeID: "node-1", Name: "new doc", DocType: "ALIDOC"}
	if err := Record(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LookupFingerprint(context.Background(), "fp")
	if err != nil || !ok || got.NodeID != "node-1" || got.CreatedAt != clock.UnixMilli() {
		t.Fatalf("lookup = %#v, %v, %v", got, ok, err)
	}
	clock = clock.Add(entryTTL + time.Second)
	if entries, err := List(context.Background()); err != nil || len(entries) != 0 {
		t.Fatalf("expired entries = %#v, %v", entries, err)
	}
}
