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
		LogLevel    string `yaml:"log_level"`  // debug, info, warn, error, fatal
		LogFormat   string `yaml:"log_format"` // text, json
	} `yaml:"server"`

	WebRTC struct {
		ICEServers []ICEServer `yaml:"ice_servers"`
	} `yaml:"webrtc"`

	Transcription struct {
		Engine string `yaml:"engine"` // ASR engine: "whisper" or "parakeet" (default: "whisper")

		// Whisper-specific settings
		Whisper struct {
			ModelPath string `yaml:"model_path"` // Path to GGML model file
			Threads   int    `yaml:"threads"`    // Number of threads for processing
			Language  string `yaml:"language"`   // Language code (e.g., "en")
		} `yaml:"whisper"`

		// Parakeet-specific settings
		Parakeet struct {
			ModelID    string `yaml:"model_id"`    // Model identifier (e.g., "mlx-community/parakeet-tdt-0.6b-v3")
			ScriptPath string `yaml:"script_path"` // Path to parakeet_worker.py script
			PythonPath string `yaml:"python_path"` // Path to Python interpreter (default: "python3")
		} `yaml:"parakeet"`

		// Shared settings
		EnableDebugWAV bool `yaml:"enable_debug_wav"` // Save chunks as WAV files for debugging
	} `yaml:"transcription"`

	NoiseSuppression struct {
		ModelPath string `yaml:"model_path"`
	} `yaml:"noise_suppression"`

	VAD struct {
		EnergyThreshold    float64 `yaml:"energy_threshold"`      // VAD energy threshold (default: 100.0)
		SilenceThresholdMs int     `yaml:"silence_threshold_ms"`  // Silence duration to trigger chunk (default: 1000ms)
		MinChunkDurationMs int     `yaml:"min_chunk_duration_ms"` // Minimum chunk duration (default: 500ms)
		MaxChunkDurationMs int     `yaml:"max_chunk_duration_ms"` // Maximum chunk duration (default: 30000ms)
	} `yaml:"vad"`
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
	cfg.Server.LogLevel = "info"
	cfg.Server.LogFormat = "text"
	return cfg
}
