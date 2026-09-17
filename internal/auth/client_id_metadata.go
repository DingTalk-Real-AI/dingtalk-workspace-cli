// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package auth

import (
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// ClientIDMetadata follows ClientID's local precedence without resolving a
// secret or using the process-wide app credential cache. The caller's config
// directory is authoritative, including when several accounts share a process.
// This accessor never fetches or persists credentials.
func ClientIDMetadata(configDir string) string {
	clientMu.RLock()
	override := runtimeClientID
	clientMu.RUnlock()
	if id := strings.TrimSpace(override); id != "" {
		return id
	}
	if id := strings.TrimSpace(edition.Get().AuthClientID); id != "" {
		return id
	}
	if cfg, err := LoadAppConfig(configDir); err == nil && cfg != nil {
		if id := strings.TrimSpace(cfg.ClientID); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(os.Getenv("DWS_CLIENT_ID")); id != "" {
		return id
	}
	if id := strings.TrimSpace(defaultAuthClientID); !strings.HasPrefix(id, "<") {
		return id
	}
	return ""
}
