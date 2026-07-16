// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"reflect"
	"testing"
)

func TestToolSchemaHintCannotOwnCommandNavigation(t *testing.T) {
	typeOfHint := reflect.TypeOf(ToolSchemaHint{})
	for _, field := range []string{"CanonicalPath", "PrimaryCLIPath", "Aliases", "Visibility"} {
		if _, exists := typeOfHint.FieldByName(field); exists {
			t.Fatalf("ToolSchemaHint.%s reintroduces a navigation source outside CommandRegistry", field)
		}
	}
}
