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
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// pluginDownloadEndpoint is the API endpoint for downloading plugin updates.
const pluginDownloadEndpoint = "https://aihub.dingtalk.com/cli/download"

// lastCheckFileName stores the last update check timestamp.
const lastCheckFileName = ".last-update-check"

// pluginDownloadTimeout is the timeout for plugin download operations.
const pluginDownloadTimeout = 5 * time.Minute

// Updater checks and applies updates for managed plugins.
type Updater struct {
	PluginsDir string
	CLIVersion string
	Platform   string // e.g. "darwin-arm64", "linux-amd64"

	mu sync.Mutex
}

// NewUpdater creates an Updater with auto-detected platform.
func NewUpdater(pluginsDir, cliVersion string) *Updater {
	return &Updater{
		PluginsDir: pluginsDir,
		CLIVersion: cliVersion,
		Platform:   runtime.GOOS + "-" + runtime.GOARCH,
	}
}

// remoteVersionInfo holds version metadata returned by the download API.
type remoteVersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	FileName    string `json:"fileName"`
	Changelog   string `json:"changelog,omitempty"`
}

// pluginDownloadResponse represents the API response from the plugin
// download endpoint.
type pluginDownloadResponse struct {
	Success   bool               `json:"success"`
	ErrorCode string             `json:"errorCode,omitempty"`
	ErrorMsg  string             `json:"errorMsg,omitempty"`
	Result    *remoteVersionInfo `json:"result,omitempty"`
}

// CheckAndUpdate checks for updates for all managed plugins.
// It reads a last-check timestamp file to avoid checking too frequently.
// Returns the list of updated plugin names.
func (u *Updater) CheckAndUpdate(ctx context.Context, accessToken string, w io.Writer) []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.shouldCheck() {
		slog.Debug("plugin: skipping update check (checked recently)")
		return nil
	}

	managedDir := filepath.Join(u.PluginsDir, config.PluginManagedDir)
	entries, err := os.ReadDir(managedDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("plugin: cannot read managed dir for update check",
				"path", managedDir, "error", err)
		}
		u.recordCheckTime()
		return nil
	}

	var updated []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(managedDir, entry.Name())
		pluginName := config.OfficialPluginWorkspace + "/" + entry.Name()

		result := u.checkAndUpdateOne(ctx, accessToken, pluginDir, pluginName, w)
		if result != "" {
			updated = append(updated, result)
		}
	}

	u.recordCheckTime()
	return updated
}

// EnsureManaged checks that every plugin in config.DefaultManagedPlugins
// exists locally under ~/.dws/plugins/managed/. Missing plugins are
// downloaded from the remote API and extracted automatically.
// This runs once on first launch (or after a user deletes the managed dir).
func (u *Updater) EnsureManaged(ctx context.Context, accessToken string, w io.Writer) []string {
	if len(config.DefaultManagedPlugins) == 0 {
		return nil
	}

	managedDir := filepath.Join(u.PluginsDir, config.PluginManagedDir)

	var installed []string
	for _, shortName := range config.DefaultManagedPlugins {
		pluginDir := filepath.Join(managedDir, shortName)

		// Already exists locally — skip.
		if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err == nil {
			continue
		}

		qualifiedName := config.OfficialPluginWorkspace + "/" + shortName
		fmt.Fprintf(w, "📦 Pulling built-in plugin %s ...\n", qualifiedName)

		remote, err := u.checkRemoteVersion(ctx, accessToken, qualifiedName)
		if err != nil {
			slog.Warn("plugin: failed to fetch remote info for default plugin",
				"plugin", qualifiedName, "error", err)
			fmt.Fprintf(w, "  ⚠️  Failed to fetch %s info: %v\n", qualifiedName, err)
			continue
		}
		if remote == nil || remote.DownloadURL == "" {
			slog.Warn("plugin: no download URL for default plugin",
				"plugin", qualifiedName)
			fmt.Fprintf(w, "  ⚠️  No version available for %s\n", qualifiedName)
			continue
		}

		if err := u.downloadAndInstall(ctx, remote.DownloadURL, pluginDir); err != nil {
			slog.Warn("plugin: failed to install default plugin",
				"plugin", qualifiedName, "error", err)
			fmt.Fprintf(w, "  ❌ Failed to install %s: %v\n", qualifiedName, err)
			continue
		}

		fmt.Fprintf(w, "  ✅ Installed %s (%s)\n", qualifiedName, remote.Version)
		installed = append(installed, qualifiedName)
	}

	return installed
}

// checkAndUpdateOne checks and potentially updates a single managed plugin.
func (u *Updater) checkAndUpdateOne(
	ctx context.Context,
	accessToken, pluginDir, pluginName string,
	w io.Writer,
) string {
	manifest, err := ParseManifest(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		slog.Warn("plugin: cannot parse manifest for update check",
			"plugin", pluginName, "error", err)
		return ""
	}

	remote, err := u.checkRemoteVersion(ctx, accessToken, pluginName)
	if err != nil {
		slog.Warn("plugin: failed to check remote version",
			"plugin", pluginName, "error", err)
		return ""
	}

	if remote == nil || remote.Version == "" || remote.DownloadURL == "" {
		slog.Debug("plugin: no remote version info available",
			"plugin", pluginName)
		return ""
	}

	if compareSemver(remote.Version, manifest.Version) <= 0 {
		slog.Debug("plugin: already up to date",
			"plugin", pluginName,
			"local", manifest.Version,
			"remote", remote.Version)
		return ""
	}

	if !promptUpdate(w, os.Stdin, pluginName, manifest.Version, remote.Version, remote.Changelog) {
		return ""
	}

	if err := u.downloadAndInstall(ctx, remote.DownloadURL, pluginDir); err != nil {
		slog.Warn("plugin: failed to download and install update",
			"plugin", pluginName, "error", err)
		fmt.Fprintf(w, "  Update failed: %v\n", err)
		return ""
	}

	fmt.Fprintf(w, "  ✅ Updated %s to %s\n", pluginName, remote.Version)
	return pluginName
}

// shouldCheck returns true if enough time has elapsed since the last check.
func (u *Updater) shouldCheck() bool {
	checkFile := filepath.Join(u.PluginsDir, lastCheckFileName)
	data, err := os.ReadFile(checkFile)
	if err != nil {
		return true
	}

	lastCheck, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}

	return time.Since(lastCheck) >= config.PluginUpdateCheckInterval
}

// recordCheckTime writes the current time to the last-check file.
func (u *Updater) recordCheckTime() {
	checkFile := filepath.Join(u.PluginsDir, lastCheckFileName)
	_ = os.MkdirAll(filepath.Dir(checkFile), config.DirPerm)
	_ = os.WriteFile(checkFile, []byte(time.Now().Format(time.RFC3339)), config.FilePerm)
}

// checkRemoteVersion queries the aihub API for the latest version.
func (u *Updater) checkRemoteVersion(ctx context.Context, accessToken, pluginName string) (*remoteVersionInfo, error) {
	apiURL := fmt.Sprintf("%s?pluginName=%s&platform=%s",
		pluginDownloadEndpoint,
		url.QueryEscape(pluginName),
		url.QueryEscape(u.Platform),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-user-access-token", accessToken)

	client := &http.Client{Timeout: config.HTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check remote version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result pluginDownloadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if !result.Success {
		errMsg := result.ErrorMsg
		if errMsg == "" {
			errMsg = result.ErrorCode
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return nil, fmt.Errorf("API error: %s", errMsg)
	}

	return result.Result, nil
}

// downloadAndInstall downloads a plugin zip and extracts it, replacing
// the previous version.
func (u *Updater) downloadAndInstall(ctx context.Context, downloadURL, pluginDir string) error {
	tempFile, err := os.CreateTemp("", "dws-plugin-update-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		tempFile.Close()
		return fmt.Errorf("create download request: %w", err)
	}

	client := &http.Client{Timeout: pluginDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		tempFile.Close()
		return fmt.Errorf("download plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tempFile.Close()
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tempFile.Close()

	// Remove old plugin directory contents before extracting.
	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("remove old plugin: %w", err)
	}

	if err := extractPluginZip(tempPath, pluginDir); err != nil {
		return fmt.Errorf("extract plugin: %w", err)
	}

	return nil
}

// extractPluginZip extracts a zip archive to the destination directory
// with zip slip protection.
func extractPluginZip(zipPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)

	for _, file := range reader.File {
		filePath := filepath.Join(destDir, file.Name)

		if !strings.HasPrefix(filepath.Clean(filePath), cleanDest) {
			return fmt.Errorf("invalid file path in zip: %s", file.Name)
		}

		// Reject symlinks in ZIP to prevent path traversal attacks.
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in plugin zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0o755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}

		if err := extractOneFile(file, filePath); err != nil {
			return err
		}
	}

	return nil
}

// extractOneFile extracts one file from a zip archive to disk.
func extractOneFile(file *zip.File, destPath string) error {
	srcFile, err := file.Open()
	if err != nil {
		return fmt.Errorf("open file in zip: %w", err)
	}
	defer srcFile.Close()

	fileMode := file.Mode()
	if fileMode&0o600 == 0 {
		fileMode = 0o644
	}

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("extract file: %w", err)
	}

	return nil
}

// promptUpdate asks the user for confirmation before applying an update.
// Returns true if the user accepts (Y or empty input means yes).
func promptUpdate(w io.Writer, r io.Reader, pluginName, oldVer, newVer, changelog string) bool {
	fmt.Fprintf(w, "🔄 %s %s → %s", pluginName, oldVer, newVer)
	if changelog != "" {
		fmt.Fprintf(w, "\n   %s", changelog)
	}
	fmt.Fprintf(w, "\n   Update? [Y/n] ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false // EOF or error: non-interactive, skip
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}
