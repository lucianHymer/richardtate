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

// SharedParakeetWorker manages a single persistent Python subprocess
// Multiple pipelines can share this worker for transcription requests
type SharedParakeetWorker struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderr    *bufio.Reader
	mu        sync.Mutex // Protects access to stdin/stdout (subprocess is single-threaded)
	log       *logger.ContextLogger
	modelPath string

	// Process management
	started  bool
	startErr error
	doneChan chan struct{}
}

// NewSharedParakeetWorker creates a single persistent Parakeet subprocess
// This should be called once at server startup and shared across all pipelines
func NewSharedParakeetWorker(config ParakeetConfig) (*SharedParakeetWorker, error) {
	log := config.Logger.With("parakeet-shared")

	// Use configured script path, or auto-detect if not provided
	scriptPath := config.ScriptPath
	if scriptPath == "" {
		scriptPath = getParakeetScript()
	}

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("Parakeet worker script not found at %s: %w", scriptPath, err)
	}

	// Use configured Python path, or default to "python3"
	pythonPath := config.PythonPath
	if pythonPath == "" {
		pythonPath = "python3"
	}

	// Create command
	cmd := exec.Command(pythonPath, scriptPath, config.ModelPath)

	// Inherit environment and ensure common FFmpeg locations are in PATH
	env := os.Environ()
	pathSet := false
	for i, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			// Prepend common FFmpeg locations to PATH
			env[i] = "PATH=/opt/homebrew/bin:/usr/local/bin:" + e[5:]
			pathSet = true
			break
		}
	}
	if !pathSet {
		// No PATH in environment, set a default
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

// start launches the Python subprocess and waits for it to be ready
func (w *SharedParakeetWorker) start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	w.log.Info("Starting shared Parakeet subprocess with model: %s", w.modelPath)

	// Debug: Log PATH being passed to subprocess
	for _, e := range w.cmd.Env {
		if len(e) > 5 && e[:5] == "PATH=" {
			w.log.Debug("Subprocess PATH: %s", e[5:])
			break
		}
	}

	// Start the process
	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Parakeet subprocess: %w", err)
	}

	w.started = true

	// Monitor stderr in background
	go w.monitorStderr()

	// Monitor process health
	go w.monitorProcess()

	// Wait for model to load (check for ready signal)
	if err := w.waitForReady(); err != nil {
		w.cmd.Process.Kill()
		return fmt.Errorf("Parakeet failed to initialize: %w", err)
	}

	w.log.Info("Shared Parakeet subprocess ready (will be reused across all pipelines)")
	return nil
}

// waitForReady sends a test request and waits for response
func (w *SharedParakeetWorker) waitForReady() error {
	// Set a timeout for initialization
	timeout := time.After(60 * time.Second) // Model loading can take time

	// Create a temporary request/response to verify the process is ready
	testSamples := make([]float32, 16000) // 1 second of silence
	testReq := ParakeetRequest{
		Audio:      encodeAudioToBase64(testSamples),
		SampleRate: 16000,
	}

	encoder := json.NewEncoder(w.stdin)

	// Send test request
	if err := encoder.Encode(testReq); err != nil {
		return fmt.Errorf("failed to send test request: %w", err)
	}

	// Wait for response or timeout
	respChan := make(chan error, 1)
	go func() {
		var resp ParakeetResponse
		decoder := json.NewDecoder(w.stdout)
		if err := decoder.Decode(&resp); err != nil {
			respChan <- fmt.Errorf("failed to decode test response: %w", err)
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

// monitorStderr logs stderr output from the Python process
func (w *SharedParakeetWorker) monitorStderr() {
	scanner := bufio.NewScanner(w.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		w.log.Debug("Parakeet stderr: %s", line)
	}
}

// monitorProcess waits for the process to exit and updates state
func (w *SharedParakeetWorker) monitorProcess() {
	err := w.cmd.Wait()
	w.mu.Lock()
	w.startErr = err
	w.started = false
	w.mu.Unlock()

	if err != nil {
		w.log.Error("Shared Parakeet subprocess exited with error: %v", err)
	} else {
		w.log.Info("Shared Parakeet subprocess exited normally")
	}

	close(w.doneChan)
}

// Transcribe sends audio to the subprocess and returns transcription
// This method is thread-safe and can be called by multiple pipelines concurrently
// Note: Requests are serialized (mutex-protected) since subprocess handles one request at a time
func (w *SharedParakeetWorker) Transcribe(audioSamples []float32) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if process is still running
	select {
	case <-w.doneChan:
		return "", fmt.Errorf("Parakeet subprocess has terminated")
	default:
	}

	// Encode audio to base64
	audioBase64 := encodeAudioToBase64(audioSamples)

	// Create request
	req := ParakeetRequest{
		Audio:      audioBase64,
		SampleRate: 16000,
	}

	// Send request
	encoder := json.NewEncoder(w.stdin)
	if err := encoder.Encode(req); err != nil {
		return "", fmt.Errorf("failed to send request to Parakeet: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(w.stdout)
	var resp ParakeetResponse
	if err := decoder.Decode(&resp); err != nil {
		return "", fmt.Errorf("failed to read response from Parakeet: %w", err)
	}

	if resp.Error != "" {
		return "", fmt.Errorf("Parakeet error: %s", resp.Error)
	}

	return resp.Text, nil
}

// Close gracefully shuts down the subprocess
func (w *SharedParakeetWorker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	w.log.Info("Shutting down shared Parakeet subprocess")

	// Close stdin to signal shutdown
	w.stdin.Close()

	// Give process time to exit gracefully
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
		w.log.Warn("Force killing shared Parakeet subprocess")
		w.cmd.Process.Kill()
	}

	return nil
}

// getParakeetScript returns the path to the appropriate Python script (fallback only)
// NOTE: This is only used as a fallback when ScriptPath is not configured.
// For daemon use, always configure an absolute path in the config YAML.
func getParakeetScript() string {
	// Use real script on macOS, mock on Linux
	if runtime.GOOS == "darwin" {
		return filepath.Join("scripts", "parakeet_worker.py")
	}
	return filepath.Join("scripts", "parakeet_mock.py")
}

// encodeAudioToBase64 converts float32 samples to base64 string
func encodeAudioToBase64(samples []float32) string {
	// Convert float32 array to bytes
	buf := make([]byte, len(samples)*4)
	for i, sample := range samples {
		bits := math.Float32bits(sample)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
