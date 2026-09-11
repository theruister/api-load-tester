package report

import (
	"encoding/json"
	"fmt"
	"io"
	"load-tester/internal/stats"
	"os"
)

func Print(w io.Writer, s stats.Summary) {
	fmt.Fprintf(w, "\nResults\n")
	fmt.Fprintf(w, "-------\n")
	fmt.Fprintf(w, "Total requests:   %d\n", s.Total)
	fmt.Fprintf(w, "Successful:       %d\n", s.Successes)
	fmt.Fprintf(w, "Errors:           %d\n", s.Errors)
	fmt.Fprintf(w, "Duration:         %s\n", s.Duration.Round(1e6))
	fmt.Fprintf(w, "Throughput:       %.1f req/s\n", s.Throughput)
	fmt.Fprintf(w, "\nLatency\n")
	fmt.Fprintf(w, "  min: %s\n", s.MinLatency)
	fmt.Fprintf(w, "  avg: %s\n", s.AvgLatency)
	fmt.Fprintf(w, "  p50: %s\n", s.P50Latency)
	fmt.Fprintf(w, "  p95: %s\n", s.P95Latency)
	fmt.Fprintf(w, "  p99: %s\n", s.P99Latency)
	fmt.Fprintf(w, "  max: %s\n", s.MaxLatency)

}

func PrintFile(f string, r *stats.Recorder) {
	file, err := os.Create(f)
	if err != nil {
		fmt.Println(fmt.Sprintf("Error creating file %s", f))
		return
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	err = encoder.Encode(r.GetResults())
	if err != nil {
		fmt.Println(fmt.Sprintf("Error encoding results in file %s", f))
	}
}
