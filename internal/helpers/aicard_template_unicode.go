// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

// aicardTemplateUnicodeValid scans JSON string tokens before encoding/json can
// replace unpaired surrogates. The caller has already checked JSON syntax and
// UTF-8 bytes. Escaped backslashes are skipped as a unit, so literal "\\ud800"
// text remains valid, as do non-BMP characters and paired surrogate escapes.
func aicardTemplateUnicodeValid(data []byte) bool {
	quoted := false
	for i := 0; i < len(data); {
		if data[i] == '"' {
			quoted = !quoted
			i++
			continue
		}
		if !quoted || data[i] != '\\' {
			i++
			continue
		}
		if i+1 >= len(data) {
			return false
		}
		if data[i+1] != 'u' {
			i += 2
			continue
		}
		unit, ok := aicardTemplateUnicodeEscape(data[i:])
		if !ok {
			return false
		}
		i += 6
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			low, ok := aicardTemplateUnicodeEscape(data[i:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		case unit >= 0xdc00 && unit <= 0xdfff:
			return false
		}
	}
	return !quoted
}

func aicardTemplateUnicodeEscape(data []byte) (uint16, bool) {
	if len(data) < 6 || data[0] != '\\' || data[1] != 'u' {
		return 0, false
	}
	var value uint16
	for _, digit := range data[2:6] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
