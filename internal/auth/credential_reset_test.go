package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// resetAppConfigCache clears cached app config so tests get a fresh load.
func resetAppConfigCache() {
	cachedAppConfigMu.Lock()
	cachedAppConfig = nil
	cachedAppConfigMu.Unlock()
	cachedAppConfigOnce = sync.Once{}

	cachedResolvedMu.Lock()
	cachedResolvedValid = false
	cachedResolvedID = ""
	cachedResolvedSecret = ""
	cachedResolvedMu.Unlock()
}

// ─── Issue #155: Defensive credential reset ────────────────────────────
//
// These tests verify that both DeviceFlowProvider and OAuthProvider always
// reset credential state and re-fetch clientID from MCP, regardless of what
// previous login methods left in app.json or runtime state.

func TestIssue155V2_OAuthLoginNoSource_ThenDeviceLogin_ResetsAndFetches(t *testing.T) {
	// Scenario: OAuth login saved app.json WITHOUT Source field (the original bug).
	// Device flow should ignore the stale clientID, reset state, and re-fetch from MCP.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	resetAppConfigCache()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	// Simulate OAuth login: saved app.json with clientId but NO Source field
	oauthAppJSON := `{"clientId":"ding-oauth-stale","clientSecret":"","createdAt":"2026-04-24T00:00:00+08:00"}`
	if err := os.WriteFile(filepath.Join(dir, appConfigFile), []byte(oauthAppJSON), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Verify precondition: ClientID() loads stale value from app.json
	resetAppConfigCache()
	gotID := ClientID()
	if gotID != "ding-oauth-stale" {
		t.Fatalf("precondition: ClientID() = %q, want 'ding-oauth-stale'", gotID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("precondition: IsClientIDFromMCP() should be false for app.json without Source")
	}

	// Now create a DeviceFlowProvider — it should pick up the stale clientID
	provider := NewDeviceFlowProvider(dir, nil)
	if provider.clientID != "ding-oauth-stale" {
		t.Fatalf("provider.clientID = %q, want 'ding-oauth-stale' (from app.json)", provider.clientID)
	}

	// Key assertion: after prepareCredentials(), the provider should have
	// cleared the stale clientID and be ready for MCP fetch.
	// We can't call Login() directly (needs real MCP server), but we can
	// verify the reset logic by calling the new method directly.
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false")
	}
}

func TestIssue155V2_LegacyAppJson_ThenDeviceLogin_ResetsAndFetches(t *testing.T) {
	// Scenario: Legacy app.json (no Source field at all) exists from an old CLI version.
	// Device flow should reset and re-fetch.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	resetAppConfigCache()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	// Write legacy app.json
	legacyJSON := `{"clientId":"ding-legacy-old","clientSecret":"","createdAt":"2026-01-01T00:00:00+08:00"}`
	if err := os.WriteFile(filepath.Join(dir, appConfigFile), []byte(legacyJSON), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	resetAppConfigCache()
	provider := NewDeviceFlowProvider(dir, nil)

	// Verify stale clientID was loaded
	if provider.clientID != "ding-legacy-old" {
		t.Fatalf("provider.clientID = %q, want 'ding-legacy-old'", provider.clientID)
	}

	// Reset should clear it
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
}

func TestIssue155V2_DirectAppJson_ThenDeviceLogin_ResetsAndFetches(t *testing.T) {
	// Scenario: User previously logged in with --client-id + --client-secret (direct mode).
	// app.json has a different clientId. Device flow should reset and re-fetch from MCP.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	resetAppConfigCache()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	// Simulate direct-mode app.json with clientSecret stored
	if err := SaveAppConfig(dir, &AppConfig{
		ClientID:     "ding-direct-custom",
		ClientSecret: PlainSecret("some-secret"),
	}); err != nil {
		t.Fatalf("SaveAppConfig error: %v", err)
	}

	resetAppConfigCache()
	provider := NewDeviceFlowProvider(dir, nil)

	// Verify the direct clientID was loaded
	if provider.clientID != "ding-direct-custom" {
		t.Fatalf("provider.clientID = %q, want 'ding-direct-custom'", provider.clientID)
	}

	// Reset should clear it
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false")
	}
}

func TestIssue155V2_MCPFlagAlreadySet_ThenDeviceLogin_StillResets(t *testing.T) {
	// Scenario: MCP flag is already set from a previous device login in same process.
	// Device flow should still reset and re-fetch to ensure freshness.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	// Simulate: MCP flag is already set from previous login
	SetClientIDFromMCP("ding-old-mcp")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	provider := NewDeviceFlowProvider(dir, nil)
	if provider.clientID != "ding-old-mcp" {
		t.Fatalf("provider.clientID = %q, want 'ding-old-mcp'", provider.clientID)
	}

	// Reset should clear both clientID and MCP flag
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false after reset")
	}
}

func TestIssue155V2_NoAppJson_DeviceLogin_StillWorks(t *testing.T) {
	// Scenario: No app.json exists (first time login). Device flow should work normally.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	resetAppConfigCache()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	provider := NewDeviceFlowProvider(dir, nil)

	// clientID should already be empty
	if provider.clientID != "" {
		t.Fatalf("provider.clientID = %q, want empty (no app.json)", provider.clientID)
	}

	// Reset should be a no-op but not crash
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false")
	}
}

// ─── OAuthProvider defensive reset (--force login) ─────────────────────

func TestIssue155V2_OAuthForceLogin_ResetsStaleCredentials(t *testing.T) {
	// Scenario: Previous login saved app.json with MCP-fetched clientID but
	// no Source marker. OAuth --force login should reset and re-fetch.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	resetAppConfigCache()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	// Simulate previous login: app.json with clientId but no Source
	staleJSON := `{"clientId":"ding-stale-oauth","clientSecret":"","createdAt":"2026-04-24T00:00:00+08:00"}`
	if err := os.WriteFile(filepath.Join(dir, appConfigFile), []byte(staleJSON), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	resetAppConfigCache()
	provider := NewOAuthProvider(dir, nil)

	// Verify stale clientID was loaded
	if provider.clientID != "ding-stale-oauth" {
		t.Fatalf("provider.clientID = %q, want 'ding-stale-oauth'", provider.clientID)
	}

	// Reset should clear it
	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false")
	}
}

func TestIssue155V2_OAuthForceLogin_MCPFlagSet_StillResets(t *testing.T) {
	// Scenario: MCP flag is already set. OAuth --force login should still reset.

	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	SetClientIDFromMCP("ding-old-mcp-oauth")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		resetAppConfigCache()
	})

	provider := NewOAuthProvider(dir, nil)

	provider.resetCredentialState()

	if provider.clientID != "" {
		t.Fatalf("after resetCredentialState: provider.clientID = %q, want empty", provider.clientID)
	}
	if IsClientIDFromMCP() {
		t.Fatal("after resetCredentialState: IsClientIDFromMCP() should be false after reset")
	}
}
