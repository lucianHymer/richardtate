package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the server configuration
type Config struct {
	Server struct {
		BindAddress string `yaml:"bind_address"`
		Debug       bool   `yaml:"debug"`
	} `yaml:"server"`

	WebRTC struct {
		ICEServers []ICEServer `yaml:"ice_servers"`
	} `yaml:"webrtc"`

	Transcription struct {
		ModelPath  string `yaml:"model_path"`
		Language   string `yaml:"language"`
		Translate  bool   `yaml:"translate"`
		Threads    int    `yaml:"threads"`
		UseGPU     bool   `yaml:"use_gpu"`
	} `yaml:"transcription"`

	NoiseSuppression struct {
		Enabled   bool   `yaml:"enabled"`
		ModelPath string `yaml:"model_path"`
	} `yaml:"noise_suppression"`
}

// ICEServer represents a WebRTC ICE server configuration
type ICEServer struct {
	URLs       []string `yaml:"urls"`
	Username   string   `yaml:"username,omitempty"`
	Credential string   `yaml:"credential,omitempty"`
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
	if cfg.Server.BindAddress == "" {
		cfg.Server.BindAddress = "localhost:8080"
	}

	return &cfg, nil
}

// Default returns a default configuration
func Default() *Config {
	cfg := &Config{}
	cfg.Server.BindAddress = "localhost:8080"
	cfg.Server.Debug = true
	return cfg
}
