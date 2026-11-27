// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func requireRawNumber(raw json.RawMessage, field string) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s requires a numeric value", field)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || !finite(value) {
		return fmt.Errorf("%s has a nonfinite or invalid numeric value", field)
	}
	return nil
}

func validateCategoryNumbers(raw json.RawMessage) error {
	var payload struct {
		Series []struct {
			Values []json.RawMessage `json:"values"`
		} `json:"series"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, series := range payload.Series {
		for j, value := range series.Values {
			if err := requireRawNumber(value, fmt.Sprintf("series[%d].values[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNamedValueNumbers(raw json.RawMessage) error {
	var payload struct {
		Items []struct {
			Value json.RawMessage `json:"value"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, item := range payload.Items {
		if err := requireRawNumber(item.Value, fmt.Sprintf("items[%d].value", i)); err != nil {
			return err
		}
	}
	return nil
}

func validatePointNumbers(raw json.RawMessage) error {
	var payload struct {
		Groups []struct {
			Points []json.RawMessage `json:"points"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, group := range payload.Groups {
		for j, rawPoint := range group.Points {
			var point []json.RawMessage
			if err := json.Unmarshal(rawPoint, &point); err != nil || len(point) != 2 {
				return fmt.Errorf("groups[%d].points[%d] requires exactly two coordinates", i, j)
			}
			for k, value := range point {
				if err := requireRawNumber(value, fmt.Sprintf("groups[%d].points[%d][%d]", i, j, k)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateMatrixNumbers(raw json.RawMessage) error {
	var payload struct {
		Cells []struct {
			X     json.RawMessage `json:"x"`
			Y     json.RawMessage `json:"y"`
			Value json.RawMessage `json:"value"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, cell := range payload.Cells {
		for _, number := range []struct {
			field string
			value json.RawMessage
		}{{"x", cell.X}, {"y", cell.Y}, {"value", cell.Value}} {
			if err := requireRawNumber(number.value, fmt.Sprintf("cells[%d].%s", i, number.field)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOHLCNumbers(raw json.RawMessage) error {
	var payload struct {
		Candles []json.RawMessage `json:"candles"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, rawCandle := range payload.Candles {
		var candle []json.RawMessage
		if err := json.Unmarshal(rawCandle, &candle); err != nil || len(candle) != 4 {
			return fmt.Errorf("candles[%d] requires exactly [open,close,low,high]", i)
		}
		for j, value := range candle {
			if err := requireRawNumber(value, fmt.Sprintf("candles[%d][%d]", i, j)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRadarNumbers(raw json.RawMessage) error {
	var payload struct {
		Indicators []struct {
			Max json.RawMessage `json:"max"`
		} `json:"indicators"`
		Groups []struct {
			Values []json.RawMessage `json:"values"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, indicator := range payload.Indicators {
		if err := requireRawNumber(indicator.Max, fmt.Sprintf("indicators[%d].max", i)); err != nil {
			return err
		}
	}
	for i, group := range payload.Groups {
		for j, value := range group.Values {
			if err := requireRawNumber(value, fmt.Sprintf("groups[%d].values[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMeasureNumbers(raw json.RawMessage) error {
	var payload struct {
		Value json.RawMessage `json:"value"`
		Min   json.RawMessage `json:"min"`
		Max   json.RawMessage `json:"max"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for _, number := range []struct {
		field string
		value json.RawMessage
	}{{"value", payload.Value}, {"min", payload.Min}, {"max", payload.Max}} {
		if err := requireRawNumber(number.value, number.field); err != nil {
			return err
		}
	}
	return nil
}

func validateFlowNumbers(raw json.RawMessage) error {
	var payload struct {
		Links []struct {
			Value json.RawMessage `json:"value"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	for i, link := range payload.Links {
		if err := requireRawNumber(link.Value, fmt.Sprintf("links[%d].value", i)); err != nil {
			return err
		}
	}
	return nil
}
