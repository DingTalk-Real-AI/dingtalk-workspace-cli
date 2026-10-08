// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
	"github.com/dlclark/regexp2"
)

func TestCrossPlatformCoverageAicardProtocolUtilities(t *testing.T) {
	if _, err := decode([]byte{0xff}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8: %v", err)
	}
	if _, err := (localLoader{}).Load("https://outside.invalid/schema"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside reference: %v", err)
	}
	re, err := regexp2.Compile("a+", 0)
	if err != nil {
		t.Fatal(err)
	}
	matcher := protocolRegexp{source: "a+", compiled: re}
	if matcher.String() != "a+" || !matcher.MatchString("aa") || matcher.MatchString("bb") {
		t.Fatal("regexp adapter changed its source or match result")
	}
	catastrophic, err := regexp2.Compile("(a+)+$", 0)
	if err != nil {
		t.Fatal(err)
	}
	catastrophic.MatchTimeout = time.Nanosecond
	func() {
		defer func() {
			if recover() == nil {
				t.Error("regexp timeout must remain an engine error, not a false match")
			}
		}()
		(protocolRegexp{compiled: catastrophic}).MatchString(strings.Repeat("a", 10000) + "b")
	}()
	p := testProtocol(t)
	manifest := p.Manifest()
	if !reflect.DeepEqual(manifest, p.assets.Manifest) {
		t.Fatal("manifest copy differs from embedded manifest")
	}
	manifest["agent-to-renderer.json"] = "changed"
	if reflect.DeepEqual(manifest, p.assets.Manifest) {
		t.Fatal("caller must not mutate the embedded manifest")
	}
	if got := mustExplain(t, p, " text "); got["name"] != "Text" {
		t.Fatal(got)
	}
	if got := mustExplain(t, p, "check_l_outlined"); got["kind"] != "unknown" {
		t.Fatal("token items must remain case sensitive", got)
	}
	mostSuggestions := 0
	for _, name := range []string{"T", "B", "C", "R", "S", "I", "F", "P", "M", "D", "L", "A"} {
		got := mustExplain(t, p, name)
		if got["kind"] != "unknown" {
			t.Fatalf("unexpected match for %q: %v", name, got["name"])
		}
		mostSuggestions = max(mostSuggestions, len(got["suggestions"].([]string)))
	}
	if mostSuggestions != 5 {
		t.Fatalf("suggestion cap changed: maximum was %d", mostSuggestions)
	}
}

func TestCrossPlatformCoverageAicardSupplementaryRuleFailures(t *testing.T) {
	base := func() *Protocol {
		return &Protocol{docs: map[string]map[string]any{"file": {"$defs": map[string]any{"existing": map[string]any{}}}}}
	}
	for _, tc := range []struct {
		name  string
		rules map[string]any
		want  string
	}{
		{"version", map[string]any{"version": json.Number("2")}, "version"},
		{"missing definitions", map[string]any{"version": json.Number("1"), "definitions": map[string]any{"missing": map[string]any{"new": true}}}, "target"},
		{"conflicting definition", map[string]any{"version": json.Number("1"), "definitions": map[string]any{"file": map[string]any{"existing": true}}}, "conflicts"},
		{"invalid array position", map[string]any{"version": json.Number("1"), "references": []any{map[string]any{"file": "file", "path": []any{json.Number("0")}}}}, "array path"},
		{"malformed path", map[string]any{"version": json.Number("1"), "references": []any{map[string]any{"file": "file", "path": []any{true}}}}, "malformed"},
		{"mismatched reference", map[string]any{"version": json.Number("1"), "references": []any{map[string]any{"file": "file", "path": []any{"$defs", "existing"}, "publicRef": "missing"}}}, "reference mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := base().applyRules(tc.rules); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	p := base()
	rules := map[string]any{"version": json.Number("1"), "definitions": map[string]any{"file": map[string]any{"added": true}}, "references": []any{map[string]any{"file": "file", "path": []any{"$defs", "existing"}, "publicRef": "public", "validationRef": "validation"}}}
	p.docs["file"]["$defs"].(map[string]any)["existing"] = map[string]any{"$ref": "public"}
	if err := p.applyRules(rules); err != nil {
		t.Fatal(err)
	}
	defs := p.docs["file"]["$defs"].(map[string]any)
	if defs["added"] != true || defs["existing"].(map[string]any)["$ref"] != "validation" {
		t.Fatal(defs)
	}
	p.docs["file"]["items"] = []any{map[string]any{"$ref": "public"}}
	rules["definitions"] = nil
	rules["references"] = []any{map[string]any{"file": "file", "path": []any{"items", json.Number("0")}, "publicRef": "public", "validationRef": "validation"}}
	if err := p.applyRules(rules); err != nil {
		t.Fatal(err)
	}
}

// Load must reject a damaged embedded index, missing resources, invalid JSON,
// and a matching digest whose content no longer compiles as a protocol.
func TestCrossPlatformCoverageAicardLoadIntegrityFailures(t *testing.T) {
	original := assetsJSON
	assetData := original
	source, err := fs.Sub(skills.FS, "multi/dingtalk-aicard")
	if err != nil {
		t.Fatal(err)
	}
	var base assets
	if err := json.Unmarshal(original, &base); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for name := range base.Manifest {
		if name == "a2ui-validation-rules.json" {
			continue
		}
		path := "references/protocol/" + name
		data, err := fs.ReadFile(source, path)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = &fstest.MapFile{Data: data}
	}
	setAssets := func(a assets) {
		b, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		assetData = b
	}
	setDoc := func(a *assets, name string, data []byte) {
		path := "references/protocol/" + name
		files[path] = &fstest.MapFile{Data: data}
		a.Manifest[name] = fmt.Sprintf("%x", sha256.Sum256(data))[:16]
	}
	clone := func() assets {
		var a assets
		if err := json.Unmarshal(original, &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	check := func(want string) {
		t.Helper()
		if _, err := load(files, assetData); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Load error = %v, want %q", err, want)
		}
	}
	assetData = []byte("{")
	check("unexpected end")
	a := clone()
	setAssets(a)
	checkSource := fstest.MapFS{}
	if _, err := load(checkSource, assetData); err == nil || !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("missing source: %v", err)
	}
	a.Manifest["agent-to-renderer.json"] = "bad-hash"
	setAssets(a)
	check("drifted")
	a = clone()
	setDoc(&a, "agent-to-renderer.json", []byte("{"))
	setAssets(a)
	check("unexpected EOF")
	// Restore the resource before checking supplementary rules.
	originalDoc, _ := fs.ReadFile(source, "references/protocol/agent-to-renderer.json")
	setDoc(&a, "agent-to-renderer.json", originalDoc)
	a.ValidationRules = `{"version":2}`
	a.Manifest["a2ui-validation-rules.json"] = fmt.Sprintf("%x", sha256.Sum256([]byte(a.ValidationRules)))[:16]
	setAssets(a)
	check("version")
	// A syntactically valid but invalid root schema must fail during compilation.
	a = clone()
	var root map[string]any
	if err := json.Unmarshal(originalDoc, &root); err != nil {
		t.Fatal(err)
	}
	root["type"] = "not-a-json-schema-type"
	badRoot, _ := json.Marshal(root)
	setDoc(&a, "agent-to-renderer.json", badRoot)
	setAssets(a)
	check("type")
	setDoc(&a, "agent-to-renderer.json", originalDoc)
	// A schema in the package may be unused by the root envelope. Validate
	// every reference in it, including nested maps and arrays.
	const otherName = "catalog-components-specialized.json"
	originalOther, _ := fs.ReadFile(source, "references/protocol/"+otherName)
	var other map[string]any
	if err := json.Unmarshal(originalOther, &other); err != nil {
		t.Fatal(err)
	}
	other["x-test"] = map[string]any{"nested": []any{map[string]any{"$ref": ":bad"}}}
	brokenRef, _ := json.Marshal(other)
	setDoc(&a, otherName, brokenRef)
	setAssets(a)
	check("missing protocol scheme")
	other["x-test"] = map[string]any{"nested": []any{map[string]any{"$ref": "https://outside.invalid/unknown"}}}
	brokenRef, _ = json.Marshal(other)
	setDoc(&a, otherName, brokenRef)
	setAssets(a)
	check("outside the embedded package")
	setDoc(&a, otherName, originalOther)
	// Duplicate document IDs are rejected before the root is compiled.
	a = clone()
	other["$id"] = root["$id"]
	duplicateID, _ := json.Marshal(other)
	setDoc(&a, otherName, duplicateID)
	setAssets(a)
	check("already exists")
	setDoc(&a, otherName, originalOther)
	// A broken regular expression inside a verified schema must fail load.
	a = clone()
	root["type"] = "array"
	root["pattern"] = "("
	badRoot, _ = json.Marshal(root)
	setDoc(&a, "agent-to-renderer.json", badRoot)
	setAssets(a)
	check("error parsing regexp")
	// Even a valid rehashed Schema must still agree with the explain bundle.
	a = clone()
	setDoc(&a, "agent-to-renderer.json", append(append([]byte{}, originalDoc...), '\n'))
	setAssets(a)
	check("explain index and protocol manifest have drifted")
}
