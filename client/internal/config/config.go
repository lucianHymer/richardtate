package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the client configuration
type Config struct {
	Client struct {
		APIBindAddress string `yaml:"api_bind_address"`
		Debug          bool   `yaml:"debug"`
		DebugLogPath   string `yaml:"debug_log_path"`
		DebugLogMaxSize int   `yaml:"debug_log_max_size"`
	} `yaml:"client"`

	Server struct {
		URL string `yaml:"url"`
	} `yaml:"server"`

	Audio struct {
		DeviceName string `yaml:"device_name"` // Empty = default device
	} `yaml:"audio"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.Client.APIBindAddress == "" {
		cfg.Client.APIBindAddress = "localhost:8081"
	}
	if cfg.Client.DebugLogPath == "" {
		cfg.Client.DebugLogPath = "./debug.log"
	}
	if cfg.Client.DebugLogMaxSize == 0 {
		cfg.Client.DebugLogMaxSize = 8388608 // 8MB
	}
	if cfg.Server.URL == "" {
		cfg.Server.URL = "ws://localhost:8080"
	}

	return &cfg, nil
}

// Default returns a default configuration
func Default() *Config {
	cfg := &Config{}
	cfg.Client.APIBindAddress = "localhost:8081"
	cfg.Client.Debug = true
	cfg.Client.DebugLogPath = "./debug.log"
	cfg.Client.DebugLogMaxSize = 8388608
	cfg.Server.URL = "ws://localhost:8080"
	return cfg
}
