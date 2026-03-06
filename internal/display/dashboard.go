package display

import (
	"context"
	"fmt"
	"time"

	"go-health-checker/internal/checker"
	"go-health-checker/internal/config"
	"go-health-checker/internal/reporter"
)

// WatchMode runs health checks continuously at the given interval.
func WatchMode(
	ctx context.Context,
	engine *checker.Engine,
	endpoints []config.Endpoint,
	rep reporter.Reporter,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial check
	fmt.Printf("Starting watch mode (interval: %s, endpoints: %d)\n", interval, len(endpoints))
	fmt.Println("Press Ctrl+C to stop.\n")

	results := engine.CheckAll(ctx, endpoints)
	rep.Report(results)

	// Continuous monitoring
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nWatch mode stopped.")
			return
		case <-ticker.C:
			// Clear screen for fresh output
			fmt.Print("\033[H\033[2J")
			fmt.Printf("🔄 Watch Mode | Interval: %s | Next check in %s\n", interval, interval)

			results := engine.CheckAll(ctx, endpoints)
			rep.Report(results)
		}
	}
}
