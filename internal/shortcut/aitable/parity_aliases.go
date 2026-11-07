// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"sort"
)

func withAITableParityAliases(s shortcut.Shortcut) shortcut.Shortcut {
	commandAliases := map[string][]string{
		"+section-list-nodes": {"+base-block-list"},
		"+section-move-node":  {"+base-block-move"},
		"+table-get":          {"+table-list"},
		"+field-get":          {"+field-list"},
		"+view-get":           {"+view-list"},
		"+base-search":        {"+title-resolve"},
		"+table-bootstrap":    {"+table-create"},
		"+record-query":       {"+record-list", "+record-search"},
		"+record-update":      {"+record-batch-update"},
		"+record-share-url":   {"+record-share-link-create"},
		"+attachment-put":     {"+record-upload-attachment"},
		"+attachment-remove":  {"+record-remove-attachment"},
		"+view-update":        {"+view-rename"},
		"+form-field-list":    {"+form-questions-list"},
	}
	for _, alias := range commandAliases[s.Command] {
		s.Aliases = append(append([]string(nil), s.Aliases...), alias)
		s.Contract.Identity.Aliases = append(append([]string(nil), s.Contract.Identity.Aliases...), "aitable "+alias)
	}

	// base-token is already owned by reviewed preparse concepts. Adding a
	// native flag would deactivate the existing delete/disable alias contract.
	aliases := map[string]string{}
	s.Flags = append([]shortcut.Flag(nil), s.Flags...)
	for i, f := range s.Flags {
		name := map[string]string{"query": "keyword", "limit": "page-size", "cursor": "page-token"}[f.Name]
		if name == "" {
			continue
		}
		exists := false
		for _, other := range s.Flags {
			if other.Name == name {
				exists = true
			}
			for _, a := range other.Aliases {
				if a == name {
					exists = true
				}
			}
		}
		if exists {
			continue
		}
		s.Flags[i].Aliases = append(append([]string(nil), f.Aliases...), name)
		s.Flags[i].AliasesVisible = false
		s.Flags[i].Desc += "；兼容别名与主参数同时提供时值必须一致"
		aliases[name] = f.Name
	}
	if len(aliases) == 0 {
		return s
	}
	var aliasFlags []string
	for _, flag := range s.Flags {
		for _, primary := range aliases {
			if flag.Name == primary {
				aliasFlags = append(aliasFlags, primary)
				break
			}
		}
	}
	s.Constraints = append(append([]shortcut.Constraint(nil), s.Constraints...), shortcut.Constraint{Kind: shortcut.ConstraintCustom, Flags: aliasFlags, Description: "兼容别名与主参数同时提供时值必须一致"})
	aliasNames := make([]string, 0, len(aliases))
	for alias := range aliases {
		aliasNames = append(aliasNames, alias)
	}
	sort.Strings(aliasNames)
	previous := s.Validate
	s.Validate = func(rt *shortcut.RuntimeContext) error {
		for _, alias := range aliasNames {
			primary := aliases[alias]
			if !rt.Changed(alias) {
				continue
			}
			af := rt.Command().Flags().Lookup(alias)
			pf := rt.Command().Flags().Lookup(primary)
			if rt.Changed(primary) && pf.Value.String() != af.Value.String() {
				return apperrors.NewValidation(fmt.Sprintf("--%s 与 --%s 冲突", primary, alias))
			}
			if err := rt.Command().Flags().Set(primary, af.Value.String()); err != nil {
				return err
			}
		}
		if previous != nil {
			return previous(rt)
		}
		return nil
	}
	return s
}
