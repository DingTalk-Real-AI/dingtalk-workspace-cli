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

package runtimeannotate

// Cobra annotation keys for Schema homology (path A). Writers and readers must
// share these exact strings; do not re-declare literals in callers.
const (
	AnnotationProduct      = "dws.schema.product"
	AnnotationTool         = "dws.schema.tool"
	AnnotationSource       = "dws.schema.source"
	AnnotationDescription  = "dws.schema.description"
	AnnotationExclude      = "dws.schema.exclude"
	AnnotationConstraints  = "dws.schema.constraints"
	AnnotationPositionals  = "dws.schema.positionals"
	AnnotationContract     = "dws.schema.contract"
	AnnotationRisk         = "dws.schema.risk"
	AnnotationRuntimeGate  = "dws.schema.runtime_gate"
	AnnotationFlagProperty = "dws.schema.property"
	AnnotationFlagType     = "dws.schema.type"
	AnnotationFlagRequired = "dws.schema.required"
	AnnotationFlagReqWhen  = "dws.schema.required_when"
	AnnotationFlagExample  = "dws.schema.example"
	AnnotationFlagFormat   = "x-cli-format"
	AnnotationFlagEnum     = "x-cli-enum"
)
