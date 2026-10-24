// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package edition

import "context"

// OIDCAuthorizationRequest contains the validated, server-issued SDK input.
// Providers must preserve the parameters, including state, nonce and PKCE.
// The portal retains the client secret and PKCE verifier; neither is included.
type OIDCAuthorizationRequest struct {
	AuthorizationEndpoint string            `json:"authorization_endpoint"`
	Parameters            map[string]string `json:"parameters"`
}

// OIDCAuthorizationResult contains only the SDK-issued code and returned state.
// The core validates both before exchanging the code and saving DWS credentials.
// Do not log this result or include SDK credentials in errors.
type OIDCAuthorizationResult struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// OIDCProvider registers one edition-owned authorization implementation.
// Name must match [a-z][a-z0-9_-]{0,63} and a provider configured in the portal.
// Registering it here does not enable or configure a provider on the server.
type OIDCProvider struct {
	Name string

	// Authorize runs only after the portal accepts the provider and the core
	// validates its authorization request. Initialize SDKs here, honor ctx's
	// cancellation/deadline, and return the original SDK code/state unchanged.
	// The implementation owns SDK authentication; the core owns DWS exchange
	// and persistence. Errors never trigger a fallback to another provider.
	Authorize func(ctx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error)
}
