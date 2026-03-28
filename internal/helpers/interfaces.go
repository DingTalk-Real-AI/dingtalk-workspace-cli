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

package helpers

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/spf13/cobra"
)

type Handler interface {
	Name() string
	Command(runner executor.Runner) *cobra.Command
}

type Factory func() Handler

type Manifest struct {
	Vendor      string
	Name        string
	Description string
}

func (m Manifest) FullName() string {
	return strings.TrimSpace(m.Vendor) + "/" + strings.TrimSpace(m.Name)
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

const (
	vendorMinLen = 2
	vendorMaxLen = 30
	nameMinLen   = 2
	nameMaxLen   = 50
)

var (
	registryMu      sync.Mutex
	publicFactories []Factory
)

func RegisterPublic(factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	publicFactories = append(publicFactories, factory)
}

func NewPublicCommands(runner executor.Runner) []*cobra.Command {
	return buildCommands(publicFactories, runner)
}

func buildCommands(factories []Factory, runner executor.Runner) []*cobra.Command {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make([]*cobra.Command, 0, len(factories))
	for _, factory := range factories {
		handler := factory()
		out = append(out, handler.Command(runner))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Use < out[j].Use
	})
	return out
}

func ValidateNaming(vendor, name string) error {
	if err := validateSegment("vendor", vendor, vendorMinLen, vendorMaxLen); err != nil {
		return err
	}
	if err := validateSegment("name", name, nameMinLen, nameMaxLen); err != nil {
		return err
	}
	return nil
}

func validateSegment(label, value string, minLen, maxLen int) error {
	value = strings.TrimSpace(value)
	if len(value) < minLen || len(value) > maxLen {
		return fmt.Errorf("%s %q length must be %d-%d, got %d", label, value, minLen, maxLen, len(value))
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("%s %q must be kebab-case (a-z0-9-), starting with a letter", label, value)
	}
	if strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") {
		return fmt.Errorf("%s %q must not start or end with a hyphen", label, value)
	}
	return nil
}
