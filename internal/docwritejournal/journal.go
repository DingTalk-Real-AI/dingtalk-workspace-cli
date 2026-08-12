// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package docwritejournal provides a short-lived, profile-isolated
// read-your-write view for newly created documents. DingTalk search/recent
// indexes are eventually consistent, while DWS commands run in separate
// processes, so an in-memory cache cannot bridge a create followed by search.
package docwritejournal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	journalFile = "doc-write-journal.json"
	entryTTL    = 15 * time.Minute
)

type Entry struct {
	ProfileKey  string `json:"profileKey"`
	Fingerprint string `json:"fingerprint,omitempty"`
	NodeID      string `json:"nodeId"`
	Name        string `json:"name"`
	DocType     string `json:"docType,omitempty"`
	URL         string `json:"url,omitempty"`
	FolderID    string `json:"folderId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	CreatorID   string `json:"creatorId,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

type fileData struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

var now = time.Now

func Record(ctx context.Context, entry Entry) error {
	entry.NodeID = strings.TrimSpace(entry.NodeID)
	entry.Name = strings.TrimSpace(entry.Name)
	if entry.NodeID == "" || entry.Name == "" {
		return fmt.Errorf("doc write journal requires nodeId and name")
	}
	key, creatorID := currentProfileKey()
	entry.ProfileKey = key
	if entry.CreatorID == "" {
		entry.CreatorID = creatorID
	}
	if entry.CreatedAt <= 0 {
		entry.CreatedAt = now().UnixMilli()
	}
	return withLockedData(ctx, func(data *fileData) bool {
		entries := prune(data.Entries)
		out := make([]Entry, 0, len(entries)+1)
		for _, existing := range entries {
			if existing.ProfileKey == key && (existing.NodeID == entry.NodeID || (entry.Fingerprint != "" && existing.Fingerprint == entry.Fingerprint)) {
				continue
			}
			out = append(out, existing)
		}
		data.Version = 1
		data.Entries = append(out, entry)
		return true
	})
}

func List(ctx context.Context) ([]Entry, error) {
	key, _ := currentProfileKey()
	var result []Entry
	err := withLockedData(ctx, func(data *fileData) bool {
		before := len(data.Entries)
		data.Entries = prune(data.Entries)
		for _, entry := range data.Entries {
			if entry.ProfileKey == key {
				result = append(result, entry)
			}
		}
		return len(data.Entries) != before
	})
	return result, err
}

func LookupFingerprint(ctx context.Context, fingerprint string) (Entry, bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	entries, err := List(ctx)
	if err != nil {
		return Entry{}, false, err
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if fingerprint != "" && entries[index].Fingerprint == fingerprint {
			return entries[index], true, nil
		}
	}
	return Entry{}, false, nil
}

func currentProfileKey() (string, string) {
	selector := strings.TrimSpace(profilectx.Get())
	profile, err := auth.ResolveProfile(config.DefaultConfigDir(), selector)
	if err == nil && profile != nil {
		return strings.TrimSpace(profile.CorpID) + "/" + strings.TrimSpace(profile.UserID), strings.TrimSpace(profile.UserID)
	}
	if selector != "" {
		return "selector:" + selector, ""
	}
	return "default", ""
}

func prune(entries []Entry) []Entry {
	cutoff := now().Add(-entryTTL).UnixMilli()
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.CreatedAt >= cutoff && strings.TrimSpace(entry.NodeID) != "" {
			out = append(out, entry)
		}
	}
	return out
}

func withLockedData(ctx context.Context, mutate func(*fileData) bool) error {
	configDir := config.DefaultConfigDir()
	lock, err := auth.AcquireDualLock(ctx, configDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	if err := os.MkdirAll(configDir, config.DirPerm); err != nil {
		return err
	}
	path := filepath.Join(configDir, journalFile)
	data := fileData{Version: 1}
	raw, readErr := os.ReadFile(path)
	if readErr == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("decode doc write journal: %w", err)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if !mutate(&data) {
		return nil
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(configDir, ".doc-write-journal-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(config.FilePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
