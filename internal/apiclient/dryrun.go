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

package apiclient

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MaskToken returns a masked version of a token for display in dry-run
// and log output. Shows the first 4 characters followed by "****".
func MaskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return token[:4] + "****"
}

// PrintDryRun outputs a dry-run preview of the API request that would be sent.
func PrintDryRun(w io.Writer, req RawAPIRequest, baseURL, token string) error {
	fullURL := NormalisePath(req.Path, baseURL)

	fmt.Fprintln(w, "=== Dry Run ===")
	fmt.Fprintf(w, "%-12s%s\n", "Method:", strings.ToUpper(req.Method))
	fmt.Fprintf(w, "%-12s%s\n", "URL:", fullURL)

	if len(req.Params) > 0 {
		paramsJSON, err := json.MarshalIndent(req.Params, "            ", "  ")
		if err == nil {
			fmt.Fprintf(w, "%-12s%s\n", "Params:", string(paramsJSON))
		}
	}
	if req.ParamsSource != "" {
		fmt.Fprintf(w, "%-12s%s\n", "Params:", deferredSourceLabel(req.ParamsSource))
	}

	if req.File != nil {
		fmt.Fprintf(w, "%-12s%s\n", "Content-Type:", "multipart/form-data; boundary=<generated at send time>")
		if req.Data != nil {
			dataJSON, err := json.MarshalIndent(req.Data, "            ", "  ")
			if err == nil {
				fmt.Fprintf(w, "%-12s%s\n", "Form fields:", string(dataJSON))
			}
		}
		if req.DataSource != "" {
			fmt.Fprintf(w, "%-12s%s\n", "Form fields:", deferredSourceLabel(req.DataSource))
		}
		path := req.File.Path
		if path == "-" {
			path = "<stdin>"
		}
		fmt.Fprintf(w, "%-12s%s=%s (not opened)\n", "File:", req.File.FieldName, path)
	} else {
		if req.Data != nil {
			dataJSON, err := json.MarshalIndent(req.Data, "            ", "  ")
			if err == nil {
				fmt.Fprintf(w, "%-12s%s\n", "Body:", string(dataJSON))
			}
		}
		if req.DataSource != "" {
			fmt.Fprintf(w, "%-12s%s\n", "Body:", deferredSourceLabel(req.DataSource))
		}
	}

	if IsLegacyAPI(fullURL) {
		fmt.Fprintf(w, "%-12s%s=%s\n", "Auth:", LegacyAuthParam, MaskToken(token))
		fmt.Fprintf(w, "%-12s%s\n", "Style:", "旧版 (oapi.dingtalk.com)")
	} else {
		fmt.Fprintf(w, "%-12s%s: %s\n", "Auth:", AuthHeader, MaskToken(token))
		fmt.Fprintf(w, "%-12s%s\n", "Style:", "新版 (api.dingtalk.com)")
	}
	fmt.Fprintln(w, "===============")
	return nil
}

func deferredSourceLabel(source string) string {
	if strings.TrimSpace(source) == "-" {
		return "<stdin> (not read)"
	}
	return source + " (not opened)"
}
