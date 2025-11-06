package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/gorilla/websocket"
	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"gopkg.in/yaml.v3"
)

// Server handles the HTTP control API
type Server struct {
	bindAddr    string
	logger      *logger.ContextLogger
	baseLog     *logger.Logger
	server      *http.Server
	onStart     func() error
	onStop      func() error
	isRunning   bool
	isRunningMu sync.RWMutex

	// WebSocket connections for transcription streaming
	wsClients   map[*websocket.Conn]bool
	wsClientsMu sync.RWMutex
	wsUpgrader  websocket.Upgrader

	// Calibration support
	cfg         *config.Config
	configPath  string
	serverURL   string
}

// New creates a new API server
func New(bindAddr string, log *logger.Logger, cfg *config.Config, configPath string) *Server {
	// Convert ws:// to http:// for REST API
	serverURL := cfg.Server.URL
	if len(serverURL) > 5 && serverURL[:5] == "ws://" {
		serverURL = "http://" + serverURL[5:]
	} else if len(serverURL) > 6 && serverURL[:6] == "wss://" {
		serverURL = "https://" + serverURL[6:]
	}

	return &Server{
		bindAddr:   bindAddr,
		logger:     log.With("api"),
		baseLog:    log,
		cfg:        cfg,
		configPath: configPath,
		serverURL:  serverURL,
		wsClients:  make(map[*websocket.Conn]bool),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for local dev
			},
		},
	}
}

// SetHandlers sets the start/stop handlers
func (s *Server) SetHandlers(onStart, onStop func() error) {
	s.onStart = onStart
	s.onStop = onStop
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/start", s.handleStart)
	mux.HandleFunc("/stop", s.handleStop)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/transcriptions", s.handleTranscriptions)

	// Calibration endpoints
	mux.HandleFunc("/api/calibrate/record", s.handleCalibrateRecord)
	mux.HandleFunc("/api/calibrate/calculate", s.handleCalibrateCalculate)
	mux.HandleFunc("/api/calibrate/save", s.handleCalibrateSave)

	s.server = &http.Server{
		Addr:         s.bindAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting control API on %s", s.bindAddr)
	return s.server.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStart handles start requests
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.isRunningMu.Lock()
	if s.isRunning {
		s.isRunningMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "already_running",
		})
		return
	}
	s.isRunning = true
	s.isRunningMu.Unlock()

	if s.onStart != nil {
		if err := s.onStart(); err != nil {
			s.isRunningMu.Lock()
			s.isRunning = false
			s.isRunningMu.Unlock()

			s.logger.Error("Failed to start: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "started",
	})
}

// handleStop handles stop requests
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.isRunningMu.Lock()
	if !s.isRunning {
		s.isRunningMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_running",
		})
		return
	}
	s.isRunning = false
	s.isRunningMu.Unlock()

	if s.onStop != nil {
		if err := s.onStop(); err != nil {
			s.logger.Error("Failed to stop: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "stopped",
	})
}

// handleStatus handles status requests
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.isRunningMu.RLock()
	running := s.isRunning
	s.isRunningMu.RUnlock()

	response := map[string]interface{}{
		"running":   running,
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTranscriptions upgrades to WebSocket and streams transcriptions
func (s *Server) handleTranscriptions(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed: %v", err)
		return
	}

	s.wsClientsMu.Lock()
	s.wsClients[conn] = true
	s.wsClientsMu.Unlock()

	s.logger.Info("WebSocket client connected")

	// Keep connection alive and handle disconnect
	defer func() {
		s.wsClientsMu.Lock()
		delete(s.wsClients, conn)
		s.wsClientsMu.Unlock()
		conn.Close()
		s.logger.Info("WebSocket client disconnected")
	}()

	// Read messages from client (mainly to detect disconnect)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// BroadcastTranscription sends a transcription chunk to all connected WebSocket clients
func (s *Server) BroadcastTranscription(text string, isFinal bool) {
	message := map[string]interface{}{
		"chunk": text,
		"final": isFinal,
	}

	data, err := json.Marshal(message)
	if err != nil {
		s.logger.Error("Failed to marshal transcription: %v", err)
		return
	}

	s.wsClientsMu.RLock()
	clientCount := len(s.wsClients)
	s.wsClientsMu.RUnlock()

	s.logger.Debug("Broadcasting to %d WebSocket clients: %s (final=%v)", clientCount, text, isFinal)

	s.wsClientsMu.RLock()
	defer s.wsClientsMu.RUnlock()

	for conn := range s.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			s.logger.Error("Failed to send to WebSocket client: %v", err)
		} else {
			s.logger.Debug("Sent chunk to WebSocket client successfully")
		}
	}
}

// AudioStatistics holds energy statistics from server analysis
type AudioStatistics struct {
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Avg         float64 `json:"avg"`
	P5          float64 `json:"p5"`
	P95         float64 `json:"p95"`
	SampleCount int     `json:"sample_count"`
}

// CalibrateRecordRequest is the request for recording calibration audio
type CalibrateRecordRequest struct {
	DurationSeconds int `json:"duration_seconds"`
}

// CalibrateCalculateRequest is the request for calculating threshold
type CalibrateCalculateRequest struct {
	Background AudioStatistics `json:"background"`
	Speech     AudioStatistics `json:"speech"`
}

// CalibrateCalculateResponse is the response with calculated threshold
type CalibrateCalculateResponse struct {
	Threshold                  float64 `json:"threshold"`
	BackgroundFramesAbove      int     `json:"background_frames_above_percent"`
	SpeechFramesAbove          int     `json:"speech_frames_above_percent"`
	RecommendationExplanation  string  `json:"explanation"`
}

// CalibrateSaveRequest is the request for saving threshold to config
type CalibrateSaveRequest struct {
	Threshold float64 `json:"threshold"`
}

// CalibrateSaveResponse is the response after saving config
type CalibrateSaveResponse struct {
	Success    bool   `json:"success"`
	ConfigPath string `json:"config_path"`
}

// handleCalibrateRecord records audio and returns energy statistics
func (s *Server) handleCalibrateRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req CalibrateRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate duration
	if req.DurationSeconds < 1 || req.DurationSeconds > 30 {
		http.Error(w, "Duration must be between 1 and 30 seconds", http.StatusBadRequest)
		return
	}

	s.logger.Info("Recording calibration audio for %d seconds", req.DurationSeconds)

	// Initialize audio capture
	capturer, err := audio.New(20, s.cfg.Audio.DeviceName, s.baseLog)
	if err != nil {
		s.logger.Error("Failed to initialize audio capture: %v", err)
		http.Error(w, fmt.Sprintf("Failed to initialize audio: %v", err), http.StatusInternalServerError)
		return
	}
	defer capturer.Close()

	// Start recording
	if err := capturer.Start(); err != nil {
		s.logger.Error("Failed to start capture: %v", err)
		http.Error(w, fmt.Sprintf("Failed to start recording: %v", err), http.StatusInternalServerError)
		return
	}
	defer capturer.Stop()

	// Collect audio for specified duration
	var allAudio []byte
	endTime := time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)

	for time.Now().Before(endTime) {
		select {
		case chunk := <-capturer.Chunks():
			allAudio = append(allAudio, chunk.Data...)
		case <-time.After(100 * time.Millisecond):
			// Continue waiting
		}
	}

	// Drain remaining chunks
	for {
		select {
		case chunk := <-capturer.Chunks():
			allAudio = append(allAudio, chunk.Data...)
		default:
			goto doneCollecting
		}
	}
doneCollecting:

	s.logger.Info("Collected %d bytes of audio, analyzing...", len(allAudio))

	// Send to server for analysis
	stats, err := s.analyzeAudio(allAudio)
	if err != nil {
		s.logger.Error("Failed to analyze audio: %v", err)
		http.Error(w, fmt.Sprintf("Failed to analyze audio: %v", err), http.StatusInternalServerError)
		return
	}

	// Return statistics
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleCalibrateCalculate calculates recommended threshold from background and speech stats
func (s *Server) handleCalibrateCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req CalibrateCalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	s.logger.Info("Calculating threshold from background (avg=%.1f, p95=%.1f) and speech (avg=%.1f, p5=%.1f)",
		req.Background.Avg, req.Background.P95, req.Speech.Avg, req.Speech.P5)

	// Calculate recommended threshold (same logic as calibrate.go)
	// Use background P95 * 1.5 for 50% safety margin
	recommendedThreshold := req.Background.P95 * 1.5

	// Safety check: ensure threshold is reasonable
	minThreshold := req.Background.Avg * 2
	if recommendedThreshold < minThreshold {
		recommendedThreshold = minThreshold
	}

	// Estimate frame percentages based on percentiles
	backgroundFramesAbove := 5 // ~5% above p95
	if recommendedThreshold <= req.Background.P95 {
		backgroundFramesAbove = 50 // much higher if threshold is low
	}

	speechFramesAbove := 95 // ~95% above p5
	if recommendedThreshold >= req.Speech.P5 {
		speechFramesAbove = 50 // lower if threshold is high
	}

	explanation := fmt.Sprintf("Calculated as background P95 (%.1f) × 1.5 for 50%% safety margin", req.Background.P95)

	response := CalibrateCalculateResponse{
		Threshold:                 recommendedThreshold,
		BackgroundFramesAbove:     backgroundFramesAbove,
		SpeechFramesAbove:         speechFramesAbove,
		RecommendationExplanation: explanation,
	}

	s.logger.Info("Recommended threshold: %.1f (~%d%% background, ~%d%% speech above)",
		recommendedThreshold, backgroundFramesAbove, speechFramesAbove)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCalibrateSave saves the threshold to client config file
func (s *Server) handleCalibrateSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req CalibrateSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	s.logger.Info("Saving threshold %.1f to config", req.Threshold)

	// Use the config path that was passed to the server
	configPath := s.configPath

	// Update config file
	if err := s.updateClientConfig(configPath, req.Threshold); err != nil {
		s.logger.Error("Failed to save config: %v", err)
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	response := CalibrateSaveResponse{
		Success:    true,
		ConfigPath: configPath,
	}

	s.logger.Info("Config saved successfully to %s", configPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// analyzeAudio sends audio to server for energy analysis
func (s *Server) analyzeAudio(audioData []byte) (*AudioStatistics, error) {
	url := s.serverURL + "/api/v1/analyze-audio"

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

// updateClientConfig updates the client config file with new threshold
func (s *Server) updateClientConfig(configPath string, threshold float64) error {
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

	// Navigate to transcription.vad_energy_threshold
	transcription, ok := configData["transcription"].(map[string]interface{})
	if !ok {
		// Create transcription section if it doesn't exist
		transcription = make(map[string]interface{})
		configData["transcription"] = transcription
	}

	// Update threshold directly (flat structure)
	transcription["vad_energy_threshold"] = threshold

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
