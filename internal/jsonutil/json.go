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

package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	maxDuplicateScanTokens      = 1_000_000
	maxDuplicateScanKeyBytes    = 4 << 20
	maxDuplicateScanStringBytes = 8 << 20
)

type duplicateScanState struct {
	tokens      int
	keyBytes    int
	stringBytes int
}

// Marshal matches json.Marshal but keeps &, <, and > unchanged. CLI JSON is
// consumed by shells and host processes, not embedded into HTML, so preserving
// URL query separators makes the output easier to copy and parse.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// MarshalIndent matches json.MarshalIndent with HTML escaping disabled.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// RejectDuplicateObjectKeys validates one complete JSON value and rejects
// objects whose meaning depends on a parser's duplicate-key policy.
func RejectDuplicateObjectKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := rejectDuplicateValue(decoder, 0, &duplicateScanState{}); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

// RejectNonCanonicalObjectKeys rejects case-insensitive spellings of known
// protocol fields while leaving unrelated, case-sensitive business keys alone.
func RejectNonCanonicalObjectKeys(data []byte, canonical ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return fmt.Errorf("expected JSON object")
	}
	for key := range object {
		for _, expected := range canonical {
			if key != expected && strings.EqualFold(key, expected) {
				return fmt.Errorf("non-canonical JSON object key %q; expected %q", key, expected)
			}
		}
	}
	return nil
}

func rejectDuplicateValue(decoder *json.Decoder, depth int, state *duplicateScanState) error {
	if depth > 256 {
		return fmt.Errorf("JSON nesting exceeds limit at depth %d", depth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	state.tokens++
	if state.tokens > maxDuplicateScanTokens {
		return fmt.Errorf("JSON token count exceeds safety limit of %d", maxDuplicateScanTokens)
	}
	delim, composite := token.(json.Delim)
	if !composite {
		if value, ok := token.(string); ok {
			state.stringBytes += len(value)
			if state.stringBytes > maxDuplicateScanStringBytes {
				return fmt.Errorf("JSON string data exceeds safety limit of %d bytes", maxDuplicateScanStringBytes)
			}
		}
		return nil
	}
	if delim == '{' {
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			// Decoder.Token guarantees object keys are strings.
			key := keyToken.(string)
			state.tokens++
			state.keyBytes += len(key)
			if state.tokens > maxDuplicateScanTokens {
				return fmt.Errorf("JSON token count exceeds safety limit of %d", maxDuplicateScanTokens)
			}
			if state.keyBytes > maxDuplicateScanKeyBytes {
				return fmt.Errorf("JSON object key data exceeds safety limit of %d bytes", maxDuplicateScanKeyBytes)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q at depth %d", key, depth)
			}
			seen[key] = struct{}{}
			if err := rejectDuplicateValue(decoder, depth+1, state); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	// A successfully read opening delimiter that is not an object is an array.
	for decoder.More() {
		if err := rejectDuplicateValue(decoder, depth+1, state); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
