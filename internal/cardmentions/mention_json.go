package cardmentions

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type mentionPatch struct {
	start, end int
	value      string
}

// rewriteMentionJSON changes selected string values without reserializing any
// surrounding JSON, retaining field order, whitespace and unrelated escapes.
func rewriteMentionJSON(source string, replacements map[string]string) string {
	original := source
	prefix := ""
	if strings.HasPrefix(source, "\ufeff") {
		prefix = "\ufeff"
		source = strings.TrimPrefix(source, prefix)
	}
	if len(replacements) == 0 {
		return original
	}
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	patches := []mentionPatch{}
	var walk func(string) error
	walk = func(path string) error {
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case string:
			if replacement, ok := replacements[path]; ok && replacement != value {
				end := int(decoder.InputOffset())
				start += strings.IndexByte(source[start:end], '"')
				patches = append(patches, mentionPatch{start, end, encodeMentionString(replacement)})
			}
		case json.Delim:
			if value == '{' {
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil {
						return err
					}
					if err := walk(path + pointer([]string{key.(string)})); err != nil {
						return err
					}
				}
			} else if value == '[' {
				for i := 0; decoder.More(); i++ {
					if err := walk(fmt.Sprintf("%s/%d", path, i)); err != nil {
						return err
					}
				}
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	if err := walk(""); err != nil {
		return original
	}
	if _, err := decoder.Token(); err != io.EOF {
		return original
	}
	for i := len(patches) - 1; i >= 0; i-- {
		patch := patches[i]
		source = source[:patch.start] + patch.value + source[patch.end:]
	}
	return prefix + source
}

func encodeMentionString(value string) string {
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	// A string encoded into strings.Builder cannot fail.
	_ = encoder.Encode(value)
	return strings.TrimSuffix(encoded.String(), "\n")
}
