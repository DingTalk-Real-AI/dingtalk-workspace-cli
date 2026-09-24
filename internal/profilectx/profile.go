// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package profilectx owns the process-local profile selector without importing
// authentication or transport packages.
package profilectx

import (
	"strings"
	"sync"
)

var (
	runtimeProfileMu sync.RWMutex
	runtimeProfile   string
	runtimeIdentity  Identity
	identityResolver IdentityResolver
)

// Identity is the non-sensitive account identity resolved for the active
// runtime profile. Keeping it beside the selector lets command packages bind
// durable state without importing the authentication package.
type Identity struct {
	CorpID string
	UserID string
}

// IdentityResolver resolves one explicit profile selector to its non-sensitive
// account identity. The application layer registers the auth-backed resolver
// so leaf command packages do not need to import the authentication package.
type IdentityResolver func(selector string) (Identity, error)

// Set records the explicit profile selector for the current process.
func Set(profile string) {
	runtimeProfileMu.Lock()
	defer runtimeProfileMu.Unlock()
	runtimeProfile = strings.TrimSpace(profile)
}

// Get returns the explicit process-local profile selector.
func Get() string {
	runtimeProfileMu.RLock()
	defer runtimeProfileMu.RUnlock()
	return runtimeProfile
}

// SetIdentity records the resolved account identity for the current process.
func SetIdentity(identity Identity) {
	runtimeProfileMu.Lock()
	defer runtimeProfileMu.Unlock()
	runtimeIdentity = Identity{CorpID: strings.TrimSpace(identity.CorpID), UserID: strings.TrimSpace(identity.UserID)}
}

// GetIdentity returns the resolved account identity for the active profile.
func GetIdentity() Identity {
	runtimeProfileMu.RLock()
	defer runtimeProfileMu.RUnlock()
	return runtimeIdentity
}

// RegisterIdentityResolver installs the application-owned resolver used when
// a leaf needs an exact identity before the transport runner starts.
func RegisterIdentityResolver(resolver IdentityResolver) {
	runtimeProfileMu.Lock()
	defer runtimeProfileMu.Unlock()
	identityResolver = resolver
}

// ResolveIdentity resolves an explicit selector without changing process-local
// selector or identity state. A nil resolver means the embedding application
// has not registered authentication support.
func ResolveIdentity(selector string) (Identity, error) {
	runtimeProfileMu.RLock()
	resolver := identityResolver
	runtimeProfileMu.RUnlock()
	if resolver == nil {
		return Identity{}, nil
	}
	identity, err := resolver(strings.TrimSpace(selector))
	if err != nil {
		return Identity{}, err
	}
	return Identity{CorpID: strings.TrimSpace(identity.CorpID), UserID: strings.TrimSpace(identity.UserID)}, nil
}
