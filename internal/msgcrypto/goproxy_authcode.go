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

import "context"

// mintAuthCodeForProxy is the goProxy-only mint path. domain is compared
// locally and never forwarded. A successful mint invalidates any unconsumed
// cache so a one-shot code cannot be reused.
func mintAuthCodeForProxy(codes AuthCodeProvider, allowedHost, corpID, domain string) (string, error) {
	if err := matchRedirectHost(domain, allowedHost); err != nil {
		return "", err
	}
	if codes == nil {
		return "", ErrNoAuthCodeProvider
	}

	var (
		code string
		err  error
	)
	if provider, ok := codes.(CorpAuthCodeProvider); ok {
		code, err = provider.AuthCodeForCorp(context.Background(), corpID)
	} else {
		code, err = codes.AuthCode(context.Background())
	}
	if err != nil {
		invalidateAuthCode(codes)
		return "", err
	}
	if code == "" {
		invalidateAuthCode(codes)
		return "", ErrNoAuthCode
	}
	invalidateAuthCode(codes)
	return code, nil
}

func invalidateAuthCode(codes AuthCodeProvider) {
	if invalidator, ok := codes.(interface{ Invalidate() }); ok {
		invalidator.Invalidate()
	}
}
