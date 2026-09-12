// Package localename owns the supported display-locale selection rule.
package localename

import "strings"

func Resolve(raw string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "zh") {
		return "zh"
	}
	return "en"
}
