package skillpaths

import "testing"

func TestCrossPlatformCoverageAgentHomes(t *testing.T) {
	homes := AgentHomes()
	if len(homes) == 0 {
		t.Fatal("AgentHomes empty")
	}
	homes[0] = "mutated"
	if AgentHomes()[0] == "mutated" {
		t.Fatal("AgentHomes shares backing array")
	}
}
