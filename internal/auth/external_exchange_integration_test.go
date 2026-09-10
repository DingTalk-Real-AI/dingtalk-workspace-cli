package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestExternalExchangeIntegrationRejectsMismatchedSecretReference(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	if err := authKeychainSet(keychain.Service, secretAccountKey("other-app"), "other-secret"); err != nil {
		t.Fatal(err)
	}
	writeCredentialConfig(t, dir, "app", SecretInput{Ref: &SecretRef{Source: "keychain", ID: secretAccountKey("other-app")}})
	if _, _, _, err := resolveExternalExchangeClient(t.Context(), dir, "app", ""); err == nil {
		t.Fatal("accepted another application's secret reference")
	}
}

func TestExternalExchangeIntegrationDerivedSecretDoesNotMigrate(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			writeCredentialConfig(t, dir, "app", SecretInput{})
			account := secretAccountKey("app")
			if legacy {
				account = legacyClientSecretAccountKey("app")
			}
			if err := authKeychainSet(keychain.Service, account, "existing-secret"); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &authKeychainSet, func(string, string, string) error { t.Fatal("resolution wrote credentials"); return nil })
			testseam.Swap(t, &authKeychainRemove, func(string, string) error { t.Fatal("resolution removed credentials"); return nil })
			id, secret, source, err := resolveExternalExchangeClient(context.Background(), dir, "app", "")
			if err != nil || id != "app" || secret != "existing-secret" || source != "app" {
				t.Fatalf("derived credential not reused: %v", err)
			}
		})
	}
}

func TestExternalExchangeIntegrationLegacyCleanupFailureRollsBack(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	old := &TokenData{CorpID: "corp", UserID: "user", ClientID: "app", Source: "flag", AccessToken: "old-token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveTokenData(dir, old); err != nil {
		t.Fatal(err)
	}
	legacyKey := legacyClientSecretAccountKey("app")
	if err := authKeychainSet(keychain.Service, legacyKey, "old-secret"); err != nil {
		t.Fatal(err)
	}
	remove := authKeychainRemove
	testseam.Swap(t, &authKeychainRemove, func(service, account string) error {
		if account == legacyKey {
			return fmt.Errorf("injected legacy cleanup failure")
		}
		return remove(service, account)
	})
	fresh := *old
	fresh.AccessToken = "new-token"
	if err := persistExternalExchangeTokenWithSecret(dir, &fresh, "new-secret"); err == nil {
		t.Fatal("committed credentials that cannot be refreshed")
	}
	canonical, legacy, err := snapshotExchangeClientSecret("app")
	if err != nil || canonical != "" || legacy != "old-secret" {
		t.Fatal("original credentials were not restored")
	}
	saved, err := LoadTokenDataForProfile(dir, "corp:user")
	if err != nil || saved.AccessToken != "old-token" {
		t.Fatal("original token was not preserved")
	}
}

func TestExternalExchangeIntegrationRollbackPreservesBothSecretSlots(t *testing.T) {
	for _, values := range [][2]string{{"canonical", ""}, {"", "legacy"}, {"same", "same"}, {"canonical", "legacy"}} {
		t.Run(fmt.Sprintf("canonical=%t/legacy=%t/equal=%t", values[0] != "", values[1] != "", values[0] == values[1]), func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			old := &TokenData{CorpID: "corp", UserID: "user", ClientID: "app", Source: "flag", AccessToken: "old-token", ExpiresAt: time.Now().Add(time.Hour)}
			if err := SaveTokenData(dir, old); err != nil {
				t.Fatal(err)
			}
			keys := [2]string{secretAccountKey("app"), legacyClientSecretAccountKey("app")}
			for i, key := range keys {
				if values[i] != "" {
					if err := authKeychainSet(keychain.Service, key, values[i]); err != nil {
						t.Fatal(err)
					}
				}
			}
			testseam.Swap(t, &tokenUpsertProfile, func(string, *TokenData, bool) error { return fmt.Errorf("injected save failure") })
			fresh := *old
			fresh.AccessToken = "new-token"
			if err := persistExternalExchangeTokenWithSecret(dir, &fresh, "new-secret"); err == nil {
				t.Fatal("expected rollback")
			}
			for i, key := range keys {
				value, err := authKeychainGet(keychain.Service, key)
				if err != nil || value != values[i] {
					t.Fatal("secret slot was not restored exactly")
				}
			}
		})
	}
}
