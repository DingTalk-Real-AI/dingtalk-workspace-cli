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

// Package config provides shared constants used across multiple internal
// packages. Only cross-cutting values belong here; package-private
// constants should remain in their own package.
package config

import (
	"os"
	"time"
)

// ── File permissions ────────────────────────────────────────────────────
// These are used consistently by auth, security, and cache packages to
// protect sensitive data on disk.

const (
	// DirPerm is the permission mode for directories that hold sensitive
	// data (token store, cache, lock files). Owner-only rwx.
	DirPerm os.FileMode = 0o700

	// FilePerm is the permission mode for sensitive files (encrypted
	// tokens, cache entries, lock files). Owner-only rw.
	FilePerm os.FileMode = 0o600
)

// ── HTTP timeouts ───────────────────────────────────────────────────────
// Shared across transport, market, auth, and device-flow packages.

const (
	// HTTPTimeout is the default timeout for outgoing HTTP requests
	// (market registry, MCP JSON-RPC, device flow).
	HTTPTimeout = 30 * time.Second

	// OAuthTimeout is the timeout for OAuth token exchange/refresh
	// requests, which are latency-sensitive and should fail fast.
	OAuthTimeout = 15 * time.Second

	// DiscoveryTimeout bounds the time spent on live registry + runtime
	// discovery before falling back to cache.
	DiscoveryTimeout = 10 * time.Second

	// LockTimeout is how long to wait when acquiring a cross-process
	// file lock for token operations.
	LockTimeout = 10 * time.Second
)

// ── Response limits ─────────────────────────────────────────────────────

const (
	// MaxResponseBodySize limits the amount of data read from a single
	// HTTP response to prevent memory exhaustion from malicious servers.
	MaxResponseBodySize = 10 * 1024 * 1024 // 10 MB
)

// ── Cache ───────────────────────────────────────────────────────────────

const (
	// DefaultPartition is the cache partition used when no tenant/org
	// context is available.
	DefaultPartition = "default/default"
)

// ── Auth flow timeouts ──────────────────────────────────────────────────

const (
	// ManualTokenExpiry is the default lifetime for manually imported tokens.
	ManualTokenExpiry = 24 * time.Hour

	// DeviceFlowTimeout is the maximum wait time for device-flow authorization.
	DeviceFlowTimeout = 16 * time.Minute

	// OAuthFlowTimeout is the maximum wait time for browser-based OAuth.
	OAuthFlowTimeout = 6 * time.Minute

	// DefaultAccessTokenExpiry is the default access token lifetime in seconds
	// when the server does not return an explicit expires_in value.
	DefaultAccessTokenExpiry = 7200

	// DefaultRefreshTokenLifetime is the default refresh token lifetime.
	DefaultRefreshTokenLifetime = 30 * 24 * time.Hour
)

// ── Market ──────────────────────────────────────────────────────────────

const (
	// DefaultFetchServersLimit is the maximum number of servers to fetch
	// from the market registry in a single request.
	DefaultFetchServersLimit = 200
)

// ── Upload limits ───────────────────────────────────────────────────────

const (
	// MaxUploadFileSize is the maximum file size for attachment uploads.
	MaxUploadFileSize int64 = 100 * 1024 * 1024 // 100 MB
)
