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

package skillgen

import (
	"os"
	"path/filepath"
)

func writeFileBytes(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if filepath.Base(path) == "SKILL.md" {
		legacyPath := filepath.Join(filepath.Dir(path), "api.md")
		if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.WriteFile(path, content, 0o644)
}
