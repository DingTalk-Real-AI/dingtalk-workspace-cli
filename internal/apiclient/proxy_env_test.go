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

package apiclient

import (
	"net/http"
	"reflect"
	"testing"
)

// TestDefaultTransportHonoursHTTPProxyEnv is the regression guard for #236
// on the apiclient transport. Same rationale as transport/proxy_env_test.go:
// a custom Transport without an explicit Proxy field silently bypasses
// HTTP_PROXY/HTTPS_PROXY.
//
// We pointer-compare against http.ProxyFromEnvironment instead of invoking
// it, because http.ProxyFromEnvironment memoises the env on first call;
// other tests that read proxy env early would make a value-based assertion
// flaky.
func TestDefaultTransportHonoursHTTPProxyEnv(t *testing.T) {
	t.Parallel()

	tr := defaultTransport()
	if tr.Proxy == nil {
		t.Fatal("defaultTransport().Proxy is nil — HTTP_PROXY env will be ignored (regression of #236)")
	}
	wantPC := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	gotPC := reflect.ValueOf(tr.Proxy).Pointer()
	if gotPC != wantPC {
		t.Errorf("defaultTransport().Proxy is not http.ProxyFromEnvironment — env-var proxy may not be honoured (regression of #236)")
	}
}
