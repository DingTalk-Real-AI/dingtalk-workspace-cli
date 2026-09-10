package profilemetadata

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageSelectorAndIndexEdges(t *testing.T) {
	if _, _, ok := ParseIdentitySelector(" :user"); ok {
		t.Fatal("whitespace corp accepted")
	}
	if _, _, ok := ParseIdentitySelector("corp: "); ok {
		t.Fatal("whitespace user accepted")
	}
	if _, _, ok := ParseIdentitySelector(":user"); ok {
		t.Fatal("empty corp accepted")
	}
	cfg := &ProfilesConfig{Profiles: []Profile{{Name: "n", CorpID: "c1", UserID: "u1"}}}
	if got := ExactProfileSelectorForCorp(cfg, "c1", "c1:missing"); got != "" {
		t.Fatalf("missing exact selector = %q", got)
	}
	if ProfileIndexByIdentity(nil, "c", "u") != -1 {
		t.Fatal("nil cfg index")
	}
	if ProfileSelectorReferenceExists(cfg, "c1:missing") {
		t.Fatal("missing exact identity exists")
	}
	if ProfilesForCorpID(nil, "c1") != nil {
		t.Fatal("nil cfg profiles")
	}
}

func TestCrossPlatformCoverageProfileMetadataRules(t *testing.T) {
	if ProfileSelector(Profile{CorpID: "c", UserID: "u"}) != "c:u" {
		t.Fatal("ProfileSelector")
	}
	if IdentitySelector("", "u") != "" || IdentitySelector("c", "") != "c" {
		t.Fatal("IdentitySelector edges")
	}
	if UnresolvedProfileSelector("") != "" {
		t.Fatal("empty unresolved selector")
	}

	Normalize(nil)
	cfg := &ProfilesConfig{
		Version:         0,
		PrimaryProfile:  "gone",
		CurrentProfile:  "gone",
		PreviousProfile: "gone",
		OrgCurrentProfiles: map[string]string{
			" ":    "x",
			"c1":   "c1:u1",
			"skip": "nope",
		},
		Profiles: []Profile{
			{CorpID: "", UserID: "x"},
			{CorpID: "c1", UserID: "u1", CorpName: "Org"},
			{CorpID: "c1", UserID: "u1", Name: "dup"},
			{CorpID: "c2", UserID: "u2", Name: "taken", CorpName: "Other"},
			{CorpID: "c3", UserID: "", Name: "legacy"},
		},
	}
	Normalize(cfg)
	if cfg.Version != 1 || cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" {
		t.Fatalf("normalize refs = %+v", cfg)
	}
	if cfg.OrgCurrentProfiles["c1"] != "c1:u1" || cfg.OrgCurrentProfiles["skip"] != "" {
		t.Fatalf("org current = %#v", cfg.OrgCurrentProfiles)
	}

	emptyOrgs := &ProfilesConfig{OrgCurrentProfiles: map[string]string{"c": "missing"}, Profiles: []Profile{{CorpID: "other", UserID: "u"}}}
	Normalize(emptyOrgs)
	if emptyOrgs.OrgCurrentProfiles != nil {
		t.Fatal("empty org map not cleared")
	}

	if ExactProfileSelectorForCorp(cfg, "c1", "other:u1") != "" {
		t.Fatal("wrong corp exact selector")
	}
	if got := ExactProfileSelectorForCorp(cfg, "c1", "c1:u1"); got != "c1:u1" {
		t.Fatalf("exact selector = %q", got)
	}

	p := FindExactProfile(cfg, "c1", "u1")
	if p == nil || !LocalProfileSelectorIsSafe(cfg, p, p.Name) && p.Name == "Org" {
		// Org name may be unsafe if it matches corp name; just exercise both outcomes.
		_ = LocalProfileSelectorIsSafe(cfg, p, p.Name)
	}
	if LocalProfileSelectorIsSafe(nil, p, "n") || LocalProfileSelectorIsSafe(cfg, nil, "n") {
		t.Fatal("nil local selector")
	}
	if LocalProfileSelectorIsSafe(cfg, p, "") || LocalProfileSelectorIsSafe(cfg, p, "c:u") || LocalProfileSelectorIsSafe(cfg, p, UnresolvedSelectorPrefix+"x") {
		t.Fatal("unsafe local names accepted")
	}
	if LocalProfileSelectorIsSafe(cfg, p, "c1") || LocalProfileSelectorIsSafe(cfg, p, "Org") {
		t.Fatal("corp-captured local name accepted")
	}
	dupName := &ProfilesConfig{Profiles: []Profile{{Name: "same", CorpID: "a", UserID: "1"}, {Name: "same", CorpID: "b", UserID: "2"}}}
	if LocalProfileSelectorIsSafe(dupName, &dupName.Profiles[0], "same") {
		t.Fatal("duplicate local name accepted")
	}

	encoded := UnresolvedProfileSelector("c3")
	if corp, ok := ParseUnresolvedProfileSelector(encoded); !ok || corp != "c3" {
		t.Fatalf("roundtrip unresolved = %q %v", corp, ok)
	}
	if _, ok := ParseUnresolvedProfileSelector("plain"); ok {
		t.Fatal("plain unresolved")
	}
	if _, ok := ParseUnresolvedProfileSelector(UnresolvedSelectorPrefix + "%%%"); ok {
		t.Fatal("bad base64 unresolved")
	}
	if _, ok := ParseUnresolvedProfileSelector(UnresolvedSelectorPrefix + base64.RawURLEncoding.EncodeToString([]byte(" "))); ok {
		t.Fatal("whitespace unresolved")
	}
	if _, ok := ParseUnresolvedProfileSelector(UnresolvedSelectorPrefix + base64.RawURLEncoding.EncodeToString([]byte("c3")) + "??"); ok {
		t.Fatal("junk unresolved")
	}

	if ProfileNameTakenByOtherIdentity(cfg, "taken", "c2", "u2") {
		t.Fatal("own name reported taken")
	}
	if !ProfileNameTakenByOtherIdentity(cfg, "taken", "c1", "u1") {
		t.Fatal("foreign name not taken")
	}

	cands := ProfileSelectorCandidates([]*Profile{nil, p})
	if len(cands) != 1 {
		t.Fatalf("candidates = %#v", cands)
	}

	if ProfileSelectorReferenceExists(nil, "c1") || ProfileSelectorReferenceExists(cfg, "") {
		t.Fatal("empty reference")
	}
	if !ProfileSelectorReferenceExists(cfg, encoded) {
		t.Fatal("unresolved reference missing")
	}
	if !ProfileSelectorReferenceExists(cfg, "c1:u1") {
		t.Fatal("exact reference missing")
	}
	aliasCfg := &ProfilesConfig{Profiles: []Profile{{CorpID: "c1", CorpName: "Org", UserID: "u1", UserName: "Alice"}}}
	if !ProfileSelectorReferenceExists(aliasCfg, "Org:Alice") {
		t.Fatal("display identity reference missing")
	}
	if ProfileSelectorReferenceExists(aliasCfg, "Org:missing") {
		t.Fatal("missing display identity exists")
	}
	if !ProfileSelectorReferenceExists(cfg, "c1") || !ProfileSelectorReferenceExists(cfg, "Org") {
		t.Fatal("org/name reference missing")
	}

	if _, err := ResolveOrganizationCorpID(nil, "x"); err != nil {
		t.Fatal(err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, ""); id != "" || err != nil {
		t.Fatalf("empty org = %q %v", id, err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, "c1"); id != "c1" || err != nil {
		t.Fatalf("corp id org = %q %v", id, err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, "missing-name"); id != "" || err != nil {
		t.Fatalf("unknown org name = %q %v", id, err)
	}
	if id, err := ResolveOrganizationCorpID(cfg, "Org"); id != "c1" || err != nil {
		t.Fatalf("unique org name = %q %v", id, err)
	}
	ambig := &ProfilesConfig{Profiles: []Profile{{CorpID: "a", CorpName: "Same"}, {CorpID: "b", CorpName: "Same"}}}
	if _, err := ResolveOrganizationCorpID(ambig, "Same"); err == nil {
		t.Fatal("ambiguous org name accepted")
	}

	if _, _, err := ResolveOrganizationDefault(cfg, "c1", "c1", nil); err == nil {
		t.Fatal("empty org default accepted")
	}
	orgCfg := &ProfilesConfig{
		OrgCurrentProfiles: map[string]string{"c1": "c1:u1"},
		Profiles:           []Profile{{CorpID: "c1", UserID: "u1"}, {CorpID: "c1", UserID: "u9"}},
	}
	if p, _, err := ResolveOrganizationDefault(orgCfg, "c1", "c1", ProfilesForCorpID(orgCfg, "c1")); err != nil || p.UserID != "u1" {
		t.Fatalf("org current default = %+v %v", p, err)
	}
	unresolvedOnly := &ProfilesConfig{Profiles: []Profile{{CorpID: "c3"}}}
	if p, _, err := ResolveOrganizationDefault(unresolvedOnly, "c3", "c3", ProfilesForCorpID(unresolvedOnly, "c3")); err != nil || p == nil || p.UserID != "" {
		t.Fatalf("unresolved default = %+v %v", p, err)
	}
	single := ProfilesForCorpID(cfg, "c2")
	if p, _, err := ResolveOrganizationDefault(cfg, "c2", "c2", single); err != nil || p.UserID != "u2" {
		t.Fatalf("single default = %+v %v", p, err)
	}
	multiAcct := &ProfilesConfig{Profiles: []Profile{{CorpID: "c1", UserID: "a"}, {CorpID: "c1", UserID: "b"}}}
	if _, _, err := ResolveOrganizationDefault(multiAcct, "c1", "c1", ProfilesForCorpID(multiAcct, "c1")); err == nil {
		t.Fatal("multi account default accepted")
	}

	if _, _, err := ResolveSelection("", nil, "x"); err == nil {
		t.Fatal("nil cfg selection")
	}
	if _, _, err := ResolveSelection("", cfg, "  "); err == nil {
		t.Fatal("empty selector")
	}
	if p, hist, err := ResolveSelection("", cfg, encoded); err != nil || !hist || p == nil {
		t.Fatalf("unresolved selection = %+v %v %v", p, hist, err)
	}
	if _, _, err := ResolveSelection("", cfg, UnresolvedProfileSelector("missing-corp")); err == nil {
		t.Fatal("missing historical accepted")
	}
	if _, _, err := ResolveSelection("", ambig, "Same:user"); err == nil {
		t.Fatal("ambiguous org compound accepted")
	}
	if _, _, err := ResolveSelection("", cfg, "no-org:user"); err == nil {
		t.Fatal("missing org compound accepted")
	}
	if p, exact, err := ResolveSelection("", cfg, "c1:u1"); err != nil || !exact || p.UserID != "u1" {
		t.Fatalf("exact compound = %+v %v %v", p, exact, err)
	}
	named := &ProfilesConfig{Profiles: []Profile{{CorpID: "c1", UserID: "u1", UserName: "Alice"}, {CorpID: "c1", UserID: "u2", UserName: "Alice"}}}
	if p, _, err := ResolveSelection("", &ProfilesConfig{Profiles: []Profile{{CorpID: "c1", UserID: "u1", UserName: "Alice"}}}, "c1:Alice"); err != nil || p.UserID != "u1" {
		t.Fatalf("username compound = %+v %v", p, err)
	}
	if _, _, err := ResolveSelection("", cfg, "c1:nobody"); err == nil {
		t.Fatal("missing account accepted")
	}
	if _, _, err := ResolveSelection("", named, "c1:Alice"); err == nil {
		t.Fatal("ambiguous account accepted")
	}
	if p, _, err := ResolveSelection("", cfg, "c2"); err != nil || p.UserID != "u2" {
		t.Fatalf("corp selection = %+v %v", p, err)
	}
	if p, _, err := ResolveSelection("", cfg, "Org"); err != nil || p.CorpID != "c1" {
		t.Fatalf("org-name selection = %+v %v", p, err)
	}
	if _, _, err := ResolveSelection("", ambig, "Same"); err == nil {
		t.Fatal("ambiguous org-name selection accepted")
	}
	if p, exact, err := ResolveSelection("", cfg, "taken"); err != nil || !exact || p.UserID != "u2" {
		t.Fatalf("unique name selection = %+v %v %v", p, exact, err)
	}
	if _, _, err := ResolveSelection("", cfg, "missing-profile"); err == nil {
		t.Fatal("missing name accepted")
	}
	if _, _, err := ResolveSelection("", dupName, "same"); err == nil {
		t.Fatal("ambiguous name accepted")
	}

	if StoredProfileSelector(cfg, nil) != "" {
		t.Fatal("nil stored selector")
	}
	if StoredProfileSelector(cfg, &Profile{CorpID: "c1", UserID: "u1"}) != "c1:u1" {
		t.Fatal("user stored selector")
	}
	if StoredProfileSelector(nil, &Profile{CorpID: "solo"}) != "solo" {
		t.Fatal("nil cfg stored selector")
	}
	if StoredProfileSelector(cfg, &Profile{CorpID: "c2"}) != "c2" {
		t.Fatal("unique corp stored selector")
	}
	multi := &ProfilesConfig{Profiles: []Profile{{CorpID: "m", Name: "safe"}, {CorpID: "m", UserID: "u", Name: "other"}}}
	if StoredProfileSelector(multi, &multi.Profiles[0]) == "" {
		t.Fatal("unsafe/unresolved stored selector empty")
	}

	if UnresolvedProfileForCorp(nil, "c") != nil || UnresolvedProfileForCorp(cfg, "c1") != nil {
		t.Fatal("resolved corp treated unresolved")
	}
	if UnresolvedProfileForCorp(cfg, "c3") == nil {
		t.Fatal("legacy corp missing")
	}
	if UnresolvedProfileForLocalName(nil, "x") != nil || UnresolvedProfileForLocalName(cfg, "") != nil {
		t.Fatal("empty local unresolved")
	}
	multiLegacy := &ProfilesConfig{Profiles: []Profile{{CorpID: "m1", Name: "legacy"}, {CorpID: "m1", UserID: "u"}, {CorpID: "m2", Name: "legacy"}, {CorpID: "m2", UserID: "v"}}}
	if UnresolvedProfileForLocalName(multiLegacy, "legacy") != nil {
		t.Fatal("ambiguous local unresolved")
	}
	if UnresolvedProfileForLocalName(multi, "safe") == nil {
		t.Fatal("safe local unresolved missing")
	}
}

func TestCrossPlatformCoverageResolveReadOnly(t *testing.T) {
	if _, err := ResolveReadOnly(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReadOnly(dir, ""); err == nil {
		t.Fatal("corrupt metadata accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReadOnly(dir, ""); err == nil {
		t.Fatal("future version accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(`{"version":1,"currentProfile":"c:u","profiles":[{"corpId":"c","userId":"u","userName":"Ada"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveReadOnly(dir, "")
	if err != nil || got == nil || got.UserID != "u" {
		t.Fatalf("current profile = %+v %v", got, err)
	}
	if _, err := ResolveReadOnly(dir, "missing"); err == nil {
		t.Fatal("missing selector accepted")
	}
	if _, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) { return nil, errors.New("io") }); err == nil {
		t.Fatal("reader error accepted")
	}
	empty, err := ResolveReadOnlyWithReader(dir, "", func(string) ([]byte, error) {
		return []byte(`{"version":1}`), nil
	})
	if err != nil || empty != nil {
		t.Fatalf("empty current = %+v %v", empty, err)
	}
}
