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

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/spf13/cobra"
)

func normalizeCanonicalParams(cmd *cobra.Command, schema map[string]any, specs []FlagSpec, hints map[string]ir.CLIFlagHint) (map[string]any, error) {
	jsonPayload, err := cmd.Flags().GetString("json")
	if err != nil {
		return nil, apperrors.NewInternal("failed to read --json")
	}

	// Support @file syntax for --json: read file contents as JSON payload.
	if content, isFile, fileErr := ReadFileArg(jsonPayload); fileErr != nil {
		return nil, fileErr
	} else if isFile {
		jsonPayload = content
	}

	// Support stdin pipe: if no --json given, read from pipe.
	if jsonPayload == "" {
		if stdinData, stdinErr := ReadStdinIfPiped(); stdinErr != nil {
			return nil, stdinErr
		} else if stdinData != "" {
			jsonPayload = stdinData
		}
	}

	paramsPayload, err := cmd.Flags().GetString("params")
	if err != nil {
		return nil, apperrors.NewInternal("failed to read --params")
	}

	effectiveHints := buildEffectiveFlagHints(schema, hints)
	params, err := mergeCanonicalPayloads(jsonPayload, paramsPayload, specs)
	if err != nil {
		return nil, err
	}

	flagValues, err := collectCanonicalFlagValues(cmd, specs, effectiveHints)
	if err != nil {
		return nil, err
	}
	for key, value := range flagValues {
		params[key] = value
	}

	if err := applyCanonicalDefaults(params, schema, specs, effectiveHints); err != nil {
		return nil, err
	}
	if err := applyCanonicalTransforms(params, schema, effectiveHints); err != nil {
		return nil, err
	}
	coerceCanonicalParamsToSchema(params, schema)
	if err := enforceRequiredParams(params, specs, effectiveHints); err != nil {
		return nil, err
	}
	nestCanonicalDottedPaths(params)
	return params, nil
}

func mergeCanonicalPayloads(jsonPayload, paramsPayload string, specs []FlagSpec) (map[string]any, error) {
	merged := make(map[string]any)
	for _, payload := range []struct {
		label string
		raw   string
	}{
		{label: "--json", raw: jsonPayload},
		{label: "--params", raw: paramsPayload},
	} {
		values, err := parseCanonicalJSONObject(payload.label, payload.raw)
		if err != nil {
			return nil, err
		}
		values = flattenCanonicalObject(values)
		resolvePayloadNames(values, specs)
		for key, value := range values {
			merged[key] = value
		}
	}
	return merged, nil
}

func collectCanonicalFlagValues(cmd *cobra.Command, specs []FlagSpec, hints map[string]ir.CLIFlagHint) (map[string]any, error) {
	params := make(map[string]any)
	for _, spec := range specs {
		flagName := strings.TrimSpace(spec.FlagName)
		if flagName == "" {
			continue
		}
		aliasName := effectiveFlagAlias(spec, specs)
		primaryChanged := flagChanged(cmd, flagName)
		aliasChanged := aliasName != "" && flagChanged(cmd, aliasName)
		if !primaryChanged && !aliasChanged {
			continue
		}
		if aliasChanged {
			flagName = aliasName
		}

		value, err := readCanonicalFlagValue(cmd, flagName, spec, hints[spec.PropertyName])
		if err != nil {
			return nil, err
		}
		params[spec.PropertyName] = value
	}
	return params, nil
}

func readCanonicalFlagValue(cmd *cobra.Command, flagName string, spec FlagSpec, hint ir.CLIFlagHint) (any, error) {
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
	}

	switch spec.Kind {
	case flagString:
		value, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
		}
		return value, nil
	case flagJSON:
		value, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
		}
		if strings.TrimSpace(hint.Transform) == "json_parse" {
			return value, nil
		}
		var parsed any
		if jsonErr := json.Unmarshal([]byte(value), &parsed); jsonErr != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid JSON for --%s: %v", flagName, jsonErr))
		}
		return parsed, nil
	case flagInteger:
		value, err := cmd.Flags().GetInt(flagName)
		if err == nil {
			return value, nil
		}
		raw := strings.TrimSpace(flag.Value.String())
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
		}
		return parsed, nil
	case flagNumber:
		value, err := cmd.Flags().GetFloat64(flagName)
		if err != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
		}
		return value, nil
	case flagBoolean:
		value, err := cmd.Flags().GetBool(flagName)
		if err != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
		}
		return value, nil
	case flagStringArray:
		value, err := cmd.Flags().GetStringSlice(flagName)
		if err != nil {
			raw := strings.TrimSpace(flag.Value.String())
			raw = strings.TrimPrefix(raw, "[")
			raw = strings.TrimSuffix(raw, "]")
			value = nil
			for _, entry := range strings.Split(raw, ",") {
				entry = strings.Trim(strings.TrimSpace(entry), "\"")
				if entry != "" {
					value = append(value, entry)
				}
			}
		}
		return stringsToAny(value), nil
	case flagIntegerList:
		value, err := cmd.Flags().GetStringSlice(flagName)
		if err != nil {
			value = fallbackStringSlice(flag.Value.String())
		}
		parsed, parseErr := parseStringList(value, strconv.Atoi)
		if parseErr != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
		}
		return intsToAny(parsed), nil
	case flagNumberList:
		value, err := cmd.Flags().GetStringSlice(flagName)
		if err != nil {
			value = fallbackStringSlice(flag.Value.String())
		}
		parsed, parseErr := parseStringList(value, func(raw string) (float64, error) {
			return strconv.ParseFloat(raw, 64)
		})
		if parseErr != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
		}
		return floatsToAny(parsed), nil
	case flagBooleanList:
		value, err := cmd.Flags().GetStringSlice(flagName)
		if err != nil {
			value = fallbackStringSlice(flag.Value.String())
		}
		parsed, parseErr := parseStringList(value, strconv.ParseBool)
		if parseErr != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
		}
		return boolsToAny(parsed), nil
	default:
		return nil, apperrors.NewInternal(fmt.Sprintf("unsupported flag kind %q", spec.Kind))
	}
}

func fallbackStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if raw == "" {
		return nil
	}
	values := make([]string, 0)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.Trim(strings.TrimSpace(entry), "\"")
		if entry != "" {
			values = append(values, entry)
		}
	}
	return values
}

func resolvePayloadNames(params map[string]any, specs []FlagSpec) {
	for _, spec := range specs {
		for _, candidate := range payloadLookupKeys(spec, specs) {
			value, ok := params[candidate]
			if !ok {
				continue
			}
			delete(params, candidate)
			if _, exists := params[spec.PropertyName]; exists {
				break
			}
			params[spec.PropertyName] = value
			break
		}
	}
}

func payloadLookupKeys(spec FlagSpec, specs []FlagSpec) []string {
	keys := make([]string, 0, 2)
	flagName := strings.TrimSpace(spec.FlagName)
	if flagName != "" && flagName != strings.TrimSpace(spec.PropertyName) {
		keys = append(keys, flagName)
	}
	if alias := effectivePayloadAlias(spec, specs); alias != "" && alias != flagName {
		keys = append(keys, alias)
	}
	return keys
}

func effectivePayloadAlias(spec FlagSpec, specs []FlagSpec) string {
	alias := strings.TrimSpace(spec.Alias)
	property := strings.TrimSpace(spec.PropertyName)
	if alias == "" || alias == property {
		return ""
	}
	for _, candidate := range specs {
		if candidate.PropertyName == spec.PropertyName {
			continue
		}
		if strings.TrimSpace(candidate.PropertyName) == alias {
			return ""
		}
		if strings.TrimSpace(candidate.FlagName) == alias {
			return ""
		}
		if strings.TrimSpace(candidate.Alias) == alias {
			return ""
		}
	}
	return alias
}

func parseCanonicalJSONObject(label, raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}

	value, err := decodeJSONValue(raw)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s must be valid JSON: %v", label, err))
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s must decode to a JSON object", label))
	}
	return object, nil
}

func applyCanonicalDefaults(params map[string]any, schema map[string]any, specs []FlagSpec, hints map[string]ir.CLIFlagHint) error {
	for property, hint := range hints {
		if _, exists := params[property]; exists {
			continue
		}
		if envVar := strings.TrimSpace(hint.EnvDefault); envVar != "" {
			if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
				coerced, err := coerceCanonicalDefaultInput(value, property, schema, specs, hint)
				if err != nil {
					return err
				}
				params[property] = coerced
				continue
			}
		}
		if hint.Default != nil {
			params[property] = hint.Default
		}
	}
	return nil
}

func coerceCanonicalDefaultInput(raw string, property string, schema map[string]any, specs []FlagSpec, hint ir.CLIFlagHint) (any, error) {
	spec, ok := flagSpecForProperty(property, specs)
	if !ok {
		propertySchema := schemaForProperty(schema, property)
		kind, hasKind := flagKindForSchema(propertySchema)
		if !hasKind {
			return raw, nil
		}
		spec = FlagSpec{
			PropertyName: property,
			FlagName:     property,
			Kind:         kind,
		}
	}

	switch spec.Kind {
	case flagString:
		return raw, nil
	case flagInteger:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %q is not an integer", property, raw))
		}
		return value, nil
	case flagNumber:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %q is not a number", property, raw))
		}
		return value, nil
	case flagBoolean:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %q is not a boolean", property, raw))
		}
		return value, nil
	case flagStringArray:
		values, err := splitCSVString(raw)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", property, err))
		}
		return values, nil
	case flagIntegerList:
		values, err := parseStringList(fallbackStringSlice(raw), strconv.Atoi)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", property, err))
		}
		return intsToAny(values), nil
	case flagNumberList:
		values, err := parseStringList(fallbackStringSlice(raw), func(value string) (float64, error) {
			return strconv.ParseFloat(value, 64)
		})
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", property, err))
		}
		return floatsToAny(values), nil
	case flagBooleanList:
		values, err := parseStringList(fallbackStringSlice(raw), strconv.ParseBool)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", property, err))
		}
		return boolsToAny(values), nil
	case flagJSON:
		if strings.TrimSpace(hint.Transform) == "json_parse" {
			return raw, nil
		}
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", property, err))
		}
		return parsed, nil
	default:
		return raw, nil
	}
}

func flagSpecForProperty(property string, specs []FlagSpec) (FlagSpec, bool) {
	for _, spec := range specs {
		if spec.PropertyName == property {
			return spec, true
		}
	}
	return FlagSpec{}, false
}

func applyCanonicalTransforms(params map[string]any, schema map[string]any, hints map[string]ir.CLIFlagHint) error {
	for property, hint := range hints {
		transform := strings.TrimSpace(hint.Transform)
		if transform == "" {
			continue
		}
		propertySchema := schemaForProperty(schema, property)
		value, exists := params[property]
		if !exists {
			if transform == "enum_map" && hint.TransformArgs != nil {
				if defaultValue, hasDefault := hint.TransformArgs["_default"]; hasDefault {
					params[property] = defaultValue
				}
			}
			continue
		}
		transformed, err := applyCanonicalTransform(value, transform, hint.TransformArgs, propertySchema)
		if err != nil {
			return err
		}
		params[property] = transformed
	}
	return nil
}

func schemaForProperty(schema map[string]any, property string) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	if direct, ok := properties[property].(map[string]any); ok {
		return direct
	}

	current := schema
	for _, part := range strings.Split(property, ".") {
		properties, ok = current["properties"].(map[string]any)
		if !ok {
			return nil
		}
		next, ok := properties[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func enforceRequiredParams(params map[string]any, specs []FlagSpec, hints map[string]ir.CLIFlagHint) error {
	for property, hint := range hints {
		if !hint.Required {
			continue
		}
		if hint.Hidden {
			continue
		}
		if _, exists := params[property]; exists {
			continue
		}
		if spec, ok := flagSpecForProperty(property, specs); ok && spec.Hidden {
			continue
		}
		flagName := requiredFlagName(property, specs)
		if flagName == "" {
			flagName = strings.ReplaceAll(property, "_", "-")
		}
		return apperrors.NewValidation(fmt.Sprintf("--%s is required", flagName))
	}
	return nil
}

func requiredFlagName(property string, specs []FlagSpec) string {
	for _, spec := range specs {
		if spec.PropertyName == property {
			return strings.TrimSpace(spec.FlagName)
		}
	}
	return ""
}

func nestCanonicalDottedPaths(params map[string]any) {
	var dottedKeys []string
	for key := range params {
		if strings.Contains(key, ".") {
			dottedKeys = append(dottedKeys, key)
		}
	}
	if len(dottedKeys) == 0 {
		return
	}
	sortStrings(dottedKeys)
	for _, key := range dottedKeys {
		value := params[key]
		delete(params, key)

		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			params[key] = value
			continue
		}
		parent, child := parts[0], parts[1]
		nested, ok := params[parent].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			params[parent] = nested
		}
		nested[child] = value
		nestCanonicalDottedPaths(nested)
	}
}

func flattenCanonicalObject(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any)
	flattenCanonicalObjectInto("", values, out)
	return out
}

func flattenCanonicalObjectInto(prefix string, values map[string]any, out map[string]any) {
	for key, value := range values {
		property := joinFlagPropertyPath(prefix, key)
		if nested, ok := value.(map[string]any); ok {
			if len(nested) == 0 {
				out[property] = map[string]any{}
				continue
			}
			flattenCanonicalObjectInto(property, nested, out)
			continue
		}
		out[property] = value
	}
}

func applyCanonicalTransform(value any, transform string, args map[string]any, schema map[string]any) (any, error) {
	switch strings.TrimSpace(transform) {
	case "":
		return value, nil
	case "iso8601_to_millis":
		return transformISO8601ToMillis(value, schema)
	case "csv_to_array":
		return transformCSVToArray(value, schema)
	case "json_parse":
		return transformJSONParse(value, schema)
	case "enum_map":
		return transformEnumMap(value, args)
	default:
		return value, nil
	}
}

func transformISO8601ToMillis(value any, schema map[string]any) (any, error) {
	if schemaPrefersString(schema) {
		return value, nil
	}

	s, ok := canonicalString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	if millis, err := strconv.ParseInt(s, 10, 64); err == nil && millis > 1_000_000_000_000 {
		return millis, nil
	}

	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05"},
		{layout: "2006-01-02 15:04:05"},
		{layout: "2006-01-02", location: time.UTC},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, s, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, s)
		}
		if err == nil {
			return parsed.UnixMilli(), nil
		}
	}
	return nil, apperrors.NewValidation(fmt.Sprintf("iso8601_to_millis: cannot parse %q as ISO-8601", s))
}

func transformCSVToArray(value any, schema map[string]any) (any, error) {
	values, handled, err := normalizeCSVInput(value)
	if err != nil {
		return nil, err
	}
	if !handled {
		return value, nil
	}
	return coerceArrayToSchema(values, schema), nil
}

func normalizeCSVInput(value any) ([]any, bool, error) {
	switch typed := value.(type) {
	case string:
		values, err := splitCSVString(typed)
		return values, true, err
	case []string:
		raw := make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
		return normalizeCSVSequence(raw)
	case []any:
		return normalizeCSVSequence(typed)
	default:
		return nil, false, nil
	}
}

func normalizeCSVSequence(values []any) ([]any, bool, error) {
	if joined, ok := joinCanonicalSequence(values); ok {
		trimmed := strings.TrimSpace(joined)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			items, err := splitCSVString(joined)
			if err != nil {
				return nil, true, err
			}
			return items, true, nil
		}
	}

	out := make([]any, 0, len(values))
	for _, value := range values {
		text, ok := canonicalString(value)
		if !ok {
			out = append(out, value)
			continue
		}
		items, err := splitCSVString(text)
		if err != nil {
			return nil, true, err
		}
		out = append(out, items...)
	}
	return out, true, nil
}

func joinCanonicalSequence(values []any) (string, bool) {
	if len(values) < 2 {
		return "", false
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := canonicalString(value)
		if !ok {
			return "", false
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ","), true
}

func splitCSVString(raw string) ([]any, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return []any{}, nil
	}
	if strings.HasPrefix(s, "[") {
		var values []any
		if err := json.Unmarshal([]byte(s), &values); err == nil {
			return values, nil
		}
	}
	parts := strings.Split(s, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values, nil
}

func coerceArrayToSchema(values []any, schema map[string]any) []any {
	if len(values) == 0 {
		return values
	}
	if len(schema) == 0 {
		return normalizeUntypedJSONArray(values)
	}
	itemSchema, ok := schema["items"].(map[string]any)
	if !ok {
		return normalizeUntypedJSONArray(values)
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = coerceValueToSchema(value, itemSchema)
	}
	return out
}

func coerceCanonicalParamsToSchema(params map[string]any, schema map[string]any) {
	for property, value := range params {
		propertySchema := schemaForProperty(schema, property)
		if len(propertySchema) == 0 {
			continue
		}
		params[property] = coerceValueToSchema(value, propertySchema)
	}
}

func coerceValueToSchema(value any, schema map[string]any) any {
	switch typed := value.(type) {
	case []any:
		return coerceArrayToSchema(typed, schema)
	case map[string]any:
		return coerceObjectToSchema(typed, schema)
	default:
		return coerceScalarToSchema(value, schema)
	}
}

func coerceObjectToSchema(values map[string]any, schema map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}
	if len(schema) == 0 {
		return normalizeUntypedJSONObject(values)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return normalizeUntypedJSONObject(values)
	}
	for key, value := range values {
		childSchema, ok := properties[key].(map[string]any)
		if !ok {
			values[key] = normalizeUntypedJSONValue(value)
			continue
		}
		values[key] = coerceValueToSchema(value, childSchema)
	}
	return values
}

func coerceScalarToSchema(value any, schema map[string]any) any {
	types := schemaTypes(schema)
	if len(types) == 0 || matchesAnyType(value, types) {
		return value
	}

	if schemaPrefersString(schema) {
		switch typed := value.(type) {
		case bool:
			return strconv.FormatBool(typed)
		case json.Number:
			return typed.String()
		case float32:
			return strconv.FormatFloat(float64(typed), 'f', -1, 32)
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.FormatInt(int64(typed), 10)
		case int8:
			return strconv.FormatInt(int64(typed), 10)
		case int16:
			return strconv.FormatInt(int64(typed), 10)
		case int32:
			return strconv.FormatInt(int64(typed), 10)
		case int64:
			return strconv.FormatInt(typed, 10)
		case uint:
			return strconv.FormatUint(uint64(typed), 10)
		case uint8:
			return strconv.FormatUint(uint64(typed), 10)
		case uint16:
			return strconv.FormatUint(uint64(typed), 10)
		case uint32:
			return strconv.FormatUint(uint64(typed), 10)
		case uint64:
			return strconv.FormatUint(typed, 10)
		default:
			if s, ok := canonicalString(value); ok {
				return s
			}
		}
	}

	s, ok := canonicalString(value)
	if !ok {
		return value
	}
	s = strings.TrimSpace(s)

	switch {
	case hasType(types, "integer"):
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			return parsed
		}
	case hasType(types, "number"):
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			return parsed
		}
	case hasType(types, "boolean"):
		if parsed, err := strconv.ParseBool(s); err == nil {
			return parsed
		}
	}
	return value
}

func transformJSONParse(value any, schema map[string]any) (any, error) {
	if schemaPrefersString(schema) {
		return value, nil
	}

	s, ok := canonicalString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	parsed, err := decodeJSONValue(s)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("json_parse: invalid JSON: %v", err))
	}
	return parsed, nil
}

func transformEnumMap(value any, args map[string]any) (any, error) {
	s, ok := canonicalString(value)
	if !ok {
		s = fmt.Sprint(value)
	}
	s = strings.TrimSpace(s)
	if mapped, exists := args[s]; exists {
		return mapped, nil
	}
	if defaultValue, exists := args["_default"]; exists {
		return defaultValue, nil
	}
	return value, nil
}

func canonicalString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case fmt.Stringer:
		return typed.String(), true
	default:
		return "", false
	}
}

func decodeJSONValue(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing content")
		}
		return nil, err
	}
	return value, nil
}

func normalizeUntypedJSONObject(values map[string]any) map[string]any {
	for key, value := range values {
		values[key] = normalizeUntypedJSONValue(value)
	}
	return values
}

func normalizeUntypedJSONArray(values []any) []any {
	for i, value := range values {
		values[i] = normalizeUntypedJSONValue(value)
	}
	return values
}

func normalizeUntypedJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeUntypedJSONObject(typed)
	case []any:
		return normalizeUntypedJSONArray(typed)
	case json.Number:
		if parsed, err := typed.Float64(); err == nil {
			return parsed
		}
		return typed.String()
	default:
		return value
	}
}

func schemaPrefersString(schema map[string]any) bool {
	types := schemaTypes(schema)
	return hasType(types, "string") && !hasType(types, "number") && !hasType(types, "integer")
}
