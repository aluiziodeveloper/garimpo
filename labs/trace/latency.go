package main

import (
	"errors"
	"fmt"
	"math"
	"runtime/metrics"
	"slices"
)

// schedLatencies is the runtime metric with the time goroutines spent runnable before
// running, the same wait the sched profile of go tool trace adds up.
const schedLatencies = "/sched/latencies:seconds"

var (
	errBadQuantile = errors.New("quantile must be in (0, 1]")
	errNoSamples   = errors.New("no new samples")
	errBuckets     = errors.New("histograms have different buckets")
)

// readSchedLatencies takes a snapshot of the scheduler latency histogram.
func readSchedLatencies() metrics.Float64Histogram {
	s := []metrics.Sample{{Name: schedLatencies}}
	metrics.Read(s)
	h := s[0].Value.Float64Histogram()
	// The runtime may reuse the slices on the next Read: keep a copy.
	return metrics.Float64Histogram{Counts: slices.Clone(h.Counts), Buckets: slices.Clone(h.Buckets)}
}

// percentile returns the upper bound of the bucket that holds the q-th quantile of the
// samples recorded between two snapshots of the same histogram.
func percentile(before, after metrics.Float64Histogram, q float64) (float64, error) {
	if q <= 0 || q > 1 {
		return 0, fmt.Errorf("q = %v: %w", q, errBadQuantile)
	}
	if !slices.Equal(before.Buckets, after.Buckets) || len(before.Counts) != len(after.Counts) {
		return 0, errBuckets
	}
	var total uint64
	for i := range after.Counts {
		total += after.Counts[i] - before.Counts[i]
	}
	if total == 0 {
		return 0, errNoSamples
	}
	// How many samples reach the quantile, rounded up.
	need := uint64(math.Ceil(q * float64(total)))
	var seen uint64
	for i := range after.Counts {
		seen += after.Counts[i] - before.Counts[i]
		if seen >= need {
			return after.Buckets[i+1], nil
		}
	}
	return after.Buckets[len(after.Buckets)-1], nil
}
