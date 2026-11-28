// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	agentCacheFormatVersion = 1
	agentCacheMaxDepth      = 6
	agentCacheTTL           = 10 * time.Minute
	agentCacheMaxFileBytes  = MaxAgentJSONBytes + 4096
	disableAgentCacheEnv    = "DWS_DISABLE_AGENT_CACHE"
)

type ancestorProcess struct {
	PID       int    `json:"pid"`
	PPID      int    `json:"ppid"`
	StartTime int64  `json:"start_time"`
	Comm      string `json:"comm"`
}

// cachedAgentRecord stores the validated host-agent snapshot keyed by the
// normalized ancestor process chain (excluding transient wrapper shells).
// With full_info=0, the SDK "process" object describes the matched long-lived
// host agent process rather than the ephemeral dws child process itself.
type cachedAgentRecord struct {
	Version        int             `json:"version"`
	PayloadVersion string          `json:"payload_version"`
	Key            string          `json:"key"`
	CreatedAtUnix  int64           `json:"created_at_unix"`
	ErrorCode      int32           `json:"error_code"`
	Raw            json.RawMessage `json:"raw"`
}

var (
	agentCacheNow        = time.Now
	agentCacheMaxEntries = 64
	agentCacheAncestry   = func() ([]ancestorProcess, bool) {
		return collectProcessAncestry(agentCacheMaxDepth)
	}
	agentCacheKey        = computeAgentCacheKey
	agentCacheLookup     = loadCachedAgentSnapshot
	agentCacheStore      = storeCachedAgentSnapshot
	agentCacheReadFile   = os.ReadFile
	agentCacheWriteFile  = os.WriteFile
	agentCacheRenameFile = os.Rename
	agentCacheLstat      = os.Lstat
)

func walkProcessAncestry(startPID, maxDepth int, minPID int, lookup func(int) (ancestorProcess, bool)) ([]ancestorProcess, bool) {
	if startPID < minPID || maxDepth <= 0 {
		return nil, false
	}
	chain := make([]ancestorProcess, 0, maxDepth)
	seen := make(map[int]struct{}, maxDepth)
	for pid := startPID; len(chain) < maxDepth && pid >= minPID; {
		if _, dup := seen[pid]; dup {
			break
		}
		seen[pid] = struct{}{}
		proc, ok := lookup(pid)
		if !ok {
			break
		}
		chain = append(chain, proc)
		if proc.PPID < minPID || proc.PPID == pid {
			break
		}
		pid = proc.PPID
	}
	return chain, len(chain) > 0
}

// normalizeAncestryForCache drops immediate transient wrapper shells (e.g. an
// ephemeral "zsh -lc ..." spawned per tool call) when a longer-lived ancestor
// exists above them, so consecutive tool calls from the same host agent share
// one cache key instead of creating a new entry per shell invocation.
func normalizeAncestryForCache(chain []ancestorProcess) []ancestorProcess {
	for len(chain) > 1 && isTransientShell(chain[0].Comm) {
		chain = chain[1:]
	}
	return chain
}

func isTransientShell(comm string) bool {
	name := strings.ToLower(strings.TrimPrefix(filepath.Base(strings.TrimSpace(comm)), "-"))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "sh", "bash", "zsh", "dash", "ksh", "fish", "csh", "tcsh", "cmd", "powershell", "pwsh":
		return true
	default:
		return false
	}
}

func agentCacheDisabled() bool {
	v := strings.TrimSpace(os.Getenv(disableAgentCacheEnv))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func computeAgentCacheKey() (string, bool) {
	if agentCacheDisabled() {
		return "", false
	}
	chain, ok := agentCacheAncestry()
	if !ok || len(chain) == 0 {
		return "", false
	}
	chain = normalizeAncestryForCache(chain)
	var exeSize, exeModUnix int64
	if executable, err := osExecutable(); err == nil {
		if resolved, err := evalSymlinks(executable); err == nil {
			executable = resolved
		}
		if info, err := os.Stat(executable); err == nil {
			exeSize = info.Size()
			exeModUnix = info.ModTime().UnixNano()
		}
	}
	// Fixed primitive struct; json.Marshal cannot fail.
	material, _ := json.Marshal(struct {
		Version        int               `json:"v"`
		PayloadVersion string            `json:"pv"`
		Target         string            `json:"t"`
		UID            int               `json:"u"`
		ExeSize        int64             `json:"es"`
		ExeMod         int64             `json:"em"`
		Chain          []ancestorProcess `json:"c"`
	}{
		Version:        agentCacheFormatVersion,
		PayloadVersion: PayloadVersion,
		Target:         currentGOOS + "/" + currentGOARCH,
		UID:            os.Geteuid(),
		ExeSize:        exeSize,
		ExeMod:         exeModUnix,
		Chain:          chain,
	})
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:]), true
}

func agentCacheFilePath(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	cacheDir, err := userCacheDir()
	if err != nil || cacheDir == "" {
		return "", false
	}
	return filepath.Join(cacheDir, "dws", "runtime-context", "agent-cache", PayloadVersion, key+".json"), true
}

func loadCachedAgentSnapshot(key string) (AgentSnapshot, bool) {
	path, ok := agentCacheFilePath(key)
	if !ok {
		return AgentSnapshot{}, false
	}
	info, err := agentCacheLstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > agentCacheMaxFileBytes {
		return AgentSnapshot{}, false
	}
	data, err := agentCacheReadFile(path)
	if err != nil {
		return AgentSnapshot{}, false
	}
	var record cachedAgentRecord
	if json.Unmarshal(data, &record) != nil {
		return AgentSnapshot{}, false
	}
	if record.Version != agentCacheFormatVersion || record.PayloadVersion != PayloadVersion || record.Key != key || record.ErrorCode != 0 {
		return AgentSnapshot{}, false
	}
	age := agentCacheNow().Unix() - record.CreatedAtUnix
	if age < 0 || time.Duration(age)*time.Second > agentCacheTTL {
		return AgentSnapshot{}, false
	}
	snapshot := parseAgentResult(record.Raw, record.ErrorCode, nil)
	if snapshot.State != StateReady && snapshot.State != AgentUnknown {
		return AgentSnapshot{}, false
	}
	return snapshot, true
}

func storeCachedAgentSnapshot(key string, snapshot AgentSnapshot) {
	if key == "" || (snapshot.State != StateReady && snapshot.State != AgentUnknown) || snapshot.ErrorCode != 0 || len(snapshot.raw) == 0 {
		return
	}
	path, ok := agentCacheFilePath(key)
	if !ok {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(cachedAgentRecord{
		Version:        agentCacheFormatVersion,
		PayloadVersion: PayloadVersion,
		Key:            key,
		CreatedAtUnix:  agentCacheNow().Unix(),
		ErrorCode:      snapshot.ErrorCode,
		Raw:            snapshot.raw,
	})
	if err != nil || len(data) > agentCacheMaxFileBytes {
		return
	}
	tmpPath := filepath.Join(dir, fmt.Sprintf(".agent-%d-%d.tmp", os.Getpid(), agentCacheNow().UnixNano()))
	if err := agentCacheWriteFile(tmpPath, data, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	if err := agentCacheRenameFile(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	pruneAgentCacheDir(dir, path)
}

func pruneAgentCacheDir(dir, keepPath string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := agentCacheNow()
	type cacheEntry struct {
		path    string
		modTime time.Time
	}
	kept := make([]cacheEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		info, err := agentCacheLstat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if fullPath != keepPath && now.Sub(info.ModTime()) > agentCacheTTL {
			_ = os.Remove(fullPath)
			continue
		}
		kept = append(kept, cacheEntry{path: fullPath, modTime: info.ModTime()})
	}
	if len(kept) <= agentCacheMaxEntries {
		return
	}
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].modTime.Before(kept[j].modTime)
	})
	excess := len(kept) - agentCacheMaxEntries
	for _, item := range kept {
		if excess <= 0 {
			break
		}
		if item.path == keepPath {
			continue
		}
		_ = os.Remove(item.path)
		excess--
	}
}
