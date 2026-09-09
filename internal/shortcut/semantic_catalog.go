// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package shortcut

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed semantic_catalog.json
var semanticCatalogJSON []byte

//go:embed semantic_catalog_doc.json
var docSemanticCatalogJSON []byte

//go:embed semantic_catalog_aitable.json
var aitableSemanticCatalogJSON []byte

//go:embed semantic_catalog_minutes.json
var minutesSemanticCatalogJSON []byte

//go:embed semantic_catalog_drive.json
var driveSemanticCatalogJSON []byte

//go:embed semantic_catalog_wiki.json
var wikiSemanticCatalogJSON []byte

//go:embed semantic_catalog_calendar.json
var calendarSemanticCatalogJSON []byte

//go:embed semantic_catalog_todo.json
var todoSemanticCatalogJSON []byte

//go:embed semantic_catalog_attendance.json
var attendanceSemanticCatalogJSON []byte

//go:embed semantic_catalog_mail.json
var mailSemanticCatalogJSON []byte

//go:embed semantic_catalog_aisearch.json
var aisearchSemanticCatalogJSON []byte

//go:embed semantic_catalog_contact.json
var contactSemanticCatalogJSON []byte

//go:embed semantic_catalog_live.json
var liveSemanticCatalogJSON []byte

//go:embed semantic_catalog_oa.json
var oaSemanticCatalogJSON []byte

//go:embed semantic_catalog_ding.json
var dingSemanticCatalogJSON []byte

//go:embed semantic_catalog_report.json
var reportSemanticCatalogJSON []byte

//go:embed semantic_catalog_sheet.json
var sheetSemanticCatalogJSON []byte

//go:embed semantic_catalog_whiteboard.json
var whiteboardSemanticCatalogJSON []byte

//go:embed semantic_catalog_devdoc.json
var devdocSemanticCatalogJSON []byte

//go:embed semantic_catalog_hrbrain.json
var hrbrainSemanticCatalogJSON []byte

//go:embed semantic_catalog_pat.json
var patSemanticCatalogJSON []byte

//go:embed semantic_catalog_devapp.json
var devAppSemanticCatalogJSON []byte

//go:embed semantic_catalog_agoal.json
var agoalSemanticCatalogJSON []byte

type semanticCatalogFile struct {
	Version           int                              `json:"version"`
	Service           string                           `json:"service"`
	Availability      Availability                     `json:"default_availability"`
	FeaturedShortcuts []string                         `json:"featured_shortcuts,omitempty"`
	AtomicOwners      map[string]string                `json:"atomic_owners,omitempty"`
	Shortcuts         map[string]semanticCatalogRecord `json:"shortcuts"`
}

type semanticCatalogRecord struct {
	Disposition          SemanticDisposition `json:"disposition"`
	SemanticDelta        string              `json:"semantic_delta"`
	Risk                 Risk                `json:"risk"`
	Availability         Availability        `json:"availability,omitempty"`
	Primary              string              `json:"primary,omitempty"`
	Public               bool                `json:"public"`
	CompatibilityVisible bool                `json:"compatibility_visible,omitempty"`
	Reviewed             bool                `json:"reviewed"`
	HelpTier             HelpTier            `json:"-"`
}

func semanticCatalogSources() [][]byte {
	return [][]byte{
		semanticCatalogJSON,
		docSemanticCatalogJSON,
		aitableSemanticCatalogJSON,
		minutesSemanticCatalogJSON,
		driveSemanticCatalogJSON,
		wikiSemanticCatalogJSON,
		calendarSemanticCatalogJSON,
		todoSemanticCatalogJSON,
		attendanceSemanticCatalogJSON,
		mailSemanticCatalogJSON,
		aisearchSemanticCatalogJSON,
		contactSemanticCatalogJSON,
		liveSemanticCatalogJSON,
		oaSemanticCatalogJSON,
		dingSemanticCatalogJSON,
		reportSemanticCatalogJSON,
		sheetSemanticCatalogJSON,
		whiteboardSemanticCatalogJSON,
		devdocSemanticCatalogJSON,
		hrbrainSemanticCatalogJSON,
		patSemanticCatalogJSON,
		devAppSemanticCatalogJSON,
		agoalSemanticCatalogJSON,
	}
}

var reviewedSemanticCatalog = mustLoadSemanticCatalogs(semanticCatalogSources()...)
var reviewedAtomicShortcutOwners = mustLoadAtomicShortcutOwners(reviewedSemanticCatalog, semanticCatalogSources()...)

func mustLoadSemanticCatalogs(sources ...[]byte) map[string]semanticCatalogRecord {
	out := make(map[string]semanticCatalogRecord)
	for _, raw := range sources {
		loadSemanticCatalog(raw, out)
	}
	return out
}

// mustLoadSemanticCatalog is retained for focused validation tests of the
// legacy single-source loader. Production loads every reviewed product source
// through mustLoadSemanticCatalogs above.
func mustLoadSemanticCatalog() map[string]semanticCatalogRecord {
	return mustLoadSemanticCatalogs(semanticCatalogJSON)
}

func mustLoadAtomicShortcutOwners(records map[string]semanticCatalogRecord, sources ...[]byte) map[string]string {
	out := make(map[string]string)
	for _, raw := range sources {
		var source semanticCatalogFile
		if err := json.Unmarshal(raw, &source); err != nil {
			panic(fmt.Sprintf("invalid shortcut semantic catalog: %v", err))
		}
		for rawPath, rawOwner := range source.AtomicOwners {
			path := strings.TrimSpace(rawPath)
			owner := strings.TrimSpace(rawOwner)
			if path != rawPath || owner != rawOwner {
				panic(fmt.Sprintf("semantic catalog atomic owner %q -> %q must be normalized", rawPath, rawOwner))
			}
			if path == "" || !strings.HasPrefix(path, source.Service+" ") || strings.HasPrefix(path, "dws ") {
				panic(fmt.Sprintf("semantic catalog atomic owner path %q must be an exact %s CLI path without dws prefix", rawPath, source.Service))
			}
			if !strings.HasPrefix(owner, "+") {
				panic(fmt.Sprintf("semantic catalog atomic owner %q for %q lacks + prefix", rawOwner, path))
			}
			record, ok := records[publicCatalogKey(source.Service, owner)]
			if !ok || !record.Public || record.Availability != AvailabilityAvailable ||
				record.Disposition == DispositionAliasInternal || record.CompatibilityVisible {
				panic(fmt.Sprintf("semantic catalog atomic owner %q for %q must name one public available canonical Shortcut", owner, path))
			}
			if previous, exists := out[path]; exists {
				panic(fmt.Sprintf("semantic catalog atomic path %q has duplicate owners %q and %q", path, previous, owner))
			}
			out[path] = source.Service + " " + owner
		}
	}
	return out
}

// PreferredShortcutForCLIPath returns the reviewed Shortcut-first owner for
// one exact atomic CLI path. It is product discovery metadata only; command
// identity and executability continue to come from ContractFinal and Cobra.
func PreferredShortcutForCLIPath(cliPath string) (string, bool) {
	owner, ok := reviewedAtomicShortcutOwners[strings.TrimSpace(cliPath)]
	return owner, ok
}

// PreferredShortcutOwnersSnapshot returns a defensive copy for exact-set
// policy tests that keep Schema exclusions aligned with reviewed discovery.
func PreferredShortcutOwnersSnapshot() map[string]string {
	out := make(map[string]string, len(reviewedAtomicShortcutOwners))
	for path, owner := range reviewedAtomicShortcutOwners {
		out[path] = owner
	}
	return out
}

func loadSemanticCatalog(raw []byte, out map[string]semanticCatalogRecord) {
	var source semanticCatalogFile
	if err := json.Unmarshal(raw, &source); err != nil {
		panic(fmt.Sprintf("invalid shortcut semantic catalog: %v", err))
	}
	if source.Version != 1 || strings.TrimSpace(source.Service) == "" {
		panic("invalid shortcut semantic catalog header")
	}
	featured := make(map[string]bool, len(source.FeaturedShortcuts))
	for _, rawCommand := range source.FeaturedShortcuts {
		command := strings.TrimSpace(rawCommand)
		if command != rawCommand {
			panic(fmt.Sprintf("semantic catalog featured command %q must be normalized", rawCommand))
		}
		if !strings.HasPrefix(command, "+") {
			panic(fmt.Sprintf("semantic catalog featured command %q lacks + prefix", command))
		}
		if featured[command] {
			panic(fmt.Sprintf("semantic catalog featured command %q is duplicated", command))
		}
		featured[command] = true
	}
	for command, record := range source.Shortcuts {
		if !strings.HasPrefix(command, "+") {
			panic(fmt.Sprintf("semantic catalog command %q lacks + prefix", command))
		}
		if !record.Reviewed || strings.TrimSpace(record.SemanticDelta) == "" {
			panic(fmt.Sprintf("semantic catalog command %q is not reviewed", command))
		}
		switch record.Disposition {
		case DispositionPrimarySmart, DispositionSemanticAdapter, DispositionSchemaLeaf, DispositionAliasInternal:
		default:
			panic(fmt.Sprintf("semantic catalog command %q has invalid disposition %q", command, record.Disposition))
		}
		switch record.Risk {
		case RiskRead, RiskWrite, RiskHighWrite:
		default:
			panic(fmt.Sprintf("semantic catalog command %q has invalid risk %q", command, record.Risk))
		}
		if record.Availability == "" {
			record.Availability = source.Availability
		}
		switch record.Availability {
		case AvailabilityAvailable, AvailabilityUnavailable, AvailabilityDeprecated:
		default:
			panic(fmt.Sprintf("semantic catalog command %q has invalid availability %q", command, record.Availability))
		}
		if record.Disposition == DispositionAliasInternal && strings.TrimSpace(record.Primary) == "" {
			panic(fmt.Sprintf("semantic alias %q must name its primary command", command))
		}
		// Public visibility is an explicit reviewed product decision. Disposition
		// remains routing metadata: a reviewed, available Schema projection or
		// compatibility alias can still be intentionally exposed as a Shortcut.
		if record.Public && record.Availability != AvailabilityAvailable {
			panic(fmt.Sprintf("semantic catalog command %q cannot be public with availability %q",
				command, record.Availability))
		}
		// A command that was already part of the visible CLI contract cannot be
		// hidden merely because Agent publication is withdrawn. Compatibility
		// visibility owns only historical CLI discovery; availability independently
		// records whether that compatibility path still executes. public=false keeps
		// both available and unavailable compatibility leaves out of Agent routes.
		if record.CompatibilityVisible && record.Public {
			panic(fmt.Sprintf("semantic catalog command %q cannot be both public and compatibility-visible", command))
		}
		switch {
		case record.Availability != AvailabilityAvailable:
			record.HelpTier = HelpTierUnavailable
		case record.CompatibilityVisible || record.Disposition == DispositionAliasInternal:
			record.HelpTier = HelpTierCompatibility
		case source.FeaturedShortcuts == nil || featured[command]:
			record.HelpTier = HelpTierFeatured
		default:
			record.HelpTier = HelpTierCatalog
		}
		key := publicCatalogKey(source.Service, command)
		if _, exists := out[key]; exists {
			panic(fmt.Sprintf("duplicate shortcut semantic catalog entry %s %s", source.Service, command))
		}
		out[key] = record
	}
	for command := range featured {
		record, ok := source.Shortcuts[command]
		if !ok {
			panic(fmt.Sprintf("semantic catalog featured command %q is not registered", command))
		}
		availability := record.Availability
		if availability == "" {
			availability = source.Availability
		}
		if !record.Public || availability != AvailabilityAvailable ||
			record.Disposition == DispositionAliasInternal || record.CompatibilityVisible {
			panic(fmt.Sprintf("semantic catalog featured command %q must be public, available, and canonical", command))
		}
	}
}

func applyReviewedSemanticCatalog(s Shortcut) (Shortcut, bool) {
	record, ok := reviewedSemanticCatalog[publicCatalogKey(s.Service, s.Command)]
	if !ok {
		return s, false
	}
	s.Disposition = record.Disposition
	s.SemanticDelta = record.SemanticDelta
	s.HelpTier = record.HelpTier
	s.Availability = record.Availability
	s.PrimaryCommand = record.Primary
	s.SemanticReviewed = record.Reviewed
	s.CompatibilityVisible = record.CompatibilityVisible
	s.Hidden = !record.Public && !record.CompatibilityVisible
	return s, true
}
