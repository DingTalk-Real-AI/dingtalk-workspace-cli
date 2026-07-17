// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package keychain

// Windows stores token entries in the DPAPI-protected user registry rather
// than auth-token*.enc files, so there is no file inventory to validate.
func platformValidateAuthTokenEntries(string) error {
	return nil
}

func platformRemoveAuthTokenEntries(service string) error {
	return registryRemoveAuthTokenEntries(service)
}
