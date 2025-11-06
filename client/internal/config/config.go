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

	Transcription struct {
		VAD struct {
			EnergyThreshold    float64 `yaml:"energy_threshold"`
			SilenceThresholdMs int     `yaml:"silence_threshold_ms"`
			MinChunkDurationMs int     `yaml:"min_chunk_duration_ms"`
			MaxChunkDurationMs int     `yaml:"max_chunk_duration_ms"`
		} `yaml:"vad"`
	} `yaml:"transcription"`

	// Internal field to track config file path for reloading
	filePath string
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

	// Store the file path for reloading
	cfg.filePath = path

	// Set defaults
	if cfg.Client.APIBindAddress == "" {
		cfg.Client.APIBindAddress = "localhost:8081"
	}
	if cfg.Client.DebugLogPath == "" {
		cfg.Client.DebugLogPath = "~/.config/richardtate/debug.log"
	}
	if cfg.Client.DebugLogMaxSize == 0 {
		cfg.Client.DebugLogMaxSize = 8388608 // 8MB
	}
	if cfg.Server.URL == "" {
		cfg.Server.URL = "ws://localhost:8080"
	}

	// Transcription defaults
	if cfg.Transcription.VAD.EnergyThreshold == 0 {
		cfg.Transcription.VAD.EnergyThreshold = 500.0 // Default threshold
	}
	if cfg.Transcription.VAD.SilenceThresholdMs == 0 {
		cfg.Transcription.VAD.SilenceThresholdMs = 1000 // 1 second
	}
	if cfg.Transcription.VAD.MinChunkDurationMs == 0 {
		cfg.Transcription.VAD.MinChunkDurationMs = 500 // 500ms
	}
	if cfg.Transcription.VAD.MaxChunkDurationMs == 0 {
		cfg.Transcription.VAD.MaxChunkDurationMs = 30000 // 30 seconds
	}

	return &cfg, nil
}

// Reload reloads the configuration from disk and updates the current config in-place.
// This allows all components holding a reference to this config to see the updated values
// without requiring a restart or passing new config references around.
func (c *Config) Reload() error {
	if c.filePath == "" {
		return fmt.Errorf("config file path not set, cannot reload")
	}

	// Load fresh config from disk
	newCfg, err := Load(c.filePath)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	// Update all fields in-place to preserve references
	// This ensures all components (WebRTC client, API server, etc.) see the new values
	c.Client = newCfg.Client
	c.Server = newCfg.Server
	c.Audio = newCfg.Audio
	c.Transcription = newCfg.Transcription
	// Keep the same filePath

	return nil
}

// Default returns a default configuration
func Default() *Config {
	cfg := &Config{}
	cfg.Client.APIBindAddress = "localhost:8081"
	cfg.Client.Debug = true
	cfg.Client.DebugLogPath = "~/.config/richardtate/debug.log"
	cfg.Client.DebugLogMaxSize = 8388608
	cfg.Server.URL = "ws://localhost:8080"
	cfg.Transcription.VAD.EnergyThreshold = 500.0
	cfg.Transcription.VAD.SilenceThresholdMs = 1000
	cfg.Transcription.VAD.MinChunkDurationMs = 500
	cfg.Transcription.VAD.MaxChunkDurationMs = 30000
	return cfg
}
