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

//go:build !windows

package msgcrypto

import (
	"fmt"
	"io/fs"
	"os"
)

// runtimeSupportsPOSIXPerm reports that the keystore directory's permission
// bits are meaningful here, so prepareKeystore enforces owner-only access.
const runtimeSupportsPOSIXPerm = true

func restrictKeystorePermissions(dir string, info fs.FileInfo) error {
	if info.Mode().Perm() == keystoreDirPerm {
		return nil
	}
	if err := chmodKeystore(dir, keystoreDirPerm); err != nil {
		return fmt.Errorf("msgcrypto: restrict keystore dir permissions: %w", err)
	}
	return nil
}

var chmodKeystore = os.Chmod
