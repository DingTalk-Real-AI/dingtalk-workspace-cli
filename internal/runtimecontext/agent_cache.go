// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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

type cachedAgentRecord struct {
	Version        int             `json:"version"`
	PayloadVersion string          `json:"payload_version"`
	Key            string          `json:"key"`
	CreatedAtUnix  int64           `json:"created_at_unix"`
	ErrorCode      int32           `json:"error_code"`
	Raw            json.RawMessage `json:"raw"`
}

var (
	agentCacheNow      = time.Now
	agentCacheAncestry = func() ([]ancestorProcess, bool) {
		return collectProcessAncestry(agentCacheMaxDepth)
	}
	agentCacheLookup = loadCachedAgentSnapshot
	agentCacheStore  = storeCachedAgentSnapshot
)

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
	material, err := json.Marshal(struct {
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
	if err != nil {
		return "", false
	}
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
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > agentCacheMaxFileBytes {
		return AgentSnapshot{}, false
	}
	data, err := os.ReadFile(path)
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
	now := agentCacheNow().Unix()
	age := now - record.CreatedAtUnix
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
	tmp, err := os.CreateTemp(dir, ".agent-*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
	}
}
