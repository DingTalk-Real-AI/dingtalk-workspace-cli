package helpers

import "testing"

func TestCrossPlatformCoverageValidateComplexValueStyle(t *testing.T) {
	valid := map[string]any{"bold": true, "italic": false, "underline": true, "strike": false, "color": "#fff", "size": float64(12)}
	if err := validateComplexValueStyle(valid, "style"); err != nil {
		t.Fatalf("valid style error = %v", err)
	}
	for _, style := range []map[string]any{
		{"unknown": true}, {"bold": "yes"}, {"color": 1}, {"size": "large"},
	} {
		if err := validateComplexValueStyle(style, "style"); err == nil {
			t.Errorf("invalid style %#v returned nil", style)
		}
	}
}

func TestCrossPlatformCoverageValidateRichTextItems(t *testing.T) {
	valid := []map[string]any{
		{"type": "text", "text": "hello", "style": map[string]any{"bold": true}},
		{"type": "link", "text": "site", "link": "https://example.test", "style": map[string]any{"color": "#fff"}},
		{"type": "attachment", "text": "file", "resourceId": "rid", "mimeType": "text/plain"},
		{"type": "image", "resourceId": "rid", "resourceUrl": "https://example.test/image"},
	}
	for _, item := range valid {
		if err := validateRichTextItem(item, "item"); err != nil {
			t.Errorf("valid rich-text item %#v: %v", item, err)
		}
	}
	invalid := []map[string]any{
		{}, {"type": 1}, {"type": "text"},
		{"type": "link", "link": "url"}, {"type": "link", "text": "x"},
		{"type": "attachment", "resourceId": "rid", "mimeType": "mime"},
		{"type": "attachment", "text": "x", "mimeType": "mime"},
		{"type": "attachment", "text": "x", "resourceId": "rid"},
		{"type": "image", "resourceUrl": "url"}, {"type": "image", "resourceId": "rid"},
		{"type": "unknown"},
		{"type": "attachment", "text": "x", "resourceId": "rid", "mimeType": "mime", "style": map[string]any{}},
		{"type": "text", "text": "x", "style": "bad"},
		{"type": "text", "text": "x", "style": map[string]any{"bold": "bad"}},
	}
	for _, item := range invalid {
		if err := validateRichTextItem(item, "item"); err == nil {
			t.Errorf("invalid rich-text item %#v returned nil", item)
		}
	}
}

func TestCrossPlatformCoverageValidateComplexValueCells(t *testing.T) {
	valid := []map[string]any{
		{"dataValidation": map[string]any{"type": "none"}},
		{"cellStyles": map[string]any{}},
		{"hyperlink": nil},
		{"type": "text"},
		{"type": "text", "text": "", "style": map[string]any{"bold": true}},
		{"type": "text", "text": "same", "hyperlink": map[string]any{"type": "path", "link": "url", "text": "same"}},
		{"type": "richText", "texts": []any{map[string]any{"type": "text", "text": "x"}}},
	}
	for _, cell := range valid {
		if err := validateComplexValueCell(cell, "cell"); err != nil {
			t.Errorf("valid cell %#v: %v", cell, err)
		}
	}
	invalid := []map[string]any{
		{}, {"dataValidation": "bad"}, {"hyperlink": "bad"}, {"type": 1},
		{"type": "text", "text": 1}, {"type": "text", "style": "bad"},
		{"type": "text", "style": map[string]any{"color": 1}},
		{"type": "text", "text": "cell", "hyperlink": map[string]any{"type": "path", "link": "url", "text": "different"}},
		{"type": "richText", "hyperlink": nil, "texts": []any{}},
		{"type": "richText"}, {"type": "richText", "texts": "bad"},
		{"type": "richText", "texts": []any{}}, {"type": "richText", "texts": []any{"bad"}},
		{"type": "richText", "texts": []any{map[string]any{"type": "text"}}},
		{"type": "number"},
	}
	for _, cell := range invalid {
		if err := validateComplexValueCell(cell, "cell"); err == nil {
			t.Errorf("invalid cell %#v returned nil", cell)
		}
	}
}

func TestCrossPlatformCoverageValidateCellHyperlinks(t *testing.T) {
	for _, value := range []any{
		nil,
		map[string]any{"type": "none"},
		map[string]any{"type": "path", "link": "url"},
		map[string]any{"type": "sheet", "link": "url", "text": "label"},
		map[string]any{"type": "range", "link": "url"},
	} {
		if err := validateCellHyperlink(value, "link"); err != nil {
			t.Errorf("valid hyperlink %#v: %v", value, err)
		}
	}
	for _, value := range []any{
		"bad", map[string]any{}, map[string]any{"type": 1},
		map[string]any{"type": "path"}, map[string]any{"type": "path", "link": " "},
		map[string]any{"type": "path", "link": "url", "text": 1},
		map[string]any{"type": "unknown"},
	} {
		if err := validateCellHyperlink(value, "link"); err == nil {
			t.Errorf("invalid hyperlink %#v returned nil", value)
		}
	}
}

func TestCrossPlatformCoverageValidateDataValidations(t *testing.T) {
	valid := []any{
		map[string]any{"type": "dropdown", "options": []any{map[string]any{"value": "one"}}},
		map[string]any{"type": "checkbox"}, map[string]any{"type": "checkbox", "checked": true},
		map[string]any{"type": "none"},
	}
	for _, value := range valid {
		if err := validateDataValidation(value, "dv"); err != nil {
			t.Errorf("valid data validation %#v: %v", value, err)
		}
	}
	invalid := []any{
		"bad", map[string]any{}, map[string]any{"type": "dropdown"},
		map[string]any{"type": "dropdown", "options": "bad"},
		map[string]any{"type": "dropdown", "options": []any{"bad"}},
		map[string]any{"type": "dropdown", "options": []any{map[string]any{}}},
		map[string]any{"type": "checkbox", "checked": "yes"},
		map[string]any{"type": "unknown"},
	}
	for _, value := range invalid {
		if err := validateDataValidation(value, "dv"); err == nil {
			t.Errorf("invalid data validation %#v returned nil", value)
		}
	}
}
