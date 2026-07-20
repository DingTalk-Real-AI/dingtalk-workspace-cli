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

import "time"

// MarkAccessTokenStale loads the persisted TokenData, sets ExpiresAt to a past
// instant (preserving access_token and refresh_token), and writes it back. The
// next OAuthProvider.GetAccessToken call will see IsAccessTokenValid() == false
// and proceed to lockedRefresh, exchanging the refresh_token for a fresh
// access_token.
//
// Use this only when the server has rejected the current access_token but the
// local expiry has not yet elapsed (zombie token scenario). It does not delete
// any token material and is safe to call concurrently — actual refresh is
// serialized by lockedRefresh's dual-layer locking.
//
// Returns the original load error when there is no usable token on disk; a
// nil error when there is no access_token to invalidate (no-op).
func MarkAccessTokenStale(configDir string) error {
	data, err := LoadTokenData(configDir)
	if err != nil {
		return err
	}
	if data == nil || data.AccessToken == "" {
		return nil
	}
	data.ExpiresAt = time.Now().Add(-1 * time.Minute)
	return SaveTokenData(configDir, data)
}
