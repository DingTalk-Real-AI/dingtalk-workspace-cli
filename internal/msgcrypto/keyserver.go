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

package msgcrypto

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateKeyServer requires an HTTPS URL with a host so the vendor C
// library cannot pick the key-request destination.
func validateKeyServer(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrNoKeyServer
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return fmt.Errorf("%w: %q", ErrInvalidKeyServer, raw)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: %q", ErrKeyServerNotHTTPS, raw)
	}
	if hostnameOf(raw) == "" {
		return fmt.Errorf("%w: %q", ErrInvalidKeyServer, raw)
	}
	return nil
}

// matchRedirectHost compares the goProxy domain to AllowedRedirectHost.
// Both sides are reduced to a hostname. An empty domain or an empty
// allowed host skips the check; the domain is never sent to portal.
func matchRedirectHost(domain, allowed string) error {
	domain = strings.TrimSpace(domain)
	allowed = strings.TrimSpace(allowed)
	if domain == "" || allowed == "" {
		return nil
	}
	got := hostnameOf(domain)
	want := hostnameOf(allowed)
	if got == "" || want == "" || got != want {
		return fmt.Errorf("%w: got %q, want %q", ErrRedirectHostMismatch, got, want)
	}
	return nil
}

// hostnameOf returns the lower-cased hostname of a URL, host:port, or bare
// host. Path, query, userinfo and port are ignored.
func hostnameOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err == nil {
			if host := strings.ToLower(u.Hostname()); host != "" {
				return host
			}
		}
	}
	candidate := raw
	if i := strings.IndexAny(candidate, "/?"); i >= 0 {
		candidate = candidate[:i]
	}
	if host, _, err := net.SplitHostPort(candidate); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(candidate)
}
