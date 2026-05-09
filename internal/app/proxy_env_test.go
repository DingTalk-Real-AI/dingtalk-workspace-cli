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

package app

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

// TestIPv4HTTPClientHonoursHTTPProxyEnv guards the fix for #236 on the
// IPv4-forcing client used by the legacy registry / discovery path. The
// custom Transport overrides DialContext to force IPv4 — without an
// explicit Proxy field it would also drop env-var proxy support.
//
// We can't reliably invoke tr.Proxy(req) here because http.ProxyFromEnvironment
// memoises the env vars on first call (Go's envProxyOnce); ordering with other
// tests that read proxy env early would make this flaky. Asserting that the
// Transport's Proxy func points at http.ProxyFromEnvironment is sufficient to
// catch the regression — the runtime takes care of reading HTTP_PROXY/HTTPS_PROXY
// at process boot.
func TestIPv4HTTPClientHonoursHTTPProxyEnv(t *testing.T) {
	t.Parallel()

	client := ipv4HTTPClient(5 * time.Second)
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("ipv4HTTPClient transport is %T, want *http.Transport", client.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("ipv4HTTPClient transport.Proxy is nil — HTTP_PROXY env will be ignored (regression of #236)")
	}
	wantPC := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	gotPC := reflect.ValueOf(tr.Proxy).Pointer()
	if gotPC != wantPC {
		t.Errorf("ipv4HTTPClient transport.Proxy is not http.ProxyFromEnvironment — env-var proxy may not be honoured (regression of #236)")
	}
}
