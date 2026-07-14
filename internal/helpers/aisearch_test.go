// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import "testing"

func TestAisearchPersonFlagsDoNotLeakIntoContentCommands(t *testing.T) {
	root := newAisearchCommand()
	person, _, err := root.Find([]string{"person"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"keyword", "dimension"} {
		flag := person.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Fatalf("person --%s = %#v, want visible local flag", name, flag)
		}
	}

	for _, path := range []string{"enterprise", "behavior"} {
		cmd, _, err := root.Find([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		if flag := cmd.Flags().Lookup("dimension"); flag != nil {
			t.Fatalf("%s unexpectedly accepts person-only --dimension", path)
		}
		for _, name := range []string{"keyword", "query"} {
			flag := cmd.Flags().Lookup(name)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%s --%s = %#v, want hidden compatibility flag", path, name, flag)
			}
		}
	}
}
