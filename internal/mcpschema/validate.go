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

// Package mcpschema owns the bounded, fail-closed contract for validating
// arguments against remotely discovered MCP input schemas.
package mcpschema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	maxSchemaDepth          = 32
	maxSchemaNodes          = 1024
	maxSchemaProperties     = 512
	maxRequiredProperties   = 512
	maxSchemaStringBytes    = 256 << 10
	maxAnnotationDepth      = 32
	maxAnnotationNodes      = 4096
	maxAnnotationBytes      = 256 << 10
	maxEnumMembers          = 1024
	maxEnumValueDepth       = 32
	maxEnumValueNodes       = 16384
	maxEnumValueBytes       = 1 << 20
	maxArrayValidationItems = 4096
	maxValidationWork       = 65536
	maxNumberLiteralBytes   = 1024
	maxNumberDigits         = 768
	maxNumberExponent       = 4096
)

const ValidationEvidence = "fresh_core_subset_snapshot"

var supportedSchemaTypes = map[string]struct{}{
	"array": {}, "boolean": {}, "integer": {}, "null": {}, "number": {}, "object": {}, "string": {},
}

// A supported URI identifies how the document is parsed. It does not expand
// the accepted keyword subset below.
var supportedSchemaDialects = map[string]struct{}{
	"http://json-schema.org/draft-06/schema#":      {},
	"http://json-schema.org/draft-07/schema#":      {},
	"https://json-schema.org/draft/2019-09/schema": {},
	"https://json-schema.org/draft/2020-12/schema": {},
}

type schemaValidationState struct {
	schemaNodes     int
	properties      int
	required        int
	schemaStrings   int
	annotationNodes int
	annotationBytes int
	enumMembers     int
	enumValueNodes  int
	enumValueBytes  int
	arrayItems      int
	validationWork  int
	enumIndexes     map[string]map[string]struct{}
}

// ValidateInputSchema validates a decoded JSON object against the deliberately
// narrow remote MCP schema subset. Unknown keywords and malformed supported
// keywords fail closed.
func ValidateInputSchema(params map[string]any, schema map[string]any) error {
	state := &schemaValidationState{enumIndexes: map[string]map[string]struct{}{}}
	if err := validateSchemaSupport("$", schema, 1, state); err != nil {
		return validationError(err)
	}
	if schemaType, valid := schema["type"].(string); !valid || schemaType != "object" {
		return validationError(fmt.Errorf(`$.type must be exactly "object" for an MCP input schema root`))
	}
	if params == nil {
		params = map[string]any{}
	}
	if err := validateValue("$", "$", params, schema, state); err != nil {
		return validationError(err)
	}
	return nil
}

// Digest returns the SHA-256 digest of the deterministic JSON encoding of a
// decoded input schema snapshot.
func Digest(schema map[string]any) (string, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return "", validationError(fmt.Errorf("input schema snapshot cannot be encoded: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validationError(err error) error {
	return apperrors.NewValidation(
		fmt.Sprintf("input schema validation failed: %v", err),
		apperrors.WithReason("mcp_input_schema_validation_failed"),
	)
}

func validateSchemaSupport(path string, schema map[string]any, depth int, state *schemaValidationState) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("%s exceeds schema recursion depth limit of %d", path, maxSchemaDepth)
	}
	state.schemaNodes++
	if state.schemaNodes > maxSchemaNodes {
		return fmt.Errorf("%s exceeds total schema node limit of %d", path, maxSchemaNodes)
	}

	keywords := make([]string, 0, len(schema))
	for keyword := range schema {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	for _, keyword := range keywords {
		raw := schema[keyword]
		if err := accountSchemaString(path, keyword, state); err != nil {
			return err
		}
		switch keyword {
		case "$schema":
			dialect, valid := raw.(string)
			if !valid {
				return fmt.Errorf("%s.$schema must be a supported dialect URI", path)
			}
			if _, supported := supportedSchemaDialects[dialect]; !supported {
				return fmt.Errorf("%s.$schema uses unsupported dialect %q", path, dialect)
			}
			if err := accountSchemaString(path+".$schema", dialect, state); err != nil {
				return err
			}
		case "$id", "$anchor", "$comment", "title", "description":
			value, valid := raw.(string)
			if !valid {
				return fmt.Errorf("%s.%s must be a string", path, keyword)
			}
			if err := accountSchemaString(path+"."+keyword, value, state); err != nil {
				return err
			}
		case "deprecated", "readOnly", "writeOnly":
			if _, valid := raw.(bool); !valid {
				return fmt.Errorf("%s.%s must be a boolean", path, keyword)
			}
		case "examples":
			if _, valid := jsonArray(raw); !valid {
				return fmt.Errorf("%s.examples must be an array", path)
			}
			if err := accountAnnotationValue(path+".examples", raw, 1, state); err != nil {
				return err
			}
		case "default":
			if err := accountAnnotationValue(path+".default", raw, 1, state); err != nil {
				return err
			}
		case "type":
			types, valid := schemaStringList(raw, true)
			if !valid || len(types) == 0 {
				return fmt.Errorf("%s.type is not a supported non-empty string or string array", path)
			}
			for _, schemaType := range types {
				if err := accountSchemaString(path+".type", schemaType, state); err != nil {
					return err
				}
				if _, supported := supportedSchemaTypes[schemaType]; !supported {
					return fmt.Errorf("%s.type contains unsupported type %q", path, schemaType)
				}
			}
		case "enum":
			if err := validateEnum(path+".enum", raw, state); err != nil {
				return err
			}
		case "required":
			required, valid := schemaStringArray(raw, true)
			if !valid {
				return fmt.Errorf("%s.required must be an array of unique strings", path)
			}
			state.required += len(required)
			if state.required > maxRequiredProperties {
				return fmt.Errorf("%s.required exceeds total member limit of %d", path, maxRequiredProperties)
			}
			for _, name := range required {
				if err := accountSchemaString(path+".required", name, state); err != nil {
					return err
				}
			}
		case "properties":
			properties, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.properties must be an object", path)
			}
			state.properties += len(properties)
			if state.properties > maxSchemaProperties {
				return fmt.Errorf("%s.properties exceeds total object property limit of %d", path, maxSchemaProperties)
			}
			names := sortedKeys(properties)
			for _, name := range names {
				if err := accountSchemaString(path+".properties", name, state); err != nil {
					return err
				}
				child, valid := properties[name].(map[string]any)
				if !valid {
					return fmt.Errorf("%s.properties[%q] uses an unsupported boolean or malformed schema", path, name)
				}
				if err := validateSchemaSupport(fmt.Sprintf("%s.properties[%q]", path, name), child, depth+1, state); err != nil {
					return err
				}
			}
		case "items":
			items, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.items uses an unsupported boolean, tuple, or malformed schema", path)
			}
			if err := validateSchemaSupport(path+".items", items, depth+1, state); err != nil {
				return err
			}
		case "additionalProperties":
			if _, valid := raw.(bool); valid {
				continue
			}
			additional, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("%s.additionalProperties must be a boolean or object schema", path)
			}
			if err := validateSchemaSupport(path+".additionalProperties", additional, depth+1, state); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s uses unsupported JSON Schema keyword %q", path, keyword)
		}
	}
	return nil
}

func accountSchemaString(path, value string, state *schemaValidationState) error {
	state.schemaStrings += len(value)
	if state.schemaStrings > maxSchemaStringBytes {
		return fmt.Errorf("%s exceeds total schema string limit of %d bytes", path, maxSchemaStringBytes)
	}
	return nil
}

func accountAnnotationValue(path string, value any, depth int, state *schemaValidationState) error {
	if depth > maxAnnotationDepth {
		return fmt.Errorf("%s exceeds annotation nesting limit of %d", path, maxAnnotationDepth)
	}
	state.annotationNodes++
	if state.annotationNodes > maxAnnotationNodes {
		return fmt.Errorf("%s exceeds total annotation node limit of %d", path, maxAnnotationNodes)
	}
	accountBytes := func(size int) error {
		state.annotationBytes += size
		if state.annotationBytes > maxAnnotationBytes {
			return fmt.Errorf("%s exceeds total annotation size limit of %d bytes", path, maxAnnotationBytes)
		}
		return nil
	}
	if text, numeric := numericLiteralText(value); numeric {
		if _, _, err := parseNumber(value); err != nil {
			return fmt.Errorf("%s has invalid numeric literal: %v", path, err)
		}
		return accountBytes(len(text))
	}
	switch typed := value.(type) {
	case nil, bool:
		return nil
	case string:
		return accountBytes(len(typed))
	case []any:
		for index, child := range typed {
			if err := accountAnnotationValue(fmt.Sprintf("%s[%d]", path, index), child, depth+1, state); err != nil {
				return err
			}
		}
	case []string:
		for index, child := range typed {
			if err := accountAnnotationValue(fmt.Sprintf("%s[%d]", path, index), child, depth+1, state); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			if err := accountBytes(len(key)); err != nil {
				return err
			}
			if err := accountAnnotationValue(path+"["+strconv.Quote(key)+"]", typed[key], depth+1, state); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s is not a JSON value", path)
	}
	return nil
}

func validateValue(path, schemaPath string, value any, schema map[string]any, state *schemaValidationState) error {
	types, _ := schemaStringList(schema["type"], true)
	if len(types) > 0 {
		matches, err := matchesAnyType(value, types)
		if err != nil {
			return fmt.Errorf("%s contains an invalid number: %v", path, err)
		}
		if !matches {
			return fmt.Errorf("%s must be %s", path, strings.Join(types, " or "))
		}
	}

	if _, exists := schema["enum"]; exists {
		matches, err := matchesEnum(value, state.enumIndexes[schemaPath+".enum"], state)
		if err != nil {
			return fmt.Errorf("%s contains an invalid enum value: %v", path, err)
		}
		if !matches {
			return fmt.Errorf("%s must be one of %v", path, schema["enum"])
		}
	}

	object, isObject := value.(map[string]any)
	if isObject {
		required, _ := schemaStringArray(schema["required"], true)
		for _, field := range required {
			if _, exists := object[field]; !exists {
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}

		properties := schemaProperties(schema)
		allowUnknown, additionalSchema, hasAdditionalSchema := additionalProperties(schema)
		_, additionalDeclared := schema["additionalProperties"]
		strictUnknown := !allowUnknown && !hasAdditionalSchema && additionalDeclared
		for _, key := range sortedKeys(object) {
			childPath := path + "." + key
			if propertySchema, known := properties[key]; known {
				childSchemaPath := fmt.Sprintf("%s.properties[%q]", schemaPath, key)
				if err := validateValue(childPath, childSchemaPath, object[key], propertySchema, state); err != nil {
					return err
				}
				continue
			}
			if strictUnknown {
				return fmt.Errorf("%s is not allowed", childPath)
			}
			if hasAdditionalSchema {
				if err := validateValue(childPath, schemaPath+".additionalProperties", object[key], additionalSchema, state); err != nil {
					return err
				}
			}
		}
	}

	if items, exists := schema["items"].(map[string]any); exists {
		if list, isArray := value.([]any); isArray {
			state.arrayItems += len(list)
			if state.arrayItems > maxArrayValidationItems {
				return fmt.Errorf("%s exceeds total array validation item limit of %d", path, maxArrayValidationItems)
			}
			for index, item := range list {
				if err := validateValue(fmt.Sprintf("%s[%d]", path, index), schemaPath+".items", item, items, state); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func schemaProperties(schema map[string]any) map[string]map[string]any {
	raw, _ := schema["properties"].(map[string]any)
	properties := make(map[string]map[string]any, len(raw))
	for key, value := range raw {
		if child, valid := value.(map[string]any); valid {
			properties[key] = child
		}
	}
	return properties
}

func additionalProperties(schema map[string]any) (bool, map[string]any, bool) {
	switch typed := schema["additionalProperties"].(type) {
	case bool:
		return typed, nil, false
	case map[string]any:
		return false, typed, true
	default:
		return false, nil, false
	}
}

func schemaStringList(raw any, allowSingle bool) ([]string, bool) {
	if value, valid := raw.(string); valid {
		return []string{value}, allowSingle && value != ""
	}
	return schemaStringArray(raw, false)
}

func schemaStringArray(raw any, allowEmptyStrings bool) ([]string, bool) {
	var values []string
	switch typed := raw.(type) {
	case []string:
		values = typed
	case []any:
		values = make([]string, 0, len(typed))
		for _, rawValue := range typed {
			value, valid := rawValue.(string)
			if !valid {
				return nil, false
			}
			values = append(values, value)
		}
	default:
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" && !allowEmptyStrings {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
	}
	return values, true
}

func jsonArray(raw any) ([]any, bool) {
	switch typed := raw.(type) {
	case []any:
		return typed, true
	case []string:
		values := make([]any, len(typed))
		for index := range typed {
			values[index] = typed[index]
		}
		return values, true
	default:
		return nil, false
	}
}

func validateEnum(path string, raw any, state *schemaValidationState) error {
	values, valid := jsonArray(raw)
	if !valid || len(values) == 0 {
		return fmt.Errorf("%s must be a non-empty array of unique JSON values", path)
	}
	state.enumMembers += len(values)
	if state.enumMembers > maxEnumMembers {
		return fmt.Errorf("%s exceeds total enum member limit of %d", path, maxEnumMembers)
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		key, err := canonicalEnumValue(fmt.Sprintf("%s[%d]", path, index), value, state)
		if err != nil {
			return err
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s must be a non-empty array of unique JSON values: duplicate at index %d", path, index)
		}
		seen[key] = struct{}{}
	}
	state.enumIndexes[path] = seen
	return nil
}

func canonicalEnumValue(path string, value any, state *schemaValidationState) (string, error) {
	var buffer bytes.Buffer
	if err := writeCanonicalEnumValue(&buffer, path, value, 1, state); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func writeCanonicalEnumValue(buffer *bytes.Buffer, path string, value any, depth int, state *schemaValidationState) error {
	if depth > maxEnumValueDepth {
		return fmt.Errorf("%s exceeds enum value nesting limit of %d", path, maxEnumValueDepth)
	}
	state.enumValueNodes++
	if state.enumValueNodes > maxEnumValueNodes {
		return fmt.Errorf("%s exceeds total enum value node limit of %d", path, maxEnumValueNodes)
	}
	if text, numeric := numericLiteralText(value); numeric {
		parsed, _, err := parseNumber(value)
		if err != nil {
			return fmt.Errorf("%s has invalid numeric literal: %v", path, err)
		}
		if err := accountEnumBytes(path, len(text), state); err != nil {
			return err
		}
		canonical := parsed.RatString()
		fmt.Fprintf(buffer, "n%d:%s", len(canonical), canonical)
		return nil
	}
	switch typed := value.(type) {
	case nil:
		buffer.WriteByte('z')
	case bool:
		if typed {
			buffer.WriteByte('t')
		} else {
			buffer.WriteByte('f')
		}
	case string:
		if err := accountEnumBytes(path, len(typed), state); err != nil {
			return err
		}
		fmt.Fprintf(buffer, "s%d:%s", len(typed), typed)
	case []any:
		fmt.Fprintf(buffer, "a%d:[", len(typed))
		for index, child := range typed {
			if err := writeCanonicalEnumValue(buffer, fmt.Sprintf("%s[%d]", path, index), child, depth+1, state); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case []string:
		fmt.Fprintf(buffer, "a%d:[", len(typed))
		for index, child := range typed {
			if err := writeCanonicalEnumValue(buffer, fmt.Sprintf("%s[%d]", path, index), child, depth+1, state); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := sortedKeys(typed)
		fmt.Fprintf(buffer, "o%d:{", len(keys))
		for _, key := range keys {
			if err := accountEnumBytes(path, len(key), state); err != nil {
				return err
			}
			fmt.Fprintf(buffer, "k%d:%s", len(key), key)
			if err := writeCanonicalEnumValue(buffer, path+"["+strconv.Quote(key)+"]", typed[key], depth+1, state); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		return fmt.Errorf("%s is not a JSON value", path)
	}
	return nil
}

func accountEnumBytes(path string, size int, state *schemaValidationState) error {
	state.enumValueBytes += size
	if state.enumValueBytes > maxEnumValueBytes {
		return fmt.Errorf("%s exceeds total enum value size limit of %d bytes", path, maxEnumValueBytes)
	}
	return nil
}

func matchesAnyType(value any, types []string) (bool, error) {
	for _, expected := range types {
		matches, err := matchesType(value, expected)
		if err != nil || matches {
			return matches, err
		}
	}
	return false, nil
}

func matchesType(value any, expected string) (bool, error) {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok, nil
	case "array":
		_, ok := value.([]any)
		return ok, nil
	case "string":
		_, ok := value.(string)
		return ok, nil
	case "boolean":
		_, ok := value.(bool)
		return ok, nil
	case "number":
		_, numeric, err := parseNumber(value)
		return numeric && err == nil, err
	case "integer":
		number, numeric, err := parseNumber(value)
		if err != nil || !numeric {
			return false, err
		}
		return number.IsInt(), nil
	case "null":
		return value == nil, nil
	default:
		return false, nil
	}
}

func matchesEnum(value any, candidates map[string]struct{}, state *schemaValidationState) (bool, error) {
	valueState := &schemaValidationState{}
	key, err := canonicalEnumValue("enum candidate", value, valueState)
	if err != nil {
		return false, err
	}
	state.validationWork += valueState.enumValueNodes + 1
	if state.validationWork > maxValidationWork {
		return false, fmt.Errorf("enum validation exceeds work limit of %d", maxValidationWork)
	}
	_, matches := candidates[key]
	return matches, nil
}

func parseNumber(value any) (*big.Rat, bool, error) {
	text, numeric := numericLiteralText(value)
	if !numeric {
		return nil, false, nil
	}
	if err := validateNumberLiteral(text); err != nil {
		return nil, true, err
	}
	// The bounded JSON number grammar above is a strict subset of big.Rat's grammar.
	parsed, _ := new(big.Rat).SetString(text)
	return parsed, true, nil
}

func numericLiteralText(value any) (string, bool) {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func validateNumberLiteral(text string) error {
	if len(text) > maxNumberLiteralBytes {
		return fmt.Errorf("numeric literal exceeds byte limit of %d", maxNumberLiteralBytes)
	}
	if text == "" {
		return fmt.Errorf("invalid numeric literal")
	}
	index := 0
	if text[index] == '-' {
		index++
		if index == len(text) {
			return fmt.Errorf("invalid numeric literal")
		}
	}
	digits := 0
	if text[index] == '0' {
		index++
		digits++
		if index < len(text) && text[index] >= '0' && text[index] <= '9' {
			return fmt.Errorf("invalid numeric literal")
		}
	} else if text[index] >= '1' && text[index] <= '9' {
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
			digits++
		}
	} else {
		return fmt.Errorf("invalid numeric literal")
	}
	if index < len(text) && text[index] == '.' {
		index++
		fractionStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
			digits++
		}
		if index == fractionStart {
			return fmt.Errorf("invalid numeric literal")
		}
	}
	if digits > maxNumberDigits {
		return fmt.Errorf("numeric literal exceeds digit limit of %d", maxNumberDigits)
	}
	if index < len(text) && (text[index] == 'e' || text[index] == 'E') {
		index++
		if index < len(text) && (text[index] == '+' || text[index] == '-') {
			index++
		}
		exponentStart := index
		exponent := 0
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			digit := int(text[index] - '0')
			if exponent > (maxNumberExponent-digit)/10 {
				return fmt.Errorf("numeric literal exponent exceeds magnitude limit of %d", maxNumberExponent)
			}
			exponent = exponent*10 + digit
			index++
		}
		if index == exponentStart {
			return fmt.Errorf("invalid numeric literal")
		}
	}
	if index != len(text) {
		return fmt.Errorf("invalid numeric literal")
	}
	return nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
