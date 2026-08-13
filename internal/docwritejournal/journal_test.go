// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package docwritejournal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
)

func TestJournalRecordLookupAndTTL(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	stubResolvedJournalProfile(t)
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

func TestJournalProfilesDeduplicationAndValidation(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldResolve := journalResolveProfile
	t.Cleanup(func() { journalResolveProfile = oldResolve; profilectx.Set("") })
	journalResolveProfile = func(string, string) (*auth.Profile, error) {
		return &auth.Profile{CorpID: " corp ", UserID: " user "}, nil
	}
	if key, creator, err := currentProfileKey(); err != nil || key != "4:corp:4:user" || creator != "user" {
		t.Fatalf("resolved profile = %q, %q, %v", key, creator, err)
	}
	journalResolveProfile = func(string, string) (*auth.Profile, error) { return nil, errors.New("missing") }
	profilectx.Set(" team ")
	if _, _, err := currentProfileKey(); err == nil {
		t.Fatal("selector resolution failure accepted")
	}
	profilectx.Set("")
	if _, _, err := currentProfileKey(); err == nil {
		t.Fatal("default resolution failure accepted")
	}
	if err := Record(context.Background(), Entry{NodeID: "blocked", Name: "blocked"}); err == nil {
		t.Fatal("record accepted unresolved profile")
	}
	if _, err := List(context.Background()); err == nil {
		t.Fatal("list accepted unresolved profile")
	}
	for _, profile := range []*auth.Profile{nil, {CorpID: "corp"}, {UserID: "user"}} {
		journalResolveProfile = func(string, string) (*auth.Profile, error) { return profile, nil }
		if _, _, err := currentProfileKey(); err == nil {
			t.Fatalf("incomplete profile accepted: %#v", profile)
		}
	}
	journalResolveProfile = func(string, string) (*auth.Profile, error) {
		return &auth.Profile{CorpID: "corp", UserID: "user"}, nil
	}

	if err := Record(context.Background(), Entry{}); err == nil {
		t.Fatal("empty entry accepted")
	}
	for _, entry := range []Entry{
		{Fingerprint: "fp", NodeID: " node-1 ", Name: " first "},
		{Fingerprint: "keep", NodeID: "node-keep", Name: "keep"},
		{Fingerprint: "fp", NodeID: "node-2", Name: "replacement"},
		{Fingerprint: "other", NodeID: "node-2", Name: "node replacement", CreatorID: "explicit"},
	} {
		if err := Record(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := List(context.Background())
	if err != nil || len(entries) != 2 || entries[1].Name != "node replacement" || entries[1].CreatorID != "explicit" {
		t.Fatalf("deduplicated entries = %#v, %v", entries, err)
	}
	if _, ok, err := LookupFingerprint(context.Background(), "missing"); err != nil || ok {
		t.Fatalf("missing lookup = %v, %v", ok, err)
	}
	if _, ok, err := LookupFingerprint(context.Background(), "  "); err != nil || ok {
		t.Fatalf("blank lookup = %v, %v", ok, err)
	}

	clock := time.Now()
	oldNow := now
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = oldNow })
	kept := prune([]Entry{{NodeID: "", CreatedAt: clock.UnixMilli()}, {NodeID: "old", CreatedAt: clock.Add(-entryTTL - time.Second).UnixMilli()}, {NodeID: "new", CreatedAt: clock.UnixMilli()}})
	if len(kept) != 1 || kept[0].NodeID != "new" {
		t.Fatalf("prune = %#v", kept)
	}
}

func TestJournalStorageErrorCoverage(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	stubResolvedJournalProfile(t)
	oldAcquire, oldMkdir, oldRead := journalAcquireLock, journalMkdirAll, journalReadFile
	oldMarshal, oldCreate := journalMarshal, journalCreateTemp
	oldChmod, oldWrite, oldClose, oldRename := journalChmod, journalWrite, journalClose, journalRename
	t.Cleanup(func() {
		journalAcquireLock, journalMkdirAll, journalReadFile = oldAcquire, oldMkdir, oldRead
		journalMarshal, journalCreateTemp = oldMarshal, oldCreate
		journalChmod, journalWrite, journalClose, journalRename = oldChmod, oldWrite, oldClose, oldRename
	})
	reset := func() {
		journalAcquireLock, journalMkdirAll, journalReadFile = oldAcquire, oldMkdir, oldRead
		journalMarshal, journalCreateTemp = oldMarshal, oldCreate
		journalChmod, journalWrite, journalClose, journalRename = oldChmod, oldWrite, oldClose, oldRename
	}
	mutate := func(data *fileData) bool {
		data.Entries = append(data.Entries, Entry{NodeID: "n", Name: "name"})
		return true
	}
	fail := errors.New("injected")

	journalAcquireLock = func(context.Context, string) (*auth.DualLock, error) { return nil, fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("lock error = %v", err)
	}
	reset()
	journalMkdirAll = func(string, os.FileMode) error { return fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("mkdir error = %v", err)
	}
	reset()
	if err := os.WriteFile(filepath.Join(configDir, journalFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := withLockedData(context.Background(), mutate); err == nil {
		t.Fatal("corrupt journal accepted")
	}
	if err := os.Remove(filepath.Join(configDir, journalFile)); err != nil {
		t.Fatal(err)
	}
	journalReadFile = func(string) ([]byte, error) { return nil, fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("read error = %v", err)
	}
	if _, _, err := LookupFingerprint(context.Background(), "fp"); !errors.Is(err, fail) {
		t.Fatalf("lookup read error = %v", err)
	}
	reset()
	if err := withLockedData(context.Background(), func(*fileData) bool { return false }); err != nil {
		t.Fatalf("no-op mutation = %v", err)
	}
	journalMarshal = func(any, string, string) ([]byte, error) { return nil, fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("marshal error = %v", err)
	}
	reset()
	journalCreateTemp = func(string, string) (*os.File, error) { return nil, fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("create temp error = %v", err)
	}
	reset()
	journalChmod = func(*os.File, os.FileMode) error { return fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("chmod error = %v", err)
	}
	reset()
	journalWrite = func(*os.File, []byte) (int, error) { return 0, fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("write error = %v", err)
	}
	reset()
	journalClose = func(file *os.File) error { _ = file.Close(); return fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("close error = %v", err)
	}
	reset()
	journalRename = func(string, string) error { return fail }
	if err := withLockedData(context.Background(), mutate); !errors.Is(err, fail) {
		t.Fatalf("rename error = %v", err)
	}
	reset()

	// Exercise successful decode of an existing journal and the cleanup-write path.
	raw, _ := json.Marshal(fileData{Version: 1, Entries: []Entry{{ProfileKey: "other", NodeID: "old", Name: "old", CreatedAt: 1}}})
	if err := os.WriteFile(filepath.Join(configDir, journalFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func stubResolvedJournalProfile(t *testing.T) {
	t.Helper()
	oldResolve := journalResolveProfile
	journalResolveProfile = func(string, string) (*auth.Profile, error) {
		return &auth.Profile{CorpID: "test-corp", UserID: "test-user"}, nil
	}
	t.Cleanup(func() { journalResolveProfile = oldResolve })
}
