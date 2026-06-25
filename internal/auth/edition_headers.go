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

package auth

import (
	"net/http"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// applyEditionEnterpriseCredentialHeaders injects overlay-provided enterprise
// credential headers (e.g. x-dws-enterprise-credential) into MCP control-plane
// and OAuth proxy requests.
func applyEditionEnterpriseCredentialHeaders(req *http.Request) {
	if req == nil {
		return
	}
	fn := edition.Get().EnterpriseCredentialHeaders
	if fn == nil {
		return
	}
	merged := fn(nil)
	for k, v := range merged {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			req.Header.Set(k, v)
		}
	}
}
