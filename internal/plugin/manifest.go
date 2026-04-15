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

// Package plugin implements the DWS CLI plugin system. It loads,
// validates, and injects plugin capabilities (MCP servers, skills,
// pipeline hooks) into the existing CLI infrastructure.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// namePattern validates plugin names: lowercase kebab-case, 3–50 chars.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,49}$`)

// Manifest represents the parsed contents of a plugin.json file.
type Manifest struct {
	Name          string                `json:"name"`
	Version       string                `json:"version"`
	Description   string                `json:"description,omitempty"`
	Type          string                `json:"type,omitempty"` // "managed" or "user"
	MinCLIVersion string                `json:"minCLIVersion,omitempty"`
	MCPServers    map[string]*MCPServer `json:"mcpServers,omitempty"`
	Skills        string                `json:"skills,omitempty"`
	Hooks         string                `json:"hooks,omitempty"`
	Permissions   []string              `json:"permissions,omitempty"`
	UserConfig    map[string]ConfigItem `json:"userConfig,omitempty"`
	Build         *BuildConfig          `json:"build,omitempty"`
}

// BuildConfig declares how to compile the plugin's stdio server into
// a native binary. DWS runs this automatically during install so that
// plugin users never need language runtimes or dependency managers.
type BuildConfig struct {
	// Command is the shell command to compile the server.
	// Executed via "sh -c" in the plugin root directory.
	// Examples: "bun build --compile src/server.ts --outfile bin/server"
	//           "go build -o bin/server ./cmd/server"
	//           "pip install pyinstaller && pyinstaller --onefile src/server.py -n server --distpath bin/"
	Command string `json:"command"`

	// Output is the path to the compiled binary, relative to the plugin root.
	// Used to verify the build succeeded. Example: "bin/server"
	Output string `json:"output"`
}

// MCPServer describes a single MCP server declared by a plugin.
type MCPServer struct {
	Type     string            `json:"type"`               // "streamable-http" or "stdio"
	Endpoint string            `json:"endpoint,omitempty"` // required for streamable-http
	Command  string            `json:"command,omitempty"`  // required for stdio
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"` // custom HTTP headers (e.g. Authorization for third-party APIs)
	CLI      json.RawMessage   `json:"cli,omitempty"`     // CLIOverlay, passed through
}

// ConfigItem describes a user-configurable setting for a plugin.
type ConfigItem struct {
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
}

// HooksConfig describes pipeline hooks declared in a hooks.json file.
type HooksConfig struct {
	Hooks []HookEntry `json:"hooks"`
}

// HookEntry describes a single pipeline hook.
type HookEntry struct {
	Phase   string `json:"phase"`             // "pre-request", "post-response", etc.
	Matcher string `json:"matcher,omitempty"` // glob pattern, e.g. "conference.*"
	Command string `json:"command"`           // shell command to execute
	Timeout int    `json:"timeout,omitempty"` // seconds, default 30
}

// Plugin is a loaded, validated plugin ready for injection.
type Plugin struct {
	Manifest  Manifest
	Root      string // absolute path to plugin directory
	IsManaged bool   // true for official (DingTalk-Real-AI) plugins
}

// ParseManifest reads and parses a plugin.json file.
func ParseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse plugin.json: %w", err)
	}
	return &m, nil
}

// Validate checks that a manifest is well-formed. It returns an error
// describing the first problem found, or nil if the manifest is valid.
// cliVersion is the current CLI version string for compatibility checks.
func (m *Manifest) Validate(cliVersion string) error {
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("invalid plugin name %q: must be lowercase kebab-case, 3–50 chars", m.Name)
	}
	if !isValidSemver(m.Version) {
		return fmt.Errorf("invalid plugin version %q: must be valid semver (e.g. 1.0.0)", m.Version)
	}
	if m.Type != "" && m.Type != "managed" && m.Type != "user" {
		return fmt.Errorf("invalid plugin type %q: must be \"managed\" or \"user\"", m.Type)
	}
	if m.MinCLIVersion != "" && cliVersion != "" && cliVersion != "dev" {
		if compareSemver(cliVersion, m.MinCLIVersion) < 0 {
			return fmt.Errorf("plugin requires CLI >= %s, current is %s", m.MinCLIVersion, cliVersion)
		}
	}
	for key, srv := range m.MCPServers {
		if err := validateMCPServer(key, srv); err != nil {
			return err
		}
	}
	if m.Skills != "" {
		if err := validateSafePath(m.Skills); err != nil {
			return fmt.Errorf("skills path: %w", err)
		}
	}
	if m.Hooks != "" {
		if err := validateSafePath(m.Hooks); err != nil {
			return fmt.Errorf("hooks path: %w", err)
		}
	}
	return nil
}

func validateMCPServer(key string, srv *MCPServer) error {
	switch srv.Type {
	case "streamable-http":
		if strings.TrimSpace(srv.Endpoint) == "" {
			return fmt.Errorf("mcpServers[%q]: streamable-http requires endpoint", key)
		}
	case "stdio":
		if strings.TrimSpace(srv.Command) == "" {
			return fmt.Errorf("mcpServers[%q]: stdio requires command", key)
		}
	default:
		return fmt.Errorf("mcpServers[%q]: unsupported type %q (must be streamable-http or stdio)", key, srv.Type)
	}
	return nil
}

// validateSafePath rejects paths containing ".." traversal.
func validateSafePath(p string) error {
	cleaned := filepath.Clean(p)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("unsafe path %q: must not contain \"..\"", p)
	}
	return nil
}

// LoadHooks reads the hooks.json file referenced by the manifest.
func (p *Plugin) LoadHooks() (*HooksConfig, error) {
	if p.Manifest.Hooks == "" {
		return nil, nil
	}
	hooksPath := filepath.Join(p.Root, p.Manifest.Hooks)
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read hooks: %w", err)
	}
	var cfg HooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}
	return &cfg, nil
}

// SkillsDir returns the absolute path to the plugin's skills directory.
func (p *Plugin) SkillsDir() string {
	dir := p.Manifest.Skills
	if dir == "" {
		dir = "./skills/"
	}
	return filepath.Join(p.Root, dir)
}

// isValidSemver checks if a string is a valid semantic version (major.minor.patch).
func isValidSemver(v string) bool {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), "-", 2)
	nums := strings.Split(parts[0], ".")
	if len(nums) != 3 {
		return false
	}
	for _, n := range nums {
		if _, err := strconv.Atoi(n); err != nil {
			return false
		}
	}
	return true
}

// parseSemver extracts major, minor, patch from a version string.
func parseSemver(v string) (int, int, int) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, "-", 2) // strip pre-release
	nums := strings.Split(parts[0], ".")
	if len(nums) != 3 {
		return 0, 0, 0
	}
	major, _ := strconv.Atoi(nums[0])
	minor, _ := strconv.Atoi(nums[1])
	patch, _ := strconv.Atoi(nums[2])
	return major, minor, patch
}

// compareSemver compares two semver strings. Returns -1, 0, or 1.
func compareSemver(a, b string) int {
	aMaj, aMin, aPat := parseSemver(a)
	bMaj, bMin, bPat := parseSemver(b)
	if aMaj != bMaj {
		return cmpInt(aMaj, bMaj)
	}
	if aMin != bMin {
		return cmpInt(aMin, bMin)
	}
	return cmpInt(aPat, bPat)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
