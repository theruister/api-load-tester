package stats

import (
	"sort"
	"sync"
	"time"
)

type Result struct {
	ScheduledAt time.Time
	Latency     time.Duration
	StatusCode  int
	Err         error
}

type Recorder struct {
	mu      sync.Mutex
	results []Result
}

func NewRecorder() *Recorder {
	return &Recorder{results: make([]Result, 0, 1024)}
}

func (r *Recorder) Record(res Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, res)
}

func (r *Recorder) GetResults() []Result {
	return r.results
}

// Summary is a point-in-time snapshot of aggregate stats
type Summary struct {
	Total      int
	Successes  int
	Errors     int
	Duration   time.Duration
	Throughput float64 // requests/sec, based on Duration
	MinLatency time.Duration
	MaxLatency time.Duration
	AvgLatency time.Duration
	P50Latency time.Duration
	P95Latency time.Duration
	P99Latency time.Duration
}

// Snapshot computes a Summary from everything we have recorded so far.
// wallDuration is the actual elapsed time of the test run, used for throughput. It's passed in rather than inferred so
// callers can compute live, in-prograss summaries too.
func (r *Recorder) Snapshot(wallDuration time.Duration, duration time.Duration) Summary {
	r.mu.Lock()
	// Copy under the lock, then release it before the expensive sort/aggregation work

	results := make([]Result, len(r.results))
	copy(results, r.results)
	r.mu.Unlock()

	s := Summary{Total: len(results)}
	if len(results) == 0 {
		return s
	}

	latencies := make([]time.Duration, 0, len(results))
	var sum time.Duration
	for _, res := range results {
		if res.Err != nil || res.StatusCode >= 500 {
			s.Errors++
		} else {
			s.Successes++
		}
		latencies = append(latencies, res.Latency)
		sum += res.Latency
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	s.MinLatency = latencies[0]
	s.MaxLatency = latencies[len(latencies)-1]
	s.AvgLatency = sum / time.Duration(len(latencies))
	s.P50Latency = percentile(latencies, 0.50)
	s.P95Latency = percentile(latencies, 0.95)
	s.P99Latency = percentile(latencies, 0.99)

	if wallDuration > 0 {
		if wallDuration < duration {
			s.Duration = wallDuration
			s.Throughput = float64(s.Total) / wallDuration.Seconds()
		} else {
			s.Duration = duration
			s.Throughput = float64(s.Total) / duration.Seconds()
		}
	}
	return s
}

// percentile assumes sorted list of times
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
