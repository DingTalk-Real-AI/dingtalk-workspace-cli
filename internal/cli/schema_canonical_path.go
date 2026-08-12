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
	"strings"
)

// SchemaVisibility is the reviewed CommandRegistry visibility class for a
// command identity.
type SchemaVisibility string

const (
	SchemaVisibilityPublic   SchemaVisibility = "public"
	SchemaVisibilityCompat   SchemaVisibility = "compat"
	SchemaVisibilityInternal SchemaVisibility = "internal"
)

// splitSchemaCanonicalPath splits "product.tool" identity keys used by the
// reviewed CommandRegistry and Schema assembly.
func splitSchemaCanonicalPath(path string) (string, string, bool) {
	path = strings.TrimSpace(path)
	productID, toolName, ok := strings.Cut(path, ".")
	productID = strings.TrimSpace(productID)
	toolName = strings.TrimSpace(toolName)
	if !ok || productID == "" || toolName == "" || strings.ContainsAny(productID+toolName, " \t\r\n") {
		return "", "", false
	}
	return productID, toolName, true
}

// splitManualSchemaCanonicalPath is a compatibility alias.
func splitManualSchemaCanonicalPath(path string) (string, string, bool) {
	return splitSchemaCanonicalPath(path)
}
