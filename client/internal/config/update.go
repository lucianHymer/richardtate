package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateVADThreshold updates the VAD energy threshold in the config file
// This function is shared between CLI calibration and API calibration endpoints
func UpdateVADThreshold(configPath string, threshold float64) error {
	// Check if file exists first
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config file not found at '%s': %w", configPath, err)
	}

	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %w", configPath, err)
	}

	// Parse YAML
	var configData map[string]interface{}
	if err := yaml.Unmarshal(data, &configData); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Navigate to transcription.vad.energy_threshold
	transcription, ok := configData["transcription"].(map[string]interface{})
	if !ok {
		// Create transcription section if it doesn't exist
		transcription = make(map[string]interface{})
		configData["transcription"] = transcription
	}

	// Navigate to vad subsection
	vad, ok := transcription["vad"].(map[string]interface{})
	if !ok {
		// Create vad subsection if it doesn't exist
		vad = make(map[string]interface{})
		transcription["vad"] = vad
	}

	// Update threshold in nested structure
	vad["energy_threshold"] = threshold

	// Write back to file
	output, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
