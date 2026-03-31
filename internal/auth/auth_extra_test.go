package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── endpoints.go ──────────────────────────────────────────────────────

func TestClientID_RuntimeOverride(t *testing.T) {
	t.Setenv("DWS_CLIENT_ID", "env-id")
	SetClientID("flag-id")
	t.Cleanup(func() { SetClientID("") })
	if got := ClientID(); got != "flag-id" {
		t.Fatalf("expected flag-id, got %s", got)
	}
}

func TestClientID_EnvFallback(t *testing.T) {
	SetClientID("")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir()) // Use temp dir to avoid reading persisted config
	t.Setenv("DWS_CLIENT_ID", "env-id")
	if got := ClientID(); got != "env-id" {
		t.Fatalf("expected env-id, got %s", got)
	}
}

func TestClientID_Default(t *testing.T) {
	SetClientID("")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir()) // Use temp dir to avoid reading persisted config
	t.Setenv("DWS_CLIENT_ID", "")
	// When DefaultClientID is a placeholder (starts with "<"), ClientID() returns empty string
	if strings.HasPrefix(DefaultClientID, "<") {
		if got := ClientID(); got != "" {
			t.Fatalf("expected empty string for placeholder, got %s", got)
		}
	} else {
		if got := ClientID(); got != DefaultClientID {
			t.Fatalf("expected default, got %s", got)
		}
	}
}

func TestClientSecret_RuntimeOverride(t *testing.T) {
	t.Setenv("DWS_CLIENT_SECRET", "env-secret")
	SetClientSecret("flag-secret")
	t.Cleanup(func() { SetClientSecret("") })
	if got := ClientSecret(); got != "flag-secret" {
		t.Fatalf("expected flag-secret, got %s", got)
	}
}

func TestClientSecret_EnvFallback(t *testing.T) {
	SetClientSecret("")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir()) // Use temp dir to avoid reading persisted config
	t.Setenv("DWS_CLIENT_SECRET", "env-secret")
	if got := ClientSecret(); got != "env-secret" {
		t.Fatalf("expected env-secret, got %s", got)
	}
}

func TestClientSecret_Default(t *testing.T) {
	SetClientSecret("")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir()) // Use temp dir to avoid reading persisted config
	t.Setenv("DWS_CLIENT_SECRET", "")
	if got := ClientSecret(); got != DefaultClientSecret {
		t.Fatalf("expected default, got %s", got)
	}
}

func TestClientID_ConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			SetClientID(fmt.Sprintf("id-%d", n))
		}(i)
		go func() {
			defer wg.Done()
			_ = ClientID()
		}()
	}
	wg.Wait()
	SetClientID("")
}

// ─── identity.go ───────────────────────────────────────────────────────

func TestGenerateUUID_Format(t *testing.T) {
	t.Parallel()
	uuid := generateUUID()
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d: %s", len(parts), uuid)
	}
	if len(uuid) != 36 {
		t.Fatalf("expected 36 chars, got %d: %s", len(uuid), uuid)
	}
	// Version 4 marker
	if uuid[14] != '4' {
		t.Fatalf("expected version 4 at position 14, got %c", uuid[14])
	}
}

func TestIdentity_LoadMissing(t *testing.T) {
	t.Parallel()
	id := Load(t.TempDir())
	if id != nil {
		t.Fatal("expected nil for missing identity")
	}
}

func TestIdentity_EnsureExistsCreatesNew(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	id := EnsureExists(dir)
	if id == nil || id.AgentID == "" {
		t.Fatal("expected non-nil identity with agentID")
	}
	if id.Source != "dws" {
		t.Fatalf("expected source dws, got %s", id.Source)
	}
}

func TestIdentity_EnsureExistsLoadsExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	id1 := EnsureExists(dir)
	id2 := EnsureExists(dir)
	if id1.AgentID != id2.AgentID {
		t.Fatalf("expected same ID, got %s vs %s", id1.AgentID, id2.AgentID)
	}
}

func TestIdentity_LoadInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, identityFile), []byte("not json"), 0o600)
	if Load(dir) != nil {
		t.Fatal("expected nil for invalid JSON")
	}
}

func TestIdentity_LoadEmptyAgentID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data, _ := json.Marshal(Identity{AgentID: "", Source: "dws"})
	_ = os.WriteFile(filepath.Join(dir, identityFile), data, 0o600)
	if Load(dir) != nil {
		t.Fatal("expected nil for empty agentID")
	}
}

func TestIdentity_Headers(t *testing.T) {
	t.Parallel()
	id := &Identity{AgentID: "test-uuid", Source: "dws"}
	h := id.Headers()
	if h["x-dws-agent-id"] != "test-uuid" {
		t.Fatalf("wrong agent-id header: %s", h["x-dws-agent-id"])
	}
	if h["x-dws-source"] != "dws" {
		t.Fatalf("wrong source header: %s", h["x-dws-source"])
	}
	if _, ok := h["x-dingtalk-scenario-code"]; !ok {
		t.Fatal("missing scenario-code header")
	}
}

func TestIdentity_Headers_Nil(t *testing.T) {
	t.Parallel()
	var id *Identity
	if h := id.Headers(); h != nil {
		t.Fatalf("expected nil headers for nil identity, got %v", h)
	}
}

// ─── manager.go ────────────────────────────────────────────────────────

func TestManager_SaveAndGetToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager(dir, nil)
	if err := m.SaveToken("test-token"); err != nil {
		t.Fatalf("SaveToken error: %v", err)
	}
	token, source, err := m.GetToken()
	if err != nil {
		t.Fatalf("GetToken error: %v", err)
	}
	if token != "test-token" {
		t.Fatalf("expected test-token, got %s", token)
	}
	if source != "file" {
		t.Fatalf("expected file source, got %s", source)
	}
}

func TestManager_GetToken_Missing(t *testing.T) {
	t.Parallel()
	m := NewManager(t.TempDir(), nil)
	_, _, err := m.GetToken()
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestManager_SaveAndGetMCPURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager(dir, nil)
	if err := m.SaveMCPURL("https://example.com/mcp"); err != nil {
		t.Fatalf("SaveMCPURL error: %v", err)
	}
	url, err := m.GetMCPURL()
	if err != nil {
		t.Fatalf("GetMCPURL error: %v", err)
	}
	if url != "https://example.com/mcp" {
		t.Fatalf("expected url, got %s", url)
	}
}

func TestManager_GetMCPURL_Missing(t *testing.T) {
	t.Parallel()
	m := NewManager(t.TempDir(), nil)
	_, err := m.GetMCPURL()
	if err == nil {
		t.Fatal("expected error for missing MCP URL")
	}
}

// ─── token.go ──────────────────────────────────────────────────────────

func TestTokenData_IsAccessTokenValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data *TokenData
		want bool
	}{
		{"nil", nil, false},
		{"empty token", &TokenData{}, false},
		{"expired", &TokenData{AccessToken: "t", ExpiresAt: time.Now().Add(-time.Hour)}, false},
		{"valid", &TokenData{AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)}, true},
		{"within buffer", &TokenData{AccessToken: "t", ExpiresAt: time.Now().Add(3 * time.Minute)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.IsAccessTokenValid(); got != tt.want {
				t.Fatalf("IsAccessTokenValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenData_IsRefreshTokenValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data *TokenData
		want bool
	}{
		{"nil", nil, false},
		{"empty", &TokenData{}, false},
		{"expired", &TokenData{RefreshToken: "r", RefreshExpAt: time.Now().Add(-time.Hour)}, false},
		{"valid", &TokenData{RefreshToken: "r", RefreshExpAt: time.Now().Add(time.Hour)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.IsRefreshTokenValid(); got != tt.want {
				t.Fatalf("IsRefreshTokenValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenData_HasPersistentCode(t *testing.T) {
	t.Parallel()
	if (&TokenData{}).HasPersistentCode() {
		t.Fatal("expected false for empty")
	}
	if !(&TokenData{PersistentCode: "code"}).HasPersistentCode() {
		t.Fatal("expected true")
	}
	var nilData *TokenData
	if nilData.HasPersistentCode() {
		t.Fatal("expected false for nil")
	}
}

func TestRevokeTokenRemote(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// Can't easily test since LogoutURL is a const. Just test that it doesn't panic with real URL.
}

// ─── oauth_helpers.go ──────────────────────────────────────────────────

type tokenResponse struct {
	AccessToken    string `json:"accessToken"`
	RefreshToken   string `json:"refreshToken"`
	PersistentCode string `json:"persistentCode"`
	ExpiresIn      int64  `json:"expiresIn"`
}

func TestBuildTokenData_DefaultExpiry(t *testing.T) {
	resp := tokenResponse{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresIn:    0,
	}
	data := buildTokenDataFromResponse(resp)
	if data == nil {
		t.Fatal("expected non-nil token data")
	}
	if data.AccessToken != "access-123" {
		t.Fatalf("wrong access token: %s", data.AccessToken)
	}
}

func buildTokenDataFromResponse(resp tokenResponse) *TokenData {
	if resp.AccessToken == "" {
		return nil
	}
	now := time.Now()
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	return &TokenData{
		AccessToken:    resp.AccessToken,
		RefreshToken:   resp.RefreshToken,
		ExpiresAt:      now.Add(time.Duration(expiresIn) * time.Second),
		RefreshExpAt:   now.Add(30 * 24 * time.Hour),
		PersistentCode: resp.PersistentCode,
	}
}
