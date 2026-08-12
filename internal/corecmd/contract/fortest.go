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

package contract

import "strings"

// Cross-package test helpers for this package's global registration store.
// Production code must not call these; the ForTest suffix is the boundary.

// ClearProductDeclForTest removes a registration (tests only).
func ClearProductDeclForTest(productID string) {
	productID = strings.TrimSpace(productID)
	if productID != "" {
		productDecls.Delete(productID)
	}
}

// StoreProductDeclRawForTest stores an arbitrary map value (tests only).
func StoreProductDeclRawForTest(productID string, value any) {
	productID = strings.TrimSpace(productID)
	if productID != "" {
		productDecls.Store(productID, value)
	}
}
