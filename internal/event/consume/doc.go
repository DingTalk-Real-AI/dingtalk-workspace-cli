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

// Package consume implements the consumer-side process of `dws event
// consume`: dial the bus, send Hello, read Event frames, format them, and
// write them out (stdout / file / dir). v1 (P3) implements the minimal
// path — NDJSON to stdout. P4 adds filter/format/route/compact pipeline.
package consume
