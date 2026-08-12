package smoke_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// TestCLISmoke_LegacyAuthSlotsMigrateThroughStatus exercises the upgrade
// boundary through a real CLI subprocess. Fixtures are deliberately written
// as version-1 JSON and raw historical keychain accounts; using the current
// auth SaveTokenData API here would only prove that the new format can read
// itself.
func TestCLISmoke_LegacyAuthSlotsMigrateThroughStatus(t *testing.T) {
	t.Run("v1.0.51 global-only slot", func(t *testing.T) {
		env, configDir := isolatedLegacyAuthCLIEnv(t)
		corpID, userID := legacyFixtureIdentity(t, "global")
		accessToken := "fixture-access-global"

		writeLegacyProfilesV1(t, configDir, []legacySmokeProfile{{
			Name: "Legacy Global Org", CorpID: corpID, CorpName: "Legacy Global Org", UserID: userID,
		}}, corpID)
		seedRawLegacyKeychainSlot(t, keychain.AccountToken, legacyRawToken(
			t, accessToken, corpID, "Legacy Global Org", "", "",
		))

		status := runLegacyAuthStatus(t, env)
		assertAuthenticatedStatus(t, status, corpID, userID)
		assertRawIdentitySlot(t, corpID, userID, accessToken)
		assertMigratedProfiles(t, configDir, map[string]string{corpID: userID})
	})

	t.Run("v1.0.52 migrates every organization", func(t *testing.T) {
		env, configDir := isolatedLegacyAuthCLIEnv(t)
		profiles := make([]legacySmokeProfile, 0, 3)
		wantIdentities := make(map[string]string, 3)
		wantAccess := make(map[string]string, 3)
		for i := 0; i < 3; i++ {
			corpID, userID := legacyFixtureIdentity(t, fmt.Sprintf("org-%d", i))
			corpName := fmt.Sprintf("Legacy Multi Org %d", i+1)
			accessToken := fmt.Sprintf("fixture-access-multi-%d", i+1)
			profiles = append(profiles, legacySmokeProfile{
				Name: corpName, CorpID: corpID, CorpName: corpName, UserID: userID,
			})
			wantIdentities[corpID] = userID
			wantAccess[corpID] = accessToken
			// v1.0.52 kept one token per organization. The token response could
			// omit userId even when the v1 profile registry still knew it.
			seedRawLegacyKeychainSlot(t, legacyOrganizationAccount(corpID), legacyRawToken(
				t, accessToken, corpID, corpName, "", "",
			))
		}
		current := profiles[1]
		writeLegacyProfilesV1(t, configDir, profiles, current.CorpID)
		// v1.0.52 also mirrored the selected organization in the global slot.
		seedRawLegacyKeychainSlot(t, keychain.AccountToken, legacyRawToken(
			t, wantAccess[current.CorpID], current.CorpID, current.CorpName, "", "",
		))

		status := runLegacyAuthStatus(t, env)
		assertAuthenticatedStatus(t, status, current.CorpID, current.UserID)
		for corpID, userID := range wantIdentities {
			assertRawIdentitySlot(t, corpID, userID, wantAccess[corpID])
		}
		assertMigratedProfiles(t, configDir, wantIdentities)
	})

	t.Run("v1.0.52 unresolved external account remains usable", func(t *testing.T) {
		env, configDir := isolatedLegacyAuthCLIEnv(t)
		corpID, _ := legacyFixtureIdentity(t, "external")
		accessToken := "fixture-access-external"
		writeLegacyProfilesV1(t, configDir, []legacySmokeProfile{{
			Name: "Legacy External Org", CorpID: corpID, CorpName: "Legacy External Org",
		}}, corpID)
		seedRawLegacyKeychainSlot(t, legacyOrganizationAccount(corpID), legacyRawToken(
			t, accessToken, corpID, "Legacy External Org", "", "",
		))
		seedRawLegacyKeychainSlot(t, keychain.AccountToken, legacyRawToken(
			t, accessToken, corpID, "Legacy External Org", "", "",
		))

		status := runLegacyAuthStatus(t, env)
		assertAuthenticatedStatus(t, status, corpID, "")
		assertRawOrganizationSlot(t, corpID, accessToken, "")

		var migrated legacyProfilesDocument
		readJSONFile(t, filepath.Join(configDir, "profiles.json"), &migrated)
		if migrated.Version != 2 || len(migrated.Profiles) != 1 {
			t.Fatalf("migrated unresolved profiles = %#v", migrated)
		}
		if migrated.Profiles[0].CorpID != corpID || migrated.Profiles[0].UserID != "" {
			t.Fatalf("unresolved external identity was changed or discarded: %#v", migrated.Profiles[0])
		}
	})

	t.Run("v1.0.53 v2 exact profile repairs global-only slot", func(t *testing.T) {
		for _, tc := range []struct {
			name              string
			globalTokenUserID bool
		}{
			{name: "global token omits user id"},
			{name: "global token user id matches", globalTokenUserID: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, configDir := isolatedLegacyAuthCLIEnv(t)
				corpID, userID := legacyFixtureIdentity(t, "v2-exact-"+tc.name)
				profile := legacySmokeProfile{
					Name: "Half Migrated Exact Account", CorpID: corpID,
					CorpName: "Half Migrated Exact Org", UserID: userID,
				}
				accessToken := "fixture-access-v2-exact-" + strings.ReplaceAll(tc.name, " ", "-")
				writeHalfMigratedProfilesV2(t, configDir, profile)

				tokenUserID := ""
				if tc.globalTokenUserID {
					tokenUserID = userID
				}
				// Reproduce the interrupted 1.0.53 migration exactly: the registry is
				// already v2, while token material still exists only in the global
				// compatibility account. Do not seed either the org or identity slot.
				seedRawLegacyKeychainSlot(t, keychain.AccountToken, legacyRawToken(
					t, accessToken, corpID, profile.CorpName, tokenUserID, "",
				))

				exactSelector := corpID + ":" + userID
				status := runLegacyAuthStatus(t, env, exactSelector)
				assertAuthenticatedStatus(t, status, corpID, userID)
				assertRawIdentitySlot(t, corpID, userID, accessToken)
				assertRawOrganizationSlot(t, corpID, accessToken, tokenUserID)
				assertMigratedProfiles(t, configDir, map[string]string{corpID: userID})
			})
		}
	})

	t.Run("v1.0.53 v2 unresolved profile repairs global-only organization slot", func(t *testing.T) {
		env, configDir := isolatedLegacyAuthCLIEnv(t)
		corpID, _ := legacyFixtureIdentity(t, "v2-unresolved")
		profile := legacySmokeProfile{
			Name: "Half Migrated External Account", CorpID: corpID,
			CorpName: "Half Migrated External Org",
		}
		accessToken := "fixture-access-v2-unresolved"
		writeHalfMigratedProfilesV2(t, configDir, profile)
		seedRawLegacyKeychainSlot(t, keychain.AccountToken, legacyRawToken(
			t, accessToken, corpID, profile.CorpName, "", "",
		))

		status := runLegacyAuthStatus(t, env, profile.Name)
		assertAuthenticatedStatus(t, status, corpID, "")
		assertRawOrganizationSlot(t, corpID, accessToken, "")

		var migrated legacyProfilesDocument
		readJSONFile(t, filepath.Join(configDir, "profiles.json"), &migrated)
		if migrated.Version != 2 || len(migrated.Profiles) != 1 {
			t.Fatalf("repaired unresolved v2 profiles = %#v", migrated)
		}
		if migrated.Profiles[0].CorpID != corpID || migrated.Profiles[0].UserID != "" {
			t.Fatalf("unresolved v2 identity was changed or discarded: %#v", migrated.Profiles[0])
		}
	})
}

// TestCLISmoke_ReservedUnresolvedAndExactProfilesStayIsolated protects the
// schema-v3 selector boundary through real CLI subprocesses. A blank external
// identity and a resolved account may legitimately coexist in one
// organization; selecting either must never read or rewrite the other's slot.
func TestCLISmoke_ReservedUnresolvedAndExactProfilesStayIsolated(t *testing.T) {
	env, configDir := isolatedLegacyAuthCLIEnv(t)
	corpID, exactUserID := legacyFixtureIdentity(t, "schema-v3-blank-exact")
	reservedSelector := "@legacy/" + base64.RawURLEncoding.EncodeToString([]byte(corpID))
	exactSelector := corpID + ":" + exactUserID
	blankAccessToken := "fixture-access-schema-v3-blank"
	exactAccessToken := "fixture-access-schema-v3-exact"
	blankRawToken := legacyRawToken(
		t, blankAccessToken, corpID, "Schema V3 Fixture Org", "", "",
	)
	exactRawToken := legacyRawToken(
		t, exactAccessToken, corpID, "Schema V3 Fixture Org", exactUserID, "Exact Fixture Account",
	)

	writeCoexistingProfilesV3(t, configDir, corpID, exactUserID, reservedSelector, exactSelector)
	organizationAccount := legacyOrganizationAccount(corpID)
	identityAccount := legacyIdentityAccount(corpID, exactUserID)
	seedRawLegacyKeychainSlot(t, organizationAccount, blankRawToken)
	seedRawLegacyKeychainSlot(t, identityAccount, exactRawToken)

	defaultStatus := runLegacyAuthStatus(t, env)
	assertAuthenticatedStatus(t, defaultStatus, corpID, "")
	exactStatus := runLegacyAuthStatus(t, env, exactSelector)
	assertAuthenticatedStatus(t, exactStatus, corpID, exactUserID)

	assertRawKeychainSlotUnchanged(t, organizationAccount, blankRawToken)
	assertRawKeychainSlotUnchanged(t, identityAccount, exactRawToken)

	var profiles legacyProfilesDocument
	readJSONFile(t, filepath.Join(configDir, "profiles.json"), &profiles)
	if profiles.Version != 3 {
		t.Fatalf("profiles version = %d, want 3", profiles.Version)
	}
	if profiles.CurrentProfile != reservedSelector {
		t.Fatalf("current profile = %q, want reserved blank selector %q", profiles.CurrentProfile, reservedSelector)
	}
	if profiles.OrgCurrentProfiles[corpID] != exactSelector {
		t.Fatalf(
			"orgCurrentProfiles[%q] = %q, want exact selector %q",
			corpID,
			profiles.OrgCurrentProfiles[corpID],
			exactSelector,
		)
	}
	if len(profiles.Profiles) != 2 {
		t.Fatalf("profiles = %#v, want one blank and one exact identity", profiles.Profiles)
	}
	identities := map[string]int{}
	for _, profile := range profiles.Profiles {
		if profile.CorpID != corpID {
			t.Fatalf("profile corpId = %q, want %q", profile.CorpID, corpID)
		}
		identities[profile.UserID]++
	}
	if identities[""] != 1 || identities[exactUserID] != 1 {
		t.Fatalf("profile identities = %#v, want one blank and one exact %q", identities, exactUserID)
	}
}

type legacySmokeProfile struct {
	Name     string
	CorpID   string
	CorpName string
	UserID   string
	UserName string
}

type legacyProfilesDocument struct {
	Version            int               `json:"version"`
	CurrentProfile     string            `json:"currentProfile"`
	OrgCurrentProfiles map[string]string `json:"orgCurrentProfiles"`
	Profiles           []struct {
		CorpID string `json:"corpId"`
		UserID string `json:"userId"`
	} `json:"profiles"`
}

type legacyAuthStatus struct {
	Success       bool   `json:"success"`
	Authenticated bool   `json:"authenticated"`
	Refreshed     bool   `json:"refreshed"`
	TokenValid    bool   `json:"token_valid"`
	CorpID        string `json:"corp_id"`
	UserID        string `json:"user_id"`
}

func isolatedLegacyAuthCLIEnv(t *testing.T) ([]string, string) {
	t.Helper()
	env := isolatedCLIEnv(t)
	configDir := smokeEnvValue(t, env, "DWS_CONFIG_DIR")
	// File-backed platforms are already isolated by DWS_KEYCHAIN_DIR. Windows
	// uses HKCU, so give the parent seeder and CLI child the same throwaway
	// registry namespace as well.
	namespace := filepath.Join(configDir, "legacy-auth-smoke")
	t.Setenv(keychain.TestNamespaceEnv, namespace)
	env = append(env, keychain.TestNamespaceEnv+"="+namespace)
	sort.Strings(env)

	if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
		t.Fatalf("clear isolated historical keychain: %v", err)
	}
	t.Cleanup(func() {
		_ = keychain.RemoveAuthTokenEntries(keychain.Service)
	})
	return env, configDir
}

func smokeEnvValue(t *testing.T, env []string, key string) string {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	t.Fatalf("CLI environment does not contain %s", key)
	return ""
}

func legacyFixtureIdentity(t *testing.T, suffix string) (string, string) {
	t.Helper()
	sum := sha256.Sum256([]byte(t.Name() + "\x00" + suffix))
	return fmt.Sprintf("ding_fixture_%x", sum[:6]), fmt.Sprintf("fixture-user-%x", sum[6:12])
}

func writeLegacyProfilesV1(t *testing.T, configDir string, profiles []legacySmokeProfile, currentCorpID string) {
	t.Helper()
	rawProfiles := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		raw := map[string]any{
			"name":      profile.Name,
			"corpId":    profile.CorpID,
			"corpName":  profile.CorpName,
			"clientId":  "fixture-legacy-client",
			"status":    "active",
			"expiresAt": "2099-01-02T03:04:05Z",
		}
		if profile.UserID != "" {
			raw["userId"] = profile.UserID
		}
		if profile.UserName != "" {
			raw["userName"] = profile.UserName
		}
		rawProfiles = append(rawProfiles, raw)
	}
	document := map[string]any{
		"version":        1,
		"primaryProfile": profiles[0].CorpID,
		"currentProfile": currentCorpID,
		"profiles":       rawProfiles,
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal raw v1 profiles fixture: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create raw v1 config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "profiles.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write raw v1 profiles fixture: %v", err)
	}
}

func writeHalfMigratedProfilesV2(t *testing.T, configDir string, profile legacySmokeProfile) {
	t.Helper()
	selector := profile.CorpID
	orgCurrentProfiles := map[string]string{}
	if profile.UserID != "" {
		selector = profile.CorpID + ":" + profile.UserID
		orgCurrentProfiles[profile.CorpID] = selector
	}
	rawProfile := map[string]any{
		"name":      profile.Name,
		"corpId":    profile.CorpID,
		"corpName":  profile.CorpName,
		"clientId":  "fixture-legacy-client",
		"status":    "active",
		"expiresAt": "2099-01-02T03:04:05Z",
	}
	if profile.UserID != "" {
		rawProfile["userId"] = profile.UserID
	}
	document := map[string]any{
		"version":            2,
		"primaryProfile":     selector,
		"currentProfile":     selector,
		"orgCurrentProfiles": orgCurrentProfiles,
		"profiles":           []map[string]any{rawProfile},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal raw half-migrated v2 profiles fixture: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create raw v2 config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "profiles.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write raw half-migrated v2 profiles fixture: %v", err)
	}
}

func writeCoexistingProfilesV3(
	t *testing.T,
	configDir string,
	corpID string,
	exactUserID string,
	reservedSelector string,
	exactSelector string,
) {
	t.Helper()
	document := map[string]any{
		"version":            3,
		"primaryProfile":     exactSelector,
		"currentProfile":     reservedSelector,
		"previousProfile":    exactSelector,
		"orgCurrentProfiles": map[string]string{corpID: exactSelector},
		"profiles": []map[string]any{
			{
				"name":      "Schema V3 Fixture Org",
				"corpId":    corpID,
				"corpName":  "Schema V3 Fixture Org",
				"clientId":  "fixture-legacy-client",
				"status":    "active",
				"expiresAt": "2099-01-02T03:04:05Z",
			},
			{
				"name":      "Schema V3 Exact Fixture",
				"corpId":    corpID,
				"corpName":  "Schema V3 Fixture Org",
				"userId":    exactUserID,
				"userName":  "Exact Fixture Account",
				"clientId":  "fixture-legacy-client",
				"status":    "active",
				"expiresAt": "2099-01-02T03:04:05Z",
			},
		},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal raw coexisting v3 profiles fixture: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create raw v3 config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "profiles.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write raw coexisting v3 profiles fixture: %v", err)
	}
}

func legacyRawToken(t *testing.T, accessToken, corpID, corpName, userID, userName string) string {
	t.Helper()
	document := map[string]any{
		"access_token":       accessToken,
		"refresh_token":      "fixture-refresh-" + accessToken,
		"persistent_code":    "fixture-persistent-" + accessToken,
		"expires_at":         "2099-01-02T03:04:05Z",
		"refresh_expires_at": "2099-02-02T03:04:05Z",
		"corp_id":            corpID,
		"corp_name":          corpName,
		"client_id":          "fixture-legacy-client",
		"source":             "mcp",
	}
	if userID != "" {
		document["user_id"] = userID
	}
	if userName != "" {
		document["user_name"] = userName
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal raw historical token: %v", err)
	}
	return string(data)
}

func seedRawLegacyKeychainSlot(t *testing.T, account, rawToken string) {
	t.Helper()
	// Write the old account directly. Do not route setup through auth.SaveTokenData,
	// which would eagerly create the very identity slot this E2E must observe.
	if err := keychain.Set(keychain.Service, account, rawToken); err != nil {
		t.Fatalf("write raw historical keychain account %q: %v", account, err)
	}
}

func assertRawKeychainSlotUnchanged(t *testing.T, account, want string) {
	t.Helper()
	got, err := keychain.Get(keychain.Service, account)
	if err != nil {
		t.Fatalf("read raw keychain account %q: %v", account, err)
	}
	if got != want {
		t.Fatalf("raw keychain account %q changed\nwant:\n%s\ngot:\n%s", account, want, got)
	}
}

func runLegacyAuthStatus(t *testing.T, env []string, profile ...string) legacyAuthStatus {
	t.Helper()
	if len(profile) > 1 {
		t.Fatalf("auth status accepts at most one profile selector, got %d", len(profile))
	}
	args := []string{"--format", "json", "auth", "status"}
	if len(profile) == 1 && strings.TrimSpace(profile[0]) != "" {
		args = append(args, "--profile", profile[0])
	}
	stdout, stderr, err := runCLI(t, env, args...)
	if err != nil {
		t.Fatalf("dws auth status failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	var status legacyAuthStatus
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("dws auth status returned invalid JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	return status
}

func assertAuthenticatedStatus(t *testing.T, status legacyAuthStatus, corpID, userID string) {
	t.Helper()
	if !status.Success || !status.Authenticated || !status.TokenValid || status.Refreshed {
		t.Fatalf("auth status did not reuse valid historical login: %#v", status)
	}
	if status.CorpID != corpID || status.UserID != userID {
		t.Fatalf("auth status identity = %q:%q, want %q:%q", status.CorpID, status.UserID, corpID, userID)
	}
}

func assertRawIdentitySlot(t *testing.T, corpID, userID, accessToken string) {
	t.Helper()
	account := legacyIdentityAccount(corpID, userID)
	raw, err := keychain.Get(keychain.Service, account)
	if err != nil {
		t.Fatalf("read migrated identity account %q: %v", account, err)
	}
	if raw == "" {
		t.Fatalf("migrated identity account %q is missing", account)
	}
	var token map[string]any
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		t.Fatalf("parse migrated identity account %q: %v", account, err)
	}
	if token["access_token"] != accessToken || token["corp_id"] != corpID || token["user_id"] != userID {
		t.Fatalf("migrated identity account %q = %#v", account, token)
	}
}

func assertRawOrganizationSlot(t *testing.T, corpID, accessToken, userID string) {
	t.Helper()
	account := legacyOrganizationAccount(corpID)
	raw, err := keychain.Get(keychain.Service, account)
	if err != nil {
		t.Fatalf("read organization account %q: %v", account, err)
	}
	var token map[string]any
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		t.Fatalf("parse organization account %q: %v", account, err)
	}
	if token["access_token"] != accessToken || token["corp_id"] != corpID {
		t.Fatalf("organization account %q = %#v", account, token)
	}
	if got, _ := token["user_id"].(string); got != userID {
		t.Fatalf("organization account %q user_id = %q, want %q", account, got, userID)
	}
}

func assertMigratedProfiles(t *testing.T, configDir string, identities map[string]string) {
	t.Helper()
	var migrated legacyProfilesDocument
	readJSONFile(t, filepath.Join(configDir, "profiles.json"), &migrated)
	if migrated.Version != 2 {
		t.Fatalf("profiles version = %d, want 2", migrated.Version)
	}
	if len(migrated.Profiles) != len(identities) {
		t.Fatalf("migrated profile count = %d, want %d", len(migrated.Profiles), len(identities))
	}
	for corpID, userID := range identities {
		selector := corpID + ":" + userID
		if migrated.OrgCurrentProfiles[corpID] != selector {
			t.Errorf("orgCurrentProfiles[%q] = %q, want %q", corpID, migrated.OrgCurrentProfiles[corpID], selector)
		}
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func legacyOrganizationAccount(corpID string) string {
	return keychain.AccountToken + ":" + strings.TrimSpace(corpID)
}

func legacyIdentityAccount(corpID, userID string) string {
	identity := strings.TrimSpace(corpID) + "\x00" + strings.TrimSpace(userID)
	return fmt.Sprintf("%s:id:%x", keychain.AccountToken, sha256.Sum256([]byte(identity)))
}
