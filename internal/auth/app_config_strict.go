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

package auth

import (
	"errors"
	"fmt"
	"os"
)

// CredentialSource identifies where a particular credential field
// (ClientID or ClientSecret) was loaded from. It is exposed in
// `dws event status` and the HelloAck IPC frame so users can verify which
// credential channel is actually in use — important because env vars,
// keychain, and config file can all coexist and silently override each
// other (see plan §1 决策 "凭证来源拆字段").
type CredentialSource string

const (
	CredentialSourceUnknown     CredentialSource = "unknown"
	CredentialSourceEnv         CredentialSource = "env"
	CredentialSourceAppConfig   CredentialSource = "app_config"   // value pulled from app config (plain or SecretRef metadata)
	CredentialSourceKeychain    CredentialSource = "keychain"     // SecretRef resolved through OS keychain
	CredentialSourcePlainConfig CredentialSource = "plain_config" // SecretInput stored as plaintext in config file (insecure but supported)
)

// Strict resolver error sentinels. Use errors.Is to distinguish failure
// modes; see plan §8 strict resolver decision (4 classes).
var (
	// ErrAppConfigMissing — no app config file on disk AND no env-var
	// credentials present. Prompt the user to either `dws config init` or
	// set DWS_CLIENT_ID + DWS_CLIENT_SECRET.
	ErrAppConfigMissing = errors.New("app config missing: run `dws config init` or set DWS_CLIENT_ID/DWS_CLIENT_SECRET env vars")
	// ErrClientIDEmpty — neither env nor config supplies a non-empty ClientID.
	ErrClientIDEmpty = errors.New("ClientID is empty")
	// ErrClientSecretEmpty — there's a ClientID but ClientSecret resolved to "".
	ErrClientSecretEmpty = errors.New("ClientSecret is empty")
	// ErrSecretResolve — the secret-resolution backend (keychain) failed
	// unrecoverably. Typically headless Linux without gnome-keyring, locked
	// macOS keychain, or CI sandboxes. Suggest the env-var fallback.
	ErrSecretResolve = errors.New("ClientSecret resolution failed (keychain unavailable?); try DWS_CLIENT_ID/DWS_CLIENT_SECRET env vars")
)

// Env var names used by the env fallback channel. Must be set as a pair —
// any single-variable configuration is rejected so users cannot accidentally
// "set the env half-way" and silently fall back to keychain.
const (
	EnvClientID     = "DWS_CLIENT_ID"
	EnvClientSecret = "DWS_CLIENT_SECRET"
)

// ResolveAppCredentialsStrict is the credentials channel used by the event
// subsystem (and by future commands that need fine-grained failure
// reporting). It distinguishes 4 failure classes and reports the source of
// each successfully-resolved field separately.
//
// Resolution order:
//  1. Env var override: if BOTH DWS_CLIENT_ID and DWS_CLIENT_SECRET are
//     set non-empty, use them as a pair and skip keychain/config entirely.
//     Single-variable configuration is detected and reported via the
//     EnvHalfSet flag in the warning channel (callers MAY log a warning).
//  2. App config from disk:
//     - ClientID from cfg.ClientID
//     - ClientSecret from ResolveSecret(cfg.ClientSecret):
//     - SecretInput.IsPlain() → CredentialSourcePlainConfig
//     - SecretRef → CredentialSourceKeychain (or whatever Ref.Source says)
//
// Empty returns: clientID and secret may be empty when err is non-nil;
// callers must NOT use them in that case.
func ResolveAppCredentialsStrict(configDir string) (
	clientID, secret string,
	clientIDSource, secretSource CredentialSource,
	err error,
) {
	// Step 1: env var fallback (atomic pair)
	envID := os.Getenv(EnvClientID)
	envSecret := os.Getenv(EnvClientSecret)
	if envID != "" && envSecret != "" {
		return envID, envSecret, CredentialSourceEnv, CredentialSourceEnv, nil
	}
	// Note: if only one of the two is set we explicitly do NOT use it.
	// The half-set warning is surfaced via EnvHalfSet() so the CLI can
	// stderr-warn the user during preflight.

	// Step 2: app config from disk
	cfg, loadErr := LoadAppConfig(configDir)
	if loadErr != nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown,
			fmt.Errorf("load app config: %w", loadErr)
	}
	if cfg == nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrAppConfigMissing
	}

	if cfg.ClientID == "" {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientIDEmpty
	}
	clientID = cfg.ClientID
	clientIDSource = CredentialSourceAppConfig

	// Resolve secret. Source depends on the SecretInput shape:
	// - IsPlain (no Ref) → it's stored as plaintext in the config file
	// - has Ref → it's a SecretRef pointing at keychain/file
	wasPlain := cfg.ClientSecret.IsPlain()
	resolved, resolveErr := ResolveSecret(cfg.ClientSecret)
	if resolveErr != nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown,
			fmt.Errorf("%w: %v", ErrSecretResolve, resolveErr)
	}
	if resolved == "" {
		return "", "", clientIDSource, CredentialSourceUnknown, ErrClientSecretEmpty
	}

	secret = resolved
	if wasPlain {
		secretSource = CredentialSourcePlainConfig
	} else {
		// For SecretRef we map Source verbatim (keychain / file / future)
		switch cfg.ClientSecret.Ref.Source {
		case "keychain":
			secretSource = CredentialSourceKeychain
		default:
			// File-backed secrets share the "plain_config" category from
			// the consumer's perspective: stored as readable bytes outside
			// keychain. Status output renders them as "plain_config" so
			// users see "secret is not in keychain".
			secretSource = CredentialSourcePlainConfig
		}
	}

	return clientID, secret, clientIDSource, secretSource, nil
}

// EnvHalfSet reports whether exactly one of (DWS_CLIENT_ID, DWS_CLIENT_SECRET)
// is set. Used by CLI preflight to emit a clear stderr warning of the form:
//
//	WARN: DWS_CLIENT_ID is set but DWS_CLIENT_SECRET is not — env fallback
//	      disabled; using keychain/app config. Set both or unset both to
//	      avoid this warning.
//
// The strict resolver itself does NOT log; logging is the caller's job.
func EnvHalfSet() bool {
	id := os.Getenv(EnvClientID) != ""
	secret := os.Getenv(EnvClientSecret) != ""
	return id != secret
}
