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
	"sync"
)

var (
	schemaAssemblyEnvironMu  sync.RWMutex
	schemaAssemblyEnviron    []string
	schemaAssemblyWorkingDir string
)

// CaptureSchemaAssemblyEnviron snapshots the environment before runtime
// plugin discovery. The snapshot is used as a child-process environment only.
func CaptureSchemaAssemblyEnviron() {
	env := os.Environ()
	workingDir, _ := os.Getwd()
	schemaAssemblyEnvironMu.Lock()
	schemaAssemblyEnviron = append([]string(nil), env...)
	schemaAssemblyWorkingDir = workingDir
	schemaAssemblyEnvironMu.Unlock()
}

// SchemaAssemblyEnvironmentSnapshot returns an immutable copy for child
// process setup. It never changes the caller's environment.
func SchemaAssemblyEnvironmentSnapshot() []string {
	schemaAssemblyEnvironMu.RLock()
	defer schemaAssemblyEnvironMu.RUnlock()
	return append([]string(nil), schemaAssemblyEnviron...)
}

// SchemaAssemblyWorkingDirectory returns the registration-time working
// directory for isolated child-process assembly.
func SchemaAssemblyWorkingDirectory() string {
	schemaAssemblyEnvironMu.RLock()
	defer schemaAssemblyEnvironMu.RUnlock()
	return schemaAssemblyWorkingDir
}

func clearSchemaAssemblyEnviron() {
	schemaAssemblyEnvironMu.Lock()
	schemaAssemblyEnviron = nil
	schemaAssemblyWorkingDir = ""
	schemaAssemblyEnvironMu.Unlock()
}
