package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// Server handles the HTTP control API
type Server struct {
	bindAddr    string
	logger      *logger.ContextLogger
	server      *http.Server
	onStart     func() error
	onStop      func() error
	isRunning   bool
	isRunningMu sync.RWMutex

	// WebSocket connections for transcription streaming
	wsClients   map[*websocket.Conn]bool
	wsClientsMu sync.RWMutex
	wsUpgrader  websocket.Upgrader
}

// New creates a new API server
func New(bindAddr string, log *logger.Logger) *Server {
	return &Server{
		bindAddr:  bindAddr,
		logger:    log.With("api"),
		wsClients: make(map[*websocket.Conn]bool),
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
	defer s.wsClientsMu.RUnlock()

	for conn := range s.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			s.logger.Error("Failed to send to WebSocket client: %v", err)
		}
	}
}
