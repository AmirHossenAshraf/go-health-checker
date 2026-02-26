package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go-health-checker/internal/config"
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

}
