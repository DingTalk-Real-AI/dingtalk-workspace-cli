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
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AllowedHosts is the set of trusted DingTalk API hosts.
// Only these hosts may receive access tokens to prevent token leakage.
var AllowedHosts = map[string]bool{
	"api.dingtalk.com":  true,
	"oapi.dingtalk.com": true,
}

// ValidateTargetHost checks that the resolved request URL targets a trusted
// DingTalk host. This prevents access-token leakage to arbitrary domains.
func ValidateTargetHost(fullURL string) error {
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return fmt.Errorf("无法解析请求 URL: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("安全限制: dws api 仅允许 HTTPS 请求")
	}
	if parsed.User != nil {
		return fmt.Errorf("安全限制: 请求 URL 不能包含 userinfo")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("安全限制: 请求 URL 不能包含 fragment")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("安全限制: 请求 URL 仅允许默认 HTTPS 端口 443，当前端口为 %q", port)
	}
	host := strings.ToLower(parsed.Hostname())
	if !AllowedHosts[host] {
		return fmt.Errorf(
			"安全限制: 目标域名 %q 不在允许列表中。\n"+
				"dws api 仅允许向以下域名发起请求:\n"+
				"  - api.dingtalk.com  (新版 API)\n"+
				"  - oapi.dingtalk.com (旧版 API)\n"+
				"请检查 URL 或 --base-url 参数是否正确。",
			host,
		)
	}
	return nil
}

// ValidateRedirect only permits same-origin HTTPS redirects. The raw API token
// is carried in a non-standard header (or a legacy query parameter), so Go's
// default cross-origin credential stripping is not sufficient here.
func ValidateRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("安全限制: API 重定向次数超过 10 次")
	}
	if err := ValidateTargetHost(req.URL.String()); err != nil {
		return fmt.Errorf("安全限制: 拒绝 API 重定向: %w", err)
	}
	if len(via) == 0 {
		return nil
	}
	origin := via[0].URL
	if !sameHTTPSOrigin(origin, req.URL) {
		return fmt.Errorf("安全限制: 拒绝跨域或 HTTPS 降级重定向 (%s -> %s)", origin.Redacted(), req.URL.Redacted())
	}
	return nil
}

func sameHTTPSOrigin(a, b *url.URL) bool {
	return a != nil && b != nil &&
		strings.EqualFold(a.Scheme, "https") && strings.EqualFold(b.Scheme, "https") &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && effectiveHTTPSPort(a) == effectiveHTTPSPort(b)
}

func effectiveHTTPSPort(u *url.URL) string {
	if u == nil || u.Port() == "" {
		return "443"
	}
	return u.Port()
}

// ValidateMethod checks that the HTTP method is one of the five allowed methods.
func ValidateMethod(method string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(method))
	if !AllowedMethods[upper] {
		return "", fmt.Errorf("不支持的 HTTP 方法: %s (允许: GET, POST, PUT, PATCH, DELETE)", method)
	}
	return upper, nil
}

// ValidatePath checks the API path for injection attacks and dangerous characters.
func ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("API 路径不能为空")
	}
	if err := rejectDangerousChars(path, "path"); err != nil {
		return err
	}
	// Reject path traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("API 路径不能包含 '..' (路径遍历)")
	}
	return nil
}

// ValidateUserInput checks a user-provided string for control characters and
// dangerous Unicode codepoints that could enable injection attacks.
func ValidateUserInput(value, fieldName string) error {
	return rejectDangerousChars(value, fieldName)
}

// rejectDangerousChars rejects C0 control characters (except \t and \n),
// DEL (0x7F), and dangerous Unicode codepoints in a string.
func rejectDangerousChars(s, fieldName string) error {
	for i, r := range s {
		// Allow tab and newline
		if r == '\t' || r == '\n' {
			continue
		}
		// Reject C0 control chars (0x00-0x1F) and DEL (0x7F)
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%s 包含非法控制字符 (位置 %d, U+%04X)", fieldName, i, r)
		}
		// Reject dangerous Unicode
		if isDangerousUnicode(r) {
			return fmt.Errorf("%s 包含危险 Unicode 字符 (位置 %d, U+%04X)", fieldName, i, r)
		}
	}
	return nil
}

// isDangerousUnicode returns true for Unicode codepoints that can be used
// for visual spoofing or terminal injection attacks.
func isDangerousUnicode(r rune) bool {
	switch {
	// Zero-width characters
	case r >= 0x200B && r <= 0x200D:
		return true
	// BOM
	case r == 0xFEFF:
		return true
	// Bidi override characters
	case r >= 0x202A && r <= 0x202E:
		return true
	// Line/paragraph separator
	case r == 0x2028 || r == 0x2029:
		return true
	// Bidi isolate characters
	case r >= 0x2066 && r <= 0x2069:
		return true
	// Additional Bidi controls
	case r == 0x061C:
		return true
	// Non-characters
	case r >= 0xFDD0 && r <= 0xFDEF:
		return true
	}
	// Object replacement (U+FFFC) / replacement (U+FFFD) characters and
	// other non-printable non-ASCII runes (e.g. CJK, symbols) are allowed
	// through — only the explicit dangerous ranges above are blocked.
	return false
}

// ValidateStdinExclusion checks that --params and --data don't both read from stdin.
func ValidateStdinExclusion(params, data string) error {
	if strings.TrimSpace(params) == "-" && strings.TrimSpace(data) == "-" {
		return fmt.Errorf("--params 和 --data 不能同时从 stdin 读取 (-)")
	}
	return nil
}

// ValidateInputStdinExclusion ensures at most one API input consumes stdin.
func ValidateInputStdinExclusion(params, data string, file *FileUpload) error {
	count := 0
	if strings.TrimSpace(params) == "-" {
		count++
	}
	if strings.TrimSpace(data) == "-" {
		count++
	}
	if file != nil && strings.TrimSpace(file.Path) == "-" {
		count++
	}
	if count > 1 {
		return fmt.Errorf("--params、--data 和 --file 最多一个可以从 stdin 读取 (-)")
	}
	return nil
}

// ValidateFlagExclusion checks mutual exclusion between flags.
func ValidateFlagExclusion(outputPath string, pageAll bool) error {
	if strings.TrimSpace(outputPath) != "" && pageAll {
		return fmt.Errorf("--output 和 --page-all 不能同时使用")
	}
	return nil
}
