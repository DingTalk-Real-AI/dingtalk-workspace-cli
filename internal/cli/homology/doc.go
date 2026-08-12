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

// Package homology holds Schema/CLI homology and ContractFinal consistency
// gates that exercise the live command tree and delivered Catalog.
//
// These tests intentionally live outside the cli root so contract / homology
// policy noise does not clutter the delivery package surface. They import
// internal/cli and related packages; they do not redefine delivery types.
package homology
