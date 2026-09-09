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
	"errors"
	"testing"
)

func TestCrossPlatformCoverageValidateKeyServerAcceptsHTTPS(t *testing.T) {
	if err := validateKeyServer("https://key.example.test/v1"); err != nil {
		t.Fatalf("validateKeyServer() = %v, want nil", err)
	}
}

func TestCrossPlatformCoverageValidateKeyServerRejectsEmpty(t *testing.T) {
	if err := validateKeyServer("  "); !errors.Is(err, ErrNoKeyServer) {
		t.Fatalf("validateKeyServer() = %v, want ErrNoKeyServer", err)
	}
}

func TestCrossPlatformCoverageValidateKeyServerRejectsHTTP(t *testing.T) {
	if err := validateKeyServer("http://key.example.test"); !errors.Is(err, ErrKeyServerNotHTTPS) {
		t.Fatalf("validateKeyServer() = %v, want ErrKeyServerNotHTTPS", err)
	}
}

func TestCrossPlatformCoverageValidateKeyServerRejectsBareHost(t *testing.T) {
	if err := validateKeyServer("key.example.test"); !errors.Is(err, ErrInvalidKeyServer) {
		t.Fatalf("validateKeyServer() = %v, want ErrInvalidKeyServer", err)
	}
}

func TestCrossPlatformCoverageMatchRedirectHostComparesHostOnly(t *testing.T) {
	if err := matchRedirectHost("https://sso.anhei.test:443/login", "https://sso.anhei.test/path"); err != nil {
		t.Fatalf("matchRedirectHost() = %v, want nil", err)
	}
	if err := matchRedirectHost("sso.anhei.test", "https://sso.anhei.test"); err != nil {
		t.Fatalf("bare host match = %v, want nil", err)
	}
}

func TestCrossPlatformCoverageMatchRedirectHostSkipsWhenEitherSideEmpty(t *testing.T) {
	if err := matchRedirectHost("", "https://sso.anhei.test"); err != nil {
		t.Fatalf("empty domain = %v, want nil", err)
	}
	if err := matchRedirectHost("sso.anhei.test", ""); err != nil {
		t.Fatalf("empty allowed host = %v, want nil", err)
	}
}

func TestCrossPlatformCoverageMatchRedirectHostRejectsMismatch(t *testing.T) {
	err := matchRedirectHost("evil.example.test", "https://sso.anhei.test")
	if !errors.Is(err, ErrRedirectHostMismatch) {
		t.Fatalf("matchRedirectHost() = %v, want ErrRedirectHostMismatch", err)
	}
}

func TestCrossPlatformCoverageHostnameOfStripsPortAndPath(t *testing.T) {
	if got := hostnameOf("https://SSO.Example.TEST:8443/login?x=1"); got != "sso.example.test" {
		t.Fatalf("hostnameOf(url) = %q", got)
	}
	if got := hostnameOf("SSO.Example.TEST:8443/login"); got != "sso.example.test" {
		t.Fatalf("hostnameOf(hostport) = %q", got)
	}
}

func TestCrossPlatformCoverageKeyServerRemainingEdges(t *testing.T) {
	if err := validateKeyServer("https://:443"); !errors.Is(err, ErrInvalidKeyServer) {
		t.Fatalf("empty hostname key server = %v, want ErrInvalidKeyServer", err)
	}
	if got := hostnameOf(" "); got != "" {
		t.Fatalf("hostnameOf(blank) = %q", got)
	}
	if got := hostnameOf("https://"); got != "https" {
		t.Fatalf("hostnameOf(malformed fallback) = %q", got)
	}
}
