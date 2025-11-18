package transcription

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// SharedParakeetWorkerStreaming manages a single persistent Python subprocess for streaming
// This replaces the batch-based SharedParakeetWorker with a streaming version
type SharedParakeetWorkerStreaming struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderr    *bufio.Reader
	mu        sync.Mutex // Protects stdin/stdout access
	log       *logger.ContextLogger
	modelPath string

	// Process management
	started  bool
	startErr error
	doneChan chan struct{}

	// Active client tracking
	activeClients map[string]*ParakeetClient
	clientsMu     sync.RWMutex
}

// ParakeetClient tracks state for each client's streaming session
type ParakeetClient struct {
	ID          string
	Buffer      []float32 // Accumulate samples until 1 second
	StreamActive bool
}

// ParakeetStreamRequest for streaming protocol
type ParakeetStreamRequest struct {
	Command     string  `json:"command"`                // "start_stream", "add_audio", "end_stream"
	ClientID    string  `json:"client_id"`              // Unique client identifier
	Audio       string  `json:"audio,omitempty"`        // Base64 encoded audio (for add_audio)
	SampleRate  int     `json:"sample_rate,omitempty"`  // Sample rate (for add_audio)
	ContextSize []int   `json:"context_size,omitempty"` // [left, right] context frames (for start_stream)
	Depth       int     `json:"depth,omitempty"`        // Encoder depth (for start_stream)
}

// ParakeetStreamResponse from streaming protocol
type ParakeetStreamResponse struct {
	Status   string `json:"status,omitempty"`   // For start/end responses
	Text     string `json:"text,omitempty"`     // Transcription text
	IsFinal  bool   `json:"is_final,omitempty"` // Whether text is finalized
	ClientID string `json:"client_id"`           // Client this response belongs to
	Error    string `json:"error,omitempty"`     // Error message if any
}

// NewSharedParakeetWorkerStreaming creates a streaming Parakeet worker
func NewSharedParakeetWorkerStreaming(config ParakeetConfig) (*SharedParakeetWorkerStreaming, error) {
	log := config.Logger.With("parakeet-streaming")

	// Use streaming script
	scriptPath := config.ScriptPath
	if scriptPath == "" {
		if runtime.GOOS == "darwin" {
			scriptPath = filepath.Join("scripts", "parakeet_worker_streaming.py")
		} else {
			// For now, fall back to mock on Linux
			// TODO: Create streaming mock
			scriptPath = filepath.Join("scripts", "parakeet_mock.py")
		}
	}

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("Parakeet streaming script not found at %s: %w", scriptPath, err)
	}

	pythonPath := config.PythonPath
	if pythonPath == "" {
		pythonPath = "python3"
	}

	// Create command
	cmd := exec.Command(pythonPath, scriptPath, config.ModelPath)

	// Setup environment
	env := os.Environ()
	pathSet := false
	for i, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			env[i] = "PATH=/opt/homebrew/bin:/usr/local/bin:" + e[5:]
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin")
	}
	cmd.Env = env

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	worker := &SharedParakeetWorkerStreaming{
		cmd:           cmd,
		stdin:         stdin,
		stdout:        bufio.NewReader(stdout),
		stderr:        bufio.NewReader(stderr),
		log:           log,
		modelPath:     config.ModelPath,
		doneChan:      make(chan struct{}),
		activeClients: make(map[string]*ParakeetClient),
	}

	// Start the subprocess
	if err := worker.start(); err != nil {
		return nil, err
	}

	return worker, nil
}

// start launches the Python subprocess
func (w *SharedParakeetWorkerStreaming) start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	w.log.Info("Starting streaming Parakeet subprocess with model: %s", w.modelPath)

	// Start the process
	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Parakeet subprocess: %w", err)
	}

	w.started = true

	// Monitor stderr in background
	go w.monitorStderr()

	// Monitor process health
	go w.monitorProcess()

	// Test readiness with a dummy stream
	if err := w.waitForReady(); err != nil {
		w.cmd.Process.Kill()
		return fmt.Errorf("Parakeet failed to initialize: %w", err)
	}

	w.log.Info("Streaming Parakeet subprocess ready")
	return nil
}

// waitForReady tests the streaming protocol
func (w *SharedParakeetWorkerStreaming) waitForReady() error {
	timeout := time.After(60 * time.Second)

	// Test start_stream
	testReq := ParakeetStreamRequest{
		Command:     "start_stream",
		ClientID:    "test-" + uuid.New().String(),
		ContextSize: []int{256, 256},
		Depth:       1,
	}

	encoder := json.NewEncoder(w.stdin)
	decoder := json.NewDecoder(w.stdout)

	if err := encoder.Encode(testReq); err != nil {
		return fmt.Errorf("failed to send test request: %w", err)
	}

	respChan := make(chan error, 1)
	go func() {
		var resp ParakeetStreamResponse
		if err := decoder.Decode(&resp); err != nil {
			respChan <- fmt.Errorf("failed to decode response: %w", err)
		} else if resp.Error != "" {
			respChan <- fmt.Errorf("Parakeet error: %s", resp.Error)
		} else {
			respChan <- nil
		}
	}()

	select {
	case err := <-respChan:
		if err != nil {
			return err
		}
	case <-timeout:
		return fmt.Errorf("timeout waiting for Parakeet to initialize")
	}

	// Clean up test stream
	endReq := ParakeetStreamRequest{
		Command:  "end_stream",
		ClientID: testReq.ClientID,
	}
	encoder.Encode(endReq)

	// Read end response (non-blocking)
	go func() {
		var resp ParakeetStreamResponse
		decoder.Decode(&resp)
	}()

	time.Sleep(100 * time.Millisecond)
	return nil
}

// monitorStderr logs stderr output
func (w *SharedParakeetWorkerStreaming) monitorStderr() {
	scanner := bufio.NewScanner(w.stderr)
	for scanner.Scan() {
		w.log.Debug("Parakeet stderr: %s", scanner.Text())
	}
}

// monitorProcess waits for process exit
func (w *SharedParakeetWorkerStreaming) monitorProcess() {
	err := w.cmd.Wait()
	w.mu.Lock()
	w.startErr = err
	w.started = false
	w.mu.Unlock()

	if err != nil {
		w.log.Error("Parakeet subprocess exited with error: %v", err)
	} else {
		w.log.Info("Parakeet subprocess exited normally")
	}

	close(w.doneChan)
}

// CreateClient creates a new client session for streaming
func (w *SharedParakeetWorkerStreaming) CreateClient() string {
	clientID := uuid.New().String()

	w.clientsMu.Lock()
	w.activeClients[clientID] = &ParakeetClient{
		ID:     clientID,
		Buffer: make([]float32, 0, 16000), // Pre-allocate for 1 second
	}
	w.clientsMu.Unlock()

	w.log.Debug("Created streaming client: %s", clientID)
	return clientID
}

// ProcessAudio processes audio for a client (handles buffering and streaming)
func (w *SharedParakeetWorkerStreaming) ProcessAudio(clientID string, samples []float32) (string, error) {
	w.clientsMu.Lock()
	client, exists := w.activeClients[clientID]
	if !exists {
		w.clientsMu.Unlock()
		return "", fmt.Errorf("no client with ID: %s", clientID)
	}
	w.clientsMu.Unlock()

	// Add samples to client's buffer
	client.Buffer = append(client.Buffer, samples...)

	// Check if we have 1 second of audio (16000 samples at 16kHz)
	if len(client.Buffer) >= 16000 {
		// Extract 1 second chunk
		chunk := client.Buffer[:16000]
		client.Buffer = client.Buffer[16000:]

		// Start stream if needed
		if !client.StreamActive {
			if err := w.startClientStream(clientID); err != nil {
				return "", fmt.Errorf("failed to start stream: %w", err)
			}
			client.StreamActive = true
		}

		// Send audio to stream
		text, _, err := w.addAudioToStream(clientID, chunk)
		if err != nil {
			return "", fmt.Errorf("failed to add audio: %w", err)
		}

		return text, nil
	}

	// Not enough audio yet
	return "", nil
}

// startClientStream starts a streaming context for a client
func (w *SharedParakeetWorkerStreaming) startClientStream(clientID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	req := ParakeetStreamRequest{
		Command:     "start_stream",
		ClientID:    clientID,
		ContextSize: []int{256, 256},
		Depth:       1,
	}

	encoder := json.NewEncoder(w.stdin)
	if err := encoder.Encode(req); err != nil {
		return err
	}

	decoder := json.NewDecoder(w.stdout)
	var resp ParakeetStreamResponse
	if err := decoder.Decode(&resp); err != nil {
		return err
	}

	if resp.Error != "" {
		return fmt.Errorf(resp.Error)
	}

	return nil
}

// addAudioToStream adds audio to an active stream
func (w *SharedParakeetWorkerStreaming) addAudioToStream(clientID string, samples []float32) (string, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	audioBase64 := encodeAudioToBase64(samples)

	req := ParakeetStreamRequest{
		Command:    "add_audio",
		ClientID:   clientID,
		Audio:      audioBase64,
		SampleRate: 16000,
	}

	encoder := json.NewEncoder(w.stdin)
	if err := encoder.Encode(req); err != nil {
		return "", false, err
	}

	decoder := json.NewDecoder(w.stdout)
	var resp ParakeetStreamResponse
	if err := decoder.Decode(&resp); err != nil {
		return "", false, err
	}

	if resp.Error != "" {
		return "", false, fmt.Errorf(resp.Error)
	}

	return resp.Text, resp.IsFinal, nil
}

// CloseClient ends a client's streaming session
func (w *SharedParakeetWorkerStreaming) CloseClient(clientID string) error {
	w.clientsMu.Lock()
	client, exists := w.activeClients[clientID]
	if !exists {
		w.clientsMu.Unlock()
		return nil
	}
	delete(w.activeClients, clientID)
	w.clientsMu.Unlock()

	// Flush remaining audio if any
	if len(client.Buffer) > 0 && client.StreamActive {
		w.addAudioToStream(clientID, client.Buffer)
	}

	// End stream if it was started
	if client.StreamActive {
		w.mu.Lock()
		req := ParakeetStreamRequest{
			Command:  "end_stream",
			ClientID: clientID,
		}
		encoder := json.NewEncoder(w.stdin)
		encoder.Encode(req)
		w.mu.Unlock()
	}

	w.log.Debug("Closed streaming client: %s", clientID)
	return nil
}

// Close shuts down the worker
func (w *SharedParakeetWorkerStreaming) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	w.log.Info("Shutting down streaming Parakeet subprocess")

	// Close all clients
	w.clientsMu.Lock()
	for clientID := range w.activeClients {
		req := ParakeetStreamRequest{
			Command:  "end_stream",
			ClientID: clientID,
		}
		encoder := json.NewEncoder(w.stdin)
		encoder.Encode(req)
	}
	w.activeClients = make(map[string]*ParakeetClient)
	w.clientsMu.Unlock()

	// Close stdin to signal shutdown
	w.stdin.Close()

	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		<-w.doneChan
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill after timeout
		w.log.Warn("Force killing streaming Parakeet subprocess")
		w.cmd.Process.Kill()
	}

	return nil
}

// Transcribe implements the simple interface for compatibility
// Creates a temporary client for one-shot transcription
func (w *SharedParakeetWorkerStreaming) Transcribe(samples []float32) (string, error) {
	// For compatibility - creates a client, processes, and cleans up
	clientID := w.CreateClient()
	defer w.CloseClient(clientID)

	// Process in chunks (may need multiple calls for full transcription)
	var result string
	for i := 0; i < len(samples); i += 3200 { // Process in ~200ms chunks
		end := i + 3200
		if end > len(samples) {
			end = len(samples)
		}

		text, err := w.ProcessAudio(clientID, samples[i:end])
		if err != nil {
			return result, err
		}
		if text != "" {
			if result != "" {
				result += " "
			}
			result += text
		}
	}

	return result, nil
}

// Helper function for encoding
func encodeAudioToBase64(samples []float32) string {
	buf := make([]byte, len(samples)*4)
	for i, sample := range samples {
		bits := math.Float32bits(sample)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return base64.StdEncoding.EncodeToString(buf)
}