// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package shortcut

var publicShortcutCatalog = generatedPublicShortcutCatalog()

func publicCatalogKey(service, command string) string {
	return service + "\x00" + command
}

func applyPublicCatalog(s Shortcut) Shortcut {
	if len(publicShortcutCatalog) == 0 {
		return s
	}
	if _, ok := publicShortcutCatalog[publicCatalogKey(s.Service, s.Command)]; !ok {
		s.Hidden = true
	}
	return s
}

// InPublicCatalog reports whether a shortcut belongs to the generated public
// shortcut surface used by help, listing, and skill generation.
func InPublicCatalog(service, command string) bool {
	_, ok := publicShortcutCatalog[publicCatalogKey(service, command)]
	return ok
}
