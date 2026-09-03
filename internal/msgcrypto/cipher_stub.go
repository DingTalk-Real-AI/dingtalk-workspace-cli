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

// The constraint below is the exact negation of the one in cipher_safechat.go;
// change both together. This file covers every default DWS build: the release
// binaries are cross-compiled with CGO_ENABLED=0, and windows/arm64 has no
// vendor static library even when the tag is set.

//go:build !(safechat && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64)))

package msgcrypto

import "context"

// BackendVersion is empty because no backend is compiled in.
const BackendVersion = ""

// Available reports that this binary has no SafeChat backend, so callers should
// not offer message encryption.
func Available() bool { return false }

// newBackend always fails here. Open checks Available first, so this exists to
// keep the package compiling and to fail safe if that check is ever bypassed.
func newBackend(context.Context, Config) (Cipher, error) { return nil, ErrUnavailable }
