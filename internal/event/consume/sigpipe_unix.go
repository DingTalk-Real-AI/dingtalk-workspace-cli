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

//go:build !windows

package consume

import (
	"errors"
	"syscall"
)

// isBrokenPipe reports whether err originates from a closed downstream
// pipe (typical: `dws event consume | head -1`). On Unix this surfaces as
// EPIPE; the Go runtime by default also raises SIGPIPE which would kill
// the process, but Go programs ignore SIGPIPE on stdio writes (since
// Go 1.x). We just need to detect EPIPE and exit cleanly.
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
