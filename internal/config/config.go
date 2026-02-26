package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the full configuration file.
type Config struct {
	Settings  Settings   `yaml:"settings" json:"settings"`
	Endpoints []Endpoint `yaml:"endpoints" json:"endpoints"`
	Alerts    Alerts     `yaml:"alerts" json:"alerts"`
}

// Settings holds global check settings.
type Settings struct {
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
	Retries  int           `yaml:"retries" json:"retries"`
	Interval time.Duration `yaml:"interval" json:"interval"`
}

// Endpoint defines a single health check target.
type Endpoint struct {
	Name                 string            `yaml:"name" json:"name"`
	URL                  string            `yaml:"url" json:"url"`
	Type                 string            `yaml:"type" json:"type"`     // http, tcp, grpc
	Method               string            `yaml:"method" json:"method"` // GET, POST, HEAD
	Host                 string            `yaml:"host" json:"host"`     // For TCP/gRPC
	Port                 int               `yaml:"port" json:"port"`     // For TCP
	ExpectedStatus       int               `yaml:"expected_status" json:"expected_status"`
	ExpectedBodyContains string            `yaml:"expected_body_contains" json:"expected_body_contains"`
	Headers              map[string]string `yaml:"headers" json:"headers"`
	Body                 string            `yaml:"body" json:"body"`
	Timeout              time.Duration     `yaml:"timeout" json:"timeout"` // Per-endpoint override
}

// Alerts defines notification configuration.
type Alerts struct {
	Slack   *SlackAlert   `yaml:"slack" json:"slack"`
	Webhook *WebhookAlert `yaml:"webhook" json:"webhook"`
}

// SlackAlert configures Slack notifications.
type SlackAlert struct {
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	OnFailure  bool   `yaml:"on_failure" json:"on_failure"`
	OnRecovery bool   `yaml:"on_recovery" json:"on_recovery"`
}

// WebhookAlert configures generic webhook notifications.
type WebhookAlert struct {
	URL        string `yaml:"url" json:"url"`
	OnFailure  bool   `yaml:"on_failure" json:"on_failure"`
	OnRecovery bool   `yaml:"on_recovery" json:"on_recovery"`
}

// LoadFile reads and parses a config file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	cfg := &Config{}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yml", ".yaml":
		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, fmt.Errorf("parse YAML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, fmt.Errorf("parse JSON config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format: %s (use .yml, .yaml, or .json)", ext)
	}

	// Apply defaults
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		if ep.Type == "" {
			ep.Type = "http"
		}
		if ep.Method == "" {
			ep.Method = "GET"
		}
		if ep.ExpectedStatus == 0 && ep.Type == "http" {
			ep.ExpectedStatus = 200
		}
		if ep.Name == "" {
			ep.Name = ep.URL
			if ep.URL == "" {
				ep.Name = fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			}
		}
	}

	return cfg, nil
}
