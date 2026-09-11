package worker

import (
	"context"
	"fmt"
	"io"
	"load-tester/internal/stats"
	"net/http"
	"slices"
	"strings"
	"time"
)

type Config struct {
	URL         string
	Method      string
	Body        io.Reader
	Rate        int
	Concurrency int
	Duration    time.Duration
	Timeout     time.Duration
	OutFile     string
}

type Pool struct {
	cfg      Config
	client   *http.Client
	recorder *stats.Recorder
}

func New(cfg Config, recorder *stats.Recorder) *Pool {
	return &Pool{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		recorder: recorder,
	}
}

// tick represents one scheduled request slot
type tick struct {
	scheduledAt time.Time
}

// Run blocks until the configured duration elapses or ctx is cancelled
// (e.g. by Ctrl+C), whichever comes first. On cancellation it stops
// scheduling new requests but lets in-flight ones finish, so results
// collected so far remain valid — that's what makes "partial results on
// Ctrl+C" possible in main.go.
// Important: schedCtx governs only when to STOP PULLING new ticks off the channel - it is never passed into an
// individual HTTP request's context. Earlier this package used the same context for both, which meant that a request
// picked up right as the run's duration elapsed got it's own request cancelled by the deadline. This was a real bug not
// test flakiness - reliably producing 1-2 errors per run against perfectly healthy servers.
func (p *Pool) Run(ctx context.Context) error {
	// Validate the Pool config before starting
	if p.cfg.Rate <= 0 {
		return fmt.Errorf("rate must be positive, got %d", p.cfg.Rate)
	}
	if p.cfg.Concurrency <= 0 {
		return fmt.Errorf("workers must be positive, got %d", p.cfg.Concurrency)
	}
	if p.cfg.Duration <= 0 {
		return fmt.Errorf("duration must be positive, got %d", p.cfg.Duration)
	}
	if p.cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %d", p.cfg.Timeout)
	}
	validMethods := []string{"GET", "HEAD", "OPTIONS", "TRACE", "PUT", "DELETE", "POST", "PATCH", "CONNECT"}
	if !slices.Contains(validMethods, strings.ToUpper(p.cfg.Method)) {
		return fmt.Errorf("method must one of the following %v, got %s", validMethods, p.cfg.Method)
	}

	// Config is valid, let us begin

	schedCtx, cancel := context.WithTimeout(ctx, p.cfg.Duration)
	defer cancel()

	// Buffered so the scheduler never blocks waiting for a worker to be free - a saturated pool just makes the channel
	// fill up, which is a useful signal we could surface later (eg. "queue depth").
	ticks := make(chan tick, p.cfg.Concurrency*4)

	workerDone := make(chan struct{})
	for i := 0; i < p.cfg.Concurrency; i++ {
		go func() {
			p.workerLoop(schedCtx, ticks)
			workerDone <- struct{}{}
		}()
	}

	p.scheduleLoop(schedCtx, ticks)
	close(ticks)

	for i := 0; i < p.cfg.Concurrency; i++ {
		<-workerDone
	}
	return nil
}

// scheduleLoop emits one tick per request at the target rate until runCtx is done. This goroutine's only job is timing
// - it never touches the network, so a slow server can't throw off the schedule.
func (p *Pool) scheduleLoop(ctx context.Context, ticks chan<- tick) {
	interval := time.Second / time.Duration(p.cfg.Rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			select {
			case ticks <- tick{scheduledAt: t}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// workLoop pulls scheduled ticks off the channel and executes requests until the channel is closed or the schedCtx is
// cancelled. schedCtx only controls whether this loop keeps pulling new work - it is deliberately NOT forwarded into
// doRequest (see note on Run for why)
func (p *Pool) workerLoop(schedCtx context.Context, ticks <-chan tick) {
	for {
		select {
		case <-schedCtx.Done():
			return
		case t, ok := <-ticks:
			if !ok {
				return
			}
			p.doRequest(t)
		}
	}
}

// doRequest executes a single request. Its context is intentionally independent of the run's scheduling deadline,
// bounded only by cfg.Timeout via the http.Client so that a request already in flight is never aborted just because the
// overall test duration ran out or ctrl+c was pressed.
func (p *Pool) doRequest(t tick) {
	req, err := http.NewRequestWithContext(context.Background(), strings.ToUpper(p.cfg.Method), p.cfg.URL, p.cfg.Body)
	if err != nil {
		p.recorder.Record(stats.Result{ScheduledAt: t.scheduledAt, Err: err})
		return
	}

	resp, err := p.client.Do(req)

	// Latency is measured from the SCHEDULED time, not from 'start' -
	// this is the crux of avoiding coordinatied omission. A request that
	// waited in the channel because the pool was saturated should show
	// that wait is part of its latency, not have it silently absorbed.
	latency := time.Since(t.scheduledAt)

	if err != nil {
		p.recorder.Record(stats.Result{ScheduledAt: t.scheduledAt, Latency: latency, Err: err})
		return
	}
	defer resp.Body.Close()

	// Drain the body so the connection can be reused by the transport's connection pool - skipping this silently
	// degrades the throughput as the test runs, which is a sublt bug worth calling out if you hit it.
	_, _ = io.Copy(io.Discard, resp.Body)

	p.recorder.Record(stats.Result{
		ScheduledAt: t.scheduledAt,
		Latency:     latency,
		StatusCode:  resp.StatusCode,
	})
}
