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

package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// Loader scans plugin directories and returns loaded, validated plugins.
type Loader struct {
	// PluginsDir is the root directory for all plugins.
	// Defaults to ~/.dws/plugins/.
	PluginsDir string

	// CLIVersion is the current CLI version, used for
	// minCLIVersion compatibility checks.
	CLIVersion string
}

// NewLoader creates a Loader with default paths.
func NewLoader(cliVersion string) *Loader {
	home, _ := os.UserHomeDir()
	return &Loader{
		PluginsDir: filepath.Join(home, ".dws", "plugins"),
		CLIVersion: cliVersion,
	}
}

// Settings holds user preferences for plugin management.
type Settings struct {
	EnabledPlugins   map[string]bool           `json:"enabledPlugins,omitempty"`
	PluginConfigs    map[string]map[string]any `json:"pluginConfigs,omitempty"`
	PluginAutoUpdate bool                      `json:"pluginAutoUpdate,omitempty"`
	DevPlugins       map[string]string         `json:"devPlugins,omitempty"` // name → absolute path
}

// LoadManaged scans ~/.dws/plugins/managed/ and returns all valid
// official plugins. Managed plugins are always enabled.
func (l *Loader) LoadManaged() []*Plugin {
	managedDir := filepath.Join(l.PluginsDir, "managed")
	return l.scanDir(managedDir, true)
}

// LoadUser scans ~/.dws/plugins/user/ and returns enabled user plugins.
func (l *Loader) LoadUser() []*Plugin {
	userDir := filepath.Join(l.PluginsDir, "user")
	settings := l.loadSettings()

	var plugins []*Plugin
	// User plugins may be nested: user/{workspace}/{name}/
	entries, err := os.ReadDir(userDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("plugin: cannot read user dir", "path", userDir, "error", err)
		}
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		entryPath := filepath.Join(userDir, entry.Name())

		// Check if this is a direct plugin directory (has plugin.json)
		if _, err := os.Stat(filepath.Join(entryPath, "plugin.json")); err == nil {
			p := l.loadPlugin(entryPath, false)
			if p != nil && isPluginEnabled(settings, p.Manifest.Name) {
				plugins = append(plugins, p)
			}
			continue
		}

		// Otherwise treat as workspace directory: user/{workspace}/{name}/
		subEntries, err := os.ReadDir(entryPath)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			subPath := filepath.Join(entryPath, sub.Name())
			p := l.loadPlugin(subPath, false)
			if p != nil {
				qualifiedName := entry.Name() + "/" + p.Manifest.Name
				if isPluginEnabled(settings, qualifiedName) {
					plugins = append(plugins, p)
				}
			}
		}
	}
	return plugins
}

// LoadAll loads both managed and user plugins.
func (l *Loader) LoadAll() []*Plugin {
	managed := l.LoadManaged()
	user := l.LoadUser()
	return append(managed, user...)
}

// scanDir reads a directory of plugin subdirectories and loads each one.
func (l *Loader) scanDir(dir string, isManaged bool) []*Plugin {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("plugin: cannot read dir", "path", dir, "error", err)
		}
		return nil
	}

	var plugins []*Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, entry.Name())
		p := l.loadPlugin(pluginDir, isManaged)
		if p != nil {
			plugins = append(plugins, p)
		}
	}
	return plugins
}

// loadPlugin reads and validates a single plugin directory.
func (l *Loader) loadPlugin(dir string, isManaged bool) *Plugin {
	manifestPath := filepath.Join(dir, "plugin.json")
	manifest, err := ParseManifest(manifestPath)
	if err != nil {
		slog.Warn("plugin: failed to parse manifest",
			"path", manifestPath, "error", err)
		return nil
	}

	if err := manifest.Validate(l.CLIVersion); err != nil {
		slog.Warn("plugin: validation failed",
			"plugin", manifest.Name, "error", err)
		return nil
	}

	return &Plugin{
		Manifest:  *manifest,
		Root:      dir,
		IsManaged: isManaged,
	}
}

// settingsPath returns the path to settings.json.
// Uses PluginsDir's parent (~/.dws/) for production, PluginsDir itself for tests.
func (l *Loader) settingsPath() string {
	// If PluginsDir ends with "plugins", go up one level to ~/.dws/
	if filepath.Base(l.PluginsDir) == "plugins" {
		return filepath.Join(filepath.Dir(l.PluginsDir), "settings.json")
	}
	// For test temp dirs, use PluginsDir directly
	return filepath.Join(l.PluginsDir, "settings.json")
}

// loadSettings reads settings.json from the parent of PluginsDir.
func (l *Loader) loadSettings() *Settings {
	settingsPath := l.settingsPath()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return &Settings{}
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		slog.Debug("plugin: failed to parse settings.json", "error", err)
		return &Settings{}
	}
	return &s
}

func isPluginEnabled(s *Settings, name string) bool {
	if s == nil || s.EnabledPlugins == nil {
		return true // default: enabled
	}
	enabled, exists := s.EnabledPlugins[name]
	if !exists {
		return true // not in list = enabled
	}
	return enabled
}

// InstalledPlugins returns the list of all installed plugins with their
// status info. Used by `dws plugin list`.
type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"` // "managed" or "user"
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// ListInstalled returns info about all installed plugins.
func (l *Loader) ListInstalled() []PluginInfo {
	var result []PluginInfo
	settings := l.loadSettings()

	// Managed plugins
	managedDir := filepath.Join(l.PluginsDir, "managed")
	if entries, err := os.ReadDir(managedDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(managedDir, entry.Name())
			m, err := ParseManifest(filepath.Join(dir, "plugin.json"))
			if err != nil {
				continue
			}
			result = append(result, PluginInfo{
				Name:        m.Name,
				Version:     m.Version,
				Type:        "managed",
				Enabled:     true, // managed plugins always enabled
				Path:        dir,
				Description: m.Description,
			})
		}
	}

	// User plugins
	userDir := filepath.Join(l.PluginsDir, "user")
	if entries, err := os.ReadDir(userDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			l.collectUserPluginInfos(filepath.Join(userDir, entry.Name()), entry.Name(), settings, &result)
		}
	}

	// Dev plugins
	for name, dir := range settings.DevPlugins {
		m, err := ParseManifest(filepath.Join(dir, "plugin.json"))
		if err != nil {
			continue
		}
		result = append(result, PluginInfo{
			Name:        name,
			Version:     m.Version,
			Type:        "dev",
			Enabled:     true,
			Path:        dir,
			Description: m.Description,
		})
	}

	return result
}

func (l *Loader) collectUserPluginInfos(dir, prefix string, settings *Settings, result *[]PluginInfo) {
	// Direct plugin
	if m, err := ParseManifest(filepath.Join(dir, "plugin.json")); err == nil {
		qualName := prefix
		*result = append(*result, PluginInfo{
			Name:        qualName,
			Version:     m.Version,
			Type:        "user",
			Enabled:     isPluginEnabled(settings, qualName),
			Path:        dir,
			Description: m.Description,
		})
		return
	}
	// Workspace: dir is a workspace, iterate sub-plugins
	subEntries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, sub := range subEntries {
		if !sub.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, sub.Name())
		m, err := ParseManifest(filepath.Join(subDir, "plugin.json"))
		if err != nil {
			continue
		}
		qualName := prefix + "/" + m.Name
		*result = append(*result, PluginInfo{
			Name:        qualName,
			Version:     m.Version,
			Type:        "user",
			Enabled:     isPluginEnabled(settings, qualName),
			Path:        subDir,
			Description: m.Description,
		})
	}
}

// InstallFromDir copies a plugin from a source directory to the user
// plugins directory.
func (l *Loader) InstallFromDir(srcDir string) (*Plugin, error) {
	manifestPath := filepath.Join(srcDir, "plugin.json")
	manifest, err := ParseManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin: %w", err)
	}
	if err := manifest.Validate(l.CLIVersion); err != nil {
		return nil, fmt.Errorf("plugin validation failed: %w", err)
	}

	destDir := filepath.Join(l.PluginsDir, "user", manifest.Name)
	if err := copyDir(srcDir, destDir); err != nil {
		return nil, fmt.Errorf("install failed: %w", err)
	}

	// Enable by default in settings
	l.setPluginEnabled(manifest.Name, true)

	return &Plugin{
		Manifest:  *manifest,
		Root:      destDir,
		IsManaged: false,
	}, nil
}

// InstallFromGit clones a git repository and installs the plugin.
// The workspace is extracted from the git URL (e.g. github.com/{workspace}/{name}).
func (l *Loader) InstallFromGit(gitURL string) (*Plugin, error) {
	workspace, repoName, err := parseGitURL(gitURL)
	if err != nil {
		return nil, fmt.Errorf("invalid git URL: %w", err)
	}

	// Clone to temp directory.
	tmpDir, err := os.MkdirTemp("", "dws-plugin-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDir := filepath.Join(tmpDir, repoName)
	cmd := exec.Command("git", "clone", "--depth", "1", gitURL, cloneDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// Parse and validate manifest.
	manifest, err := ParseManifest(filepath.Join(cloneDir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("invalid plugin: %w", err)
	}
	if err := manifest.Validate(l.CLIVersion); err != nil {
		return nil, fmt.Errorf("plugin validation failed: %w", err)
	}

	// Determine install path based on workspace.
	var destDir string
	var isManaged bool
	if workspace == config.OfficialPluginWorkspace {
		destDir = filepath.Join(l.PluginsDir, config.PluginManagedDir, manifest.Name)
		isManaged = true
	} else {
		destDir = filepath.Join(l.PluginsDir, config.PluginUserDir, workspace, manifest.Name)
		isManaged = false
	}

	// Remove .git directory before copying.
	_ = os.RemoveAll(filepath.Join(cloneDir, ".git"))

	if err := copyDir(cloneDir, destDir); err != nil {
		return nil, fmt.Errorf("install failed: %w", err)
	}

	if !isManaged {
		qualifiedName := workspace + "/" + manifest.Name
		l.setPluginEnabled(qualifiedName, true)
	}

	return &Plugin{
		Manifest:  *manifest,
		Root:      destDir,
		IsManaged: isManaged,
	}, nil
}

// parseGitURL extracts workspace and repo name from a git URL.
// Supports: https://github.com/org/repo.git, git@github.com:org/repo.git
func parseGitURL(gitURL string) (workspace, repoName string, err error) {
	gitURL = strings.TrimSpace(gitURL)

	// Handle SSH format: git@github.com:org/repo.git
	if strings.HasPrefix(gitURL, "git@") {
		parts := strings.SplitN(gitURL, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("cannot parse SSH URL %q", gitURL)
		}
		path := strings.TrimSuffix(parts[1], ".git")
		segments := strings.Split(path, "/")
		if len(segments) < 2 {
			return "", "", fmt.Errorf("SSH URL %q must have org/repo format", gitURL)
		}
		return segments[len(segments)-2], segments[len(segments)-1], nil
	}

	// Handle HTTPS format.
	u, err := url.Parse(gitURL)
	if err != nil {
		return "", "", fmt.Errorf("cannot parse URL %q: %w", gitURL, err)
	}

	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("URL %q must have org/repo format", gitURL)
	}

	return segments[len(segments)-2], segments[len(segments)-1], nil
}

// RemovePlugin removes a user plugin. Returns an error if it's managed.
func (l *Loader) RemovePlugin(name string, keepData bool) error {
	// Check managed first — official plugins cannot be removed.
	managedDir := filepath.Join(l.PluginsDir, "managed", name)
	if _, err := os.Stat(managedDir); err == nil {
		return fmt.Errorf("%s 是官方插件（DingTalk-Real-AI/%s），不支持卸载。\n   如需停用，请使用：dws plugin disable %s", name, name, name)
	}

	pluginDir := l.findUserPluginDir(name)
	if pluginDir == "" {
		return fmt.Errorf("plugin %q not found", name)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin: %w", err)
	}

	if !keepData {
		dataDir := filepath.Join(l.PluginsDir, "data", name)
		_ = os.RemoveAll(dataDir)
	}

	l.setPluginEnabled(name, false)
	return nil
}

// SetEnabled enables or disables a plugin in settings.json.
func (l *Loader) SetEnabled(name string, enabled bool) error {
	// Verify plugin exists
	if l.findUserPluginDir(name) == "" {
		managedDir := filepath.Join(l.PluginsDir, "managed", name)
		if _, err := os.Stat(managedDir); err != nil {
			return fmt.Errorf("plugin %q not found", name)
		}
	}
	l.setPluginEnabled(name, enabled)
	return nil
}

func (l *Loader) findUserPluginDir(name string) string {
	// Try direct: user/{name}/
	dir := filepath.Join(l.PluginsDir, "user", name)
	if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err == nil {
		return dir
	}
	// Try workspace: user/{workspace}/{plugin}/
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		dir = filepath.Join(l.PluginsDir, "user", parts[0], parts[1])
		if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err == nil {
			return dir
		}
	}
	return ""
}

func (l *Loader) setPluginEnabled(name string, enabled bool) {
	settings := l.loadSettings()
	if settings.EnabledPlugins == nil {
		settings.EnabledPlugins = make(map[string]bool)
	}
	settings.EnabledPlugins[name] = enabled
	l.saveSettings(settings)
}

func (l *Loader) saveSettings(s *Settings) {
	settingsPath := l.settingsPath()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Debug("plugin: failed to marshal settings", "error", err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(settingsPath), 0o700)
	_ = os.WriteFile(settingsPath, data, 0o600)
}

// GetPluginConfig returns the value of a config key for a plugin.
// It checks pluginConfigs in settings.json first, then falls back to
// the userConfig default in the plugin's manifest.
func (l *Loader) GetPluginConfig(pluginName, key string) (string, bool) {
	settings := l.loadSettings()
	if settings.PluginConfigs != nil {
		if pluginCfg, ok := settings.PluginConfigs[pluginName]; ok {
			if val, ok := pluginCfg[key]; ok {
				if s, ok := val.(string); ok {
					return s, true
				}
			}
		}
	}
	return "", false
}

// SetPluginConfig persists a config key-value pair for a plugin.
func (l *Loader) SetPluginConfig(pluginName, key, value string) {
	settings := l.loadSettings()
	if settings.PluginConfigs == nil {
		settings.PluginConfigs = make(map[string]map[string]any)
	}
	if settings.PluginConfigs[pluginName] == nil {
		settings.PluginConfigs[pluginName] = make(map[string]any)
	}
	settings.PluginConfigs[pluginName][key] = value
	l.saveSettings(settings)
}

// UnsetPluginConfig removes a config key for a plugin.
func (l *Loader) UnsetPluginConfig(pluginName, key string) bool {
	settings := l.loadSettings()
	if settings.PluginConfigs == nil {
		return false
	}
	pluginCfg, ok := settings.PluginConfigs[pluginName]
	if !ok {
		return false
	}
	if _, exists := pluginCfg[key]; !exists {
		return false
	}
	delete(pluginCfg, key)
	if len(pluginCfg) == 0 {
		delete(settings.PluginConfigs, pluginName)
	}
	l.saveSettings(settings)
	return true
}

// ListPluginConfig returns all config key-value pairs for a plugin.
func (l *Loader) ListPluginConfig(pluginName string) map[string]string {
	settings := l.loadSettings()
	result := make(map[string]string)
	if settings.PluginConfigs == nil {
		return result
	}
	pluginCfg, ok := settings.PluginConfigs[pluginName]
	if !ok {
		return result
	}
	for k, v := range pluginCfg {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// InjectPluginConfigEnv reads pluginConfigs from settings.json and sets
// environment variables for each configured key. This allows
// expandPluginVars (which calls os.Expand) to resolve ${KEY} references
// in plugin.json headers, endpoints, etc.
//
// Environment variables already set by the user take precedence — only
// keys not already present in the environment are injected.
func (l *Loader) InjectPluginConfigEnv() {
	settings := l.loadSettings()
	if len(settings.PluginConfigs) == 0 {
		return
	}
	for _, pluginCfg := range settings.PluginConfigs {
		for key, val := range pluginCfg {
			strVal, ok := val.(string)
			if !ok || strVal == "" {
				continue
			}
			// Do not override existing environment variables.
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
			_ = os.Setenv(key, strVal)
		}
	}
}

// LoadDev loads dev plugins registered via `dws plugin dev`.
// Dev plugins are loaded from their source directories without copying.
func (l *Loader) LoadDev() []*Plugin {
	settings := l.loadSettings()
	if len(settings.DevPlugins) == 0 {
		return nil
	}

	var plugins []*Plugin
	for name, dir := range settings.DevPlugins {
		if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err != nil {
			slog.Debug("plugin: dev plugin directory missing, skipping",
				"name", name, "dir", dir)
			continue
		}
		p := l.loadPlugin(dir, false)
		if p != nil {
			plugins = append(plugins, p)
			slog.Debug("plugin: loaded dev plugin", "name", name, "dir", dir)
		}
	}
	return plugins
}

// RegisterDevPlugin registers a source directory as a dev plugin.
func (l *Loader) RegisterDevPlugin(name, absDir string) error {
	settings := l.loadSettings()
	if settings.DevPlugins == nil {
		settings.DevPlugins = make(map[string]string)
	}
	settings.DevPlugins[name] = absDir
	l.saveSettings(settings)
	return nil
}

// UnregisterDevPlugin removes a dev plugin registration.
func (l *Loader) UnregisterDevPlugin(name string) error {
	settings := l.loadSettings()
	if settings.DevPlugins == nil || settings.DevPlugins[name] == "" {
		return fmt.Errorf("dev plugin %q is not registered", name)
	}
	delete(settings.DevPlugins, name)
	l.saveSettings(settings)
	return nil
}

// SyncSkills copies plugin SKILL.md files into all detected agent
// skill directories (e.g. ~/.claude/skills/dws/, ~/.cursor/skills/dws/).
// This makes plugin skills available to AI agents without CLI releases.
func SyncSkills(plugins []*Plugin) {
	if len(plugins) == 0 {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Debug("plugin: cannot get home dir for skill sync", "error", err)
		return
	}

	// Known agent skill directories (subset of upgrade/paths.go knownSkillDirs).
	agentDirs := []string{
		".agents/skills",
		".claude/skills",
		".cursor/skills",
		".qoder/skills",
		".codex/skills",
	}

	for _, p := range plugins {
		skillsDir := p.SkillsDir()
		if _, err := os.Stat(skillsDir); err != nil {
			continue
		}

		// Walk the plugin's skills directory and copy files to each agent dir.
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}

		for _, agentDir := range agentDirs {
			agentBase := filepath.Join(homeDir, agentDir)
			// Only sync to agents that are actually installed (parent dir exists).
			parentGate := filepath.Dir(agentBase)
			if _, err := os.Stat(parentGate); os.IsNotExist(err) {
				continue
			}

			for _, entry := range entries {
				src := filepath.Join(skillsDir, entry.Name())
				// Place plugin skills under dws/plugins/{plugin-name}/
				dest := filepath.Join(agentBase, "dws", "plugins", p.Manifest.Name, entry.Name())
				if entry.IsDir() {
					_ = copyDir(src, dest)
				} else {
					_ = os.MkdirAll(filepath.Dir(dest), 0o755)
					data, readErr := os.ReadFile(src)
					if readErr == nil {
						_ = os.WriteFile(dest, data, 0o644)
					}
				}
			}
		}
	}

	slog.Debug("plugin: skill sync completed", "plugins", len(plugins))
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
