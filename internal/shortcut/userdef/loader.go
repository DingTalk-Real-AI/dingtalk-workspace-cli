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

// Package userdef loads user-defined shortcuts from ~/.dws/shortcuts/*.yaml and
// compiles them into the same shortcut.Shortcut model as the built-ins, so a
// distilled high-frequency operation behaves exactly like a native `+command`.
// This is the runtime half of the "high-frequency auto-distillation" story
// (see docs/shortcut-p2-design.md); the `dws shortcut add` command writes these
// YAML files.
package userdef

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"gopkg.in/yaml.v3"
)

var commandSegmentRE = regexp.MustCompile(`^[\p{L}\p{N}_-][\p{L}\p{N}._-]*$`)

// Spec is the on-disk YAML form of a user-defined shortcut. It mirrors
// shortcut.Shortcut but expresses the Execute step declaratively via Exec.Bind.
type Spec struct {
	Version     int        `yaml:"version"`
	Service     string     `yaml:"service"`
	Command     string     `yaml:"command"`
	Product     string     `yaml:"product"`
	Description string     `yaml:"description"`
	Intent      string     `yaml:"intent"`
	Risk        string     `yaml:"risk"`
	Source      string     `yaml:"source"` // auto | manual
	Flags       []FlagSpec `yaml:"flags"`
	Exec        ExecSpec   `yaml:"execute"`
}

// FlagSpec is the YAML form of shortcut.Flag.
type FlagSpec struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Default  string   `yaml:"default"`
	Desc     string   `yaml:"desc"`
	Required bool     `yaml:"required"`
	Enum     []string `yaml:"enum"`
}

// ExecSpec declares the single MCP call. Bind maps each tool parameter key to
// either a constant string, or a "${flag}" reference resolved from a flag value.
type ExecSpec struct {
	Tool string            `yaml:"tool"`
	Bind map[string]string `yaml:"bind"`
}

// Dir returns the directory holding user shortcut YAML files (~/.dws/shortcuts).
func Dir() string { return filepath.Join(config.DefaultConfigDir(), "shortcuts") }

// Load reads every ~/.dws/shortcuts/*.yaml, compiles valid specs, and registers
// them (skipping any that collide with an already-registered service+command,
// so built-ins always win). Returns the number registered and any per-file
// errors. It never fails hard — a bad file is reported and skipped.
func Load() (registered int, errs []error) {
	files, _ := filepath.Glob(filepath.Join(Dir(), "*.yaml"))
	existing := registeredKeys()
	for _, f := range files {
		s, err := parseFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(f), err))
			continue
		}
		key := s.Service + " " + s.Command
		if existing[key] {
			errs = append(errs, fmt.Errorf("%s: 与已有 shortcut %q 冲突，已跳过", filepath.Base(f), key))
			continue
		}
		shortcut.Register(Compile(s))
		existing[key] = true
		registered++
	}
	return registered, errs
}

func registeredKeys() map[string]bool {
	set := map[string]bool{}
	for _, s := range shortcut.All() {
		set[s.Service+" "+s.Command] = true
	}
	return set
}

func parseFile(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Spec{}, err
	}
	return s, Validate(s)
}

// Validate checks the minimum required fields of a spec.
func Validate(s Spec) error {
	if s.Service == "" || s.Command == "" {
		return fmt.Errorf("service 和 command 为必填")
	}
	if !strings.HasPrefix(s.Command, "+") {
		return fmt.Errorf("command 必须以 + 开头，当前 %q", s.Command)
	}
	if !commandSegmentRE.MatchString(s.Service) ||
		!commandSegmentRE.MatchString(strings.TrimPrefix(s.Command, "+")) {
		return fmt.Errorf("service 和 command 只能包含字母、数字、点、下划线和连字符，且不能以点开头")
	}
	if s.Exec.Tool == "" {
		return fmt.Errorf("execute.tool 为必填")
	}
	return nil
}

// FilePath returns the safe on-disk path for a user-defined shortcut. Validate
// already constrains both components, while the Rel check is a final defense
// against future validation changes reintroducing directory traversal.
func FilePath(service, command string) (string, error) {
	probe := Spec{Service: service, Command: command, Exec: ExecSpec{Tool: "path-check"}}
	if err := Validate(probe); err != nil {
		return "", err
	}
	dir := Dir()
	name := service + "." + strings.TrimPrefix(command, "+") + ".yaml"
	path := filepath.Join(dir, name)
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("shortcut 文件路径超出配置目录")
	}
	return path, nil
}

// Compile turns a validated Spec into a runnable shortcut.Shortcut. Its Execute
// builds the MCP params from Exec.Bind: "${flag}" values are resolved from the
// flag (respecting its type), everything else is a literal constant. Optional
// flag references that the user did not set are omitted.
func Compile(s Spec) shortcut.Shortcut {
	flags := make([]shortcut.Flag, 0, len(s.Flags))
	flagType := map[string]shortcut.FlagType{}
	flagRequired := map[string]bool{}
	flagHasDefault := map[string]bool{}
	for _, f := range s.Flags {
		t := shortcut.FlagType(f.Type)
		if t == "" {
			t = shortcut.FlagString
		}
		flagType[f.Name] = t
		flagRequired[f.Name] = f.Required
		flagHasDefault[f.Name] = f.Default != ""
		flags = append(flags, shortcut.Flag{
			Name: f.Name, Type: t, Default: f.Default,
			Desc: f.Desc, Required: f.Required, Enum: f.Enum,
		})
	}

	risk := shortcut.Risk(s.Risk)
	if risk == "" {
		risk = shortcut.RiskRead
	}
	desc := s.Description
	if desc == "" {
		desc = s.Command + "（自定义）"
	}
	intent := s.Intent
	if intent == "" {
		intent = "用户自定义 shortcut（沉淀自高频操作）：" + desc
	}

	bind := s.Exec.Bind
	tool := s.Exec.Tool

	return shortcut.Shortcut{
		Service:     s.Service,
		Command:     s.Command,
		Product:     s.Product,
		Description: desc,
		Intent:      intent,
		Risk:        risk,
		Flags:       flags,
		Execute: func(rt *shortcut.RuntimeContext) error {
			params := map[string]any{}
			for key, tmpl := range bind {
				if name, ok := flagRef(tmpl); ok {
					if !rt.Changed(name) && !flagRequired[name] && !flagHasDefault[name] {
						continue // optional flag not provided → omit param
					}
					params[key] = readFlag(rt, name, flagType[name])
				} else {
					params[key] = tmpl // constant
				}
			}
			return rt.CallMCP(tool, params)
		},
	}
}

// flagRef reports whether v is a "${name}" reference and returns name.
func flagRef(v string) (string, bool) {
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return strings.TrimSpace(v[2 : len(v)-1]), true
	}
	return "", false
}

func readFlag(rt *shortcut.RuntimeContext, name string, t shortcut.FlagType) any {
	switch t {
	case shortcut.FlagBool:
		return rt.Bool(name)
	case shortcut.FlagInt:
		return rt.Int(name)
	case shortcut.FlagStringSlice:
		return rt.StrSlice(name)
	default:
		return rt.Str(name)
	}
}
