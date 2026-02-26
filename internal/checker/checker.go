package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-health-checker/internal/config"
)

// Result holds the outcome of a health check.
type Result struct {
	Name       string        `json:"name"`
	URL        string        `json:"url"`
	Type       string        `json:"type"`
	Healthy    bool          `json:"healthy"`
	StatusCode int           `json:"status_code,omitempty"`
	Latency    time.Duration `json:"latency_ms"`
	Error      string        `json:"error,omitempty"`
	Body       string        `json:"body,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
	Retries    int           `json:"retries"`
}

// Options configures the check engine.
type Options struct {
	Timeout    time.Duration
	Retries    int
	WebhookURL string
	Verbose    bool
}

// Engine performs concurrent health checks.
type Engine struct {
	opts   Options
	client *http.Client
}

// NewEngine creates a new health check engine.
func NewEngine(opts Options) *Engine {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
		DialContext: (&net.Dialer{
			Timeout:   opts.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	client := &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &Engine{opts: opts, client: client}
}

// CheckAll runs health checks on all endpoints concurrently.
func (e *Engine) CheckAll(ctx context.Context, endpoints []config.Endpoint) []Result {
	results := make([]Result, len(endpoints))
	var wg sync.WaitGroup

	for i, ep := range endpoints {
		wg.Add(1)
		go func(idx int, endpoint config.Endpoint) {
			defer wg.Done()
			results[idx] = e.checkWithRetry(ctx, endpoint)
		}(i, ep)
	}

	wg.Wait()
	return results
}

// checkWithRetry performs a health check with configurable retry logic.
func (e *Engine) checkWithRetry(ctx context.Context, ep config.Endpoint) Result {
	var lastResult Result

	maxAttempts := e.opts.Retries + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				lastResult.Error = "check cancelled"
				return lastResult
			case <-time.After(backoff):
			}
		}

		lastResult = e.check(ctx, ep)
		lastResult.Retries = attempt

		if lastResult.Healthy {
			return lastResult
		}
	}

	return lastResult
}

// check performs a single health check based on endpoint type.
func (e *Engine) check(ctx context.Context, ep config.Endpoint) Result {
	switch ep.Type {
	case "tcp":
		return e.checkTCP(ctx, ep)
	case "grpc":
		return e.checkGRPC(ctx, ep)
	default:
		return e.checkHTTP(ctx, ep)
	}
}

// checkHTTP performs an HTTP health check.
func (e *Engine) checkHTTP(ctx context.Context, ep config.Endpoint) Result {
	result := Result{
		Name:      ep.Name,
		URL:       ep.URL,
		Type:      "http",
		Timestamp: time.Now().UTC(),
	}

	// Build request
	var bodyReader io.Reader
	if ep.Body != "" {
		bodyReader = strings.NewReader(ep.Body)
	}

	req, err := http.NewRequestWithContext(ctx, ep.Method, ep.URL, bodyReader)
	if err != nil {
		result.Error = fmt.Sprintf("build request: %v", err)
		return result
	}

	// Add headers
	for k, v := range ep.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "go-health-checker/1.0")
	}

	// Execute request
	start := time.Now()
	resp, err := e.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Read body if needed for body check
	if ep.ExpectedBodyContains != "" || e.opts.Verbose {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024)) // Max 10KB
		if err == nil {
			result.Body = string(body)
		}
	}

	// Evaluate health
	result.Healthy = true

	if ep.ExpectedStatus > 0 && resp.StatusCode != ep.ExpectedStatus {
		result.Healthy = false
		result.Error = fmt.Sprintf("expected status %d, got %d", ep.ExpectedStatus, resp.StatusCode)
	}

	if ep.ExpectedBodyContains != "" && !strings.Contains(result.Body, ep.ExpectedBodyContains) {
		result.Healthy = false
		result.Error = fmt.Sprintf("response body does not contain '%s'", ep.ExpectedBodyContains)
	}

	return result
}

// checkTCP performs a TCP connectivity check.
func (e *Engine) checkTCP(ctx context.Context, ep config.Endpoint) Result {
	result := Result{
		Name:      ep.Name,
		URL:       fmt.Sprintf("%s:%d", ep.Host, ep.Port),
		Type:      "tcp",
		Timestamp: time.Now().UTC(),
	}

	addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
	dialer := &net.Dialer{Timeout: e.opts.Timeout}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = fmt.Sprintf("tcp connect: %v", err)
		return result
	}
	conn.Close()

	result.Healthy = true
	return result
}

// checkGRPC performs a gRPC health check.
func (e *Engine) checkGRPC(ctx context.Context, ep config.Endpoint) Result {
	result := Result{
		Name:      ep.Name,
		URL:       ep.Host,
		Type:      "grpc",
		Timestamp: time.Now().UTC(),
	}

	// For now, do a TCP check to the gRPC port
	// In a full implementation, use grpc-health-probe protocol
	dialer := &net.Dialer{Timeout: e.opts.Timeout}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", ep.Host)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = fmt.Sprintf("grpc connect: %v", err)
		return result
	}
	conn.Close()

	result.Healthy = true
	return result
}
