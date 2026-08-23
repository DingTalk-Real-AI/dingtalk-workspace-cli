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

// Package contractfinal owns the Cobra-keyed ContractFinal runtime store and
// the annotate+store registration seam.
//
// All registration — products, helpers, shortcuts, and framework code
// (corecmd.AttachContract) — calls RegisterRuntimeContractFinal here directly.
// internal/corecmd never imports any internal/cli package.
//
// Types remain in internal/corecmd/contract (DTO only — no cobra store).
// Annotate writers live in internal/corecmd/runtimeannotate.
package contractfinal
