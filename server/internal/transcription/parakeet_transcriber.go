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

// ParakeetTranscriber manages a Python subprocess running Parakeet MLX
type ParakeetTranscriber struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderr    *bufio.Reader
	mu        sync.Mutex
	log       *logger.ContextLogger
	modelPath string

	// Process management
	started  bool
	startErr error
	doneChan chan struct{}
}

// ParakeetRequest is the JSON message sent to the subprocess
type ParakeetRequest struct {
	Audio      string `json:"audio"`       // Base64 encoded float32 array
	SampleRate int    `json:"sample_rate"` // Always 16000
}

// ParakeetResponse is the JSON message received from the subprocess
type ParakeetResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// NewParakeetTranscriber creates a new Parakeet transcriber with subprocess
func NewParakeetTranscriber(config ParakeetConfig) (*ParakeetTranscriber, error) {
	log := config.Logger.With("parakeet")

	// Determine which Python script to use
	scriptPath := getParakeetScript()

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("Parakeet worker script not found at %s: %w", scriptPath, err)
	}

	// Create command
	cmd := exec.Command("python3", scriptPath, config.ModelPath)

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

	pt := &ParakeetTranscriber{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		stderr:    bufio.NewReader(stderr),
		log:       log,
		modelPath: config.ModelPath,
		doneChan:  make(chan struct{}),
	}

	// Start the subprocess
	if err := pt.start(); err != nil {
		return nil, err
	}

	return pt, nil
}

// start launches the Python subprocess and waits for it to be ready
func (pt *ParakeetTranscriber) start() error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.started {
		return nil
	}

	pt.log.Info("Starting Parakeet subprocess with model: %s", pt.modelPath)

	// Start the process
	if err := pt.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Parakeet subprocess: %w", err)
	}

	pt.started = true

	// Monitor stderr in background
	go pt.monitorStderr()

	// Monitor process health
	go pt.monitorProcess()

	// Wait for model to load (check for ready signal)
	if err := pt.waitForReady(); err != nil {
		pt.cmd.Process.Kill()
		return fmt.Errorf("Parakeet failed to initialize: %w", err)
	}

	pt.log.Info("Parakeet subprocess ready")
	return nil
}

// waitForReady sends a test request and waits for response
func (pt *ParakeetTranscriber) waitForReady() error {
	// Set a timeout for initialization
	timeout := time.After(60 * time.Second) // Model loading can take time

	// Create a temporary request/response to verify the process is ready
	testSamples := make([]float32, 16000) // 1 second of silence
	testReq := ParakeetRequest{
		Audio:      encodeAudioToBase64(testSamples),
		SampleRate: 16000,
	}

	encoder := json.NewEncoder(pt.stdin)

	// Send test request
	if err := encoder.Encode(testReq); err != nil {
		return fmt.Errorf("failed to send test request: %w", err)
	}

	// Wait for response or timeout
	respChan := make(chan error, 1)
	go func() {
		var resp ParakeetResponse
		decoder := json.NewDecoder(pt.stdout)
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
func (pt *ParakeetTranscriber) monitorStderr() {
	scanner := bufio.NewScanner(pt.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		pt.log.Debug("Parakeet stderr: %s", line)
	}
}

// monitorProcess waits for the process to exit and updates state
func (pt *ParakeetTranscriber) monitorProcess() {
	err := pt.cmd.Wait()
	pt.mu.Lock()
	pt.startErr = err
	pt.started = false
	pt.mu.Unlock()

	if err != nil {
		pt.log.Error("Parakeet subprocess exited with error: %v", err)
	} else {
		pt.log.Info("Parakeet subprocess exited normally")
	}

	close(pt.doneChan)
}

// Transcribe sends audio to the subprocess and returns transcription
func (pt *ParakeetTranscriber) Transcribe(audioSamples []float32) (string, error) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Check if process is still running
	select {
	case <-pt.doneChan:
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
	encoder := json.NewEncoder(pt.stdin)
	if err := encoder.Encode(req); err != nil {
		return "", fmt.Errorf("failed to send request to Parakeet: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(pt.stdout)
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
func (pt *ParakeetTranscriber) Close() error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.started {
		return nil
	}

	pt.log.Info("Shutting down Parakeet subprocess")

	// Close stdin to signal shutdown
	pt.stdin.Close()

	// Give process time to exit gracefully
	done := make(chan struct{})
	go func() {
		<-pt.doneChan
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill after timeout
		pt.log.Warn("Force killing Parakeet subprocess")
		pt.cmd.Process.Kill()
	}

	return nil
}

// getParakeetScript returns the path to the appropriate Python script
func getParakeetScript() string {
	// Use real script on macOS, mock on Linux
	if runtime.GOOS == "darwin" {
		return filepath.Join("scripts", "parakeet_worker.py")
	}
	// Use mock for testing on Linux
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
