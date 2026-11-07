// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

func at(value any, parts []string) any {
	for _, key := range parts {
		switch n := value.(type) {
		case map[string]any:
			value = n[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(n) {
				return nil
			}
			value = n[i]
		default:
			return nil
		}
	}
	return value
}

func (p *Protocol) schemaAt(location string) map[string]any {
	u, err := url.Parse(location)
	if err != nil {
		return nil
	}
	for _, doc := range p.docs {
		base, _ := url.Parse(str(doc["$id"]))
		if base == nil {
			continue
		}
		base.Fragment = ""
		target := *u
		target.Fragment = ""
		if base.String() != target.String() {
			continue
		}
		if u.Fragment == "" {
			return doc
		}
		parts := strings.Split(strings.TrimPrefix(u.Fragment, "/"), "/")
		for i, s := range parts {
			parts[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
		}
		return object(at(doc, parts))
	}
	return nil
}

func resolve(base, ref string) string {
	u, _ := url.Parse(base)
	r, _ := url.Parse(ref)
	if u == nil || r == nil {
		return ""
	}
	return u.ResolveReference(r).String()
}

func (p *Protocol) declared(schema map[string]any, value any, base string, seen map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key := range object(schema["properties"]) {
		out[key] = true
	}
	merge := func(other map[string]bool) {
		for key := range other {
			out[key] = true
		}
	}
	if ref := str(schema["$ref"]); ref != "" {
		loc := resolve(base, ref)
		if !seen[loc] {
			next := map[string]bool{}
			for k, v := range seen {
				next[k] = v
			}
			next[loc] = true
			merge(p.declared(p.schemaAt(loc), value, loc, next))
		}
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		branches := array(schema[keyword])
		matching := []any{}
		for _, b := range branches {
			if !p.wrongTag(object(b), object(value), base, map[string]bool{}) {
				matching = append(matching, b)
			}
		}
		if keyword == "allOf" || len(matching) == 0 {
			matching = branches
		}
		for _, b := range matching {
			merge(p.declared(object(b), value, base, seen))
		}
	}
	// Tag guards contain the same declared fields as their then branch.
	// Other conditions contribute potential fields for diagnostics only, never validity.
	for _, key := range []string{"then", "else"} {
		if s := object(schema[key]); s != nil {
			merge(p.declared(s, value, base, seen))
		}
	}
	return out
}

func (p *Protocol) wrongTag(schema, value map[string]any, base string, seen map[string]bool) bool {
	for _, tag := range []string{"component", "call"} {
		actual, ok := value[tag].(string)
		if !ok {
			continue
		}
		if wanted, ok := object(object(schema["properties"])[tag])["const"].(string); ok && actual != wanted {
			return true
		}
	}
	if ref := str(schema["$ref"]); ref != "" {
		loc := resolve(base, ref)
		if !seen[loc] {
			seen[loc] = true
			return p.wrongTag(p.schemaAt(loc), value, loc, seen)
		}
	}
	if condition := object(schema["if"]); condition != nil {
		return p.wrongTag(condition, value, base, seen)
	}
	for _, sub := range array(schema["allOf"]) {
		if p.wrongTag(object(sub), value, base, seen) {
			return true
		}
	}
	return false
}

func keyword(e *jsonschema.ValidationError) string {
	a := e.ErrorKind.KeywordPath()
	if len(a) > 0 {
		return a[0]
	}
	return ""
}

func (p *Protocol) leaves(e *jsonschema.ValidationError, value any) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	groups := [][]*jsonschema.ValidationError{}
	for _, child := range e.Causes {
		groups = append(groups, p.leaves(child, value))
	}
	kw := keyword(e)
	if kw != "oneOf" && kw != "anyOf" {
		out := []*jsonschema.ValidationError{}
		for _, g := range groups {
			out = append(out, g...)
		}
		return out
	}
	score := func(items []*jsonschema.ValidationError) [6]int {
		s := [6]int{}
		base := pointer(e.InstanceLocation)
		instance := object(at(value, e.InstanceLocation))
		for _, leaf := range items {
			path := pointer(leaf.InstanceLocation)
			kw := keyword(leaf)
			same := path == base
			if kw == "additionalProperties" && same {
				props := object(p.schemaAt(leaf.SchemaURL)["properties"])
				for _, tag := range []string{"component", "call"} {
					if _, has := instance[tag]; has && props[tag] == nil {
						s[0]++
					}
				}
			}
			if kw == "type" && same {
				s[1]++
			}
			if (kw == "const" || kw == "enum") && (path == base+"/component" || path == base+"/call") {
				s[2]++
			}
			if kw == "required" && same {
				s[3]++
			}
			s[4] = min(s[4], -len(leaf.InstanceLocation))
		}
		s[5] = len(items)
		return s
	}
	best := 0
	for i := 1; i < len(groups); i++ {
		a, b := score(groups[i]), score(groups[best])
		for j := range a {
			if a[j] < b[j] {
				best = i
				break
			}
			if a[j] > b[j] {
				break
			}
		}
	}
	return groups[best]
}

func (p *Protocol) diagnostics(root *jsonschema.ValidationError, value any, prefix string) []Diagnostic {
	codes := map[string]string{"type": "type_mismatch", "const": "bad_enum", "enum": "bad_enum", "required": "missing_required", "minimum": "below_minimum", "maximum": "above_maximum", "minItems": "too_few_items", "maxItems": "too_many_items", "minLength": "too_short", "maxLength": "too_long", "pattern": "pattern_mismatch"}
	labels := map[string]string{"type": "Field type mismatch", "const": "Value does not match the required constant", "enum": "Value is not in the allowed enumeration", "minimum": "Number is below the minimum", "maximum": "Number exceeds the maximum", "minItems": "Array has too few items", "maxItems": "Array has too many items", "minLength": "Text is too short", "maxLength": "Text is too long", "pattern": "Text does not match the required pattern"}
	out := []Diagnostic{}
	seen := map[string]bool{}
	add := func(code, path, message, hint string) {
		key := code + " " + path
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Diagnostic{code, "error", prefix + path, message, hint})
	}
	for _, e := range p.leaves(root, value) {
		path := pointer(e.InstanceLocation)
		kw := keyword(e)
		switch k := e.ErrorKind.(type) {
		case *kind.AdditionalProperties:
			for _, name := range k.Properties {
				add("schema.unknown_property", path+pointer([]string{name}), "Unknown property: "+name, "Remove the property or correct its name")
			}
			continue
		case *kind.FalseSchema:
			if strings.HasSuffix(e.SchemaURL, "/unevaluatedProperties") && len(e.InstanceLocation) > 0 {
				parent := e.InstanceLocation[:len(e.InstanceLocation)-1]
				name := e.InstanceLocation[len(parent)]
				base := strings.TrimSuffix(e.SchemaURL, "/unevaluatedProperties")
				if !p.declared(p.schemaAt(base), at(value, parent), base, map[string]bool{})[name] {
					add("schema.unknown_property", path, "Unknown property: "+name, "Remove the property or correct its name")
				}
				continue
			}
		case *kind.Required:
			sort.Strings(k.Missing)
			add("schema.missing_required", path, "Missing required field: "+strings.Join(k.Missing, ", "), "Add the fields required by the contract")
			continue
		}
		code := codes[kw]
		if code == "" {
			code = "invalid"
		}
		message := labels[kw]
		if message == "" {
			message = "Input does not satisfy the protocol schema"
		}
		if k, ok := e.ErrorKind.(*kind.Type); ok {
			message += fmt.Sprintf("; expected %s", strings.Join(k.Want, " / "))
		}
		add("schema."+code, path, message, "Use dws aicard explain to inspect the field contract")
	}
	// A failed inner branch may leave a declared parent unevaluated.
	// Prefer the specific inner diagnostic; never discard the validation failure.
	filtered := []Diagnostic{}
	for _, d := range out {
		suppress := false
		if d.Code == "schema.unknown_property" {
			for _, other := range out {
				if other.Code != d.Code && (other.Pointer == d.Pointer || strings.HasPrefix(other.Pointer, d.Pointer+"/")) {
					suppress = true
					break
				}
			}
		}
		if !suppress {
			filtered = append(filtered, d)
		}
	}
	if len(filtered) == 0 {
		filtered = append(filtered, Diagnostic{"schema.invalid", "error", prefix, "Input does not satisfy the protocol schema", "Use dws aicard explain to inspect the field contract"})
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Pointer != filtered[j].Pointer {
			return filtered[i].Pointer < filtered[j].Pointer
		}
		return filtered[i].Code < filtered[j].Code
	})
	return filtered
}
