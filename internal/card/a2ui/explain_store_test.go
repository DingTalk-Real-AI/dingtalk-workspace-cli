// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAicardExplainBundleCompatibilityAndIsolation(t *testing.T) {
	store, err := BundledExplain()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range store.Names() {
		got, err := store.Lookup(name)
		if err != nil || got["name"] != name {
			t.Fatal(name, err)
		}
	}
	got, err := store.Lookup("text")
	if err != nil || got["name"] != "Text" {
		t.Fatal(got, err)
	}
	got["kind"] = "mutated"
	again, _ := store.Lookup("Text")
	if again["kind"] != "component" {
		t.Fatal("shared mutable result")
	}
	for name, entry := range store.index.Entries {
		if entry.Kind == "token-item" && strings.ToUpper(name) != name {
			result, err := store.Lookup(strings.ToUpper(name))
			if err != nil || result["kind"] != "unknown" {
				t.Fatal("token lookup lost case sensitivity", result, err)
			}
			break
		}
	}
	many, err := store.LookupMany([]string{"Text", "Text", "Row"}, false)
	if err != nil || len(many["contracts"].([]any)) != 2 {
		t.Fatal(many, err)
	}
	unknown, err := store.Lookup("Tabss")
	if err != nil || !reflect.DeepEqual(unknown["suggestions"], []string{"Tabs", "Table"}) {
		t.Fatal(unknown, err)
	}
	if err := store.CheckAll(testProtocol(t).assets.Manifest, testProtocol(t).assets.ExplainSHA256); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckAll(map[string]string{}, testProtocol(t).assets.ExplainSHA256); err == nil {
		t.Fatal("accepted stale manifest")
	}
}

func TestCrossPlatformCoverageAicardExplainBundleInvalidData(t *testing.T) {
	for _, raw := range []string{"{", `{}`, `{"formatVersion":2}`} {
		if _, err := NewExplainStore(fstest.MapFS{"explain.json": &fstest.MapFile{Data: []byte(raw)}}); err == nil {
			t.Fatal("accepted invalid bundle")
		}
	}
	if _, err := NewExplainStore(fstest.MapFS{}); err == nil {
		t.Fatal("accepted missing bundle")
	}
	store, _ := BundledExplain()
	raw, _ := json.Marshal(store.index)
	var fixture explainIndex
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Entries["Chart"] = explainEntry{Kind: "component", Definition: json.RawMessage(`{"name":"wrong","kind":"component"}`)}
	raw, _ = json.Marshal(fixture)
	broken, err := NewExplainStore(fstest.MapFS{"explain.json": &fstest.MapFile{Data: raw}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broken.Lookup("Text"); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.Lookup("Chart"); err == nil {
		t.Fatal("accepted mismatched definition")
	}
	if err := broken.CheckAll(testProtocol(t).assets.Manifest, testProtocol(t).assets.ExplainSHA256); err == nil {
		t.Fatal("self-check missed invalid definition")
	}
}

func TestExplainLookupDoesNotInitializeValidatorAssets(t *testing.T) {
	testseam.Swap(t, &assetsJSON, []byte("invalid validator metadata"))
	store, err := BundledExplain()
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Lookup("Text"); err != nil || result["name"] != "Text" {
		t.Fatal(result, err)
	}
}

func TestCrossPlatformCoverageAicardProtocolExplainPropagatesErrors(t *testing.T) {
	// Initialize Once before replacing the cached result; cleanup restores both
	// values and does not copy or reset a used synchronization primitive.
	if _, err := BundledExplain(); err != nil {
		t.Fatal(err)
	}
	p := &Protocol{}
	t.Run("bundle load", func(t *testing.T) {
		want := errors.New("broken explain bundle")
		testseam.Swap(t, &explainCachedError, want)
		if _, err := p.Explain("Text"); !errors.Is(err, want) {
			t.Fatalf("Explain load error = %v", err)
		}
		if _, err := p.ExplainMany([]string{"Text"}, true); !errors.Is(err, want) {
			t.Fatalf("ExplainMany load error = %v", err)
		}
	})
	t.Run("contract decode", func(t *testing.T) {
		broken := &ExplainStore{index: explainIndex{Entries: map[string]explainEntry{
			"Text": {Kind: "component", Definition: json.RawMessage(`{"name":"wrong","kind":"component"}`)},
		}}}
		testseam.Swap(t, &explainCached, broken)
		if _, err := p.Explain("Text"); err == nil {
			t.Fatal("Explain swallowed a corrupt definition")
		}
		if _, err := p.ExplainMany([]string{"Text"}, true); err == nil {
			t.Fatal("ExplainMany swallowed a corrupt definition")
		}
	})
}

func TestCrossPlatformCoverageAicardExplainBundleIntegrity(t *testing.T) {
	p := testProtocol(t)
	original, err := bundledExplain.ReadFile("explain.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"deleted", "truncated", "extra"} {
		t.Run(change, func(t *testing.T) {
			var index explainIndex
			if err := json.Unmarshal(original, &index); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "deleted":
				delete(index.Entries, "Text")
			case "truncated":
				index.Entries["Text"] = explainEntry{Kind: "component", Definition: json.RawMessage(`{"name":"Text","kind":"component"}`)}
			case "extra":
				index.Entries["Unexpected"] = explainEntry{Kind: "component", Definition: json.RawMessage(`{"name":"Unexpected","kind":"component"}`)}
			}
			changed, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewExplainStore(fstest.MapFS{"explain.json": &fstest.MapFile{Data: changed}})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CheckManifest(p.assets.Manifest); err != nil {
				t.Fatalf("mutation unexpectedly changed the protocol manifest: %v", err)
			}
			if _, err := store.Lookup("Row"); err != nil {
				t.Fatalf("unrelated named lookup should remain lazy: %v", err)
			}
			if err := store.CheckAll(p.assets.Manifest, p.assets.ExplainSHA256); err == nil {
				t.Fatal("full self-check accepted modified explain content")
			}
		})
	}
	store, err := BundledExplain()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CheckAll(p.assets.Manifest, ""); err == nil {
		t.Fatal("full self-check accepted a missing expected digest")
	}
}

func TestCrossPlatformCoverageAicardExplainIncompleteEntries(t *testing.T) {
	for _, tc := range []struct {
		name, entryName string
		entry           explainEntry
	}{
		{"empty name", "", explainEntry{Kind: "component", Definition: json.RawMessage(`{}`)}},
		{"missing kind", "Text", explainEntry{Definition: json.RawMessage(`{}`)}},
		{"missing definition", "Text", explainEntry{Kind: "component"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := explainIndex{FormatVersion: 1, ManifestSHA256: strings.Repeat("0", 64), Entries: map[string]explainEntry{tc.entryName: tc.entry}}
			raw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			// A missing RawMessage marshals as null; omit the field to represent absence.
			if tc.entry.Definition == nil {
				raw = []byte(strings.Replace(string(raw), `,"definition":null`, "", 1))
			}
			_, err = NewExplainStore(fstest.MapFS{"explain.json": &fstest.MapFile{Data: raw}})
			if err == nil || !strings.Contains(err.Error(), "invalid explain index entry") {
				t.Fatalf("incomplete entry accepted: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardExplainDecodeAndSelfCheckErrors(t *testing.T) {
	original, err := BundledExplain()
	if err != nil {
		t.Fatal(err)
	}
	p := testProtocol(t)
	for _, definition := range []json.RawMessage{json.RawMessage(`{`), json.RawMessage(`{"name":"wrong","kind":"component"}`)} {
		broken := &ExplainStore{index: explainIndex{ManifestSHA256: original.index.ManifestSHA256, Entries: map[string]explainEntry{
			"Text": {Kind: "component", Definition: definition},
		}}, sourceSHA256: original.sourceSHA256}
		if _, err := broken.Lookup("Text"); err == nil {
			t.Fatal("Lookup accepted corrupt definition")
		}
		if _, err := broken.LookupMany([]string{"Text"}, true); err == nil {
			t.Fatal("LookupMany swallowed definition error")
		}
		// Match the fixture digest so this specifically exercises per-definition checks.
		if err := broken.CheckAll(p.assets.Manifest, broken.sourceSHA256); err == nil {
			t.Fatal("CheckAll swallowed definition error")
		}
		t.Run(string(definition), func(t *testing.T) {
			testseam.Swap(t, &explainCached, broken)
			if err := p.CheckExplainAssets(); err == nil {
				t.Fatal("protocol self-check swallowed definition error")
			}
		})
	}
	t.Run("bundle load", func(t *testing.T) {
		want := errors.New("unreadable explain bundle")
		testseam.Swap(t, &explainCachedError, want)
		if err := p.CheckExplainAssets(); !errors.Is(err, want) {
			t.Fatalf("self-check load error = %v", err)
		}
		if _, err := Load(fstest.MapFS{}); !errors.Is(err, want) {
			t.Fatalf("protocol load lost the explain error: %v", err)
		}
	})
}
