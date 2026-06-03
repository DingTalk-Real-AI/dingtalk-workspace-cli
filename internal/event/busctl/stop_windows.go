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

//go:build windows

package busctl

import "os"

// stopSignal returns the graceful-shutdown signal for Windows. The Go
// runtime maps os.Interrupt to TerminateProcess for non-console-group
// processes — not truly graceful, but acceptable for v1 (Windows graceful
// shutdown via Ctrl+Break is in the v2 backlog, plan §16).
func stopSignal() os.Signal { return os.Interrupt }
