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

package consume

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isBrokenPipe reports whether err is the Windows equivalent of EPIPE
// (ERROR_BROKEN_PIPE / ERROR_NO_DATA) surfaced when a downstream pipe
// consumer closes its read end.
func isBrokenPipe(err error) bool {
	return errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_NO_DATA)
}
