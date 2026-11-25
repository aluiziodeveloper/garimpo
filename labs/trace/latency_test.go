package main

import (
	"errors"
	"math"
	"runtime/metrics"
	"testing"
)

func TestPercentileUsesOnlyTheNewSamples(t *testing.T) {
	buckets := []float64{0, 1e-6, 1e-5, 1e-4, 1e-3}
	before := metrics.Float64Histogram{Counts: []uint64{50, 0, 0, 0}, Buckets: buckets}
	after := metrics.Float64Histogram{Counts: []uint64{50, 90, 9, 1}, Buckets: buckets}

	tests := []struct {
		q    float64
		want float64
	}{
		{0.5, 1e-5},  // 50 of the 100 new samples are under 10µs
		{0.99, 1e-4}, // the 99th is in [10µs, 100µs)
		{1, 1e-3},
	}
	for _, tt := range tests {
		got, err := percentile(before, after, tt.q)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("percentile(q=%v) = %v, want %v", tt.q, got, tt.want)
		}
	}
}

func TestPercentileOfTheLastBucketIsInfinite(t *testing.T) {
	buckets := []float64{0, 1, math.Inf(1)}
	after := metrics.Float64Histogram{Counts: []uint64{0, 3}, Buckets: buckets}
	got, err := percentile(metrics.Float64Histogram{Counts: []uint64{0, 0}, Buckets: buckets}, after, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(got, 1) {
		t.Errorf("percentile() = %v, want +Inf", got)
	}
}

func TestPercentileErrors(t *testing.T) {
	h := metrics.Float64Histogram{Counts: []uint64{1}, Buckets: []float64{0, 1}}
	other := metrics.Float64Histogram{Counts: []uint64{1, 1}, Buckets: []float64{0, 1, 2}}
	tests := []struct {
		name          string
		before, after metrics.Float64Histogram
		q             float64
		want          error
	}{
		{"q zero", h, h, 0, errBadQuantile},
		{"q above one", h, h, 1.5, errBadQuantile},
		{"no new samples", h, h, 0.5, errNoSamples},
		{"different buckets", h, other, 0.5, errBuckets},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := percentile(tt.before, tt.after, tt.q); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
