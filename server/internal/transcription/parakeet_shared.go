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

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// SharedParakeetWorker manages a single persistent Python subprocess for batch transcription
// All pipelines share this worker to avoid loading the model multiple times
type SharedParakeetWorker struct {
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
}

// ParakeetBatchRequest is the JSON message sent to the subprocess
type ParakeetBatchRequest struct {
	Audio      string `json:"audio"`       // Base64 encoded float32 array
	SampleRate int    `json:"sample_rate"` // Always 16000
}

// ParakeetBatchResponse is the JSON message received from the subprocess
type ParakeetBatchResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// NewSharedParakeetWorker creates a batch Parakeet worker
func NewSharedParakeetWorker(config ParakeetConfig) (*SharedParakeetWorker, error) {
	log := config.Logger.With("parakeet-batch")

	// Use batch script
	scriptPath := config.ScriptPath
	if scriptPath == "" {
		if runtime.GOOS == "darwin" {
			scriptPath = filepath.Join("scripts", "parakeet_worker.py")
		} else {
			// Fall back to mock on Linux for testing
			scriptPath = filepath.Join("scripts", "parakeet_mock.py")
		}
	}

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("Parakeet batch script not found at %s: %w", scriptPath, err)
	}

	pythonPath := config.PythonPath
	if pythonPath == "" {
		pythonPath = "python3"
	}

	// Create command
	cmd := exec.Command(pythonPath, scriptPath, config.ModelPath)

	// Setup environment - ensure FFmpeg is in PATH
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

	worker := &SharedParakeetWorker{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		stderr:    bufio.NewReader(stderr),
		log:       log,
		modelPath: config.ModelPath,
		doneChan:  make(chan struct{}),
	}

	// Start the subprocess
	if err := worker.start(); err != nil {
		return nil, err
	}

	return worker, nil
}

// start launches the Python subprocess
func (w *SharedParakeetWorker) start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	w.log.Info("Starting batch Parakeet subprocess with model: %s", w.modelPath)

	// Start the process
	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Parakeet subprocess: %w", err)
	}

	w.started = true

	// Monitor stderr in background
	go w.monitorStderr()

	// Monitor process health
	go w.monitorProcess()

	// Test readiness with an empty request
	if err := w.waitForReady(); err != nil {
		w.cmd.Process.Kill()
		return fmt.Errorf("Parakeet failed to initialize: %w", err)
	}

	w.log.Info("Batch Parakeet subprocess ready")
	return nil
}

// waitForReady tests the batch protocol with an empty request
func (w *SharedParakeetWorker) waitForReady() error {
	timeout := time.After(60 * time.Second)

	// Send empty request to test readiness
	testReq := ParakeetBatchRequest{
		Audio:      "",
		SampleRate: 16000,
	}

	encoder := json.NewEncoder(w.stdin)
	decoder := json.NewDecoder(w.stdout)

	if err := encoder.Encode(testReq); err != nil {
		return fmt.Errorf("failed to send test request: %w", err)
	}

	respChan := make(chan error, 1)
	go func() {
		var resp ParakeetBatchResponse
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
		return err
	case <-timeout:
		return fmt.Errorf("timeout waiting for Parakeet to initialize")
	}
}

// monitorStderr logs stderr output
func (w *SharedParakeetWorker) monitorStderr() {
	scanner := bufio.NewScanner(w.stderr)
	for scanner.Scan() {
		w.log.Debug("Parakeet stderr: %s", scanner.Text())
	}
}

// monitorProcess waits for process exit
func (w *SharedParakeetWorker) monitorProcess() {
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

// Transcribe sends audio to the subprocess and returns transcription
func (w *SharedParakeetWorker) Transcribe(samples []float32) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return "", fmt.Errorf("Parakeet subprocess not running")
	}

	// Encode audio to base64
	audioBase64 := encodeAudioToBase64(samples)

	// Create request
	req := ParakeetBatchRequest{
		Audio:      audioBase64,
		SampleRate: 16000,
	}

	// Send request
	encoder := json.NewEncoder(w.stdin)
	if err := encoder.Encode(req); err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(w.stdout)
	var resp ParakeetBatchResponse
	if err := decoder.Decode(&resp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return "", fmt.Errorf("Parakeet error: %s", resp.Error)
	}

	return resp.Text, nil
}

// Close shuts down the worker
func (w *SharedParakeetWorker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	w.log.Info("Shutting down batch Parakeet subprocess")

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
		w.log.Warn("Force killing batch Parakeet subprocess")
		w.cmd.Process.Kill()
	}

	return nil
}

// Helper function for encoding audio to base64
func encodeAudioToBase64(samples []float32) string {
	buf := make([]byte, len(samples)*4)
	for i, sample := range samples {
		bits := math.Float32bits(sample)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
