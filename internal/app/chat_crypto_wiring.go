// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/messagecrypto"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
)

func init() {
	helpers.SetChatCryptoClient(newAppMessageCryptoClient())
}

func newAppMessageCryptoClient() *messagecrypto.Client {
	return &messagecrypto.Client{
		Identity: func(ctx context.Context, configDir string) (messagecrypto.Identity, error) {
			identity, err := msgcrypto.CurrentIdentity(ctx, configDir)
			return messagecrypto.Identity{CorpID: identity.CorpID, StaffID: identity.StaffID}, err
		},
		OpenSession: func(ctx context.Context, opts messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			session, err := msgcrypto.OpenSession(ctx, msgcrypto.SessionOptions{
				ConfigDir:           opts.ConfigDir,
				CLIVersion:          firstNonEmptyAppCrypto(opts.CLIVersion, RawVersion()),
				KeyServer:           firstNonEmptyAppCrypto(opts.KeyServer, msgcrypto.DefaultSafeChatKeyServer),
				AllowedRedirectHost: firstNonEmptyAppCrypto(opts.AllowedRedirectHost, msgcrypto.DefaultSafeChatRedirectHost),
				KeystoreDir:         opts.KeystoreDir,
			})
			if err != nil {
				return nil, err
			}
			return &messagecrypto.Session{
				Cipher:  session.Cipher,
				CorpID:  session.CorpID,
				StaffID: session.StaffID,
				Close:   session.Close,
			}, nil
		},
		BackendReady: msgcrypto.Available,
		PolicyCache:  messagecrypto.NewPolicyCache(time.Now),
	}
}

func firstNonEmptyAppCrypto(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
