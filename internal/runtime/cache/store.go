// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

// HasActionVersionChanged compares cached actionVersion strings against the
// versions reported by a fresh Detail API response. It returns true when at
// least one tool's version has changed, signalling that the tools cache
// should be refreshed even if the TTL has not expired.
func HasActionVersionChanged(cached map[string]string, detailTools []market.DetailTool) bool {
	if len(cached) == 0 {
		return false // no prior version data → not a change
	}
	for _, tool := range detailTools {
		name := strings.TrimSpace(tool.ToolName)
		version := strings.TrimSpace(tool.ActionVersion)
		if name == "" || version == "" {
			continue
		}
		if oldVersion, exists := cached[name]; exists && oldVersion != version {
			return true
		}
	}
	return false
}

// ExtractActionVersions builds a tool-name → actionVersion map from detail tools.
func ExtractActionVersions(detailTools []market.DetailTool) map[string]string {
	if len(detailTools) == 0 {
		return nil
	}
	versions := make(map[string]string, len(detailTools))
	for _, tool := range detailTools {
		name := strings.TrimSpace(tool.ToolName)
		version := strings.TrimSpace(tool.ActionVersion)
		if name != "" && version != "" {
			versions[name] = version
		}
	}
	if len(versions) == 0 {
		return nil
	}
	return versions
}

const (
	RegistryTTL     = 24 * time.Hour
	ToolsTTL        = 7 * 24 * time.Hour
	DetailTTL       = 7 * 24 * time.Hour
	RevalidateAfter = 1 * time.Hour
)

type Freshness string

const (
	FreshnessFresh Freshness = "fresh"
	FreshnessStale Freshness = "stale"
)

type Store struct {
	Root string
	Now  func() time.Time
}

type RegistrySnapshot struct {
	SavedAt  time.Time                 `json:"saved_at"`
	Servers  []market.ServerDescriptor `json:"servers"`
	Complete bool                      `json:"complete,omitempty"`
}

type ToolsSnapshot struct {
	SavedAt         time.Time                  `json:"saved_at"`
	ServerKey       string                     `json:"server_key"`
	ProtocolVersion string                     `json:"protocol_version"`
	Tools           []transport.ToolDescriptor `json:"tools"`
	ActionVersions  map[string]string          `json:"action_versions,omitempty"`
}

type DetailSnapshot struct {
	SavedAt time.Time       `json:"saved_at"`
	MCPID   int             `json:"mcp_id"`
	Payload json.RawMessage `json:"payload"`
}

func NewStore(root string) *Store {
	if strings.TrimSpace(root) == "" {
		root = defaultCacheRoot()
	}
	return &Store{
		Root: root,
		Now:  time.Now,
	}
}

// defaultCacheRoot returns a stable, persistent cache directory.
// Prefers ~/.dws/cache (matches defaultConfigDir in app/config.go).
// Falls back to os.TempDir()/dws-cache only when $HOME is unavailable.
func defaultCacheRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".dws", "cache")
	}
	return filepath.Join(os.TempDir(), "dws-cache")
}

func (s *Store) SaveRegistry(partition string, snapshot RegistrySnapshot) error {
	if snapshot.SavedAt.IsZero() {
		snapshot.SavedAt = s.Now().UTC()
	}
	snapshot.Complete = true
	return s.saveJSON(s.registryPath(partition), snapshot)
}

func (s *Store) LoadRegistry(partition string) (RegistrySnapshot, Freshness, error) {
	var snapshot RegistrySnapshot
	if err := s.loadJSON(s.registryPath(partition), &snapshot); err != nil {
		return RegistrySnapshot{}, "", err
	}
	return snapshot, freshness(s.Now().UTC(), snapshot.SavedAt, RegistryTTL), nil
}

func (s *Store) SaveTools(partition, serverKey string, snapshot ToolsSnapshot) error {
	if snapshot.SavedAt.IsZero() {
		snapshot.SavedAt = s.Now().UTC()
	}
	return s.saveJSON(s.toolsPath(partition, serverKey), snapshot)
}

func (s *Store) LoadTools(partition, serverKey string) (ToolsSnapshot, Freshness, error) {
	var snapshot ToolsSnapshot
	if err := s.loadJSON(s.toolsPath(partition, serverKey), &snapshot); err != nil {
		return ToolsSnapshot{}, "", err
	}
	return snapshot, freshness(s.Now().UTC(), snapshot.SavedAt, ToolsTTL), nil
}

// DeleteTools removes the cached tools snapshot for a server, forcing a
// re-fetch on the next DiscoverServerRuntime call.
func (s *Store) DeleteTools(partition, serverKey string) error {
	path := s.toolsPath(partition, serverKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ToolsCacheEntrySummary summarises one cached tools snapshot.
type ToolsCacheEntrySummary struct {
	ServerKey    string    `json:"server_key"`
	Freshness    Freshness `json:"freshness"`
	SavedAt      time.Time `json:"saved_at"`
	ToolCount    int       `json:"tool_count"`
	TTLRemaining string    `json:"ttl_remaining"`
}

// ListToolsCacheEntries walks the cache directory and returns a summary for
// each server whose tools snapshot is cached.
func (s *Store) ListToolsCacheEntries(partition string) ([]ToolsCacheEntrySummary, error) {
	toolsDir := filepath.Join(s.Root, sanitize(partition), "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	now := s.Now().UTC()
	summaries := make([]ToolsCacheEntrySummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var snapshot ToolsSnapshot
		path := filepath.Join(toolsDir, entry.Name())
		if loadErr := s.loadJSON(path, &snapshot); loadErr != nil {
			continue
		}
		f := freshness(now, snapshot.SavedAt, ToolsTTL)
		remaining := ""
		if f == FreshnessFresh {
			rem := ToolsTTL - now.Sub(snapshot.SavedAt)
			if rem > 0 {
				remaining = rem.Truncate(time.Minute).String()
			}
		}
		summaries = append(summaries, ToolsCacheEntrySummary{
			ServerKey:    snapshot.ServerKey,
			Freshness:    f,
			SavedAt:      snapshot.SavedAt,
			ToolCount:    len(snapshot.Tools),
			TTLRemaining: remaining,
		})
	}
	return summaries, nil
}

func (s *Store) SaveDetail(partition, serverKey string, snapshot DetailSnapshot) error {
	if snapshot.SavedAt.IsZero() {
		snapshot.SavedAt = s.Now().UTC()
	}
	return s.saveJSON(s.detailPath(partition, serverKey), snapshot)
}

func (s *Store) LoadDetail(partition, serverKey string) (DetailSnapshot, Freshness, error) {
	var snapshot DetailSnapshot
	if err := s.loadJSON(s.detailPath(partition, serverKey), &snapshot); err != nil {
		return DetailSnapshot{}, "", err
	}
	return snapshot, freshness(s.Now().UTC(), snapshot.SavedAt, DetailTTL), nil
}

// DeleteDetail removes the cached detail snapshot for a server.
func (s *Store) DeleteDetail(partition, serverKey string) error {
	path := s.detailPath(partition, serverKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) registryPath(partition string) string {
	return filepath.Join(s.Root, sanitize(partition), "market", "servers.json")
}

func (s *Store) toolsPath(partition, serverKey string) string {
	return filepath.Join(s.Root, sanitize(partition), "tools", sanitize(serverKey)+".json")
}

func (s *Store) detailPath(partition, serverKey string) string {
	return filepath.Join(s.Root, sanitize(partition), "detail", sanitize(serverKey)+".json")
}

func (s *Store) saveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write with fsync to ensure data durability
	tmpPath := path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	writeSuccess := false
	defer func() {
		if !writeSuccess {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	writeSuccess = true
	return nil
}

func (s *Store) loadJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func freshness(now, savedAt time.Time, ttl time.Duration) Freshness {
	if savedAt.IsZero() || now.Sub(savedAt) > ttl {
		return FreshnessStale
	}
	return FreshnessFresh
}

// ShouldRevalidate reports whether a still-valid snapshot is old enough to
// merit a live revalidation attempt before trusting it as the current truth.
func ShouldRevalidate(now, savedAt time.Time) bool {
	if savedAt.IsZero() {
		return true
	}
	return now.Sub(savedAt) >= RevalidateAfter
}

func sanitize(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(value)
}

func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
