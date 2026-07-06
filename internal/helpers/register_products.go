package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// wukongHandler adapts wukong-style command constructors (which take no
// runner parameter and use callMCPTool via package-level deps) to the
// open-source Handler interface used by RegisterPublic / NewPublicCommands.
type wukongHandler struct {
	name    string
	buildFn func() *cobra.Command
}

func (h wukongHandler) Name() string { return h.name }
func (h wukongHandler) Command(_ executor.Runner) *cobra.Command {
	return h.buildFn()
}

func init() {
	products := []struct {
		name string
		fn   func() *cobra.Command
	}{
		{"aitable", newAitableCommand},
		{"calendar", newCalendarCommand},
		{"contact", newContactCommand},
		{"todo", newTodoCommand},
		{"doc", newDocCommand},
		{"chat", newChatCommand},
		{"oa", newOaCommand},
		{"mail", newMailCommand},
		{"ding", newDingCommand},
		{"devdoc", newDevdocCommand},
		{"attendance", newAttendanceCommand},
		{"conference", newConferenceCommand},
		{"live", newLiveCommand},
		{"aiapp", newAiappCommand},
		{"minutes", newMinutesCommand},
		{"finance", newFinanceCommand},
		{"report", newReportCommand},
		{"drive", newDriveCommand},
		{"blackboard", newBlackboardCommand},
		{"credit", newCreditCommand},
		{"docparse", newDocparseCommand},
		{"aidesign", newAidesignCommand},
		{"sheet", newSheetCommand},
		{"wiki", newWikiCommand},
		{"aisearch", newAisearchCommand},
		{"yida", newYidaCommand},
		{"agoal", newAgoalCommand},
	}
	for _, p := range products {
		p := p
		RegisterPublic(func() Handler {
			return wukongHandler{name: p.name, buildFn: p.fn}
		})
	}
}
