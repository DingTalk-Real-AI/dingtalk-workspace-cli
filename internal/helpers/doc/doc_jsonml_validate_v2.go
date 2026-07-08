//go:build !goexperiment.jsonv2

package doc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateJsonMLBodyV2 validates a JSONML body using schema-v2.
// Only type mismatches are errors; everything else is a warning.
func ValidateJsonMLBodyV2(body []any) *JsonMLValidationResult {
	r := &JsonMLValidationResult{}
	if len(body) == 0 {
		return r
	}

	// Root-wrapped, single block node, or future/unknown single node. If the
	// first element is a string, the JSONML shape is a node; unknown tags should
	// be warnings, not a cascade of "node must be array" errors.
	if _, ok := body[0].(string); ok {
		validateNodeV2(body, "$", nil, r)
		return r
	}

	// Array of blocks
	for i, node := range body {
		nodePath := fmt.Sprintf("$[%d]", i)
		if arr, ok := node.([]any); ok && len(arr) > 0 {
			if t, ok := arr[0].(string); ok {
				nodePath = fmt.Sprintf("$[%d:%s]", i, t)
			}
		}
		validateNodeV2(node, nodePath, nil, r)
	}
	return r
}

// ValidateJsonMLNodeV2 validates a single JSONML node using schema-v2.
func ValidateJsonMLNodeV2(node any) *JsonMLValidationResult {
	r := &JsonMLValidationResult{}
	validateNodeV2(node, "$", nil, r)
	return r
}

// ValidateJsonMLSource is a fallback for non-jsonv2 builds.
// It unmarshals the source and delegates to ValidateJsonMLBodyV2.
// Positions in diagnostics will not be source-accurate in this build.
func ValidateJsonMLSource(src []byte) *JsonMLValidationResult {
	if len(src) == 0 {
		return &JsonMLValidationResult{}
	}
	var body any
	if err := json.Unmarshal(src, &body); err != nil {
		r := &JsonMLValidationResult{}
		r.Errors = append(r.Errors, "JSON parse error: "+err.Error())
		return r
	}
	switch v := body.(type) {
	case []any:
		return ValidateJsonMLBodyV2(v)
	case map[string]any:
		if jml, ok := v["jsonml"]; ok {
			if arr, ok := jml.([]any); ok {
				return ValidateJsonMLBodyV2(arr)
			}
		}
	}
	r := &JsonMLValidationResult{}
	r.Errors = append(r.Errors, "input is not a valid JSONML array or {\"jsonml\": [...]} wrapper")
	return r
}

func validateNodeV2(node any, path string, parentSchema *TagSchema, r *JsonMLValidationResult) {
	arr, ok := node.([]any)
	if !ok {
		r.addError(path, fmt.Sprintf("node must be array, got %T", node), "")
		return
	}
	if len(arr) < 1 {
		r.addError(path, "node array must not be empty", "")
		return
	}

	tag, ok := arr[0].(string)
	if !ok {
		r.addError(path, fmt.Sprintf("tag must be string, got %T", arr[0]), "")
		return
	}

	// Check if child is allowed by parent
	if parentSchema != nil && !parentSchema.IsAllowedChild(tag) {
		r.addWarn(path,
			fmt.Sprintf("tag %q not in parent's allowed_children", tag), "")
	}

	tagSchema := schemaV2.TagSchemaFor(tag)
	if tagSchema == nil {
		r.addWarn(path, fmt.Sprintf("unknown tag %q", tag), "")
		return
	}

	// Extract attrs
	childStart := 1
	var attrs map[string]any
	if len(arr) > 1 {
		if m, ok := arr[1].(map[string]any); ok {
			attrs = m
			childStart = 2
		}
	}

	// Validate attrs
	for key, val := range attrs {
		spec, known := tagSchema.Attrs[key]
		if !known {
			r.addWarn(path+".attrs."+key,
				fmt.Sprintf("unknown attr %q", key), "")
			continue
		}
		checkTypeV2(val, &spec, path+".attrs."+key, r)
	}

	// Validate children
	for i := childStart; i < len(arr); i++ {
		child := arr[i]
		childPath := fmt.Sprintf("%s[%d]", path, i)
		switch c := child.(type) {
		case string:
			if !tagSchema.IsAllowedChild("#text") {
				r.addWarn(childPath,
					fmt.Sprintf("bare text not allowed in %q", tag), "")
			}
			_ = c
		case []any:
			if len(c) > 0 {
				if childTag, ok := c[0].(string); ok {
					childPath = fmt.Sprintf("%s[%d:%s]", path, i, childTag)
				}
			}
			validateNodeV2(child, childPath, tagSchema, r)
		default:
			// null, number etc — skip
		}
	}
}

// checkTypeV2 validates a value against a TypeSpec.
// Type and enum mismatches are blocking schema errors.
func checkTypeV2(value any, spec *TypeSpec, path string, r *JsonMLValidationResult) {
	switch spec.Type {
	case "any":
		return

	case "string":
		if _, ok := value.(string); !ok {
			r.addError(path,
				fmt.Sprintf("expected string, got %T", value), "")
		}

	case "number":
		num, ok := toFloat64(value)
		if !ok {
			r.addError(path,
				fmt.Sprintf("expected number, got %T", value), "")
			return
		}
		if spec.Min != nil && num < *spec.Min {
			r.addError(path,
				fmt.Sprintf("value %v < min %v", num, *spec.Min), "")
		}
		if spec.Max != nil && num > *spec.Max {
			r.addError(path,
				fmt.Sprintf("value %v > max %v", num, *spec.Max), "")
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			r.addError(path,
				fmt.Sprintf("expected boolean, got %T", value), "")
		}

	case "array":
		if _, ok := value.([]any); !ok {
			r.addError(path,
				fmt.Sprintf("expected array, got %T", value), "")
		}

	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			r.addError(path,
				fmt.Sprintf("expected object, got %T", value), "")
			return
		}
		// Deep validation if fields defined
		if spec.Fields != nil {
			for key, val := range obj {
				fieldSpec, known := spec.Fields[key]
				if !known {
					r.addWarn(path+"."+key,
						fmt.Sprintf("unknown field %q", key), "")
					continue
				}
				checkTypeV2(val, &fieldSpec, path+"."+key, r)
			}
		}

	case "enum":
		str, ok := value.(string)
		if !ok {
			r.addError(path,
				fmt.Sprintf("enum expects string, got %T", value), "")
			return
		}
		found := false
		for _, v := range spec.Values {
			if v == str {
				found = true
				break
			}
		}
		if !found {
			r.addError(path,
				fmt.Sprintf("value %q not in enum [%s]", str, strings.Join(spec.Values, ", ")), "")
		}

	case "union":
		if matchesUnion(value, spec.Types) {
			return
		}
		if len(spec.WarnTypes) > 0 && matchesUnion(value, spec.WarnTypes) {
			r.addWarn(path,
				fmt.Sprintf("value (%T) matches warn_types [%s], expected [%s]", value, strings.Join(spec.WarnTypes, ", "), strings.Join(spec.Types, ", ")), "")
			return
		}
		r.addError(path,
			fmt.Sprintf("value (%T) doesn't match any of [%s]", value, strings.Join(spec.Types, ", ")), "")
	}
}
