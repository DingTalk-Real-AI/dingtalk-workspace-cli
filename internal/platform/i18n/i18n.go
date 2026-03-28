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

// Package i18n provides lightweight internationalization for the DWS CLI.
//
// At startup call Init() (or let the package auto-init via the init function).
// Use T("key") to get a translated string, or Tf("key", args...) for formatted
// translations with fmt.Sprintf-style placeholders.
//
// The active language is resolved from the DWS_LANG environment variable first,
// then falls back to the LANG environment variable. If neither is set or does
// not match a supported locale, "en" (English) is used as the default.
//
// Message catalogs are stored as JSON files under locales/ and embedded at
// compile time via go:embed.
//
// Supported locales:
//   - "en" — English (default)
//   - "zh" — Simplified Chinese
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

// lang is the resolved language tag for the current session.
var (
	lang      language.Tag
	langStr   string
	once      sync.Once
	enCatalog map[string]string
	zhCatalog map[string]string
)

func init() {
	Init()
}

// Init resolves the active locale from environment variables and loads
// the message catalogs from embedded JSON files. Safe to call multiple
// times; only the first call takes effect.
func Init() {
	once.Do(func() {
		enCatalog = loadCatalog("en")
		zhCatalog = loadCatalog("zh")

		raw := strings.TrimSpace(os.Getenv("DWS_LANG"))
		if raw == "" {
			raw = strings.TrimSpace(os.Getenv("LANG"))
		}
		setLangFromRaw(raw)
	})
}

// SetLang overrides the active locale. Intended for testing.
func SetLang(tag string) {
	setLangFromRaw(tag)
}

func setLangFromRaw(raw string) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(raw, "zh"):
		lang = language.Chinese
		langStr = "zh"
	default:
		lang = language.English
		langStr = "en"
	}
}

// Lang returns the current language tag string ("en" or "zh").
func Lang() string {
	return langStr
}

// LangTag returns the resolved language.Tag.
func LangTag() language.Tag {
	return lang
}

// T returns the translated string for the given message key.
// If the key is not found in the active catalog, the key itself is returned.
func T(key string) string {
	catalog := catalogForLang(langStr)
	if msg, ok := catalog[key]; ok {
		return msg
	}
	// Fallback: try English catalog
	if langStr != "en" {
		if msg, ok := enCatalog[key]; ok {
			return msg
		}
	}
	return key
}

// Tf returns a formatted translated string. The key is looked up in the
// active message catalog and the result is passed through fmt.Sprintf with
// the provided arguments.
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

func catalogForLang(l string) map[string]string {
	switch l {
	case "zh":
		return zhCatalog
	default:
		return enCatalog
	}
}

func loadCatalog(lang string) map[string]string {
	data, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return map[string]string{}
	}
	var catalog map[string]string
	if err := json.Unmarshal(data, &catalog); err != nil {
		return map[string]string{}
	}
	return catalog
}
