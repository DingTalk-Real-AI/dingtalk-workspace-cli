//go:build goexperiment.jsonv2

package doc

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateWithSchema 使用 schema 对 AST 进行语义校验 (Layer 2)。
// 仅遍历 IsError == false 的节点。
func ValidateWithSchema(doc *JsonMLDocument, schema *SchemaV2) DiagnosticList {
	var diags DiagnosticList
	for _, node := range doc.Nodes {
		validateSchemaNode(node, nil, schema, &diags)
	}
	return diags
}

func validateSchemaNode(node *JsonMLNode, parentSchema *TagSchema, schema *SchemaV2, diags *DiagnosticList) {
	if node == nil || node.IsError || node.IsText {
		return
	}

	tag := node.Tag

	// Check if child is allowed by parent
	if parentSchema != nil && !parentSchema.IsAllowedChild(tag) {
		*diags = append(*diags, Diagnostic{
			Range:    node.TagRange,
			Severity: SeverityWarning, // 保持现有行为：warning
			Code:     CodeJSONMLSchemaChildNotAllowed,
			Source:   SourceJSONMLSchema,
			Message:  fmt.Sprintf("tag %q not in parent's allowed_children", tag),
			Context: &DiagnosticContext{
				AllowedChildren: parentSchema.AllowedChildren,
			},
		})
	}

	tagSchema := schema.TagSchemaFor(tag)
	if tagSchema == nil {
		candidates := didYouMean(tag, schema.AllTagNames(), 3)
		msg := fmt.Sprintf("unknown tag %q", tag)
		if len(candidates) > 0 {
			msg += fmt.Sprintf(". Did you mean: %s?", strings.Join(candidates, ", "))
		}
		*diags = append(*diags, Diagnostic{
			Range:    node.TagRange,
			Severity: SeverityWarning, // 保持现有行为：warning
			Code:     CodeJSONMLSchemaUnknownTag,
			Source:   SourceJSONMLSchema,
			Message:  msg,
			Context: &DiagnosticContext{
				AllValidTags: schema.AllTagNames(),
			},
		})
		return // 无 schema 可参照，不继续校验
	}

	// Validate attrs
	if node.Attrs != nil {
		for key, val := range node.Attrs {
			spec, known := tagSchema.Attrs[key]
			if !known {
				// P1: Check if key is a known alias
				if canonicalName, isAlias := tagSchema.Aliases[key]; isAlias {
					*diags = append(*diags, Diagnostic{
						Range:      node.Range,
						Severity:   SeverityWarning,
						Code:       CodeJSONMLSchemaAttrAlias,
						Source:     SourceJSONMLSchema,
						Message:    fmt.Sprintf("unknown attribute %q on tag %q; %q is a known alias for %q, use %q instead", key, tag, key, canonicalName, canonicalName),
						Suggestion: canonicalName,
						Context: &DiagnosticContext{
							TagName:       tag,
							AllValidAttrs: tagSchema.AttrKeys(),
						},
					})
					// Still validate the VALUE against the canonical spec
					if canonicalSpec, ok := tagSchema.Attrs[canonicalName]; ok {
						checkTypeSchema(val, &canonicalSpec, canonicalName, tag, node, diags)
					}
					continue
				}
				// Otherwise: regular unknown attr with did-you-mean
				candidates := didYouMean(key, tagSchema.AllAttrCandidates(), 3)
				msg := fmt.Sprintf("unknown attr %q on tag %q", key, tag)
				if len(candidates) > 0 {
					msg += fmt.Sprintf(". Did you mean: %s?", strings.Join(candidates, ", "))
				}
				*diags = append(*diags, Diagnostic{
					Range:    node.Range, // 使用节点范围（精确 attr range 待后续）
					Severity: SeverityWarning,
					Code:     CodeJSONMLSchemaUnknownAttr,
					Source:   SourceJSONMLSchema,
					Message:  msg,
					Context: &DiagnosticContext{
						TagName:       tag,
						AllValidAttrs: tagSchema.AttrKeys(),
					},
				})
				continue
			}
			// P1: Check deprecated
			if spec.Deprecated {
				*diags = append(*diags, Diagnostic{
					Range:    node.Range,
					Severity: SeverityWarning,
					Code:     CodeJSONMLSchemaDeprecatedAttr,
					Source:   SourceJSONMLSchema,
					Message:  fmt.Sprintf("attribute %q on tag %q is deprecated", key, tag),
				})
			}
			checkTypeSchema(val, &spec, key, tag, node, diags)
		}
	}

	// P1: Check required attrs
	for attrKey, spec := range tagSchema.Attrs {
		if spec.Required {
			if node.Attrs == nil || node.Attrs[attrKey] == nil {
				*diags = append(*diags, Diagnostic{
					Range:      node.Range,
					Severity:   SeverityError,
					Code:       CodeJSONMLSchemaRequiredMissing,
					Source:     SourceJSONMLSchema,
					Message:    fmt.Sprintf("tag %q requires attribute %q", tag, attrKey),
					Suggestion: fmt.Sprintf("%q: %s", attrKey, typePlaceholder(&spec)),
					Context: &DiagnosticContext{
						TagName:     tag,
						Description: tagSchema.Description,
					},
				})
			}
		}
	}

	// Validate children
	for _, child := range node.Children {
		if child.IsText {
			if !tagSchema.IsAllowedChild("#text") {
				*diags = append(*diags, Diagnostic{
					Range:    child.Range,
					Severity: SeverityWarning,
					Code:     CodeJSONMLSchemaChildNotAllowed,
					Source:   SourceJSONMLSchema,
					Message:  fmt.Sprintf("bare text not allowed in %q", tag),
					Context: &DiagnosticContext{
						AllowedChildren: tagSchema.AllowedChildren,
					},
				})
			}
			continue
		}
		validateSchemaNode(child, tagSchema, schema, diags)
	}

	// P1: CustomValidator hooks
	if validators, ok := customValidators[tag]; ok {
		for _, v := range validators {
			v(node, node.Attrs, tagSchema, func(d Diagnostic) { *diags = append(*diags, d) })
		}
	}
}

// checkTypeSchema validates a value against a TypeSpec, emitting Diagnostics.
func checkTypeSchema(value any, spec *TypeSpec, attrKey, tagName string, node *JsonMLNode, diags *DiagnosticList) {
	attrPath := fmt.Sprintf("%s.attrs.%s", tagName, attrKey)

	switch spec.Type {
	case "any":
		return

	case "string":
		if _, ok := value.(string); !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: expected string, got %T", attrPath, value),
			})
		}

	case "number":
		num, ok := toFloat64(value)
		if !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: expected number, got %T", attrPath, value),
			})
			return
		}
		if spec.Min != nil && num < *spec.Min {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaNumberRange,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: value %v < min %v", attrPath, num, *spec.Min),
			})
		}
		if spec.Max != nil && num > *spec.Max {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaNumberRange,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: value %v > max %v", attrPath, num, *spec.Max),
			})
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: expected boolean, got %T", attrPath, value),
			})
		}

	case "array":
		if _, ok := value.([]any); !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: expected array, got %T", attrPath, value),
			})
		}

	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: expected object, got %T", attrPath, value),
			})
			return
		}
		if spec.Fields != nil {
			for key, val := range obj {
				fieldSpec, known := spec.Fields[key]
				if !known {
					*diags = append(*diags, Diagnostic{
						Range:    node.Range,
						Severity: SeverityWarning,
						Code:     CodeJSONMLSchemaUnknownAttr,
						Source:   SourceJSONMLSchema,
						Message:  fmt.Sprintf("%s.%s: unknown field %q", attrPath, key, key),
					})
					continue
				}
				checkTypeSchema(val, &fieldSpec, attrKey+"."+key, tagName, node, diags)
			}
		}

	case "enum":
		str, ok := value.(string)
		if !ok {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message:  fmt.Sprintf("%s: enum expects string, got %T", attrPath, value),
			})
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
			*diags = append(*diags, Diagnostic{
				Range:      node.Range,
				Severity:   SeverityError, // P1-j: enum 约束为硬性限制
				Code:       CodeJSONMLSchemaEnumInvalid,
				Source:     SourceJSONMLSchema,
				Message:    fmt.Sprintf("%s: value %q not in enum [%s]", attrPath, str, strings.Join(spec.Values, ", ")),
				Suggestion: fmt.Sprintf("valid values: [%s]", strings.Join(spec.Values, ", ")),
			})
		}

	case "union":
		if matchesUnion(value, spec.Types) {
			return
		}
		if len(spec.WarnTypes) > 0 && matchesUnion(value, spec.WarnTypes) {
			*diags = append(*diags, Diagnostic{
				Range:    node.Range,
				Severity: SeverityWarning,
				Code:     CodeJSONMLSchemaTypeMismatch,
				Source:   SourceJSONMLSchema,
				Message: fmt.Sprintf("%s: value (%T) matches warn_types [%s], expected [%s]",
					attrPath, value, strings.Join(spec.WarnTypes, ", "), strings.Join(spec.Types, ", ")),
			})
			return
		}
		*diags = append(*diags, Diagnostic{
			Range:    node.Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSchemaTypeMismatch,
			Source:   SourceJSONMLSchema,
			Message: fmt.Sprintf("%s: value (%T) doesn't match any of [%s]",
				attrPath, value, strings.Join(spec.Types, ", ")),
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Schema 辅助方法
// ──────────────────────────────────────────────────────────────────────────────

// AllTagNames 返回 schema 中所有已知 tag 名称（排序）。
func (s *SchemaV2) AllTagNames() []string {
	names := make([]string, 0, len(s.Tags))
	for name := range s.Tags {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AttrKeys 返回 TagSchema 中所有已知 attr key（排序）。
func (ts *TagSchema) AttrKeys() []string {
	keys := make([]string, 0, len(ts.Attrs))
	for k := range ts.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AllAttrCandidates 返回 canonical attr names + alias names（排序）。
// 用于 did-you-mean 候选集，当 attr 既不在 Attrs 也不在 Aliases 时，将两者合并提供近似建议。
func (ts *TagSchema) AllAttrCandidates() []string {
	keys := ts.AttrKeys()
	for alias := range ts.Aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	return keys
}

// ──────────────────────────────────────────────────────────────────────────────
// did-you-mean (Levenshtein 编辑距离)
// ──────────────────────────────────────────────────────────────────────────────

// didYouMean 返回编辑距离最近的 top-N 候选。
// 阈值: 编辑距离 <= max(2, len(input)/3) 才纳入。
func didYouMean(input string, candidates []string, topN int) []string {
	if len(candidates) == 0 || input == "" {
		return nil
	}
	threshold := len(input) / 3
	if threshold < 2 {
		threshold = 2
	}

	type scored struct {
		name string
		dist int
	}
	var matches []scored
	for _, c := range candidates {
		d := levenshtein(input, c)
		if d <= threshold {
			matches = append(matches, scored{c, d})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].dist < matches[j].dist
	})
	if len(matches) > topN {
		matches = matches[:topN]
	}
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m.name
	}
	return result
}

// levenshtein 计算两个字符串之间的编辑距离。
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	// 使用单行 DP
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// typePlaceholder generates a placeholder value string for a TypeSpec.
// Used to auto-derive Suggestion content.
func typePlaceholder(spec *TypeSpec) string {
	switch spec.Type {
	case "string":
		return `"<string>"`
	case "number":
		if spec.Min != nil {
			return fmt.Sprintf("%v", *spec.Min)
		}
		return "0"
	case "boolean":
		return "true"
	case "array":
		return "[]"
	case "object":
		if len(spec.Fields) > 0 {
			parts := make([]string, 0, len(spec.Fields))
			for k, f := range spec.Fields {
				parts = append(parts, fmt.Sprintf("%q: %s", k, typePlaceholder(&f)))
			}
			sort.Strings(parts)
			return "{" + strings.Join(parts, ", ") + "}"
		}
		return "{}"
	case "enum":
		if len(spec.Values) > 0 {
			return fmt.Sprintf("%q", spec.Values[0])
		}
		return `"<enum>"`
	case "union":
		if len(spec.Types) > 0 {
			return unionFirstPlaceholder(spec.Types[0])
		}
		return "null"
	case "any":
		return "null"
	default:
		return "null"
	}
}

// unionFirstPlaceholder returns placeholder for the first type in a union.
func unionFirstPlaceholder(typeName string) string {
	switch typeName {
	case "string":
		return `"<string>"`
	case "number":
		return "0"
	case "boolean":
		return "true"
	case "array":
		return "[]"
	case "object":
		return "{}"
	case "null":
		return "null"
	default:
		return "null"
	}
}
