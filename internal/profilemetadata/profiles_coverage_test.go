// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package profilemetadata

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageProfilesNormalizeResolveAndReadOnly(t *testing.T) {
	Normalize(nil)
	if corp, user, ok := ParseIdentitySelector(" corp : user "); !ok || corp != "corp" || user != "user" {
		t.Fatalf("selector parse = %q %q %v", corp, user, ok)
	}
	if ProfileSelector(Profile{CorpID: "c", UserID: "u"}) != "c:u" {
		t.Fatal("profile selector")
	}
	if IdentitySelector("", "u") != "" || IdentitySelector("c", "") != "c" {
		t.Fatal("identity selector edges")
	}
	if ProfileIdentityKey(" a ", " b ") != "a\x00b" {
		t.Fatal("identity key")
	}

	cfg := &ProfilesConfig{
		Version:         0,
		PrimaryProfile:  "missing",
		CurrentProfile:  "gone",
		PreviousProfile: "old",
		OrgCurrentProfiles: map[string]string{
			" c1 ": "c1:u1",
			"c2":   "nope",
		},
		Profiles: []Profile{
			{Name: "  ", CorpID: "c1", UserID: "u1", CorpName: "Acme"},
			{CorpID: "", UserID: "skip"},
			{Name: "dup", CorpID: "c1", UserID: "u1"},
			{Name: "taken", CorpID: "c2", UserID: "u2", Status: "active"},
			{Name: "", CorpID: "c3", UserID: "u3", CorpName: "taken"},
			{Name: "hist", CorpID: "c4", UserID: ""},
			{Name: "hist2", CorpID: "c4", UserID: "u4"},
			{Name: "shared", CorpID: "c5", UserID: "a", CorpName: "Twin"},
			{Name: "shared-b", CorpID: "c6", UserID: "b", CorpName: "Twin"},
			{Name: "solo", CorpID: "c7", UserID: "s"},
			{Name: "ambig", CorpID: "c8", UserID: "x"},
			{Name: "ambig", CorpID: "c9", UserID: "y"},
		},
	}
	Normalize(cfg)
	if cfg.Version != 1 || cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" {
		t.Fatalf("normalize refs = %#v", cfg)
	}
	if cfg.OrgCurrentProfiles["c1"] != "c1:u1" || cfg.OrgCurrentProfiles["c2"] != "" && len(cfg.OrgCurrentProfiles) != 1 {
		t.Fatalf("org current = %#v", cfg.OrgCurrentProfiles)
	}
	emptyOrg := &ProfilesConfig{OrgCurrentProfiles: map[string]string{"x": "y"}, Profiles: []Profile{{CorpID: "c", UserID: "u"}}}
	Normalize(emptyOrg)
	if emptyOrg.OrgCurrentProfiles != nil {
		t.Fatalf("empty org map retained: %#v", emptyOrg.OrgCurrentProfiles)
	}

	if ExactProfileSelectorForCorp(cfg, "c1", "other:u1") != "" {
		t.Fatal("wrong corp exact")
	}
	if ExactProfileSelectorForCorp(cfg, "c1", "c1:u1") != "c1:u1" {
		t.Fatal("exact selector")
	}
	if FindExactProfile(cfg, "missing", "u") != nil {
		t.Fatal("missing exact")
	}
	if !SameProfileIdentity("c", "u", " c ", " u ") {
		t.Fatal("same identity")
	}
	if !ProfileNameTakenByOtherIdentity(cfg, "taken", "c1", "u1") {
		t.Fatal("name taken")
	}
	if ProfileNameTakenByOtherIdentity(cfg, "solo", "c7", "s") {
		t.Fatal("own name reported taken")
	}
	if LocalProfileSelectorIsSafe(nil, &Profile{}, "n") || LocalProfileSelectorIsSafe(cfg, nil, "n") {
		t.Fatal("nil safe")
	}
	if LocalProfileSelectorIsSafe(cfg, &cfg.Profiles[0], "") || LocalProfileSelectorIsSafe(cfg, &cfg.Profiles[0], "c1:u") {
		t.Fatal("unsafe empty/colon")
	}
	if LocalProfileSelectorIsSafe(cfg, &cfg.Profiles[0], UnresolvedSelectorPrefix+"x") {
		t.Fatal("unresolved prefix safe")
	}
	if LocalProfileSelectorIsSafe(cfg, &cfg.Profiles[0], "c1") {
		t.Fatal("corp id as local name")
	}
	if !LocalProfileSelectorIsSafe(cfg, FindExactProfile(cfg, "c7", "s"), "solo") {
		t.Fatal("unique local name should be safe")
	}

	if UnresolvedProfileSelector("") != "" || !strings.HasPrefix(UnresolvedProfileSelector("c4"), UnresolvedSelectorPrefix) {
		t.Fatal("unresolved selector")
	}
	if corp, ok := ParseUnresolvedProfileSelector("nope"); ok || corp != "" {
		t.Fatal("non unresolved")
	}
	if _, ok := ParseUnresolvedProfileSelector(UnresolvedSelectorPrefix + "%%%"); ok {
		t.Fatal("bad base64")
	}
	if _, ok := ParseUnresolvedProfileSelector(UnresolvedSelectorPrefix); ok {
		t.Fatal("empty corp unresolved")
	}
	validUnresolved := UnresolvedProfileSelector("c4")
	if corp, ok := ParseUnresolvedProfileSelector(validUnresolved); !ok || corp != "c4" {
		t.Fatalf("roundtrip unresolved = %q %v", corp, ok)
	}
	if UnresolvedProfileForCorp(nil, "c4") != nil || UnresolvedProfileForCorp(cfg, "missing") != nil {
		t.Fatal("unresolved corp miss")
	}
	if UnresolvedProfileForCorp(cfg, "c4") == nil {
		t.Fatal("unresolved corp hit")
	}
	if UnresolvedProfileForLocalName(nil, "n") != nil || UnresolvedProfileForLocalName(cfg, "") != nil {
		t.Fatal("local name miss")
	}
	if UnresolvedProfileForLocalName(cfg, "solo") != nil {
		t.Fatal("single-account local name is not unresolved")
	}
	multi := &ProfilesConfig{Profiles: []Profile{
		{Name: "dupname", CorpID: "m1", UserID: ""},
		{Name: "other", CorpID: "m1", UserID: "u"},
		{Name: "dupname", CorpID: "m2", UserID: ""},
		{Name: "keep", CorpID: "m2", UserID: "v"},
	}}
	if UnresolvedProfileForLocalName(multi, "dupname") != nil {
		t.Fatal("ambiguous local unresolved")
	}
	if UnresolvedProfileForLocalName(multi, "keep") != nil {
		t.Fatal("named exact account")
	}
	only := &ProfilesConfig{Profiles: []Profile{
		{Name: "only", CorpID: "m3", UserID: ""},
		{Name: "other", CorpID: "m3", UserID: "u"},
	}}
	if UnresolvedProfileForLocalName(only, "only") == nil {
		t.Fatal("unique unresolved local name")
	}

	if StoredProfileSelector(cfg, nil) != "" {
		t.Fatal("nil stored")
	}
	if StoredProfileSelector(nil, &Profile{CorpID: "cx", UserID: ""}) != "cx" {
		t.Fatal("nil cfg stored")
	}
	if StoredProfileSelector(cfg, FindExactProfile(cfg, "c1", "u1")) != "c1:u1" {
		t.Fatal("exact stored")
	}
	if StoredProfileSelector(cfg, UnresolvedProfileForCorp(cfg, "c4")) != UnresolvedProfileSelector("c4") &&
		StoredProfileSelector(cfg, UnresolvedProfileForCorp(cfg, "c4")) == "c4" {
		t.Fatal("multi-account unresolved stored")
	}
	if StoredProfileSelector(cfg, FindExactProfile(cfg, "c7", "s")) != "c7:s" {
		t.Fatal("solo stored")
	}

	cands := ProfileSelectorCandidates([]*Profile{nil, FindExactProfile(cfg, "c1", "u1")})
	if len(cands) != 1 || cands[0] != "c1:u1" {
		t.Fatalf("candidates = %#v", cands)
	}
	if ProfileSelectorReferenceExists(nil, "c1:u1") || ProfileSelectorReferenceExists(cfg, "") {
		t.Fatal("exists miss")
	}
	if !ProfileSelectorReferenceExists(cfg, validUnresolved) {
		t.Fatal("unresolved exists")
	}
	if !ProfileSelectorReferenceExists(cfg, "c1:u1") || !ProfileSelectorReferenceExists(cfg, "c1") {
		t.Fatal("exact/org exists")
	}
	if !ProfileSelectorReferenceExists(cfg, "Acme:u1") {
		t.Fatal("org name + user exists")
	}
	if !ProfileSelectorReferenceExists(cfg, "solo") {
		t.Fatal("local name exists")
	}
	if !ProfileSelectorReferenceExists(only, "only") {
		t.Fatal("unresolved local name exists")
	}
	soloHist := &ProfilesConfig{Profiles: []Profile{{Name: "onlyhist", CorpID: "cOnly", UserID: ""}}}
	if StoredProfileSelector(soloHist, &soloHist.Profiles[0]) != "cOnly" {
		t.Fatal("single unresolved stored as corp")
	}
	unsafeHist := &ProfilesConfig{Profiles: []Profile{
		{Name: "c4", CorpID: "c4", UserID: ""},
		{Name: "other", CorpID: "c4", UserID: "u"},
	}}
	if StoredProfileSelector(unsafeHist, &unsafeHist.Profiles[0]) != UnresolvedProfileSelector("c4") {
		t.Fatal("unsafe multi-account unresolved stored")
	}
	if ProfileSelectorReferenceExists(cfg, "no-such") {
		t.Fatal("missing exists")
	}

	if _, err := ResolveOrganizationCorpID(nil, "x"); err != nil {
		t.Fatal(err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, ""); id != "" || err != nil {
		t.Fatal("empty org")
	}
	if id, err := ResolveOrganizationCorpID(cfg, "c1"); id != "c1" || err != nil {
		t.Fatal(err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, "missing-name"); id != "" || err != nil {
		t.Fatal(err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, "Acme"); id != "c1" || err != nil {
		t.Fatalf("unique org name = %q %v", id, err)
	}
	if _, err := ResolveOrganizationCorpID(cfg, "Twin"); err == nil {
		t.Fatal("ambiguous org name")
	}

	if _, _, err := ResolveOrganizationDefault(cfg, "c1", "c1", nil); err == nil {
		t.Fatal("empty org default")
	}
	cfg.OrgCurrentProfiles = map[string]string{"c1": "c1:u1"}
	if p, _, err := ResolveOrganizationDefault(cfg, "c1", "c1", ProfilesForCorpID(cfg, "c1")); err != nil || p == nil || p.UserID != "u1" {
		t.Fatalf("org current default = %#v %v", p, err)
	}
	if p, _, err := ResolveOrganizationDefault(cfg, "c4", "c4", ProfilesForCorpID(cfg, "c4")); err != nil || p == nil || p.UserID != "" {
		t.Fatalf("unresolved default = %#v %v", p, err)
	}
	if p, _, err := ResolveOrganizationDefault(cfg, "c7", "c7", ProfilesForCorpID(cfg, "c7")); err != nil || p == nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveOrganizationDefault(cfg, "c8", "c8", append(ProfilesForCorpID(cfg, "c8"), ProfilesForCorpID(cfg, "c9")...)); err == nil {
		t.Fatal("multi default")
	}

	if _, _, err := ResolveSelection("", nil, "x"); err == nil {
		t.Fatal("nil cfg resolve")
	}
	if _, _, err := ResolveSelection("", cfg, "  "); err == nil {
		t.Fatal("empty selector")
	}
	if p, hist, err := ResolveSelection("", cfg, validUnresolved); err != nil || !hist || p == nil {
		t.Fatalf("unresolved resolve = %#v %v %v", p, hist, err)
	}
	if _, _, err := ResolveSelection("", cfg, UnresolvedProfileSelector("missing")); err == nil {
		t.Fatal("missing historical")
	}
	if _, _, err := ResolveSelection("", cfg, "Twin:u"); err == nil {
		t.Fatal("ambiguous org compound")
	}
	if _, _, err := ResolveSelection("", cfg, "no-org:u"); err == nil {
		t.Fatal("missing org compound")
	}
	if p, exact, err := ResolveSelection("", cfg, "c1:u1"); err != nil || !exact || p.UserID != "u1" {
		t.Fatal("exact compound")
	}
	cfg.Profiles = append(cfg.Profiles, Profile{Name: "nick", CorpID: "c1", UserID: "u9", UserName: "Pat"})
	if p, _, err := ResolveSelection("", cfg, "c1:Pat"); err != nil || p.UserID != "u9" {
		t.Fatalf("username compound = %#v %v", p, err)
	}
	cfg.Profiles = append(cfg.Profiles, Profile{Name: "nick2", CorpID: "c1", UserID: "u10", UserName: "Pat"})
	if _, _, err := ResolveSelection("", cfg, "c1:Pat"); err == nil {
		t.Fatal("ambiguous username")
	}
	if _, _, err := ResolveSelection("", cfg, "c1:nobody"); err == nil {
		t.Fatal("missing account")
	}
	if p, _, err := ResolveSelection("", cfg, "c7"); err != nil || p == nil {
		t.Fatal(err)
	}
	if p, _, err := ResolveSelection("", cfg, "Acme"); err != nil || p == nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveSelection("", cfg, "Twin"); err == nil {
		t.Fatal("ambiguous org name resolve")
	}
	if p, exact, err := ResolveSelection("", cfg, "solo"); err != nil || !exact || p.UserID != "s" {
		t.Fatal("local name resolve")
	}
	if _, _, err := ResolveSelection("", cfg, "nope"); err == nil {
		t.Fatal("missing name")
	}
	if _, _, err := ResolveSelection("", cfg, "ambig"); err == nil {
		t.Fatal("ambiguous name")
	}

	dir := t.TempDir()
	if meta, err := ResolveReadOnly(dir, ""); err != nil || meta != nil {
		t.Fatalf("missing file = %#v %v", meta, err)
	}
	if _, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) { return nil, errors.New("io") }); err == nil {
		t.Fatal("read error")
	}
	if _, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) { return []byte("{"), nil }); err == nil {
		t.Fatal("json error")
	}
	if _, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) {
		return []byte(`{"version":99}`), nil
	}); err == nil {
		t.Fatal("future version")
	}
	body, err := json.Marshal(ProfilesConfig{
		Version:        1,
		CurrentProfile: "c1:u1",
		Profiles:       []Profile{{Name: "a", CorpID: "c1", UserID: "u1", UserName: "Ada"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := ResolveReadOnly(dir, "")
	if err != nil || meta == nil || meta.UserID != "u1" || meta.CorpID != "c1" {
		t.Fatalf("current resolve = %#v %v", meta, err)
	}
	if _, err := ResolveReadOnly(dir, "missing"); err == nil {
		t.Fatal("missing selector")
	}
	emptyCurrent, err := json.Marshal(ProfilesConfig{Version: 1, Profiles: []Profile{{CorpID: "c1", UserID: "u1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if meta, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) { return emptyCurrent, nil }); err != nil || meta != nil {
		t.Fatalf("empty current = %#v %v", meta, err)
	}
}
