package main

import (
	"context"
	"flag"
	"fmt"
	"go-health-checker/internal/checker"
	"go-health-checker/internal/config"
	"go-health-checker/internal/display"
	"go-health-checker/internal/reporter"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	version = "dev"
)

func main() {
	// Flags
	configFile := flag.String("c", "", "Config file path (YAML/JSON)")
	timeout := flag.Duration("t", 5*time.Second, "Request timeout")
	retries := flag.Int("r", 0, "Retry count on failure")
	interval := flag.Duration("i", 30*time.Second, "Check interval (watch mode)")
	watch := flag.Bool("w", false, "Continuous monitoring mode")
	format := flag.String("f", "table", "Output format: table, json, csv, prometheus")
	verbose := flag.Bool("v", false, "Show detailed response info")
	quiet := flag.Bool("q", false, "Only show failures")
	webhookURL := flag.String("webhook", "", "Alert webhook URL")
	showVersion := flag.Bool("version", false, "Show version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: healthcheck [flags] [urls...]\n\n")
		fmt.Fprintf(os.Stderr, "A fast, concurrent API health checker.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  healthcheck https://api.example.com/health\n")
		fmt.Fprintf(os.Stderr, "  healthcheck -c endpoints.yml --watch\n")
		fmt.Fprintf(os.Stderr, "  healthcheck -c endpoints.yml --format json\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("healthcheck %s\n", version)
		os.Exit(0)
	}

	// Build endpoint list from config file and/or CLI args
	var endpoints []config.Endpoint

	if *configFile != "" {
		cfg, err := config.LoadFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		endpoints = cfg.Endpoints

		// Apply settings from config if not overridden by flags
		if cfg.Settings.Timeout > 0 {
			*timeout = cfg.Settings.Timeout
		}
		if cfg.Settings.Retries > 0 {
			*retries = cfg.Settings.Retries
		}
		if cfg.Settings.Interval > 0 {
			*interval = cfg.Settings.Interval
		}
	}

	// Add URLs from CLI arguments
	for _, arg := range flag.Args() {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			endpoints = append(endpoints, config.Endpoint{
				Name:           arg,
				URL:            arg,
				Type:           "http",
				Method:         "GET",
				ExpectedStatus: 200,
			})
		}
	}

	if len(endpoints) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no endpoints specified. Use -c <config> or pass URLs as arguments.")
		flag.Usage()
		os.Exit(1)
	}

	// Create checker engine
	engine := checker.NewEngine(checker.Options{
		Timeout:    *timeout,
		Retries:    *retries,
		WebhookURL: *webhookURL,
		Verbose:    *verbose,
	})

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Choose output formatter
	var rep reporter.Reporter
	switch *format {
	case "json":
		rep = reporter.NewJSONReporter(os.Stdout)
	case "csv":
		rep = reporter.NewCSVReporter(os.Stdout)
	case "prometheus":
		rep = reporter.NewPrometheusReporter(os.Stdout)
	default:
		rep = reporter.NewTableReporter(os.Stdout, *quiet)
	}

	// Run checks
	if *watch {
		display.WatchMode(ctx, engine, endpoints, rep, *interval)
	} else {
		results := engine.CheckAll(ctx, endpoints)
		rep.Report(results)

		// Exit with error if any endpoint is unhealthy
		for _, r := range results {
			if !r.Healthy {
				os.Exit(1)
			}
		}
	}
}
