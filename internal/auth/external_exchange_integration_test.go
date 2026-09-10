package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

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
