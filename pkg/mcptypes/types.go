package mcptypes

import "encoding/json"

type ServerDescriptor struct {
	Key         string
	DisplayName string
	Description string
	Endpoint    string
	Source      string
	CLI         CLIOverlay
	HasCLIMeta  bool
	AuthHeaders map[string]string
}

type CLIOverlay struct {
	ID            string                     `json:"id"`
	Command       string                     `json:"command"`
	Aliases       []string                   `json:"aliases"`
	Prefixes      []string                   `json:"prefixes"`
	Skip          bool                       `json:"skip"`
	Tools         []CLITool                  `json:"tools"`
	ToolOverrides map[string]CLIToolOverride `json:"toolOverrides,omitempty"`
}

type CLITool struct {
	Name string `json:"name"`
}

type CLIToolOverride struct {
	ServerOverride string `json:"serverOverride,omitempty"`
}

func OverlayFromJSON(data json.RawMessage) CLIOverlay {
	var overlay CLIOverlay
	if len(data) > 0 {
		_ = json.Unmarshal(data, &overlay)
	}
	return overlay
}
