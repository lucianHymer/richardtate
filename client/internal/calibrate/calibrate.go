package calibrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"gopkg.in/yaml.v3"
)

// AudioStatistics holds energy statistics from server
type AudioStatistics struct {
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Avg         float64 `json:"avg"`
	P5          float64 `json:"p5"`
	P95         float64 `json:"p95"`
	SampleCount int     `json:"sample_count"`
}

// Wizard runs the calibration wizard
type Wizard struct {
	cfg         *config.Config
	baseLog     *logger.Logger
	log         *logger.ContextLogger
	serverURL   string
	capturer    *audio.Capturer
}

// NewWizard creates a new calibration wizard
func NewWizard(cfg *config.Config, log *logger.Logger) (*Wizard, error) {
	// Convert ws:// to http:// for REST API
	serverURL := cfg.Server.URL
	if serverURL[:5] == "ws://" {
		serverURL = "http://" + serverURL[5:]
	} else if serverURL[:6] == "wss://" {
		serverURL = "https://" + serverURL[6:]
	}

	return &Wizard{
		cfg:       cfg,
		baseLog:   log,
		log:       log.With("calibrate"),
		serverURL: serverURL,
	}, nil
}

// Run executes the calibration wizard
func (w *Wizard) Run(clientConfigPath string, autoSave bool) error {
	fmt.Println()
	fmt.Println("🎤 VAD Calibration Wizard")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Initialize audio capture
	w.log.Info("Initializing audio capture...")
	capturer, err := audio.New(20, w.cfg.Audio.DeviceName, w.baseLog)
	if err != nil {
		return fmt.Errorf("failed to initialize audio capture: %w", err)
	}
	w.capturer = capturer
	defer capturer.Close()

	// Step 1: Record background noise
	fmt.Println("Step 1/3: Background Noise Recording")
	fmt.Println("  Be quiet and don't speak.")
	fmt.Print("  Press Enter when ready...")
	fmt.Scanln()

	fmt.Println("  Recording for 5 seconds...")
	backgroundAudio, err := w.recordAudio(5 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to record background: %w", err)
	}

	// Analyze background
	w.log.Info("Analyzing background noise...")
	backgroundStats, err := w.analyzeAudio(backgroundAudio)
	if err != nil {
		return fmt.Errorf("failed to analyze background: %w", err)
	}

	fmt.Printf("  ✓ Done\n\n")

	// Step 2: Record speech
	fmt.Println("Step 2/3: Speech Recording")
	fmt.Println("  Speak normally into the microphone.")
	fmt.Print("  Press Enter when ready...")
	fmt.Scanln()

	fmt.Println("  Recording for 5 seconds...")
	speechAudio, err := w.recordAudio(5 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to record speech: %w", err)
	}

	// Analyze speech
	w.log.Info("Analyzing speech...")
	speechStats, err := w.analyzeAudio(speechAudio)
	if err != nil {
		return fmt.Errorf("failed to analyze speech: %w", err)
	}

	fmt.Printf("  ✓ Done\n\n")

	// Step 3: Calculate recommendation
	fmt.Println("Step 3/3: Analysis")
	visualizeComparison(backgroundStats, speechStats)

	// Calculate recommended threshold
	// Research shows threshold should be above background noise with safety margin
	// Common approaches:
	// 1. background_p95 * 1.5 (50% margin)
	// 2. background_p95 + (speech_avg - background_avg) * 0.3 (30% into the gap)
	// We use approach 1 with a minimum of background_avg * 2 for very quiet environments
	recommendedThreshold := backgroundStats.P95 * 1.5

	// Safety check: ensure threshold is reasonable
	minThreshold := backgroundStats.Avg * 2
	if recommendedThreshold < minThreshold {
		recommendedThreshold = minThreshold
	}

	fmt.Printf("\n  📊 Recommended threshold: %.0f\n", recommendedThreshold)
	fmt.Printf("     (background P95 × 1.5 for 50%% safety margin)\n")

	// Show what this means
	backgroundFramesAboveThreshold := 0
	speechFramesAboveThreshold := 0
	// Estimate based on percentiles
	if recommendedThreshold > backgroundStats.P95 {
		backgroundFramesAboveThreshold = 5 // ~5% above p95
	} else {
		backgroundFramesAboveThreshold = 50 // much higher
	}
	if recommendedThreshold < speechStats.P5 {
		speechFramesAboveThreshold = 95 // ~95% above p5
	} else {
		speechFramesAboveThreshold = 50 // lower
	}

	fmt.Printf("     (~%d%% of background frames, ~%d%% of speech frames above threshold)\n\n",
		backgroundFramesAboveThreshold, speechFramesAboveThreshold)

	// Save to config
	// Auto-detect server config path
	defaultConfigPath := "config.yaml"
	if _, err := os.Stat("server/config.yaml"); err == nil {
		defaultConfigPath = "server/config.yaml"
	} else if _, err := os.Stat("../server/config.yaml"); err == nil {
		defaultConfigPath = "../server/config.yaml"
	}

	var serverConfigPath string
	if autoSave {
		fmt.Printf("  💾 Enter server config path [%s]: ", defaultConfigPath)
		fmt.Scanln(&serverConfigPath)
		if serverConfigPath == "" {
			serverConfigPath = defaultConfigPath
		}

		if err := w.updateServerConfig(serverConfigPath, recommendedThreshold); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("  ✓ Config updated successfully!")
	} else {
		fmt.Print("  💾 Save to server config? [Y/n] ")
		var response string
		fmt.Scanln(&response)
		if response == "" || response == "Y" || response == "y" {
			fmt.Printf("  📄 Enter server config path [%s]: ", defaultConfigPath)
			fmt.Scanln(&serverConfigPath)
			if serverConfigPath == "" {
				serverConfigPath = defaultConfigPath
			}

			if err := w.updateServerConfig(serverConfigPath, recommendedThreshold); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			fmt.Println("  ✓ Config updated successfully!")
		} else {
			fmt.Printf("  ℹ️  Not saved. You can manually set energy_threshold: %.0f in server config.yaml\n", recommendedThreshold)
		}
	}

	fmt.Println()
	return nil
}

// recordAudio records audio for the specified duration
func (w *Wizard) recordAudio(duration time.Duration) ([]byte, error) {
	if err := w.capturer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start capture: %w", err)
	}
	defer w.capturer.Stop()

	var allAudio []byte
	endTime := time.Now().Add(duration)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	fmt.Print("  ")

	// Collect audio until duration expires
	for time.Now().Before(endTime) {
		select {
		case chunk := <-w.capturer.Chunks():
			allAudio = append(allAudio, chunk.Data...)
		case <-ticker.C:
			fmt.Print(".")
		case <-time.After(100 * time.Millisecond):
			// Continue waiting
		}
	}

	fmt.Println() // Newline after dots

	// Drain remaining chunks
	for {
		select {
		case chunk := <-w.capturer.Chunks():
			allAudio = append(allAudio, chunk.Data...)
		default:
			return allAudio, nil
		}
	}
}

// analyzeAudio sends audio to server for analysis
func (w *Wizard) analyzeAudio(audioData []byte) (*AudioStatistics, error) {
	url := w.serverURL + "/api/v1/analyze-audio"

	requestBody := map[string]interface{}{
		"audio": audioData,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned error: %s - %s", resp.Status, string(body))
	}

	var stats AudioStatistics
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &stats, nil
}

// updateServerConfig updates the server config file with new threshold
func (w *Wizard) updateServerConfig(configPath string, threshold float64) error {
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
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Navigate to vad.energy_threshold
	vad, ok := config["vad"].(map[string]interface{})
	if !ok {
		// Create vad section if it doesn't exist
		vad = make(map[string]interface{})
		config["vad"] = vad
	}

	// Update threshold
	vad["energy_threshold"] = threshold

	// Write back to file
	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// visualizeComparison shows a visual comparison of background vs speech energy
func visualizeComparison(background, speech *AudioStatistics) {
	fmt.Println("  Background Noise:")
	fmt.Printf("    Min: %.1f  |  Avg: %.1f  |  Max: %.1f  |  P95: %.1f\n",
		background.Min, background.Avg, background.Max, background.P95)

	fmt.Println("\n  Speech:")
	fmt.Printf("    Min: %.1f  |  Avg: %.1f  |  Max: %.1f  |  P5: %.1f\n",
		speech.Min, speech.Avg, speech.Max, speech.P5)

	// Show visual comparison using averages
	maxVal := max(background.Avg, speech.Avg) * 1.2
	if maxVal == 0 {
		maxVal = 1
	}

	bgBar := int((background.Avg / maxVal) * 30)
	speechBar := int((speech.Avg / maxVal) * 30)

	fmt.Println("\n  Visual Comparison (Average Energy):")
	fmt.Println("    Background: " + visualBar(bgBar, 30))
	fmt.Println("    Speech:     " + visualBar(speechBar, 30))
}

// visualBar creates a visual bar chart
func visualBar(filled, total int) string {
	bar := ""
	for i := 0; i < total; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}

// max returns the maximum of two float64 values
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
