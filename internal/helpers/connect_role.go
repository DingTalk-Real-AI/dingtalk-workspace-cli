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

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Role configuration is a standalone building block for the "digital employee"
// evolution of the DingTalk connector: one role == one dedicated bot
// (clientId-isolated). Each role carries its own knowledge scope, capability
// boundary and persona so a role answers within its lane (the HR assistant must
// not touch code repositories). This file only defines the schema, loader and
// validation — it deliberately wires into no runtime path so it can be reused
// freely by the later runtime-integration wave.

// ConfirmPolicy controls how a role asks its owner before taking an action.
type ConfirmPolicy string

const (
	// ConfirmManual asks the owner to confirm every action.
	ConfirmManual ConfirmPolicy = "manual"
	// ConfirmAuto lets the bot judge for itself whether confirmation is needed.
	ConfirmAuto ConfirmPolicy = "auto"
	// ConfirmRemember reuses the owner's previous choice for the same kind of
	// operation.
	ConfirmRemember ConfirmPolicy = "remember"
)

// defaultConfirmPolicy is the safest default when confirm_policy is omitted:
// confirm everything, so an unconfigured role never acts silently.
const defaultConfirmPolicy = ConfirmManual

// valid reports whether p is one of the recognized confirm policies.
func (p ConfirmPolicy) valid() bool {
	switch p {
	case ConfirmManual, ConfirmAuto, ConfirmRemember:
		return true
	default:
		return false
	}
}

// RoleConfig is the on-disk (YAML) definition of one digital-employee role.
// It maps 1:1 to a single bot via ClientID.
type RoleConfig struct {
	// Name is the human-facing role name, e.g. "人事助理".
	Name string `yaml:"name"`
	// ClientID is the DingTalk bot clientId this role is bound to. One role owns
	// exactly one bot, so this is also the unique key across a role set.
	ClientID string `yaml:"client_id"`
	// Persona is a professional prompt fragment merged into the agent's system
	// prompt to shape tone and expertise.
	Persona string `yaml:"persona"`
	// KnowledgeSources lists knowledge sources using the existing
	// --knowledge-source syntax: a bare path is a local directory, "wiki:<spaceId>"
	// a whole knowledge space, "doc:<docId>" a single document.
	KnowledgeSources []string `yaml:"knowledge_sources"`
	// AllowedScopes names the dws capabilities/products this role may use, e.g.
	// ["todo", "approval", "attendance"]. Consumed by later permission checks.
	AllowedScopes []string `yaml:"allowed_scopes"`
	// OwnerUserID is the userId of the role's owner; confirmation requests go here.
	OwnerUserID string `yaml:"owner_user_id"`
	// ConfirmPolicy selects the confirmation strategy; empty defaults to manual.
	ConfirmPolicy ConfirmPolicy `yaml:"confirm_policy"`
	// Extra is an open-ended bag for forward-compatible keys, so the schema can
	// grow without a breaking change. Intentionally minimal — not a config DSL.
	Extra map[string]string `yaml:"extra"`
}

// RoleConfigExample is a complete, copy-pasteable role definition users can
// follow when authoring their own. It is also parsed in tests to keep the
// example honest against the schema.
const RoleConfigExample = `# Role: HR assistant (人事助理)
# One role == one dedicated bot (its own clientId).
name: 人事助理
client_id: dingxxxxxxxxxxxxxxxx
owner_user_id: "012345678901234567"
# Confirmation strategy: manual | auto | remember
confirm_policy: manual
persona: |
  You are the company HR assistant. Answer questions about leave, attendance,
  benefits and onboarding in a warm, precise tone. Never touch code
  repositories or engineering systems — that is out of your lane.
# Knowledge sources reuse the --knowledge-source syntax:
#   - a bare path is a local directory
#   - wiki:<spaceId> is a whole DingTalk knowledge space
#   - doc:<docId> is a single document node
knowledge_sources:
  - ./knowledge/hr
  - wiki:1234567890
# Capabilities this role may exercise (consumed by later permission checks).
allowed_scopes:
  - attendance
  - approval
  - todo
`

// LoadRoleConfig reads, parses and validates a single role YAML file.
func LoadRoleConfig(path string) (*RoleConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read role config %q: %w", path, err)
	}
	var cfg RoleConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse role config %q: %w", path, err)
	}
	if err := cfg.normalizeAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid role config %q: %w", path, err)
	}
	return &cfg, nil
}

// normalizeAndValidate trims required fields, applies defaults and reports the
// first violation with a clear, actionable message. It never panics.
func (c *RoleConfig) normalizeAndValidate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.OwnerUserID = strings.TrimSpace(c.OwnerUserID)

	if c.Name == "" {
		return fmt.Errorf("field %q is required", "name")
	}
	if c.ClientID == "" {
		return fmt.Errorf("field %q is required", "client_id")
	}
	if c.OwnerUserID == "" {
		return fmt.Errorf("field %q is required", "owner_user_id")
	}

	if c.ConfirmPolicy == "" {
		c.ConfirmPolicy = defaultConfirmPolicy
	}
	if !c.ConfirmPolicy.valid() {
		return fmt.Errorf("field %q must be one of [%s, %s, %s], got %q",
			"confirm_policy", ConfirmManual, ConfirmAuto, ConfirmRemember, c.ConfirmPolicy)
	}

	for i, src := range c.KnowledgeSources {
		trimmed := strings.TrimSpace(src)
		if trimmed == "" {
			return fmt.Errorf("knowledge_sources[%d] is empty", i)
		}
		// Reuse the connector's own source grammar so role config stays in lockstep
		// with --knowledge-source (bare path / wiki:<spaceId> / doc:<docId>).
		if _, err := parseKnowledgeSource(trimmed); err != nil {
			return fmt.Errorf("knowledge_sources[%d]: %w", i, err)
		}
		c.KnowledgeSources[i] = trimmed
	}
	return nil
}

// applyRoleConfig folds a loaded role into the connector options, filling only
// the fields the operator did not set explicitly — an explicit flag/env ALWAYS
// wins over the role profile, so a role config is a reusable baseline that ad-hoc
// flags can still override. It is a pure function (no I/O) so it is trivially
// testable; the caller validates client_id match and reports the merge.
//
// Mapped fields:
//   - OwnerUserID   ← role.OwnerUserID   (only when opts has no owner yet)
//   - Persona       ← role.Persona       (only when opts has no persona yet)
//   - KnowledgeSources ← role.KnowledgeSources (appended; the role's sources are
//     additive to any --knowledge-source/--knowledge-dir already configured)
//
// AllowedScopes and ConfirmPolicy are intentionally NOT mapped here: scope
// enforcement and the auto/remember confirm strategies are a later slice, and we
// do not want to carry options the runtime does not yet honor. The caller logs
// them so they are visible.
func applyRoleConfig(opts connectAgentOptions, role *RoleConfig) connectAgentOptions {
	if role == nil {
		return opts
	}
	if opts.OwnerUserID == "" {
		opts.OwnerUserID = role.OwnerUserID
	}
	if opts.Persona == "" {
		opts.Persona = strings.TrimSpace(role.Persona)
	}
	if len(role.KnowledgeSources) > 0 {
		opts.KnowledgeSources = append(opts.KnowledgeSources, role.KnowledgeSources...)
	}
	return opts
}

// LoadRoleConfigs loads every *.yaml / *.yml file directly under dir and indexes
// the roles by ClientID — the lookup a multi-bot, one-role-per-bot deployment
// needs. A duplicate ClientID is an error: two roles must not share one bot.
func LoadRoleConfigs(dir string) (map[string]*RoleConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read role config dir %q: %w", dir, err)
	}
	// Sort for deterministic load order and stable error reporting.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	roles := make(map[string]*RoleConfig, len(names))
	for _, name := range names {
		cfg, err := LoadRoleConfig(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if existing, dup := roles[cfg.ClientID]; dup {
			return nil, fmt.Errorf("duplicate client_id %q: roles %q and %q both bind the same bot",
				cfg.ClientID, existing.Name, cfg.Name)
		}
		roles[cfg.ClientID] = cfg
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("no role config (*.yaml/*.yml) found in %q", dir)
	}
	return roles, nil
}
