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

package helpers

import "strings"

// CmdClass describes whether a dws command is read-only or mutating. It is the
// signal a connector confirmation gate consumes to decide whether a robot may
// run a command directly (read-only) or must first ask the principal to
// approve it (write / mutating).
//
// SAFETY CONTRACT: callers MUST treat CmdClassUnknown conservatively, i.e. as
// if it were CmdClassWrite (require confirmation). The classifier deliberately
// returns Unknown rather than silently coercing it to Write so that the
// confirmation gate keeps full information and can, for example, log/telemeter
// "unclassified" commands separately. Never auto-allow an Unknown command.
type CmdClass int

const (
	// CmdClassUnknown means the leaf verb was not recognised by the
	// heuristics or any override. Callers must default to requiring
	// confirmation (treat as write) for safety.
	CmdClassUnknown CmdClass = iota
	// CmdClassRead is a read-only / non-mutating command that may be run
	// without principal confirmation.
	CmdClassRead
	// CmdClassWrite is a mutating / state-changing command that must be
	// confirmed by the principal before it runs.
	CmdClassWrite
)

// String renders the class as a stable lowercase token, handy for logs and
// telemetry.
func (c CmdClass) String() string {
	switch c {
	case CmdClassRead:
		return "read"
	case CmdClassWrite:
		return "write"
	default:
		return "unknown"
	}
}

// readVerbs and writeVerbs are calibrated against the verbs that real dws
// cobra commands expose (scanned from `Use:` definitions across the repo, e.g.
// todo task create/list/get/update/delete/done, attendance get, approval
// submit, chat send / recall, drive upload / download / mkdir / chmod, doc
// export, contact search, etc). They are NOT invented: every entry below has
// at least one real command using it (or a "<verb>-by-..." / "batch-<verb>"
// compound that normalises to it).
var (
	readVerbs = map[string]struct{}{
		"list":     {},
		"get":      {},
		"search":   {},
		"read":     {},
		"view":     {},
		"query":    {},
		"show":     {},
		"detail":   {},
		"details":  {},
		"status":   {},
		"download": {},
		"export":   {},
		"fetch":    {},
		"info":     {},
		"find":     {},
		"stats":    {},
		"stat":     {},
		"summary":  {},
		"check":    {},
		"inspect":  {},
		"doctor":   {},
		"version":  {},
		"whoami":   {},
		"ls":       {},
		"cat":      {},
		"describe": {},
		"diff":     {},
		"preview":  {},
	}

	writeVerbs = map[string]struct{}{
		"create":     {},
		"update":     {},
		"delete":     {},
		"submit":     {},
		"send":       {},
		"done":       {},
		"cancel":     {},
		"offline":    {},
		"online":     {},
		"enable":     {},
		"disable":    {},
		"remove":     {},
		"add":        {},
		"set":        {},
		"unset":      {},
		"approve":    {},
		"reject":     {},
		"write":      {},
		"upload":     {},
		"move":       {},
		"mv":         {},
		"copy":       {},
		"cp":         {},
		"rename":     {},
		"reply":      {},
		"recall":     {},
		"publish":    {},
		"insert":     {},
		"import":     {},
		"install":    {},
		"uninstall":  {},
		"mkdir":      {},
		"share":      {},
		"unshare":    {},
		"commit":     {},
		"reset":      {},
		"stop":       {},
		"start":      {},
		"restart":    {},
		"hide":       {},
		"unhide":     {},
		"finalize":   {},
		"execute":    {},
		"run":        {},
		"exec":       {},
		"chmod":      {},
		"rm":         {},
		"clean":      {},
		"clear":      {},
		"refresh":    {},
		"recover":    {},
		"recovery":   {},
		"login":      {},
		"logout":     {},
		"register":   {},
		"connect":    {},
		"disconnect": {},
		"upgrade":    {},
		"setup":      {},
		"generate":   {},
		"batch":      {},
		"apply":      {},
		"patch":      {},
		"put":        {},
		"post":       {},
		"sync":       {},
		"link":       {},
		"unlink":     {},
		"grant":      {},
		"revoke":     {},
		"assign":     {},
		"close":      {},
		"open":       {},
		"archive":    {},
		"restore":    {},
		"finish":     {},
	}
)

// defaultClassOverrides is the package-level override table. It is intentionally
// empty by default: heuristics are expected to cover the common case, and the
// override table exists only to correct individual misclassifications without
// touching the heuristic verb tables. Mutate it via SetCmdClassOverride.
//
// Keys may be either a single verb ("download") or a full space-joined command
// path ("doc export"); both are matched case-insensitively. A full-path key
// wins over a single-verb key when both are present.
var defaultClassOverrides = map[string]CmdClass{}

// SetCmdClassOverride registers a process-wide classification override for the
// given key. The key may be a single verb or a space-joined command path; it is
// normalised to lowercase. Passing CmdClassUnknown removes the override.
func SetCmdClassOverride(key string, class CmdClass) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	if class == CmdClassUnknown {
		delete(defaultClassOverrides, key)
		return
	}
	defaultClassOverrides[key] = class
}

// ClassifyDwsCommand classifies a dws command given its path segments (e.g.
// "todo", "task", "create"). It consults the package-level override table and
// then the read/write verb heuristics, scanning segments right-to-left so the
// leaf action verb dominates a container/noun segment.
//
// Remember the SAFETY CONTRACT on CmdClass: a CmdClassUnknown result MUST be
// treated as write (require confirmation) by the caller.
func ClassifyDwsCommand(parts ...string) CmdClass {
	return ClassifyDwsCommandWith(defaultClassOverrides, parts...)
}

// ClassifyDwsCommandWith is like ClassifyDwsCommand but lets the caller supply
// an explicit override table (e.g. a per-tenant or per-request map) instead of
// the package-level one. A nil overrides map is allowed and means "no
// overrides". Override lookups always win over the heuristics.
func ClassifyDwsCommandWith(overrides map[string]CmdClass, parts ...string) CmdClass {
	norm := normalizeParts(parts)
	if len(norm) == 0 {
		return CmdClassUnknown
	}

	// 1. Full-path override (most specific) wins.
	if overrides != nil {
		fullKey := strings.Join(norm, " ")
		if c, ok := overrides[fullKey]; ok {
			return c
		}
	}

	// 2. Walk segments right-to-left: the leaf action verb is the most
	//    meaningful, but a leaf may be a noun/placeholder (e.g. an id) that
	//    we do not recognise, so fall back toward the root.
	for i := len(norm) - 1; i >= 0; i-- {
		seg := norm[i]
		if overrides != nil {
			if c, ok := overrides[seg]; ok {
				return c
			}
		}
		if c, ok := classifyVerb(seg); ok {
			return c
		}
	}

	return CmdClassUnknown
}

// classifyVerb classifies a single segment, handling compound verbs such as
// "send-by-bot", "list-forms", "batch-update" or "create_inline" by also
// trying the token before the first separator. Returns (class, true) when the
// segment maps to a known verb.
func classifyVerb(seg string) (CmdClass, bool) {
	candidates := verbCandidates(seg)
	for _, v := range candidates {
		if _, ok := readVerbs[v]; ok {
			return CmdClassRead, true
		}
		if _, ok := writeVerbs[v]; ok {
			return CmdClassWrite, true
		}
	}
	return CmdClassUnknown, false
}

// verbCandidates expands a segment into the tokens worth checking against the
// verb tables, in priority order: the whole segment first, then the leading
// token before the first '-' or '_' separator (so "batch-update" tries
// "update"-as-prefix... no: it tries "batch" first which is itself a write
// verb; and "send-by-bot" tries "send"). This handles both "<verb>-<qualifier>"
// and "<verb>_<qualifier>" compounds where the action sits at the front.
func verbCandidates(seg string) []string {
	out := []string{seg}
	if i := strings.IndexAny(seg, "-_"); i > 0 {
		out = append(out, seg[:i])
	}
	return out
}

// normalizeParts lowercases, trims and drops empty/whitespace segments so that
// callers can pass raw argv-style slices without pre-cleaning them.
func normalizeParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
