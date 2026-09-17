// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Signed link targets are replaced explicitly; ordinary links remain intact.
var exportSummaryURL = regexp.MustCompile(`(?i)https?://[^\s<>"'()]+`)

const exportRemovedURL = "[signed-url-removed]"

var exportCredentialKey = regexp.MustCompile(`(?i)^(ossaccesskeyid|signature|security-token|x-oss-(signature|credential|security-token)|x-amz-(signature|credential|security-token))$`)
var exportCredentialAssignment = regexp.MustCompile(`(?i)(ossaccesskeyid|signature|security-token|x-oss-(signature|credential|security-token)|x-amz-(signature|credential|security-token))\s*=`)

type exportRedactions struct {
	Count int
}

func (r *exportRedactions) text(text string) string {
	return exportSummaryURL.ReplaceAllStringFunc(text, func(raw string) string {
		queryStart := strings.IndexByte(raw, '?')
		if queryStart < 0 {
			return raw
		}
		query := html.UnescapeString(raw[queryStart+1:])
		for _, part := range strings.Split(query, "&") {
			key := strings.SplitN(part, "=", 2)[0]
			decoded, err := url.QueryUnescape(key)
			if err != nil {
				// A malformed query cannot be certified credential-free.
				r.Count++
				return exportRemovedURL
			}
			if exportCredentialKey.MatchString(decoded) {
				r.Count++
				return exportRemovedURL
			}
		}
		return raw
	})
}

// Convert through JSON with UseNumber so typed slices/maps are traversed without
// changing large integer IDs or mutating the caller's business payload.
func sanitizeExportArtifact(value any) (any, int, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, 0, fmt.Errorf("export artifact cannot be encoded")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, 0, fmt.Errorf("export artifact cannot be decoded")
	}
	r := &exportRedactions{}
	clean := r.value(normalized)
	return clean, r.Count, nil
}

func (r *exportRedactions) value(v any) any {
	switch x := v.(type) {
	case string:
		// Rich-text fields sometimes contain a JSON document encoded as a string.
		if json.Valid([]byte(x)) && (strings.HasPrefix(strings.TrimSpace(x), "{") || strings.HasPrefix(strings.TrimSpace(x), "[")) {
			d := json.NewDecoder(strings.NewReader(x))
			d.UseNumber()
			var nested any
			if d.Decode(&nested) == nil {
				before := r.Count
				clean := r.value(nested)
				if r.Count != before {
					raw, _ := json.Marshal(clean)
					return string(raw)
				}
			}
		}
		return r.text(x)
	case []any:
		for i := range x {
			x[i] = r.value(x[i])
		}
	case map[string]any:
		for key, child := range x {
			if exportCredentialKey.MatchString(key) {
				if child != "[credential-removed]" {
					x[key] = "[credential-removed]"
					r.Count++
				}
			} else {
				x[key] = r.value(child)
			}
		}
	}
	return v
}

// This is a credential scan before publication, not P1 hash/content readback.
// Unexpected files and symlinks fail closed. Binary media is not parsed as text.
func scanExportCredentials(dir string, artifacts []string, includeMedia bool) error {
	allowed := map[string]bool{"manifest.json": true}
	for _, name := range artifacts {
		suffix := ".json"
		if name == "summary" {
			suffix = ".md"
		}
		allowed[name+suffix] = true
	}
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("export credential scan failed")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("export credential scan rejected symlink")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := minutesRel(dir, path)
		if err != nil {
			return fmt.Errorf("export credential scan failed")
		}
		if includeMedia && strings.HasPrefix(filepath.ToSlash(relative), "media/") {
			return nil
		}
		if !allowed[relative] {
			return fmt.Errorf("export credential scan rejected unexpected file")
		}
		raw, err := minutesReadFile(path)
		if err != nil {
			return fmt.Errorf("export credential scan cannot read artifact")
		}
		var value any = string(raw)
		if filepath.Ext(path) == ".json" {
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			if d.Decode(&value) != nil || !json.Valid(raw) {
				return fmt.Errorf("export credential scan found invalid JSON")
			}
		}
		if containsExportCredentials(value) {
			return fmt.Errorf("export credential scan found sensitive content")
		}
		return nil
	})
}

func containsExportCredentials(v any) bool {
	switch x := v.(type) {
	case string:
		r := &exportRedactions{}
		_ = r.value(x)
		return r.Count > 0 || exportCredentialAssignment.MatchString(html.UnescapeString(x))
	case []any:
		for _, child := range x {
			if containsExportCredentials(child) {
				return true
			}
		}
	case map[string]any:
		for key, child := range x {
			if containsExportCredentials(key) {
				return true
			}
			if exportCredentialKey.MatchString(key) && child != "[credential-removed]" {
				return true
			}
			if containsExportCredentials(child) {
				return true
			}
		}
	}
	return false
}
