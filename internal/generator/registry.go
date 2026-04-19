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

package generator

import (
	"fmt"
	"os"
	"slices"
	"strings"

	registryassets "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/registry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"gopkg.in/yaml.v3"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_SKILLS_PERSONAS_FILE",
		Category:    configmeta.CategoryDebug,
		Description: "覆盖内置 personas.yaml 的本地文件路径",
		Example:     "/path/to/personas.yaml",
		Hidden:      true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_SKILLS_RECIPES_FILE",
		Category:    configmeta.CategoryDebug,
		Description: "覆盖内置 recipes.yaml 的本地文件路径",
		Example:     "/path/to/recipes.yaml",
		Hidden:      true,
	})
}

const (
	PersonaRegistryPathEnv = "DWS_SKILLS_PERSONAS_FILE"
	RecipeRegistryPathEnv  = "DWS_SKILLS_RECIPES_FILE"
)

type PersonaRegistry struct {
	Personas []PersonaEntry `yaml:"personas"`
}

type PersonaEntry struct {
	Name         string   `yaml:"name"`
	Title        string   `yaml:"title"`
	Description  string   `yaml:"description"`
	Services     []string `yaml:"services"`
	Products     []string `yaml:"products,omitempty"` // Backward-compatible alias of services.
	Workflows    []string `yaml:"workflows"`
	Instructions []string `yaml:"instructions"`
	Tips         []string `yaml:"tips"`
}

type RecipeRegistry struct {
	Recipes []RecipeEntry `yaml:"recipes"`
}

type RecipeEntry struct {
	Name        string   `yaml:"name"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Category    string   `yaml:"category"`
	Services    []string `yaml:"services"`
	Products    []string `yaml:"products,omitempty"` // Backward-compatible alias of services.
	Steps       []string `yaml:"steps"`
	Caution     string   `yaml:"caution"`
}

var knownRegistryProducts = map[string]struct{}{
	"aiapp":      {},
	"aidesign":   {},
	"aitable":    {},
	"attendance": {},
	"calendar":   {},
	"chat":       {},
	"conference": {},
	"contact":    {},
	"devdoc":     {},
	"ding":       {},
	"doc":        {},
	"docparse":   {},
	"drive":      {},
	"finance":    {},
	"law":        {},
	"live":       {},
	"mail":       {},
	"minutes":    {},
	"oa":         {},
	"report":     {},
	"todo":       {},
	"workbench":  {},
}

func loadPersonaRegistry() (PersonaRegistry, error) {
	data, err := readRegistryYAML(PersonaRegistryPathEnv, registryassets.PersonasYAML())
	if err != nil {
		return PersonaRegistry{}, err
	}
	var registry PersonaRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return PersonaRegistry{}, fmt.Errorf("decode personas registry: %w", err)
	}
	if err := validatePersonaRegistry(registry); err != nil {
		return PersonaRegistry{}, err
	}
	return registry, nil
}

func loadRecipeRegistry() (RecipeRegistry, error) {
	data, err := readRegistryYAML(RecipeRegistryPathEnv, registryassets.RecipesYAML())
	if err != nil {
		return RecipeRegistry{}, err
	}
	var registry RecipeRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return RecipeRegistry{}, fmt.Errorf("decode recipes registry: %w", err)
	}
	if err := validateRecipeRegistry(registry); err != nil {
		return RecipeRegistry{}, err
	}
	return registry, nil
}

func validatePersonaRegistry(registry PersonaRegistry) error {
	if len(registry.Personas) == 0 {
		return fmt.Errorf("personas registry is empty")
	}
	seen := map[string]struct{}{}
	for _, persona := range registry.Personas {
		name := strings.TrimSpace(persona.Name)
		if name == "" {
			return fmt.Errorf("personas registry has empty name")
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("personas registry has duplicate name %q", name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(persona.Title) == "" {
			return fmt.Errorf("persona %q missing title", name)
		}
		if strings.TrimSpace(persona.Description) == "" {
			return fmt.Errorf("persona %q missing description", name)
		}
		services := persona.serviceRefs()
		if len(services) == 0 {
			return fmt.Errorf("persona %q missing services", name)
		}
		if len(persona.Instructions) == 0 {
			return fmt.Errorf("persona %q missing instructions", name)
		}
		for _, service := range services {
			service = normalizeRegistryToken(service)
			if service == "" {
				return fmt.Errorf("persona %q has empty service", name)
			}
			if _, ok := knownRegistryProducts[service]; !ok {
				return fmt.Errorf("persona %q references unknown service %q", name, service)
			}
		}
	}
	return nil
}

func validateRecipeRegistry(registry RecipeRegistry) error {
	if len(registry.Recipes) == 0 {
		return fmt.Errorf("recipes registry is empty")
	}
	seen := map[string]struct{}{}
	for _, recipe := range registry.Recipes {
		name := strings.TrimSpace(recipe.Name)
		if name == "" {
			return fmt.Errorf("recipes registry has empty name")
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("recipes registry has duplicate name %q", name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(recipe.Title) == "" {
			return fmt.Errorf("recipe %q missing title", name)
		}
		if strings.TrimSpace(recipe.Description) == "" {
			return fmt.Errorf("recipe %q missing description", name)
		}
		if strings.TrimSpace(recipe.Category) == "" {
			return fmt.Errorf("recipe %q missing category", name)
		}
		services := recipe.serviceRefs()
		if len(services) == 0 {
			return fmt.Errorf("recipe %q missing services", name)
		}
		if len(recipe.Steps) == 0 {
			return fmt.Errorf("recipe %q missing steps", name)
		}
		for _, service := range services {
			service = normalizeRegistryToken(service)
			if service == "" {
				return fmt.Errorf("recipe %q has empty service", name)
			}
			if _, ok := knownRegistryProducts[service]; !ok {
				return fmt.Errorf("recipe %q references unknown service %q", name, service)
			}
		}
	}
	return nil
}

func validateRegistryReferences(personas PersonaRegistry, recipes RecipeRegistry) error {
	validWorkflows := map[string]struct{}{}
	for _, recipe := range recipes.Recipes {
		name := normalizeRegistryToken(recipe.Name)
		if name == "" {
			continue
		}
		validWorkflows[name] = struct{}{}
		validWorkflows["recipe-"+name] = struct{}{}
	}
	for _, persona := range personas.Personas {
		name := strings.TrimSpace(persona.Name)
		for _, workflow := range persona.Workflows {
			token := normalizeRegistryToken(workflow)
			if token == "" {
				return fmt.Errorf("persona %q has empty workflow", name)
			}
			if _, ok := validWorkflows[token]; !ok {
				return fmt.Errorf("persona %q references unknown workflow %q", name, workflow)
			}
		}
	}
	return nil
}

func uniqueSkillProducts(products []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(products))
	for _, product := range products {
		product = normalizeRegistryToken(product)
		if product == "" {
			continue
		}
		if _, ok := seen[product]; ok {
			continue
		}
		seen[product] = struct{}{}
		out = append(out, product)
	}
	slices.Sort(out)
	return out
}

func (p PersonaEntry) serviceRefs() []string {
	refs := make([]string, 0, len(p.Services)+len(p.Products))
	refs = append(refs, p.Services...)
	refs = append(refs, p.Products...)
	return uniqueSkillProducts(refs)
}

func (r RecipeEntry) serviceRefs() []string {
	refs := make([]string, 0, len(r.Services)+len(r.Products))
	refs = append(refs, r.Services...)
	refs = append(refs, r.Products...)
	return uniqueSkillProducts(refs)
}

func normalizeRegistryToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	slug := safeSkillSegment(value)
	if slug == "unknown" {
		return ""
	}
	return slug
}

func readRegistryYAML(pathEnv string, embedded []byte) ([]byte, error) {
	if path, ok := os.LookupEnv(pathEnv); ok && strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return nil, fmt.Errorf("read registry file %q: %w", strings.TrimSpace(path), err)
		}
		return data, nil
	}
	if len(embedded) == 0 {
		return nil, fmt.Errorf("embedded registry content is empty")
	}
	return embedded, nil
}
