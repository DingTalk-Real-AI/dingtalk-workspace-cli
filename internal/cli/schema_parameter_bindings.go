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

package cli

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

const schemaParameterBindingsVersion = 1

//go:embed schema_parameter_bindings.json
var embeddedSchemaParameterBindingsJSON []byte

type schemaParameterBindingSnapshot struct {
	Version  int                          `json:"version"`
	Bindings map[string]map[string]string `json:"bindings"`
}

var runtimeSchemaParameterBindings = loadSchemaParameterBindings()

func loadSchemaParameterBindings() map[string]map[string]string {
	var snapshot schemaParameterBindingSnapshot
	if err := json.Unmarshal(embeddedSchemaParameterBindingsJSON, &snapshot); err != nil ||
		snapshot.Version != schemaParameterBindingsVersion {
		return nil
	}
	return snapshot.Bindings
}

func applyRuntimeSchemaParameterBindings(cmd *cobra.Command, canonical string) {
	for flagName, propertyName := range runtimeSchemaParameterBindings[strings.TrimSpace(canonical)] {
		AnnotateRuntimeFlagProperty(cmd, flagName, propertyName)
	}
}
