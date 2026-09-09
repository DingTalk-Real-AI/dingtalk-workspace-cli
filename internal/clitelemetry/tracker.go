// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package clitelemetry owns the reviewed CLI tracking configuration and privacy
// projection. The process entry uses the official SDK with reviewed CLI defaults.
package clitelemetry

import (
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/clitrack"
)

type Config = clitrack.Config

type Identity struct {
	UserID   string
	UserName string
	CorpID   string
}

func IdentityFromProfile(profile *profilemetadata.ProfileMetadata) Identity {
	if profile == nil {
		return Identity{}
	}
	return Identity{UserID: strings.TrimSpace(profile.UserID), UserName: strings.TrimSpace(profile.UserName), CorpID: strings.TrimSpace(profile.CorpID)}
}

// DefaultIdentity is only for invocations with no profile flag. The general
// argv/profile resolver remains with the CLI. No credentials are opened,
// refreshed or written: this reads the same metadata-only auth API as core.
func DefaultIdentity(configDir string) (identity Identity) {
	defer func() {
		if recover() != nil {
			identity = Identity{}
		}
	}()
	profile, err := profilemetadata.ResolveReadOnly(configDir, "")
	if err != nil {
		return Identity{}
	}
	return IdentityFromProfile(profile)
}

func Configuration(version string, identity Identity, commandPath, errorMessage *string) Config {
	return Config{
		PID: "wcCRwZ", App: "dws", Version: version, UID: identity.UserID, Username: identity.UserName,
		NoCommandLine: true, NoCwd: true, NoAutomaticDimensions: true,
		FlushTimeout: 50 * time.Millisecond,
		ExtraFields: func() map[string]string {
			fields := map[string]string{"c9": *commandPath}
			if identity.CorpID != "" {
				fields["c10"] = identity.CorpID
			}
			if *errorMessage != "" {
				fields["c5"] = *errorMessage
			}
			return fields
		},
	}
}

// RenderedError prevents the SDK from printing an already presented error a
// second time; its reviewed summary is supplied through the shared c5 field.
type RenderedError struct{}

func (RenderedError) Error() string { return "" }

func Run(cfg Config, execute func() error, exitCode func(error) int) {
	clitrack.New(cfg).Run(execute, exitCode)
}
