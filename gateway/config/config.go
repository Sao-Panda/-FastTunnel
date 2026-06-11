package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the API tunnel.
type Config struct {
	Gateway  GatewayConfig  `yaml:"gateway"`
	Upstream UpstreamConfig `yaml:"upstream"`
	Log      LogConfig      `yaml:"log"`
}

type GatewayConfig struct {
	// ListenAddr is the address the gateway/proxy listens on.
	ListenAddr string `yaml:"listen_addr"`
	// BindIP is the local IP or interface name to bind outbound connections to.
	// Leave empty for default route.
	BindIP string `yaml:"bind_ip"`
	// BindInterface is the network interface name (e.g. "eth1") for outbound traffic.
	// Only used if BindIP is empty.
	BindInterface string `yaml:"bind_interface"`
	// ReadTimeout is the max duration for reading the entire request.
	ReadTimeout time.Duration `yaml:"read_timeout"`
	// WriteTimeout is the max duration for writing the response.
	WriteTimeout time.Duration `yaml:"write_timeout"`
	// IdleTimeout is the max idle time for keep-alive connections.
	IdleTimeout time.Duration `yaml:"idle_timeout"`
	// MaxRetries is the number of retries on upstream failure.
	MaxRetries int `yaml:"max_retries"`
	// RetryDelay is the base delay between retries (exponential backoff).
	RetryDelay time.Duration `yaml:"retry_delay"`
	// TLS is optional TLS configuration.
	TLS *TLSConfig `yaml:"tls"`
	// AuthToken if set requires requests to include this Bearer token.
	AuthToken string `yaml:"auth_token"`
}

type UpstreamConfig struct {
	// URL is the upstream API base URL to forward requests to.
	URL string `yaml:"url"`
	// Timeout for upstream requests.
	Timeout time.Duration `yaml:"timeout"`
	// PreserveHost preserves the original Host header instead of rewriting to upstream.
	PreserveHost bool `yaml:"preserve_host"`
	// AllowedHeaders is the list of headers to forward. Empty = forward all.
	AllowedHeaders []string `yaml:"allowed_headers"`
	// StripHeaders removes these headers from the request.
	StripHeaders []string `yaml:"strip_headers"`
	// SetHeaders adds/overwrites these headers.
	SetHeaders map[string]string `yaml:"set_headers"`
}

type TLSConfig struct {
	// CertFile is the path to the TLS certificate.
	CertFile string `yaml:"cert_file"`
	// KeyFile is the path to the TLS key.
	KeyFile string `yaml:"key_file"`
}

type LogConfig struct {
	// Level: debug, info, warn, error.
	Level string `yaml:"level"`
	// File is the path to the log file. Empty = stdout.
	File string `yaml:"file"`
	// Format: json or text.
	Format string `yaml:"format"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
			MaxRetries:   3,
			RetryDelay:   500 * time.Millisecond,
		},
		Upstream: UpstreamConfig{
			Timeout: 30 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads and parses a YAML config file, merging with defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Gateway.ListenAddr == "" {
		cfg.Gateway.ListenAddr = ":8080"
	}
	if cfg.Upstream.URL == "" {
		return nil, fmt.Errorf("upstream.url is required")
	}

	return cfg, nil
}

// MustLoad loads config or panics.
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(fmt.Sprintf("config load failed: %v", err))
	}
	return cfg
}
