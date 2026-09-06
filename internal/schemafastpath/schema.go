// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemafastpath

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillpaths"
)

// Dependencies is the invocation snapshot and the filesystem boundary. It does
// not load user configuration or perform any network or credential operation.
type Dependencies struct {
	Args        []string
	Environment []string
	Lstat       func(string) (os.FileInfo, error)
	OpenCache   func(string) (*schemacache.Cache, error)
}

// Prepared owns the exact JSON bytes, including the terminating newline.
// JSONErrors records whether the accepted invocation explicitly requested JSON.
type Prepared struct {
	Data       []byte
	JSONErrors bool
}

// Write starts publication. Callers must never fall back after this call,
// including on a short write or an error after a partial write.
func (p Prepared) Write(w io.Writer) error {
	n, err := w.Write(p.Data)
	if err == nil && n != len(p.Data) {
		return io.ErrShortWrite
	}
	return err
}

type schemaRequest struct {
	path       string
	compact    bool
	jsonErrors bool
}

// parseSchemaRequest recognizes a strict subset of the public argv contract.
// All unsupported/ambiguous arguments remain byte-for-byte owned by Cobra.
func parseSchemaRequest(args []string) (schemaRequest, bool) {
	var request schemaRequest
	if len(args) < 2 || args[1] != "schema" {
		return request, false
	}
	seen := map[string]bool{}
	positional, cliPath := "", ""
	hasPositional := false
	for i := 2; i < len(args); i++ {
		arg := args[i]
		name, value, assigned := strings.Cut(arg, "=")
		switch name {
		case "--compact":
			if seen[name] {
				return schemaRequest{}, false
			}
			seen[name] = true
			if !assigned {
				// Core's preparse phase consumes detached boolean values.
				// Decline them instead of treating them as Schema paths.
				if i+1 < len(args) {
					if _, err := strconv.ParseBool(args[i+1]); err == nil {
						return schemaRequest{}, false
					}
				}
				value = "true"
			}
			if value != "true" && value != "false" {
				return schemaRequest{}, false
			}
			request.compact = value == "true"
		case "--cli-path", "--format", "-f":
			key := name
			if key == "-f" {
				key = "--format"
			}
			if seen[key] {
				return schemaRequest{}, false
			}
			seen[key] = true
			if !assigned {
				i++
				if i == len(args) {
					return schemaRequest{}, false
				}
				value = args[i]
			}
			if name == "--cli-path" {
				if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "-") {
					return schemaRequest{}, false
				}
				cliPath = strings.TrimSpace(value)
			} else if value != "json" {
				return schemaRequest{}, false
			} else {
				request.jsonErrors = true
			}
		default:
			if strings.HasPrefix(arg, "-") || hasPositional || strings.TrimSpace(arg) == "" {
				return schemaRequest{}, false
			}
			hasPositional = true
			positional = strings.TrimSpace(arg)
		}
	}
	if cliPath != "" && hasPositional {
		return schemaRequest{}, false
	}
	if cliPath != "" {
		request.path = cliPath
	} else if !strings.EqualFold(positional, "list") {
		request.path = positional
	}
	return request, true
}

// schemaEnvironmentIsPlain leaves metadata validation, diagnostics, profile
// overrides and future DWS behavior to core. Never read a credential/config
// file merely to authorize a fast path.
func schemaEnvironmentIsPlain(environment []string) bool {
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "DWS_") || value == "" {
			continue
		}
		switch key {
		case "DWS_CONFIG_DIR":
			if strings.TrimSpace(value) != value || !filepath.IsAbs(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// SupportsRequest reports whether argv and environment are inside the closed
// launcher Schema capability before any cache or user filesystem access.
func SupportsRequest(args, environment []string) bool {
	_, ok := parseSchemaRequest(args)
	return ok && schemaEnvironmentIsPlain(environment)
}

// PlainEnvironment reports the shared build-time allowlist for presentation
// capabilities. Filesystem extension absence remains part of PlainInvocation.
func PlainEnvironment(environment []string) bool {
	return schemaEnvironmentIsPlain(environment)
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		if name == key {
			return value
		}
	}
	return ""
}

// Extensions can replace the CLI tree or install preparse/output hooks. Their
// absence is a prerequisite, not an interpretation of their configuration.
func schemaExtensionsAbsent(deps Dependencies) bool {
	directory := invocationConfigDir(deps.Environment)
	if directory == "" {
		return false
	}
	// Reject dangling links and unreadable/non-directory ancestry. Absence below
	// a plain directory is safe; no creation or user-file reading is performed.
	for parent := directory; ; parent = filepath.Dir(parent) {
		info, err := deps.Lstat(parent)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0) {
			return false
		}
		if next := filepath.Dir(parent); next == parent {
			break
		}
	}
	// Core emits a compatibility warning for nested installs left by older
	// upgraders. Let it diagnose any candidate layout; never hide that warning.
	home := environmentValue(deps.Environment, "HOME")
	if !filepath.IsAbs(home) {
		return false
	}
	for _, relative := range skillpaths.AgentHomes() {
		if _, err := deps.Lstat(filepath.Join(home, relative, "dws", "multi")); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	for _, name := range []string{"settings.json", "plugins", "shortcuts"} {
		if _, err := deps.Lstat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

func invocationConfigDir(environment []string) string {
	if directory := environmentValue(environment, "DWS_CONFIG_DIR"); directory != "" {
		return directory
	}
	if home := environmentValue(environment, "HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".dws")
	}
	return ""
}

// PlainInvocation checks the proven edition and the shared diagnostic/extension
// boundary without opening a Schema cache. Presentation-only callers may use
// the returned config directory for the same read-only identity snapshot as core.
// A successful check does not replace the caller's tracker or output lifecycle.
func PlainInvocation(edition string, identity *schemareader.Identity, deps Dependencies) (string, bool) {
	if identity == nil || !schemaEnvironmentIsPlain(deps.Environment) {
		return "", false
	}
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		return "", false
	}
	if edition != "open" || identity.Edition != edition || identity.Validate() != nil || deps.Lstat == nil {
		return "", false
	}
	if !schemaExtensionsAbsent(deps) {
		return "", false
	}
	return invocationConfigDir(deps.Environment), true
}

// Prepare authenticates and renders a supported, plain invocation before output.
// The caller owns telemetry, signal handling and output error presentation.
// A miss leaves all output untouched and must use normal command execution.
func Prepare(edition string, identity *schemareader.Identity, deps Dependencies) (Prepared, bool) {
	request, ok := parseSchemaRequest(deps.Args)
	if !ok || deps.OpenCache == nil {
		return Prepared{}, false
	}
	if _, ok := PlainInvocation(edition, identity, deps); !ok {
		return Prepared{}, false
	}
	cache, err := deps.OpenCache(identity.Edition)
	if err != nil {
		return Prepared{}, false
	}
	defer cache.Close()
	meta, err := schemareader.ReadMeta(cache, *identity)
	if err != nil {
		return Prepared{}, false
	}
	var payload map[string]any
	if request.path == "" {
		payload, err = meta.Overview.ToPayload()
		if err == nil {
			schemaruntime.StampTrustedHashes(payload, schemaruntime.TrustedHashes{
				CatalogHash: "sha256:" + hex.EncodeToString(identity.SourceSHA256[:]),
				SurfaceHash: "sha256:" + hex.EncodeToString(identity.SurfaceSHA256[:]),
			})
		}
	} else {
		productID, found := schemareader.Locator(meta, request.path)
		if !found {
			return Prepared{}, false
		}
		product, readErr := schemareader.ReadProduct(cache, *identity, meta, productID)
		if readErr != nil {
			return Prepared{}, false
		}
		payload, err = schemaruntime.RenderQuery(product.Registry, product.Index, request.path)
	}
	if err != nil {
		return Prepared{}, false
	}
	if request.compact {
		payload = schemaruntime.Compact(payload)
	}
	data, err := jsonutil.MarshalIndent(payload, "", "  ")
	if err != nil {
		return Prepared{}, false
	}
	return Prepared{Data: append(data, '\n'), JSONErrors: request.jsonErrors}, true
}
