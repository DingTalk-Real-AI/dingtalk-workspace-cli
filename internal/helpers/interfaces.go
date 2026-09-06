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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
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
	publicFactories []registeredFactory
	publicIndex     = make(map[string]string)
)

type registeredFactory struct {
	name    string
	aliases []string
	factory Factory
}

func RegisterPublicNamed(name string, factory Factory, aliases ...string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	publicFactories = append(publicFactories, registeredFactory{name: name, aliases: aliases, factory: factory})
	if _, exists := publicIndex[name]; !exists {
		publicIndex[name] = name
	}
	for _, alias := range aliases {
		if _, exists := publicIndex[alias]; !exists {
			publicIndex[alias] = name
		}
	}
}

func NewPublicCommands(runner executor.Runner) []*cobra.Command {
	return buildCommands(publicFactories, runner, "")
}

// NewPublicCommandsFor constructs only the named top-level product. The name
// comes from the same registration that feeds the full command tree, so the
// fast path does not create a second command or contract authority.
func NewPublicCommandsFor(runner executor.Runner, name string) []*cobra.Command {
	return buildCommands(publicFactories, runner, strings.TrimSpace(name))
}

func ResolvePublicCommand(name string) (string, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	canonical, ok := publicIndex[name]
	return canonical, ok
}

func buildCommands(factories []registeredFactory, runner executor.Runner, selected string) []*cobra.Command {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make([]*cobra.Command, 0, len(factories))
	for _, registered := range factories {
		if selected != "" && registered.name != selected {
			continue
		}
		handler := registered.factory()
		command := handler.Command(runner)
		out = append(out, command)
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
	if strings.HasSuffix(value, "-") {
		return fmt.Errorf("%s %q must not start or end with a hyphen", label, value)
	}
	return nil
}
