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

package cli

import (
	"os"
	"strings"
	"sync"
)

// Schema assembly must observe the process environment as it was when the
// distribution registered the schema source root — before runtime plugin
// discovery. A plugin's settings.json config may inject any non-blacklisted,
// previously-unset environment variable (including DWS_-prefixed ones), so
// assembling under the live environment could let a plugin-present process
// publish a schema surface that only holds in its own environment. Running
// declaration assembly under the registration-time snapshot is the
// structural boundary: plugin-injected variables are invisible to assembly
// by construction, not by enumerating tested variable names, while the live
// environment is restored afterwards so plugin command execution keeps its
// injected configuration.
var (
	schemaAssemblyEnvironMu sync.Mutex
	schemaAssemblyEnviron   []string
)

// CaptureSchemaAssemblyEnviron snapshots the current process environment as
// the pristine schema-assembly environment. Production captures at schema
// source registration, which happens before runtime plugin discovery.
func CaptureSchemaAssemblyEnviron() {
	env := os.Environ()
	schemaAssemblyEnvironMu.Lock()
	schemaAssemblyEnviron = env
	schemaAssemblyEnvironMu.Unlock()
}

// clearSchemaAssemblyEnviron drops the snapshot so later assembly runs under
// the live environment again.
func clearSchemaAssemblyEnviron() {
	schemaAssemblyEnvironMu.Lock()
	schemaAssemblyEnviron = nil
	schemaAssemblyEnvironMu.Unlock()
}

// withSchemaAssemblyEnviron runs fn under the captured registration-time
// environment and restores the live environment when fn returns. Without a
// snapshot fn runs unchanged.
func withSchemaAssemblyEnviron(fn func()) {
	schemaAssemblyEnvironMu.Lock()
	snapshot := schemaAssemblyEnviron
	schemaAssemblyEnvironMu.Unlock()
	if snapshot == nil {
		fn()
		return
	}
	live := os.Environ()
	applySchemaAssemblyEnviron(snapshot)
	defer applySchemaAssemblyEnviron(live)
	fn()
}

// applySchemaAssemblyEnviron makes the process environment exactly equal to
// want: variables absent from want are unset and differing values are
// overwritten.
func applySchemaAssemblyEnviron(want []string) {
	target := make(map[string]string, len(want))
	for _, entry := range want {
		if i := strings.IndexByte(entry, '='); i > 0 {
			target[entry[:i]] = entry[i+1:]
		}
	}
	for _, entry := range os.Environ() {
		if i := strings.IndexByte(entry, '='); i > 0 {
			if _, ok := target[entry[:i]]; !ok {
				_ = os.Unsetenv(entry[:i])
			}
		}
	}
	for key, value := range target {
		if current, ok := os.LookupEnv(key); !ok || current != value {
			_ = os.Setenv(key, value)
		}
	}
}
