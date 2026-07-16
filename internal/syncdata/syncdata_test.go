package syncdata

import "testing"

func TestCrossPlatformCoverageGeneratedSyncDataIsSelfConsistent(t *testing.T) {
	servers := StaticServers()
	routes := CmdToProduct()
	if len(servers) == 0 || len(routes) == 0 {
		t.Fatal("generated sync data is empty")
	}
	serverIDs := make(map[string]bool, len(servers))
	for _, server := range servers {
		if server.ID == "" || server.Name == "" || server.Endpoint == "" {
			t.Errorf("incomplete server: %#v", server)
		}
		if serverIDs[server.ID] {
			t.Errorf("duplicate server ID %q", server.ID)
		}
		serverIDs[server.ID] = true
	}
	for command, product := range routes {
		if command == "" || product == "" {
			t.Errorf("invalid route %q -> %q", command, product)
		}
		if !serverIDs[product] {
			t.Errorf("route %q targets unknown product %q", command, product)
		}
	}
}
