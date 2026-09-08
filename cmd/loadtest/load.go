package main

import (
	"context"
	"flag"
	"fmt"
	"load-tester/internal/config"
	"load-tester/internal/report"
	"load-tester/internal/stats"
	"load-tester/internal/worker"
	"os"
	"os/signal"
	"strings"
	"time"
)

func main() {
	var (
		url      = flag.String("url", "", "target URL (required)")
		method   = flag.String("method", "GET", "HTTP method")
		body     = flag.String("body", "", "HTTP request body")
		rate     = flag.Int("rate", 50, "target requests per second")
		workers  = flag.Int("workers", 10, "number of concurrent workers")
		duration = flag.Int("duration", 10, "duration of the test")
		timeout  = flag.Int("timeout", 5, "per-request timeout in seconds")
		file     = flag.String("file", "", "config file")
	)

	flag.Parse()

	// worker.Config is set up as follows:
	// first populate with default values
	// second override with values defined in 'file'
	// third override with any values specifically defined on the command line
	cfg := worker.Config{
		URL:         "",
		Method:      "GET",
		Rate:        50,
		Concurrency: 10,
		Duration:    10,
		Timeout:     5,
	}

	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		seen[f.Name] = true
	})

	// If file exists, use it. Values can later be overwritten from params so users don't have to edit json files for
	// simple tweaks to a test
	if seen["file"] {
		config.ParseConfig(*file, &cfg)
	}

	// Now update values from flag params
	if seen["url"] {
		cfg.URL = *url
	}
	if seen["method"] {
		cfg.Method = *method
	}
	if seen["body"] {
		cfg.Body = strings.NewReader(*body)
	}
	if seen["rate"] {
		cfg.Rate = *rate
	}
	if seen["workers"] {
		cfg.Concurrency = *workers
	}
	if seen["duration"] {
		cfg.Duration = time.Duration(*duration) * time.Second
	}
	if seen["timeout"] {
		cfg.Timeout = time.Duration(*timeout) * time.Second
	}

	// Check we have at least a URL defined everything else should have a defaultif required
	if cfg.URL == "" {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("load testing %s - %d req/s target, %d workers, for %s\n", cfg.URL, cfg.Rate, cfg.Concurrency, cfg.Duration)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	recorder := stats.NewRecorder()
	pool := worker.New(cfg, recorder)

	start := time.Now()
	if err := pool.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	elapsed := time.Since(start)

	summary := recorder.Snapshot(elapsed)
	report.Print(os.Stdout, summary)
}
