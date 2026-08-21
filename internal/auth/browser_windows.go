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

package auth

import (
	"fmt"
	"net/url"
	"os/exec"
	"syscall"
)

func openBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !allowedBrowserSchemes[parsed.Scheme] {
		return fmt.Errorf("refused to open URL with disallowed scheme %q", parsed.Scheme)
	}

	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for process to prevent resource leak
	// Run in goroutine to avoid blocking browser launch
	go func() {
		_ = cmd.Wait()
	}()

	return nil
}
