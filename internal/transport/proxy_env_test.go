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

package transport

import (
	"net/http"
	"reflect"
	"testing"
)

// TestDefaultTransportHonoursHTTPProxyEnv guards the fix for issue #236:
// the MCP HTTP transport must honour HTTP_PROXY / HTTPS_PROXY env vars.
// A custom Transport built without an explicit Proxy field defaults to
// "no proxy" — sandboxed deployments behind an outbound proxy would then
// silently bypass the proxy and fail.
//
// We assert tr.Proxy points at http.ProxyFromEnvironment rather than invoking
// it, because http.ProxyFromEnvironment memoises env on first call (Go's
// envProxyOnce). Test ordering with anything else that reads proxy env early
// would make a value-based assertion flaky. The runtime reads env at process
// boot — pointing at the stdlib resolver is the contract we need.
func TestDefaultTransportHonoursHTTPProxyEnv(t *testing.T) {
	t.Parallel()

	tr := defaultTransport()
	if tr.Proxy == nil {
		t.Fatal("defaultTransport().Proxy is nil — HTTP_PROXY/HTTPS_PROXY env will be ignored (regression of #236)")
	}
	wantPC := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	gotPC := reflect.ValueOf(tr.Proxy).Pointer()
	if gotPC != wantPC {
		t.Errorf("defaultTransport().Proxy is not http.ProxyFromEnvironment — env-var proxy may not be honoured (regression of #236)")
	}
}
