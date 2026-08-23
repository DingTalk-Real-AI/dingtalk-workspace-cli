// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageIndexCommandSpecsValidationEdges(t *testing.T) {
	cases := []struct {
		name string
		spec CommandSpec
		want string
	}{
		{name: "invalid canonical", spec: CommandSpec{CanonicalPath: "bad", PrimaryCLIPath: "bad path"}, want: "invalid canonical path"},
		{name: "invalid source product", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", SourceProductID: "Bad Product!"}, want: "invalid source_product_id"},
		{name: "invalid primary", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: ""}, want: "invalid primary cli path"},
		{name: "invalid alias", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", Aliases: []string{""}}, want: "invalid alias path"},
		{name: "alias duplicates primary", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", Aliases: []string{"doc create"}}, want: "duplicates its primary path"},
		{name: "duplicate alias", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", Aliases: []string{"doc old", "doc old"}}, want: "duplicate alias path"},
		{name: "invalid visibility", spec: CommandSpec{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", Visibility: "secret"}, want: "invalid visibility"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newEffectiveCommandRegistry([]CommandSpec{test.spec}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	if _, err := newEffectiveCommandRegistry([]CommandSpec{
		{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create"},
		{CanonicalPath: "doc.create", PrimaryCLIPath: "doc make"},
	}); err == nil || !strings.Contains(err.Error(), "duplicate command identity canonical path") {
		t.Fatalf("duplicate canonical error = %v", err)
	}
	if _, err := newEffectiveCommandRegistry([]CommandSpec{
		{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create"},
		{CanonicalPath: "doc.copy", PrimaryCLIPath: "doc create"},
	}); err == nil || !strings.Contains(err.Error(), "belongs to both") {
		t.Fatalf("path ownership error = %v", err)
	}
	if _, err := newEffectiveCommandRegistry([]CommandSpec{
		{CanonicalPath: "doc.create", PrimaryCLIPath: "doc create", Aliases: []string{"doc.copy"}},
		{CanonicalPath: "doc.copy", PrimaryCLIPath: "doc copy"},
	}); err == nil || !strings.Contains(err.Error(), "conflicts with canonical identity") {
		t.Fatalf("alias/canonical conflict error = %v", err)
	}

	reg, err := newEffectiveCommandRegistry([]CommandSpec{{
		CanonicalPath: "doc.create", PrimaryCLIPath: "doc create",
	}})
	if err != nil {
		t.Fatalf("valid registry error = %v", err)
	}
	if got := reg.Commands[0].Source; got != CommandSourceContractIdentity {
		t.Fatalf("default source = %q", got)
	}
	if got := reg.Commands[0].Visibility; got != SchemaVisibilityPublic {
		t.Fatalf("default visibility = %q", got)
	}

	if _, err := BuildEffectiveCommandRegistry(nil); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("BuildEffectiveCommandRegistry(nil) error = %v", err)
	}
	if _, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"}); err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry(root) error = %v", err)
	}
}
