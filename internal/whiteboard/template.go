// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package whiteboard

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	TemplateType = 9

	PersonalTemplateSaveTool   = "save_personal_whiteboard_tpl"
	PersonalTemplateListTool   = "list_personal_whiteboard_tpls"
	PersonalTemplateCreateTool = "apply_personal_whiteboard_tpl"
	TeamTemplateSaveTool       = "save_team_whiteboard_tpl"
	TeamTemplateListTool       = "list_team_whiteboard_tpls"
	TeamTemplateCreateTool     = "apply_team_whiteboard_tpl"
)

type TemplateScope string

const (
	TemplateScopePersonal TemplateScope = "personal"
	TemplateScopeTeam     TemplateScope = "team"
)

func ValidateTemplateRequestID(value string) error {
	return ValidateCreateRequestID(value)
}

func ValidateTemplatePageSize(value int) error {
	if value < 1 || value > 50 {
		return validation("--limit 必须在 1 到 50 之间")
	}
	return nil
}

func ValidateTemplateCursor(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return validation(fmt.Sprintf("--cursor %q 必须是非负十进制整数", value))
	}
	return nil
}

func ValidateTemplateMaxPages(value int) error {
	if value < 1 || value > 100 {
		return validation("--max-pages 必须在 1 到 100 之间")
	}
	return nil
}

// ValidateTemplatePreviewCall bounds the remote preflight exception to four
// reviewed tools and an explicit boolean true; strings and missing flags fail.
func ValidateTemplatePreviewCall(server, tool string, args map[string]any) error {
	if server != ServerID || args["dryRun"] != true {
		return fmt.Errorf("whiteboard template preview requires whiteboard server and boolean dryRun=true")
	}
	switch tool {
	case PersonalTemplateSaveTool, TeamTemplateSaveTool, PersonalTemplateCreateTool, TeamTemplateCreateTool:
		return nil
	default:
		return fmt.Errorf("tool %q does not support whiteboard template preview", tool)
	}
}
