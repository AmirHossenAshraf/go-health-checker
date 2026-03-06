package reporter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"go-health-checker/internal/checker"
)

// Reporter defines the interface for outputting health check results.
type Reporter interface {
	Report(results []checker.Result)
}

// ── Table Reporter ───────────────────────────────────────

// TableReporter outputs results as a formatted ASCII table.
type TableReporter struct {
	w     io.Writer
	quiet bool
}

func NewTableReporter(w io.Writer, quiet bool) *TableReporter {
	return &TableReporter{w: w, quiet: quiet}
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

func (r *TableReporter) Report(results []checker.Result) {
	healthy := 0
	unhealthy := 0

	// Calculate column widths
	maxName := 20
	for _, res := range results {
		if len(res.Name) > maxName {
			maxName = len(res.Name)
		}
	}
	if maxName > 40 {
		maxName = 40
	}

	// Header
	totalWidth := maxName + 42
	fmt.Fprintf(r.w, "\n%s%s%s\n", colorBold, strings.Repeat("─", totalWidth), colorReset)
	fmt.Fprintf(r.w, "%s%-*s  %-8s  %-10s  %s%s\n",
		colorBold, maxName, "ENDPOINT", "STATUS", "LATENCY", "DETAILS", colorReset)
	fmt.Fprintf(r.w, "%s%s\n", strings.Repeat("─", totalWidth), colorReset)

	// Rows
	for _, res := range results {
		if r.quiet && res.Healthy {
			healthy++
			continue
		}

		name := res.Name
		if len(name) > maxName {
			name = name[:maxName-3] + "..."
		}

		status := fmt.Sprintf("%s✅ UP%s", colorGreen, colorReset)
		if !res.Healthy {
			status = fmt.Sprintf("%s❌ DOWN%s", colorRed, colorReset)
			unhealthy++
		} else {
			healthy++
		}

		latency := formatLatency(res.Latency)
		details := formatDetails(res)

		fmt.Fprintf(r.w, "%-*s  %-18s  %-10s  %s\n", maxName, name, status, latency, details)
	}

	// Footer
	fmt.Fprintf(r.w, "%s\n", strings.Repeat("─", totalWidth))

	summaryColor := colorGreen
	if unhealthy > 0 {
		summaryColor = colorRed
	}

	fmt.Fprintf(r.w, "%s%s%d/%d healthy%s | Checked at %s\n\n",
		colorBold, summaryColor,
		healthy, healthy+unhealthy, colorReset,
		time.Now().Format("2006-01-02 15:04:05 MST"))
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dμs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatDetails(res checker.Result) string {
	if res.Error != "" {
		if len(res.Error) > 30 {
			return res.Error[:30] + "..."
		}
		return res.Error
	}
	if res.StatusCode > 0 {
		return fmt.Sprintf("HTTP %d", res.StatusCode)
	}
	if res.Type == "tcp" {
		return "TCP connected"
	}
	if res.Type == "grpc" {
		return "gRPC serving"
	}
	return "OK"
}

// ── JSON Reporter ────────────────────────────────────────

type JSONReporter struct {
	w io.Writer
}

func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{w: w}
}

type jsonOutput struct {
	Timestamp string           `json:"timestamp"`
	Total     int              `json:"total"`
	Healthy   int              `json:"healthy"`
	Unhealthy int              `json:"unhealthy"`
	Results   []checker.Result `json:"results"`
}

func (r *JSONReporter) Report(results []checker.Result) {
	healthy := 0
	for _, res := range results {
		if res.Healthy {
			healthy++
		}
	}

	out := jsonOutput{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Total:     len(results),
		Healthy:   healthy,
		Unhealthy: len(results) - healthy,
		Results:   results,
	}

	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ── CSV Reporter ─────────────────────────────────────────

type CSVReporter struct {
	w io.Writer
}

func NewCSVReporter(w io.Writer) *CSVReporter {
	return &CSVReporter{w: w}
}

func (r *CSVReporter) Report(results []checker.Result) {
	writer := csv.NewWriter(r.w)
	defer writer.Flush()

	writer.Write([]string{"name", "url", "type", "healthy", "status_code", "latency_ms", "error", "timestamp"})

	for _, res := range results {
		writer.Write([]string{
			res.Name,
			res.URL,
			res.Type,
			fmt.Sprintf("%t", res.Healthy),
			fmt.Sprintf("%d", res.StatusCode),
			fmt.Sprintf("%d", res.Latency.Milliseconds()),
			res.Error,
			res.Timestamp.Format(time.RFC3339),
		})
	}
}

// ── Prometheus Reporter ──────────────────────────────────

type PrometheusReporter struct {
	w io.Writer
}

func NewPrometheusReporter(w io.Writer) *PrometheusReporter {
	return &PrometheusReporter{w: w}
}

func (r *PrometheusReporter) Report(results []checker.Result) {
	fmt.Fprintln(r.w, "# HELP healthcheck_up Whether the endpoint is healthy (1) or not (0)")
	fmt.Fprintln(r.w, "# TYPE healthcheck_up gauge")
	for _, res := range results {
		val := 0
		if res.Healthy {
			val = 1
		}
		fmt.Fprintf(r.w, "healthcheck_up{name=%q,url=%q,type=%q} %d\n",
			res.Name, res.URL, res.Type, val)
	}

	fmt.Fprintln(r.w, "# HELP healthcheck_latency_ms Response latency in milliseconds")
	fmt.Fprintln(r.w, "# TYPE healthcheck_latency_ms gauge")
	for _, res := range results {
		fmt.Fprintf(r.w, "healthcheck_latency_ms{name=%q,url=%q} %d\n",
			res.Name, res.URL, res.Latency.Milliseconds())
	}
}
