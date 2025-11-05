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
		URL                     string  `yaml:"url"`
		ReconnectDelayMS        int     `yaml:"reconnect_delay_ms"`
		MaxReconnectDelayMS     int     `yaml:"max_reconnect_delay_ms"`
		ReconnectBackoffMultiplier float64 `yaml:"reconnect_backoff_multiplier"`
	} `yaml:"server"`

	Audio struct {
		SampleRate        int    `yaml:"sample_rate"`
		Channels          int    `yaml:"channels"`
		BitsPerSample     int    `yaml:"bits_per_sample"`
		ChunkDurationMS   int    `yaml:"chunk_duration_ms"`
		DeviceName        string `yaml:"device_name"`
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
	if cfg.Server.ReconnectDelayMS == 0 {
		cfg.Server.ReconnectDelayMS = 1000
	}
	if cfg.Server.MaxReconnectDelayMS == 0 {
		cfg.Server.MaxReconnectDelayMS = 30000
	}
	if cfg.Server.ReconnectBackoffMultiplier == 0 {
		cfg.Server.ReconnectBackoffMultiplier = 2.0
	}
	if cfg.Audio.SampleRate == 0 {
		cfg.Audio.SampleRate = 16000
	}
	if cfg.Audio.Channels == 0 {
		cfg.Audio.Channels = 1
	}
	if cfg.Audio.BitsPerSample == 0 {
		cfg.Audio.BitsPerSample = 16
	}
	if cfg.Audio.ChunkDurationMS == 0 {
		cfg.Audio.ChunkDurationMS = 150
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
	cfg.Server.ReconnectDelayMS = 1000
	cfg.Server.MaxReconnectDelayMS = 30000
	cfg.Server.ReconnectBackoffMultiplier = 2.0
	cfg.Audio.SampleRate = 16000
	cfg.Audio.Channels = 1
	cfg.Audio.BitsPerSample = 16
	cfg.Audio.ChunkDurationMS = 150
	return cfg
}
