// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const (
	markdownThemeFrontmatterField = "x-we-markdown-theme"
	markdownFrontmatterFence      = "---"
	markdownFrontmatterMaxLines   = 256
	markdownFrontmatterMaxBytes   = 64 * 1024
)

// markdownThemeIDs is the single DWS authority for runtime validation, Cobra
// help and Runtime Schema enum delivery. Keep this list aligned with the
// we-markdown theme registry.
var markdownThemeIDs = [...]string{
	"default",
	"songyan",
	"taiying",
	"sujian",
	"juxia",
	"qingya",
}

type markdownThemeSelection struct {
	id      string
	enabled bool
}

func markdownThemeIDValues() []string {
	return append([]string(nil), markdownThemeIDs[:]...)
}

func markdownThemeHelp() string {
	return "Markdown 主题，可选值: " + strings.Join(markdownThemeIDs[:], " / ") + "；不传则不改写主题 metadata"
}

func markdownThemeFromCommand(cmd *cobra.Command) (markdownThemeSelection, error) {
	if cmd == nil || cmd.Flags().Lookup("theme") == nil || !cmd.Flags().Changed("theme") {
		return markdownThemeSelection{}, nil
	}
	themeID, err := cmd.Flags().GetString("theme")
	if err != nil {
		return markdownThemeSelection{}, fmt.Errorf("读取 --theme 失败: %w", err)
	}
	for _, allowed := range markdownThemeIDs {
		if themeID == allowed {
			return markdownThemeSelection{id: themeID, enabled: true}, nil
		}
	}
	return markdownThemeSelection{}, fmt.Errorf("--theme 取值 %q 不合法（合法值: %s）", themeID, strings.Join(markdownThemeIDs[:], ", "))
}

func markdownTextFileUploadOptionsFromCommand(cmd *cobra.Command) (textFileUploadOptions, error) {
	theme, err := markdownThemeFromCommand(cmd)
	if err != nil {
		return textFileUploadOptions{}, err
	}
	options := textFileUploadOptions{
		dryRunDetails: map[string]any{"theme": theme.id},
	}
	if theme.enabled {
		options.transform = func(source []byte) []byte {
			return applyMarkdownTheme(source, theme.id)
		}
	}
	return options, nil
}

type markdownLineToken struct {
	content []byte
	ending  []byte
	start   int
	end     int
}

type markdownFrontmatterEnvelope struct {
	payloadStart int
	payloadEnd   int
	newline      []byte
}

func readMarkdownBoundedLine(source []byte, start, maxBytes int) (markdownLineToken, bool) {
	if maxBytes <= 0 || start < 0 || start > len(source) {
		return markdownLineToken{}, false
	}
	boundedEnd := start + maxBytes
	if boundedEnd > len(source) {
		boundedEnd = len(source)
	}
	window := source[start:boundedEnd]
	newlineOffset := bytes.IndexByte(window, '\n')
	if newlineOffset < 0 {
		if boundedEnd < len(source) {
			return markdownLineToken{}, false
		}
		return markdownLineToken{content: window, start: start, end: boundedEnd}, true
	}

	newlineIndex := start + newlineOffset
	contentEnd := newlineIndex
	ending := []byte{'\n'}
	if newlineIndex > start && source[newlineIndex-1] == '\r' {
		contentEnd--
		ending = []byte{'\r', '\n'}
	}
	return markdownLineToken{
		content: source[start:contentEnd],
		ending:  ending,
		start:   start,
		end:     newlineIndex + 1,
	}, true
}

func markdownRuneIsSpace(source []byte) bool {
	if len(source) == 0 {
		return false
	}
	r, _ := utf8.DecodeRune(source)
	// Match ECMAScript RegExp \s, which the we-markdown baseline uses. Go's
	// unicode.IsSpace differs for U+0085 and U+FEFF.
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		'\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	default:
		return r >= '\u2000' && r <= '\u200a'
	}
}

func markdownLineHasTopLevelMapping(line []byte) bool {
	if len(line) == 0 || markdownRuneIsSpace(line) || line[0] == '#' {
		return false
	}
	if line[0] == '-' && len(line) > 1 && markdownRuneIsSpace(line[1:]) {
		return false
	}

	if line[0] == '\'' || line[0] == '"' {
		quote := line[0]
		closingOffset := bytes.IndexByte(line[1:], quote)
		if closingOffset > 0 {
			closingIndex := 1 + closingOffset
			if closingIndex+1 < len(line) && line[closingIndex+1] == ':' {
				afterColon := line[closingIndex+2:]
				return len(afterColon) == 0 || markdownRuneIsSpace(afterColon)
			}
		}
	}
	// This is intentionally also the fallback for a quote-prefixed line. It
	// matches the baseline's final unquoted-key regex alternative, which treats
	// a malformed quote as ordinary key data instead of validating YAML.
	colonIndex := bytes.IndexByte(line, ':')
	if colonIndex <= 0 || line[0] == ':' {
		return false
	}
	afterColon := line[colonIndex+1:]
	return len(afterColon) == 0 || markdownRuneIsSpace(afterColon)
}

func parseMarkdownFrontmatterEnvelope(source []byte) *markdownFrontmatterEnvelope {
	bomLength := 0
	if bytes.HasPrefix(source, []byte{0xef, 0xbb, 0xbf}) {
		bomLength = 3
	}
	openingLine, ok := readMarkdownBoundedLine(source, bomLength, len(markdownFrontmatterFence)+2)
	if !ok || !bytes.Equal(openingLine.content, []byte(markdownFrontmatterFence)) || len(openingLine.ending) == 0 {
		return nil
	}

	cursor := openingLine.end
	scannedBytes := 0
	scannedLines := 0
	hasTopLevelMapping := false
	for cursor < len(source) {
		remainingBytes := markdownFrontmatterMaxBytes - scannedBytes
		line, ok := readMarkdownBoundedLine(source, cursor, remainingBytes)
		if !ok {
			return nil
		}
		scannedLines++
		scannedBytes += line.end - line.start
		if scannedLines > markdownFrontmatterMaxLines || scannedBytes > markdownFrontmatterMaxBytes {
			return nil
		}

		if bytes.Equal(line.content, []byte(markdownFrontmatterFence)) {
			if !hasTopLevelMapping {
				return nil
			}
			return &markdownFrontmatterEnvelope{
				payloadStart: openingLine.end,
				payloadEnd:   line.start,
				newline:      append([]byte(nil), openingLine.ending...),
			}
		}

		if markdownLineHasTopLevelMapping(line.content) {
			hasTopLevelMapping = true
		}
		cursor = line.end
	}
	return nil
}

func tokenizeMarkdownLines(source []byte) []markdownLineToken {
	tokens := make([]markdownLineToken, 0)
	for cursor := 0; cursor < len(source); {
		// The full non-empty remainder always yields a line and advances cursor.
		line, _ := readMarkdownBoundedLine(source, cursor, len(source)-cursor)
		tokens = append(tokens, line)
		cursor = line.end
	}
	return tokens
}

func consumeMarkdownSpaces(source []byte, cursor int) int {
	for cursor < len(source) {
		_, size := utf8.DecodeRune(source[cursor:])
		if !markdownRuneIsSpace(source[cursor:]) {
			break
		}
		cursor += size
	}
	return cursor
}

func isMarkdownThemeFieldLine(line []byte) bool {
	field := []byte(markdownThemeFrontmatterField)
	cursor := 0
	switch {
	case bytes.HasPrefix(line, field):
		cursor = len(field)
	case len(line) >= len(field)+2 && (line[0] == '\'' || line[0] == '"') &&
		bytes.Equal(line[1:1+len(field)], field) && line[1+len(field)] == line[0]:
		cursor = len(field) + 2
	default:
		return false
	}
	cursor = consumeMarkdownSpaces(line, cursor)
	if cursor >= len(line) || line[cursor] != ':' {
		return false
	}
	// The value is deliberately not parsed. Once the containing envelope is
	// valid, any duplicate, quoted or malformed product field is normalized to
	// the one canonical line selected by --theme.
	return true
}

func updateMarkdownThemeField(payload, newline []byte, themeID string) []byte {
	tokens := tokenizeMarkdownLines(payload)
	fieldTokenIndexes := make(map[int]struct{})
	firstFieldTokenIndex := -1
	for index, token := range tokens {
		if !isMarkdownThemeFieldLine(token.content) {
			continue
		}
		fieldTokenIndexes[index] = struct{}{}
		if firstFieldTokenIndex < 0 {
			firstFieldTokenIndex = index
		}
	}

	canonicalLine := []byte(markdownThemeFrontmatterField + ": " + themeID)
	if firstFieldTokenIndex < 0 {
		result := append([]byte(nil), payload...)
		if !bytes.HasSuffix(payload, []byte{'\n'}) {
			result = append(result, newline...)
		}
		result = append(result, canonicalLine...)
		result = append(result, newline...)
		return result
	}

	var result bytes.Buffer
	result.Grow(len(payload) + len(canonicalLine) + len(newline))
	for index, token := range tokens {
		if index == firstFieldTokenIndex {
			result.Write(canonicalLine)
			if len(token.ending) > 0 {
				result.Write(token.ending)
			} else {
				result.Write(newline)
			}
			continue
		}
		if _, remove := fieldTokenIndexes[index]; remove {
			continue
		}
		result.Write(payload[token.start:token.end])
	}
	return result.Bytes()
}

func detectMarkdownNewline(source []byte) []byte {
	newlineIndex := bytes.IndexByte(source, '\n')
	if newlineIndex > 0 && source[newlineIndex-1] == '\r' {
		return []byte{'\r', '\n'}
	}
	return []byte{'\n'}
}

// applyMarkdownTheme preserves a valid leading Front Matter envelope and only
// rewrites the product-owned top-level field. Invalid, unclosed or bounded-scan
// failures remain user content under a new independent product envelope.
func applyMarkdownTheme(source []byte, themeID string) []byte {
	if envelope := parseMarkdownFrontmatterEnvelope(source); envelope != nil {
		payload := updateMarkdownThemeField(source[envelope.payloadStart:envelope.payloadEnd], envelope.newline, themeID)
		result := make([]byte, 0, len(source)+len(payload)-(envelope.payloadEnd-envelope.payloadStart))
		result = append(result, source[:envelope.payloadStart]...)
		result = append(result, payload...)
		result = append(result, source[envelope.payloadEnd:]...)
		return result
	}

	bom := []byte(nil)
	body := source
	if bytes.HasPrefix(source, []byte{0xef, 0xbb, 0xbf}) {
		bom = source[:3]
		body = source[3:]
	}
	newline := detectMarkdownNewline(body)
	result := make([]byte, 0, len(source)+len(themeID)+64)
	result = append(result, bom...)
	result = append(result, markdownFrontmatterFence...)
	result = append(result, newline...)
	result = append(result, markdownThemeFrontmatterField...)
	result = append(result, ':', ' ')
	result = append(result, themeID...)
	result = append(result, newline...)
	result = append(result, markdownFrontmatterFence...)
	if len(body) > 0 {
		result = append(result, newline...)
		result = append(result, body...)
	}
	return result
}
