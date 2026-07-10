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

package auth

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// PortableImportReport summarizes bundle metadata consumed during import.
type PortableImportReport struct {
	BundleOS   string
	OSMismatch bool
}

// PortableExportSupported reports whether the current platform can produce a
// bundle that includes the file-based DEK required for import elsewhere.
func PortableExportSupported() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	return os.Getenv(keychain.DisableKeychainEnv) != ""
}

// PortableAuthTargetPopulated reports whether local auth files would be
// overwritten by a portable import.
func PortableAuthTargetPopulated(configDir string) bool {
	if TokenDataExistsKeychain() {
		return true
	}
	if _, err := os.Stat(ProfilesPath(configDir)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(configDir, "app.json")); err == nil {
		return true
	}
	encPath := filepath.Join(keychain.StorageDir(keychain.Service), keychain.AccountToken+".enc")
	if _, err := os.Stat(encPath); err == nil {
		return true
	}
	return false
}

// PortableAuthSourceReady reports whether encrypted auth token exists for export.
func PortableAuthSourceReady() bool {
	if !portableAuthSourcePopulated(keychain.StorageDir(keychain.Service)) {
		return false
	}
	_, err := LoadTokenDataKeychain()
	return err == nil
}

func portableAuthSourcePopulated(keychainDir string) bool {
	_, err := os.Stat(filepath.Join(keychainDir, keychain.AccountToken+".enc"))
	return err == nil
}

const portableAuthManifest = "manifest.json"

type portableAuthBundleManifest struct {
	Version         int      `json:"version"`
	CreatedAt       string   `json:"created_at"`
	OS              string   `json:"os"`
	KeychainService string   `json:"keychain_service"`
	ConfigFiles     []string `json:"config_files,omitempty"`
}

// ExportPortableAuthBundle writes a portable auth bundle as tar.gz.
// It copies the encrypted keychain files plus the small config files needed
// to refresh tokens in another Linux sandbox.
func ExportPortableAuthBundle(configDir string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("missing output writer")
	}
	if !PortableExportSupported() {
		return fmt.Errorf("portable export requires file-DEK mode on macOS; set %s=1 and verify auth first, resetting and re-logging in only if the existing token cannot be decrypted", keychain.DisableKeychainEnv)
	}
	keychainDir := keychain.StorageDir(keychain.Service)
	if _, err := os.Stat(keychainDir); err != nil {
		return fmt.Errorf("auth keychain directory is not available: %w", err)
	}
	if !portableAuthSourcePopulated(keychainDir) {
		return fmt.Errorf("auth token is not available for export; run dws auth login first")
	}
	if _, err := LoadTokenDataKeychain(); err != nil {
		return fmt.Errorf("auth token cannot be decrypted with the portable file DEK: %w", err)
	}

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	configFiles, err := portableConfigFiles(configDir)
	if err != nil {
		return err
	}
	manifest := portableAuthBundleManifest{
		Version:         1,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		OS:              runtime.GOOS,
		KeychainService: keychain.Service,
		ConfigFiles:     configFiles,
	}
	if err := writePortableManifest(tw, manifest); err != nil {
		return err
	}

	if err := addPortableDir(tw, keychainDir, path.Join("keychain", keychain.Service)); err != nil {
		return err
	}
	for _, name := range configFiles {
		src := filepath.Join(configDir, name)
		if err := addPortableFile(tw, src, path.Join("config", filepath.ToSlash(name))); err != nil {
			return err
		}
	}
	return nil
}

// ImportPortableAuthBundle extracts a tar.gz auth bundle into the current
// config and keychain locations.
func ImportPortableAuthBundle(configDir string, r io.Reader) (PortableImportReport, error) {
	if r == nil {
		return PortableImportReport{}, fmt.Errorf("missing input reader")
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return PortableImportReport{}, fmt.Errorf("open auth bundle: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	keychainDir := keychain.StorageDir(keychain.Service)
	var manifest portableAuthBundleManifest
	manifestRead := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return PortableImportReport{}, fmt.Errorf("read auth bundle: %w", err)
		}
		if hdr == nil {
			continue
		}
		cleanName, err := cleanPortableName(hdr.Name)
		if err != nil {
			return PortableImportReport{}, err
		}
		if cleanName == portableAuthManifest {
			if err := json.NewDecoder(tr).Decode(&manifest); err != nil {
				return PortableImportReport{}, fmt.Errorf("read auth bundle manifest: %w", err)
			}
			manifestRead = true
			continue
		}

		var target string
		switch {
		case strings.HasPrefix(cleanName, "keychain/"+keychain.Service+"/"):
			rel := strings.TrimPrefix(cleanName, "keychain/"+keychain.Service+"/")
			target, err = safeJoin(keychainDir, rel)
		case strings.HasPrefix(cleanName, "config/"):
			rel := strings.TrimPrefix(cleanName, "config/")
			target, err = safeJoin(configDir, rel)
		default:
			return PortableImportReport{}, fmt.Errorf("unsupported auth bundle path %q", hdr.Name)
		}
		if err != nil {
			return PortableImportReport{}, err
		}
		if err := extractPortableEntry(target, hdr, tr); err != nil {
			return PortableImportReport{}, err
		}
	}
	report := PortableImportReport{}
	if manifestRead {
		report.BundleOS = manifest.OS
		report.OSMismatch = manifest.OS != "" && manifest.OS != runtime.GOOS
	}
	return report, nil
}

func portableConfigFiles(configDir string) ([]string, error) {
	var files []string
	patterns := []string{"app*.json", profilesJSONFile, "mcp_url", "terminal_url"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(configDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("scan config files: %w", err)
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(configDir, match)
			if err != nil {
				return nil, fmt.Errorf("resolve config file: %w", err)
			}
			files = append(files, rel)
		}
	}
	sort.Strings(files)
	return files, nil
}

func writePortableManifest(tw *tar.Writer, manifest portableAuthBundleManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth bundle manifest: %w", err)
	}
	return writePortableBytes(tw, portableAuthManifest, append(data, '\n'), config.FilePerm)
}

func addPortableDir(tw *tar.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if filePath == root {
				return nil
			}
			rel, err := filepath.Rel(root, filePath)
			if err != nil {
				return err
			}
			name := path.Join(prefix, filepath.ToSlash(rel))
			return tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: int64(config.DirPerm)})
		}
		return addPortableFile(tw, filePath, path.Join(prefix, mustPortableRel(root, filePath)))
	})
}

func mustPortableRel(root, filePath string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return filepath.Base(filePath)
	}
	return filepath.ToSlash(rel)
}

func addPortableFile(tw *tar.Writer, src, name string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if info.IsDir() {
		return nil
	}
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer file.Close()

	if err := tw.WriteHeader(&tar.Header{
		Name:    path.Clean(name),
		Size:    info.Size(),
		Mode:    int64(config.FilePerm),
		ModTime: info.ModTime(),
	}); err != nil {
		return fmt.Errorf("write auth bundle header: %w", err)
	}
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("write auth bundle file: %w", err)
	}
	return nil
}

func writePortableBytes(tw *tar.Writer, name string, data []byte, mode os.FileMode) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:    path.Clean(name),
		Size:    int64(len(data)),
		Mode:    int64(mode),
		ModTime: time.Now(),
	}); err != nil {
		return fmt.Errorf("write auth bundle header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write auth bundle data: %w", err)
	}
	return nil
}

func cleanPortableName(name string) (string, error) {
	name = path.Clean(strings.TrimSpace(name))
	if name == "." || name == "/" || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe auth bundle path %q", name)
	}
	return name, nil
}

func safeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty auth bundle path")
	}
	rel = filepath.FromSlash(path.Clean(rel))
	if filepath.IsAbs(rel) || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("unsafe auth bundle path %q", rel)
	}
	target := filepath.Join(root, rel)
	cleanRoot := filepath.Clean(root) + string(filepath.Separator)
	if target != filepath.Clean(root) && !strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), cleanRoot) {
		return "", fmt.Errorf("unsafe auth bundle path %q", rel)
	}
	return target, nil
}

func extractPortableEntry(target string, hdr *tar.Header, r io.Reader) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, config.DirPerm); err != nil {
			return fmt.Errorf("create auth bundle directory: %w", err)
		}
		return os.Chmod(target, config.DirPerm)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), config.DirPerm); err != nil {
			return fmt.Errorf("create auth bundle directory: %w", err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*.tmp")
		if err != nil {
			return fmt.Errorf("create auth bundle temp file: %w", err)
		}
		tmpName := tmp.Name()
		success := false
		defer func() {
			if !success {
				tmp.Close()
				_ = os.Remove(tmpName)
			}
		}()
		if err := tmp.Chmod(config.FilePerm); err != nil {
			return fmt.Errorf("set auth bundle file permissions: %w", err)
		}
		if _, err := io.Copy(tmp, r); err != nil {
			return fmt.Errorf("write auth bundle file: %w", err)
		}
		if err := tmp.Sync(); err != nil {
			return fmt.Errorf("sync auth bundle file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("close auth bundle file: %w", err)
		}
		if err := os.Rename(tmpName, target); err != nil {
			return fmt.Errorf("install auth bundle file: %w", err)
		}
		success = true
		return nil
	default:
		return fmt.Errorf("unsupported auth bundle entry type %d for %q", hdr.Typeflag, hdr.Name)
	}
}
