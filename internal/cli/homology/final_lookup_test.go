package homology

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestContractFinalFoundForHrbrain(t *testing.T) {
	root := app.NewSchemaSourceRootCommand()
	hrbrain, _, err := root.Find([]string{"hrbrain", "search", "employees"})
	if err != nil || hrbrain == nil {
		t.Fatalf("hrbrain search employees not found: %v", err)
	}
	final, ok := contractfinal.RuntimeContractFinal(hrbrain)
	if !ok {
		t.Fatal("ContractFinal NOT found for hrbrain search employees — this is the root cause of contract.ParamDecl not working")
	}
	t.Logf("ContractFinal found: title=%s params=%d", final.Title, len(final.Parameters))
}

func TestContractFinalFoundForAisearch(t *testing.T) {
	root := app.NewSchemaSourceRootCommand()
	enterprise, _, err := root.Find([]string{"aisearch", "enterprise"})
	if err != nil || enterprise == nil {
		t.Fatalf("aisearch enterprise not found: %v", err)
	}
	final, ok := contractfinal.RuntimeContractFinal(enterprise)
	if !ok {
		t.Fatal("ContractFinal NOT found for aisearch enterprise")
	}
	t.Logf("ContractFinal found: title=%s params=%d", final.Title, len(final.Parameters))
}
