package helpers

import (
	"fmt"
	"strings"
)

// ============================================================================
// range update 富格式 cell 字段级校验
// 与 MCP update_range.complexValues schema 对齐：
//   cell.type 仅枚举 ["text", "richText"]
//   richText.texts[].type 枚举 ["text", "link", "attachment", "image"]
// 服务端会再做一遍校验，CLI 提前拦截是为了给更友好的本地错误信息。
// ============================================================================

var complexValueStyleFields = map[string]string{
	"bold":      "bool",
	"italic":    "bool",
	"underline": "bool",
	"strike":    "bool",
	"color":     "string",
	"size":      "number",
}

func validateComplexValueStyle(style map[string]any, path string) error {
	for k, v := range style {
		kind, ok := complexValueStyleFields[k]
		if !ok {
			return fmt.Errorf("%s 含未知字段 %q（合法字段: bold/italic/underline/strike/color/size）", path, k)
		}
		switch kind {
		case "bool":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s.%s 必须为 boolean", path, k)
			}
		case "string":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s.%s 必须为字符串（如 \"#FF0000\"）", path, k)
			}
		case "number":
			if _, ok := v.(float64); !ok {
				return fmt.Errorf("%s.%s 必须为数字", path, k)
			}
		}
	}
	return nil
}

func validateRichTextItem(item map[string]any, path string) error {
	typeRaw, ok := item["type"]
	if !ok {
		return fmt.Errorf("%s: 缺少 type 字段（合法值: text/link/attachment/image）", path)
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return fmt.Errorf("%s.type 必须为字符串", path)
	}
	switch typeVal {
	case "text":
		if _, ok := item["text"].(string); !ok {
			return fmt.Errorf("%s: type=text 必须包含 text 字符串字段", path)
		}
	case "link":
		if _, ok := item["text"].(string); !ok {
			return fmt.Errorf("%s: type=link 必须包含 text 字符串字段（显示文字）", path)
		}
		if s, ok := item["link"].(string); !ok || s == "" {
			return fmt.Errorf("%s: type=link 必须包含非空 link 字符串字段（超链接 URL）", path)
		}
	case "attachment":
		if _, ok := item["text"].(string); !ok {
			return fmt.Errorf("%s: type=attachment 必须包含 text 字符串字段（显示文件名）", path)
		}
		if s, ok := item["resourceId"].(string); !ok || s == "" {
			return fmt.Errorf("%s: type=attachment 必须包含非空 resourceId 字符串字段（通过 dws sheet media-upload 获取）", path)
		}
		if s, ok := item["mimeType"].(string); !ok || s == "" {
			return fmt.Errorf("%s: type=attachment 必须包含非空 mimeType 字符串字段", path)
		}
	case "image":
		if s, ok := item["resourceId"].(string); !ok || s == "" {
			return fmt.Errorf("%s: type=image 必须包含非空 resourceId 字符串字段（通过 dws sheet media-upload 获取）", path)
		}
		if s, ok := item["resourceUrl"].(string); !ok || s == "" {
			return fmt.Errorf("%s: type=image 必须包含非空 resourceUrl 字符串字段", path)
		}
	default:
		return fmt.Errorf("%s.type 非法值 %q（合法值: text/link/attachment/image）", path, typeVal)
	}
	if styleRaw, exists := item["style"]; exists {
		if typeVal != "text" && typeVal != "link" {
			return fmt.Errorf("%s: style 字段仅 type=text / link 子项支持", path)
		}
		style, ok := styleRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.style 必须为 object", path)
		}
		if err := validateComplexValueStyle(style, path+".style"); err != nil {
			return err
		}
	}
	return nil
}

func validateComplexValueCell(cell map[string]any, path string) error {
	// dataValidation 可以和 type 共存，也可以单独出现（如纯 checkbox）
	if dvRaw, exists := cell["dataValidation"]; exists {
		if err := validateDataValidation(dvRaw, path+".dataValidation"); err != nil {
			return err
		}
	}

	if hyperlinkRaw, exists := cell["hyperlink"]; exists {
		if err := validateCellHyperlink(hyperlinkRaw, path+".hyperlink"); err != nil {
			return err
		}
	}

	typeRaw, hasType := cell["type"]
	// 没有 type 时，必须有 metadata 字段（不写值，只更新元数据）
	if !hasType {
		_, hasDV := cell["dataValidation"]
		_, hasCS := cell["cellStyles"]
		_, hasHyperlink := cell["hyperlink"]
		if hasDV || hasCS || hasHyperlink {
			return nil
		}
		return fmt.Errorf("%s: 缺少 type 字段（合法值: text/richText）", path)
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return fmt.Errorf("%s.type 必须为字符串", path)
	}
	switch typeVal {
	case "text":
		var cellText string
		hasCellText := false
		if t, exists := cell["text"]; exists {
			textValue, ok := t.(string)
			if !ok {
				return fmt.Errorf("%s.text 必须为字符串（text=\"\" 表示清空 cell）", path)
			}
			cellText = textValue
			hasCellText = true
		}
		if styleRaw, exists := cell["style"]; exists {
			style, ok := styleRaw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.style 必须为 object", path)
			}
			if err := validateComplexValueStyle(style, path+".style"); err != nil {
				return err
			}
		}
		if hyperlinkRaw, exists := cell["hyperlink"]; exists && hyperlinkRaw != nil {
			hyperlink, _ := hyperlinkRaw.(map[string]any)
			if hyperlinkText, ok := hyperlink["text"].(string); ok && hasCellText && hyperlinkText != cellText {
				return fmt.Errorf("%s.hyperlink.text 与 %s.text 不一致，请只传 cell.text 或保持两者相同", path, path)
			}
		}
	case "richText":
		if _, hasHyperlink := cell["hyperlink"]; hasHyperlink {
			return fmt.Errorf("%s: cell-level hyperlink 不能与 type=richText 同时使用；整格链接用 hyperlink，片段链接用 richText.texts[].type=link", path)
		}
		textsRaw, ok := cell["texts"]
		if !ok {
			return fmt.Errorf("%s: type=richText 必须包含 texts 数组", path)
		}
		texts, ok := textsRaw.([]any)
		if !ok {
			return fmt.Errorf("%s.texts 必须为数组", path)
		}
		if len(texts) == 0 {
			return fmt.Errorf("%s.texts 不能为空数组", path)
		}
		for i, item := range texts {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.texts[%d] 必须为 object", path, i)
			}
			if err := validateRichTextItem(itemMap, fmt.Sprintf("%s.texts[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s.type 非法值 %q（合法值: text / richText；不再支持 number/boolean/null，数字布尔请用 {type:text,text:\"...\"} 字符串形式）", path, typeVal)
	}
	return nil
}

func validateCellHyperlink(raw any, path string) error {
	if raw == nil {
		return nil
	}
	hyperlink, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s 必须为 object 或 null", path)
	}
	typeRaw, ok := hyperlink["type"]
	if !ok {
		return fmt.Errorf("%s: 缺少 type 字段（合法值: path / sheet / range / none）", path)
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return fmt.Errorf("%s.type 必须为字符串", path)
	}
	switch typeVal {
	case "path", "sheet", "range":
		// 写新 hyperlink：link 必填
		linkRaw, ok := hyperlink["link"]
		if !ok {
			return fmt.Errorf("%s: 缺少 link 字段", path)
		}
		link, ok := linkRaw.(string)
		if !ok || strings.TrimSpace(link) == "" {
			return fmt.Errorf("%s.link 必须为非空字符串", path)
		}
		if textRaw, exists := hyperlink["text"]; exists {
			if _, ok := textRaw.(string); !ok {
				return fmt.Errorf("%s.text 必须为字符串", path)
			}
		}
	case "none":
		// type=none 表示显式清除单元格级超链接，不需其他字段
	default:
		return fmt.Errorf("%s.type 非法值 %q（合法值: path / sheet / range / none）", path, typeVal)
	}
	return nil
}

func validateDataValidation(dvRaw any, path string) error {
	dv, ok := dvRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s 必须为 object", path)
	}
	dvType, ok := dv["type"].(string)
	if !ok || dvType == "" {
		return fmt.Errorf("%s.type 必须为非空字符串（合法值: dropdown / checkbox / none）", path)
	}
	switch dvType {
	case "dropdown":
		optionsRaw, ok := dv["options"]
		if !ok {
			return fmt.Errorf("%s: type=dropdown 必须包含 options 数组", path)
		}
		options, ok := optionsRaw.([]any)
		if !ok || len(options) == 0 {
			return fmt.Errorf("%s.options 必须为非空数组", path)
		}
		for i, opt := range options {
			optMap, ok := opt.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.options[%d] 必须为 object（如 {\"value\":\"选项\"}）", path, i)
			}
			val, ok := optMap["value"].(string)
			if !ok || val == "" {
				return fmt.Errorf("%s.options[%d].value 必须为非空字符串", path, i)
			}
		}
	case "checkbox":
		// checked 可选，布尔
		if c, exists := dv["checked"]; exists {
			if _, ok := c.(bool); !ok {
				return fmt.Errorf("%s.checked 必须为 boolean", path)
			}
		}
	case "none":
		// type=none 表示显式清除单元格 DV，不需其他字段
	default:
		return fmt.Errorf("%s.type 非法值 %q（合法值: dropdown / checkbox / none）", path, dvType)
	}
	return nil
}
