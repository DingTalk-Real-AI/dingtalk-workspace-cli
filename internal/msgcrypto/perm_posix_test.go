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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoveragePrepareKeystoreReportsChmodFailure(t *testing.T) {
	oldStat := statKeystore
	oldChmod := chmodKeystore
	t.Cleanup(func() {
		statKeystore = oldStat
		chmodKeystore = oldChmod
	})
	statKeystore = func(string) (os.FileInfo, error) {
		return fakeFileInfo{mode: 0o755 | fs.ModeDir, isDir: true}, nil
	}
	wantErr := errors.New("chmod failed")
	chmodKeystore = func(string, fs.FileMode) error {
		return wantErr
	}

	err := prepareKeystore(filepath.Join(t.TempDir(), "keystore"))
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "restrict keystore dir permissions") {
		t.Fatalf("prepareKeystore() = %v, want chmod failure", err)
	}
}
