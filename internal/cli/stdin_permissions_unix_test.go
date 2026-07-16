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

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageReadFileBoundedPermissionDenied(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "noperm.txt")
	writeTestFile(t, path, []byte("secret"))
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if _, _, err := ReadFileArg("@" + path); err == nil {
		t.Fatal("expected error for permission denied")
	}
}
