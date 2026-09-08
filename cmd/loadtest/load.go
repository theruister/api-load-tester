package main

import (
	"context"
	"flag"
	"fmt"
	"load-tester/internal/report"
	"load-tester/internal/stats"
	"load-tester/internal/worker"
	"os"
	"os/signal"
	"time"
)

func main() {
	var (
		url      = flag.String("url", "", "target URL (required)")
		method   = flag.String("method", "GET", "HTTP method")
		rate     = flag.Int("rate", 50, "target requests per second")
		workers  = flag.Int("workers", 10, "number of concurrent workers")
		duration = flag.Int("duration", 10, "duration of the test")
		timeout  = flag.Int("timeout", 5, "per-request timeout in seconds")
	)

	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "error: -url is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	/* debug only
	fmt.Println(*url)
	fmt.Println(*method)
	fmt.Println(*rate)
	fmt.Println(*workers)
	fmt.Println(*duration)
	fmt.Println(*timeout)
	*/

	recorder := stats.NewRecorder()
	pool := worker.New(worker.Config{
		URL:         *url,
		Method:      *method,
		Rate:        *rate,
		Concurrency: *workers,
		Duration:    time.Duration(*duration) * time.Second,
		Timeout:     time.Duration(*timeout) * time.Second,
	}, recorder)

	fmt.Printf("load testing %s - %d req/s target, %d workers, for %s\n", *url, *rate, *workers, time.Duration(*duration)*time.Second)

	start := time.Now()
	if err := pool.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	elapsed := time.Since(start)

	summary := recorder.Snapshot(elapsed)
	report.Print(os.Stdout, summary)
}
