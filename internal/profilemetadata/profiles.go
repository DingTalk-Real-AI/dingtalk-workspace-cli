// Package profilemetadata owns non-sensitive profile data and pure selector rules.
// It has no dependency on authentication, credential stores, or command execution.
package profilemetadata

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxVersion               = 3
	UnresolvedSelectorPrefix = "@legacy/"
	ProfileStatusActive      = "active"
)

type ProfilesConfig struct {
	Version            int               `json:"version"`
	PrimaryProfile     string            `json:"primaryProfile,omitempty"`
	CurrentProfile     string            `json:"currentProfile,omitempty"`
	PreviousProfile    string            `json:"previousProfile,omitempty"`
	OrgCurrentProfiles map[string]string `json:"orgCurrentProfiles,omitempty"`
	Profiles           []Profile         `json:"profiles,omitempty"`
}

type Profile struct {
	Name              string   `json:"name"`
	CorpID            string   `json:"corpId"`
	CorpName          string   `json:"corpName,omitempty"`
	UserID            string   `json:"userId,omitempty"`
	UserName          string   `json:"userName,omitempty"`
	ClientID          string   `json:"clientId,omitempty"`
	Status            string   `json:"status,omitempty"`
	AuthorizedDomains []string `json:"authorizedDomains,omitempty"`
	ExpiresAt         string   `json:"expiresAt,omitempty"`
	RefreshExpAt      string   `json:"refreshExpAt,omitempty"`
	LastLoginAt       string   `json:"lastLoginAt,omitempty"`
	LastUsedAt        string   `json:"lastUsedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
}

func ParseIdentitySelector(selector string) (corpID, userID string, ok bool) {
	selector = strings.TrimSpace(selector)
	idx := strings.Index(selector, ":")
	if idx <= 0 || idx >= len(selector)-1 {
		return "", "", false
	}
	return strings.TrimSpace(selector[:idx]), strings.TrimSpace(selector[idx+1:]), true
}

func ProfileSelector(profile Profile) string {
	return IdentitySelector(profile.CorpID, profile.UserID)
}

func ExactProfileSelectorForCorp(cfg *ProfilesConfig, corpID, selector string) string {
	selectedCorpID, userID, exact := ParseIdentitySelector(selector)
	if !exact || strings.TrimSpace(selectedCorpID) != strings.TrimSpace(corpID) {
		return ""
	}
	if p := FindExactProfile(cfg, selectedCorpID, userID); p != nil {
		return ProfileSelector(*p)
	}
	return ""
}

func FindExactProfile(cfg *ProfilesConfig, corpID, userID string) *Profile {
	idx := ProfileIndexByIdentity(cfg, corpID, userID)
	if idx < 0 {
		return nil
	}
	return &cfg.Profiles[idx]
}

func LocalProfileSelectorIsSafe(cfg *ProfilesConfig, profile *Profile, name string) bool {
	if cfg == nil || profile == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ":") || strings.HasPrefix(name, UnresolvedSelectorPrefix) {
		return false
	}
	nameMatches := 0
	for i := range cfg.Profiles {
		candidate := &cfg.Profiles[i]
		if strings.TrimSpace(candidate.Name) == name {
			nameMatches++
		}
		// Organization selectors are resolved before ordinary local names.
		// Never persist a local selector that can be captured by that grammar.
		if strings.TrimSpace(candidate.CorpID) == name || strings.TrimSpace(candidate.CorpName) == name {
			return false
		}
	}
	return nameMatches == 1
}

func Normalize(cfg *ProfilesConfig) {
	if cfg == nil {
		return
	}
	if cfg.Version <= 0 {
		cfg.Version = 1
	}
	seen := make(map[string]bool, len(cfg.Profiles))
	profiles := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		p.CorpID = strings.TrimSpace(p.CorpID)
		p.UserID = strings.TrimSpace(p.UserID)
		identity := ProfileIdentityKey(p.CorpID, p.UserID)
		if p.CorpID == "" || seen[identity] {
			continue
		}
		seen[identity] = true
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			p.Name = p.CorpID
		}
		if corpName := strings.TrimSpace(p.CorpName); p.Name == p.CorpID && corpName != "" && !ProfileNameTakenByOtherIdentity(cfg, corpName, p.CorpID, p.UserID) {
			p.Name = corpName
		}
		if p.Status == "" {
			p.Status = ProfileStatusActive
		}
		profiles = append(profiles, p)
	}
	cfg.Profiles = profiles
	if len(cfg.OrgCurrentProfiles) > 0 {
		normalized := make(map[string]string, len(cfg.OrgCurrentProfiles))
		for corpID, selector := range cfg.OrgCurrentProfiles {
			corpID = strings.TrimSpace(corpID)
			if exact := ExactProfileSelectorForCorp(cfg, corpID, selector); exact != "" {
				normalized[corpID] = exact
			}
		}
		if len(normalized) == 0 {
			cfg.OrgCurrentProfiles = nil
		} else {
			cfg.OrgCurrentProfiles = normalized
		}
	}
	if cfg.PrimaryProfile != "" && !ProfileSelectorReferenceExists(cfg, cfg.PrimaryProfile) {
		cfg.PrimaryProfile = ""
	}
	if cfg.CurrentProfile != "" && !ProfileSelectorReferenceExists(cfg, cfg.CurrentProfile) {
		cfg.CurrentProfile = ""
	}
	if cfg.PreviousProfile != "" && !ProfileSelectorReferenceExists(cfg, cfg.PreviousProfile) {
		cfg.PreviousProfile = ""
	}
}

func ParseUnresolvedProfileSelector(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if !strings.HasPrefix(selector, UnresolvedSelectorPrefix) {
		return "", false
	}
	encoded := strings.TrimPrefix(selector, UnresolvedSelectorPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	corpID := strings.TrimSpace(string(decoded))
	if corpID == "" || UnresolvedProfileSelector(corpID) != selector {
		return "", false
	}
	return corpID, true
}

func ProfileIdentityKey(corpID, userID string) string {
	return strings.TrimSpace(corpID) + "\x00" + strings.TrimSpace(userID)
}

func ProfileIndexByIdentity(cfg *ProfilesConfig, corpID, userID string) int {
	if cfg == nil {
		return -1
	}
	for i := range cfg.Profiles {
		if SameProfileIdentity(cfg.Profiles[i].CorpID, cfg.Profiles[i].UserID, corpID, userID) {
			return i
		}
	}
	return -1
}

func ProfileNameTakenByOtherIdentity(cfg *ProfilesConfig, name, corpID, userID string) bool {
	name = strings.TrimSpace(name)
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	for _, p := range cfg.Profiles {
		if strings.TrimSpace(p.Name) == name && !SameProfileIdentity(p.CorpID, p.UserID, corpID, userID) {
			return true
		}
	}
	return false
}

func IdentitySelector(corpID, userID string) string {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" {
		return ""
	}
	if userID == "" {
		return corpID
	}
	return corpID + ":" + userID
}

func ProfileSelectorCandidates(profiles []*Profile) []string {
	cfg := &ProfilesConfig{Profiles: make([]Profile, 0, len(profiles))}
	for _, profile := range profiles {
		if profile != nil {
			cfg.Profiles = append(cfg.Profiles, *profile)
		}
	}
	candidates := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if p == nil {
			continue
		}
		candidates = append(candidates, StoredProfileSelector(cfg, p))
	}
	sort.Strings(candidates)
	return candidates
}

func ProfileSelectorReferenceExists(cfg *ProfilesConfig, selector string) bool {
	if cfg == nil {
		return false
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	if corpID, unresolved := ParseUnresolvedProfileSelector(selector); unresolved {
		return UnresolvedProfileForCorp(cfg, corpID) != nil
	}
	if UnresolvedProfileForLocalName(cfg, selector) != nil {
		return true
	}
	if corpID, userID, exact := ParseIdentitySelector(selector); exact {
		if FindExactProfile(cfg, corpID, userID) != nil {
			return true
		}
		for i := range cfg.Profiles {
			p := &cfg.Profiles[i]
			orgMatches := strings.TrimSpace(p.CorpID) == corpID || strings.TrimSpace(p.CorpName) == corpID
			accountMatches := strings.TrimSpace(p.UserID) == userID || strings.TrimSpace(p.UserName) == userID
			if orgMatches && accountMatches {
				return true
			}
		}
		return false
	}
	if len(ProfilesForCorpID(cfg, selector)) > 0 {
		return true
	}
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) == selector ||
			strings.TrimSpace(cfg.Profiles[i].Name) == selector {
			return true
		}
	}
	return false
}

func ProfilesForCorpID(cfg *ProfilesConfig, corpID string) []*Profile {
	if cfg == nil {
		return nil
	}
	corpID = strings.TrimSpace(corpID)
	result := make([]*Profile, 0)
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpID) == corpID {
			result = append(result, &cfg.Profiles[i])
		}
	}
	return result
}

func ResolveOrganizationCorpID(cfg *ProfilesConfig, selector string) (string, error) {
	if cfg == nil {
		return "", nil
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", nil
	}
	if len(ProfilesForCorpID(cfg, selector)) > 0 {
		return selector, nil
	}
	orgIDs := make(map[string]struct{})
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) != selector {
			continue
		}
		orgIDs[strings.TrimSpace(cfg.Profiles[i].CorpID)] = struct{}{}
	}
	if len(orgIDs) == 0 {
		return "", nil
	}
	if len(orgIDs) == 1 {
		for corpID := range orgIDs {
			return corpID, nil
		}
	}
	candidates := make([]string, 0, len(orgIDs))
	for corpID := range orgIDs {
		candidates = append(candidates, corpID)
	}
	sort.Strings(candidates)
	return "", fmt.Errorf(
		"organization name %q is ambiguous; use one of: %s",
		selector,
		strings.Join(candidates, ", "),
	)
}

func ResolveOrganizationDefault(cfg *ProfilesConfig, corpID, displaySelector string, profiles []*Profile) (*Profile, bool, error) {
	if len(profiles) == 0 {
		return nil, false, fmt.Errorf("organization %q not found", displaySelector)
	}
	if exact := ExactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]); exact != "" {
		selectedCorpID, userID, _ := ParseIdentitySelector(exact)
		if p := FindExactProfile(cfg, selectedCorpID, userID); p != nil {
			return p, false, nil
		}
	}
	if unresolved := UnresolvedProfileForCorp(cfg, corpID); unresolved != nil {
		// With no exact organization-current selection, the organization slot
		// belongs to the sole unresolved historical account. Do not choose an
		// arbitrary exact identity merely because it shares the corpId.
		return unresolved, false, nil
	}
	if len(profiles) == 1 {
		return profiles[0], false, nil
	}
	return nil, false, fmt.Errorf(
		"organization %q has multiple accounts and no current account; use one of: %s",
		displaySelector,
		strings.Join(ProfileSelectorCandidates(profiles), ", "),
	)
}

func ResolveSelection(_ string, cfg *ProfilesConfig, selector string) (*Profile, bool, error) {
	if cfg == nil {
		return nil, false, fmt.Errorf("profile %q not found", strings.TrimSpace(selector))
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, false, fmt.Errorf("profile selector is empty")
	}
	if corpID, unresolved := ParseUnresolvedProfileSelector(selector); unresolved {
		if profile := UnresolvedProfileForCorp(cfg, corpID); profile != nil {
			return profile, true, nil
		}
		return nil, true, fmt.Errorf("historical profile for organization %q not found", corpID)
	}

	if organization, account, compound := ParseIdentitySelector(selector); compound {
		corpID, err := ResolveOrganizationCorpID(cfg, organization)
		if err != nil {
			return nil, true, err
		}
		if corpID == "" {
			return nil, true, fmt.Errorf("organization %q not found", organization)
		}
		if p := FindExactProfile(cfg, corpID, account); p != nil {
			return p, true, nil
		}
		var matches []*Profile
		for _, p := range ProfilesForCorpID(cfg, corpID) {
			if strings.TrimSpace(p.UserName) == account {
				matches = append(matches, p)
			}
		}
		switch len(matches) {
		case 1:
			return matches[0], true, nil
		case 0:
			return nil, true, fmt.Errorf("account %q not found in organization %q", account, organization)
		default:
			return nil, true, fmt.Errorf(
				"account name %q is ambiguous in organization %q; use one of: %s",
				account,
				organization,
				strings.Join(ProfileSelectorCandidates(matches), ", "),
			)
		}
	}

	if profiles := ProfilesForCorpID(cfg, selector); len(profiles) > 0 {
		return ResolveOrganizationDefault(cfg, selector, selector, profiles)
	}

	orgIDs := make(map[string]struct{})
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].CorpName) != selector {
			continue
		}
		orgIDs[strings.TrimSpace(cfg.Profiles[i].CorpID)] = struct{}{}
	}
	if len(orgIDs) == 1 {
		for corpID := range orgIDs {
			return ResolveOrganizationDefault(cfg, corpID, selector, ProfilesForCorpID(cfg, corpID))
		}
	}
	if len(orgIDs) > 1 {
		candidates := make([]string, 0, len(orgIDs))
		for corpID := range orgIDs {
			candidates = append(candidates, corpID)
		}
		sort.Strings(candidates)
		return nil, false, fmt.Errorf(
			"organization name %q is ambiguous; use one of: %s",
			selector,
			strings.Join(candidates, ", "),
		)
	}

	var nameMatches []*Profile
	for i := range cfg.Profiles {
		if strings.TrimSpace(cfg.Profiles[i].Name) != selector {
			continue
		}
		nameMatches = append(nameMatches, &cfg.Profiles[i])
	}
	switch len(nameMatches) {
	case 1:
		return nameMatches[0], true, nil
	case 0:
		return nil, false, fmt.Errorf("profile %q not found", selector)
	default:
		return nil, true, fmt.Errorf(
			"profile name %q is ambiguous; use one of: %s",
			selector,
			strings.Join(ProfileSelectorCandidates(nameMatches), ", "),
		)
	}
}

func SameProfileIdentity(leftCorpID, leftUserID, rightCorpID, rightUserID string) bool {
	return ProfileIdentityKey(leftCorpID, leftUserID) == ProfileIdentityKey(rightCorpID, rightUserID)
}

func StoredProfileSelector(cfg *ProfilesConfig, profile *Profile) string {
	if profile == nil {
		return ""
	}
	if strings.TrimSpace(profile.UserID) != "" {
		return ProfileSelector(*profile)
	}
	if cfg == nil {
		return strings.TrimSpace(profile.CorpID)
	}
	if len(ProfilesForCorpID(cfg, profile.CorpID)) <= 1 {
		return strings.TrimSpace(profile.CorpID)
	}
	name := strings.TrimSpace(profile.Name)
	if LocalProfileSelectorIsSafe(cfg, profile, name) {
		return name
	}
	return UnresolvedProfileSelector(profile.CorpID)
}

func UnresolvedProfileForCorp(cfg *ProfilesConfig, corpID string) *Profile {
	if cfg == nil {
		return nil
	}
	corpID = strings.TrimSpace(corpID)
	for i := range cfg.Profiles {
		profile := &cfg.Profiles[i]
		if strings.TrimSpace(profile.CorpID) == corpID && strings.TrimSpace(profile.UserID) == "" {
			return profile
		}
	}
	return nil
}

func UnresolvedProfileForLocalName(cfg *ProfilesConfig, name string) *Profile {
	if cfg == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	var match *Profile
	for i := range cfg.Profiles {
		profile := &cfg.Profiles[i]
		if strings.TrimSpace(profile.UserID) != "" || strings.TrimSpace(profile.Name) != name ||
			len(ProfilesForCorpID(cfg, profile.CorpID)) <= 1 {
			continue
		}
		if match != nil {
			return nil
		}
		match = profile
	}
	return match
}

func UnresolvedProfileSelector(corpID string) string {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return ""
	}
	return UnresolvedSelectorPrefix + base64.RawURLEncoding.EncodeToString([]byte(corpID))
}
