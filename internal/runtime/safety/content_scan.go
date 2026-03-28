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

package safety

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Finding struct {
	Path     string `json:"path"`
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
	Snippet  string `json:"snippet,omitempty"`
}

type Report struct {
	Scanned  bool      `json:"scanned"`
	Findings []Finding `json:"findings,omitempty"`
}

type Scanner interface {
	ScanPayload(payload any) Report
}

type ContentScanner struct {
	patterns    []scanPattern
	maxFindings int
}

type scanPattern struct {
	Name     string
	Severity string
	Expr     *regexp.Regexp
}

func NewContentScanner() *ContentScanner {
	return &ContentScanner{
		patterns: []scanPattern{
			{
				Name:     "ignore_previous_instructions",
				Severity: "high",
				Expr:     regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior)\s+instructions?`),
			},
			{
				Name:     "reveal_system_prompt",
				Severity: "high",
				Expr:     regexp.MustCompile(`(?i)(reveal|show|print).*(system\s+prompt|developer\s+message|hidden\s+instructions?)`),
			},
			{
				Name:     "policy_bypass",
				Severity: "medium",
				Expr:     regexp.MustCompile(`(?i)(bypass|disable|override).*(safety|guardrails?|restrictions?)`),
			},
			{
				Name:     "cn_ignore_instruction",
				Severity: "high",
				Expr:     regexp.MustCompile(`(?i)(忽略|无视).*(之前|上述).*(指令|说明)`),
			},
			{
				Name:     "cn_reveal_prompt",
				Severity: "high",
				Expr:     regexp.MustCompile(`(?i)(泄露|暴露|显示).*(系统提示|系统指令|开发者消息)`),
			},
		},
		maxFindings: 20,
	}
}

func (s *ContentScanner) ScanPayload(payload any) Report {
	if s == nil {
		return Report{Scanned: false}
	}

	findings := make([]Finding, 0, 4)
	s.walkPayload("$", payload, &findings)
	if len(findings) > s.maxFindings {
		findings = findings[:s.maxFindings]
	}
	return Report{
		Scanned:  true,
		Findings: findings,
	}
}

func (s *ContentScanner) walkPayload(path string, value any, findings *[]Finding) {
	if s == nil || findings == nil {
		return
	}
	if len(*findings) >= s.maxFindings {
		return
	}

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "." + key
			s.walkPayload(childPath, typed[key], findings)
			if len(*findings) >= s.maxFindings {
				return
			}
		}
	case []any:
		for idx, item := range typed {
			childPath := path + "[" + strconv.Itoa(idx) + "]"
			s.walkPayload(childPath, item, findings)
			if len(*findings) >= s.maxFindings {
				return
			}
		}
	case string:
		s.scanString(path, typed, findings)
	}
}

func (s *ContentScanner) scanString(path, value string, findings *[]Finding) {
	if s == nil || findings == nil {
		return
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	for _, pattern := range s.patterns {
		if !pattern.Expr.MatchString(trimmed) {
			continue
		}
		*findings = append(*findings, Finding{
			Path:     path,
			Pattern:  pattern.Name,
			Severity: pattern.Severity,
			Snippet:  sanitizeSnippet(trimmed),
		})
		if len(*findings) >= s.maxFindings {
			return
		}
	}
}

func sanitizeSnippet(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.Join(strings.Fields(value), " ")
	const limit = 160
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-3]) + "..."
}
