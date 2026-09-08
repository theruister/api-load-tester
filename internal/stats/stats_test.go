package stats

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewRecorder_EmptySnapshot(t *testing.T) {
	r := NewRecorder()
	s := r.Snapshot(time.Second)

	if s.Total != 0 {
		t.Errorf("Total = %d, want 0", s.Total)
	}
	if s.Successes != 0 || s.Errors != 0 {
		t.Errorf("Successes = %d, Errors = %d, want 0, 0", s.Successes, s.Errors)
	}
	if s.MinLatency != 0 || s.MaxLatency != 0 || s.AvgLatency != 0 {
		t.Errorf("expected zero-value latencies on empty snapshot, got min=%v max=%v avg=%v",
			s.MinLatency, s.MaxLatency, s.AvgLatency)
	}
	if s.Throughput != 0 {
		t.Errorf("Throughput = %v, want 0 on empty snapshot", s.Throughput)
	}
	if s.Duration != time.Second {
		t.Errorf("Duration = %v, want %v (should reflect wallDuration even when empty)", s.Duration, time.Second)
	}
}

func TestRecorder_RecordAndSnapshot_Basic(t *testing.T) {
	r := NewRecorder()
	results := []Result{
		{Latency: 10 * time.Millisecond, StatusCode: 200},
		{Latency: 20 * time.Millisecond, StatusCode: 200},
		{Latency: 30 * time.Millisecond, StatusCode: 404},                          // non-5xx, no err -> counted as success
		{Latency: 40 * time.Millisecond, StatusCode: 500},                          // >=500 -> error
		{Latency: 50 * time.Millisecond, StatusCode: 200, Err: errors.New("boom")}, // Err set -> error
	}
	for _, res := range results {
		r.Record(res)
	}

	s := r.Snapshot(0)

	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.Successes != 3 {
		t.Errorf("Successes = %d, want 3", s.Successes)
	}
	if s.Errors != 2 {
		t.Errorf("Errors = %d, want 2", s.Errors)
	}
	if s.MinLatency != 10*time.Millisecond {
		t.Errorf("MinLatency = %v, want 10ms", s.MinLatency)
	}
	if s.MaxLatency != 50*time.Millisecond {
		t.Errorf("MaxLatency = %v, want 50ms", s.MaxLatency)
	}
	if want := 30 * time.Millisecond; s.AvgLatency != want {
		t.Errorf("AvgLatency = %v, want %v", s.AvgLatency, want)
	}

	// n=5, sorted latencies [10,20,30,40,50]ms; idx = int(p*n)
	if want := 30 * time.Millisecond; s.P50Latency != want { // int(0.50*5)=2 -> sorted[2]
		t.Errorf("P50Latency = %v, want %v", s.P50Latency, want)
	}
	if want := 50 * time.Millisecond; s.P95Latency != want { // int(0.95*5)=4 -> sorted[4]
		t.Errorf("P95Latency = %v, want %v", s.P95Latency, want)
	}
	if want := 50 * time.Millisecond; s.P99Latency != want { // int(0.99*5)=4 -> sorted[4]
		t.Errorf("P99Latency = %v, want %v", s.P99Latency, want)
	}
}

func TestRecorder_Snapshot_PercentilesLargerSample(t *testing.T) {
	r := NewRecorder()
	// Latencies 1ms..100ms, recorded out of order to also exercise the sort.
	for _, i := range []int{50, 1, 100, 99, 2, 96, 95, 3} {
		r.Record(Result{Latency: time.Duration(i) * time.Millisecond, StatusCode: 200})
	}
	for i := 4; i <= 94; i++ {
		if i == 50 { // already added above
			continue
		}
		r.Record(Result{Latency: time.Duration(i) * time.Millisecond, StatusCode: 200})
	}
	for i := 97; i <= 98; i++ {
		r.Record(Result{Latency: time.Duration(i) * time.Millisecond, StatusCode: 200})
	}

	s := r.Snapshot(0)
	if s.Total != 100 {
		t.Fatalf("Total = %d, want 100 (test setup bug)", s.Total)
	}

	// sorted values are 1ms..100ms (index i -> (i+1)ms). idx = int(p*100).
	if want := 51 * time.Millisecond; s.P50Latency != want { // idx 50
		t.Errorf("P50Latency = %v, want %v", s.P50Latency, want)
	}
	if want := 96 * time.Millisecond; s.P95Latency != want { // idx 95
		t.Errorf("P95Latency = %v, want %v", s.P95Latency, want)
	}
	if want := 100 * time.Millisecond; s.P99Latency != want { // idx 99
		t.Errorf("P99Latency = %v, want %v", s.P99Latency, want)
	}
}

func TestRecorder_Snapshot_Throughput(t *testing.T) {
	r := NewRecorder()
	for i := 0; i < 100; i++ {
		r.Record(Result{Latency: time.Millisecond, StatusCode: 200})
	}

	s := r.Snapshot(10 * time.Second)
	if want := 10.0; s.Throughput != want {
		t.Errorf("Throughput = %v, want %v", s.Throughput, want)
	}
}

func TestRecorder_Snapshot_ZeroDurationAvoidsDivideByZero(t *testing.T) {
	r := NewRecorder()
	r.Record(Result{Latency: time.Millisecond, StatusCode: 200})
	r.Record(Result{Latency: 2 * time.Millisecond, StatusCode: 200})

	s := r.Snapshot(0)
	if s.Throughput != 0 {
		t.Errorf("Throughput = %v, want 0 when wallDuration is 0", s.Throughput)
	}
	// Latency stats should still be computed even though duration is zero.
	if s.Total != 2 || s.MinLatency != time.Millisecond {
		t.Errorf("expected latency stats to still be computed, got %+v", s)
	}
}

func TestRecorder_Snapshot_IsIndependentSnapshot(t *testing.T) {
	// Snapshot copies results under the lock, so recording after a snapshot
	// must not mutate a previously returned Summary or its backing data.
	r := NewRecorder()
	r.Record(Result{Latency: time.Millisecond, StatusCode: 200})

	first := r.Snapshot(0)
	r.Record(Result{Latency: 5 * time.Second, StatusCode: 200})

	if first.Total != 1 {
		t.Errorf("earlier snapshot mutated: Total = %d, want 1", first.Total)
	}
	if first.MaxLatency != time.Millisecond {
		t.Errorf("earlier snapshot mutated: MaxLatency = %v, want 1ms", first.MaxLatency)
	}
}

func TestRecorder_ConcurrentRecord(t *testing.T) {
	r := NewRecorder()
	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				r.Record(Result{
					Latency:    time.Duration(id+1) * time.Millisecond,
					StatusCode: 200,
				})
			}
		}(g)
	}
	wg.Wait()

	s := r.Snapshot(time.Second)
	want := goroutines * perGoroutine
	if s.Total != want {
		t.Errorf("Total = %d, want %d", s.Total, want)
	}
	if s.Successes != want {
		t.Errorf("Successes = %d, want %d", s.Successes, want)
	}
}

func TestPercentile_EmptySlice(t *testing.T) {
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile(nil, 0.5) = %v, want 0", got)
	}
}

func TestPercentile_SingleElement(t *testing.T) {
	sorted := []time.Duration{5 * time.Millisecond}
	for _, p := range []float64{0, 0.5, 0.99, 1.0} {
		if got := percentile(sorted, p); got != 5*time.Millisecond {
			t.Errorf("percentile(sorted, %v) = %v, want 5ms", p, got)
		}
	}
}

func TestPercentile_ClampsAtUpperBound(t *testing.T) {
	sorted := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
	// p=1.0 -> idx = int(1.0*3) = 3, out of range, must clamp to len-1.
	if got := percentile(sorted, 1.0); got != 3*time.Millisecond {
		t.Errorf("percentile(sorted, 1.0) = %v, want 3ms (clamped to max)", got)
	}
}

func TestPercentile_ZeroReturnsMin(t *testing.T) {
	sorted := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
	if got := percentile(sorted, 0); got != 1*time.Millisecond {
		t.Errorf("percentile(sorted, 0) = %v, want 1ms (min)", got)
	}
}
