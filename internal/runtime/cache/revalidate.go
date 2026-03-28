package cache

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"

// ChangedServerKeysByUpdatedAt returns the set of live server keys that should
// be refreshed because they are new or their existing registry updatedAt value
// changed. This intentionally uses only existing market registry metadata.
func ChangedServerKeysByUpdatedAt(cached, live []market.ServerDescriptor) map[string]bool {
	cachedByKey := make(map[string]market.ServerDescriptor, len(cached))
	for _, server := range cached {
		cachedByKey[server.Key] = server
	}

	changed := make(map[string]bool)
	for _, server := range live {
		previous, ok := cachedByKey[server.Key]
		if !ok {
			changed[server.Key] = true
			continue
		}
		if !server.UpdatedAt.Equal(previous.UpdatedAt) {
			changed[server.Key] = true
		}
	}
	return changed
}
