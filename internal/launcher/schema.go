// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillpaths"
)

type schemaRequest struct {
	path    string
	compact bool
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
func schemaExtensionsAbsent(deps dependencies) bool {
	directory := environmentValue(deps.environ, "DWS_CONFIG_DIR")
	if directory == "" {
		home := environmentValue(deps.environ, "HOME")
		if !filepath.IsAbs(home) {
			return false
		}
		directory = filepath.Join(home, ".dws")
	}
	// Reject dangling links and unreadable/non-directory ancestry. Absence below
	// a plain directory is safe; no creation or user-file reading is performed.
	for parent := directory; ; parent = filepath.Dir(parent) {
		info, err := deps.lstat(parent)
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
	home := environmentValue(deps.environ, "HOME")
	if !filepath.IsAbs(home) {
		return false
	}
	for _, relative := range skillpaths.AgentHomes() {
		if _, err := deps.lstat(filepath.Join(home, relative, "dws", "multi")); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	for _, name := range []string{"settings.json", "plugins"} {
		if _, err := deps.lstat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

func trySchema(options Options, deps dependencies) (bool, error) {
	request, ok := parseSchemaRequest(deps.args)
	identity := options.SchemaIdentity
	if !ok || identity == nil || !telemetryOptedOut(deps.environ) || !schemaEnvironmentIsPlain(deps.environ) {
		return false, nil
	}
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		return false, nil
	}
	// Only the overlay-free open edition is currently proven by the generator.
	if options.Edition != "open" || identity.Edition != options.Edition || identity.Validate() != nil || deps.openSchemaCache == nil {
		return false, nil
	}
	if !schemaExtensionsAbsent(deps) {
		return false, nil
	}
	cache, err := deps.openSchemaCache(identity.Edition)
	if err != nil {
		return false, nil
	}
	defer cache.Close()
	meta, err := schemareader.ReadMeta(cache, *identity)
	if err != nil {
		return false, nil
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
			return false, nil
		}
		product, readErr := schemareader.ReadProduct(cache, *identity, meta, productID)
		if readErr != nil {
			return false, nil
		}
		payload, err = schemaruntime.RenderQuery(product.Registry, product.Index, request.path)
	}
	if err != nil {
		return false, nil
	}
	if request.compact {
		payload = schemaruntime.Compact(payload)
	}
	data, err := jsonutil.MarshalIndent(payload, "", "  ")
	if err != nil {
		return false, nil
	}
	// Commit output only after all authentication, decoding and rendering succeed.
	data = append(data, '\n')
	n, err := deps.stdout.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return true, err
}
