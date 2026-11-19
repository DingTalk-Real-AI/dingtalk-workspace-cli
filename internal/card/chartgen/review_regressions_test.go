// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChartBusinessNumbersRejectMissingAndNull(t *testing.T) {
	category := chartPreset("G04", "category", "bar")
	measure := chartPreset("G26", "measure", "gauge")
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "category null value",
			run: func() error {
				_, _, err := buildCategory(category, "T", []byte(`{"kind":"category","categories":["A"],"series":[{"name":"S","values":[null]}]}`))
				return err
			},
			want: "series[0].values[0]",
		},
		{
			name: "gauge missing value",
			run: func() error {
				_, _, err := buildMeasure(measure, "T", []byte(`{"kind":"measure","name":"M","min":0,"max":10}`))
				return err
			},
			want: "value",
		},
		{
			name: "gauge null value",
			run: func() error {
				_, _, err := buildMeasure(measure, "T", []byte(`{"kind":"measure","name":"M","value":null,"min":0,"max":10}`))
				return err
			},
			want: "value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChartTuplesRequireExactArity(t *testing.T) {
	points := chartPreset("G21", "points", "effectScatter")
	ohlc := chartPreset("G24", "ohlc", "candlestick")
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "short point",
			run: func() error {
				_, _, err := buildPoints(points, "T", []byte(`{"kind":"points","xLabel":"X","yLabel":"Y","groups":[{"name":"G","points":[[7]]}]}`))
				return err
			},
			want: "exactly two",
		},
		{
			name: "null point coordinate",
			run: func() error {
				_, _, err := buildPoints(points, "T", []byte(`{"kind":"points","xLabel":"X","yLabel":"Y","groups":[{"name":"G","points":[[7,null]]}]}`))
				return err
			},
			want: "points[0][1]",
		},
		{
			name: "long candle",
			run: func() error {
				_, _, err := buildOHLC(ohlc, "T", []byte(`{"kind":"ohlc","categories":["A"],"name":"S","candles":[[1,2,0,3,4]]}`))
				return err
			},
			want: "exactly [open,close,low,high]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
