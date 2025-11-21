package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Subprocess manages the Swift UI subprocess
type Subprocess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

// NewSubprocess spawns the Swift UI process
func NewSubprocess(binaryPath string) (*Subprocess, error) {
	// If binaryPath is empty, try to find the binary
	if binaryPath == "" {
		binaryPath = findBinary()
	}

	cmd := exec.Command(binaryPath)

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

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start UI process: %w", err)
	}

	w := &Subprocess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	// Monitor stderr for errors and READY signal
	go w.monitorStderr()

	// Wait for READY signal
	if err := w.waitForReady(); err != nil {
		w.Close()
		return nil, err
	}

	return w, nil
}

// findBinary locates the richardtate-ui binary
func findBinary() string {
	// Try locations in order:
	// 1. Same directory as client binary (daemon install location)
	// 2. ~/.config/richardtate/bin/ (daemon install)
	// 3. Project bin/ directory (build-mac.sh output)
	// 4. Development build location (ui-macos/.build/release/)
	// 5. System install (/usr/local/bin)

	execPath, err := os.Executable()
	if err == nil {
		// Same directory as client binary
		dir := filepath.Dir(execPath)
		path := filepath.Join(dir, "richardtate-ui")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Daemon install location (~/.config/richardtate/bin/)
	if homeDir, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(homeDir, ".config", "richardtate", "bin", "richardtate-ui")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Project bin/ directory (where build-mac.sh puts it)
	if wd, err := os.Getwd(); err == nil {
		// Try relative to current directory
		path := filepath.Join(wd, "bin", "richardtate-ui")
		if _, err := os.Stat(path); err == nil {
			return path
		}

		// Try relative to parent (if we're in client/ or server/)
		path = filepath.Join(wd, "..", "bin", "richardtate-ui")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Development location
	if wd, err := os.Getwd(); err == nil {
		path := filepath.Join(wd, "..", "ui-macos", ".build", "release", "richardtate-ui")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// System install
	return "/usr/local/bin/richardtate-ui"
}

// waitForReady waits for the READY signal from stderr
func (w *Subprocess) waitForReady() error {
	timeout := time.After(5 * time.Second)
	ready := make(chan bool)

	go func() {
		scanner := bufio.NewScanner(w.stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "READY" {
				ready <- true
				return
			}
		}
	}()

	select {
	case <-ready:
		return nil
	case <-timeout:
		return fmt.Errorf("UI process did not start within timeout")
	}
}

// monitorStderr logs any errors from the Swift process
func (w *Subprocess) monitorStderr() {
	scanner := bufio.NewScanner(w.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "READY" {
			fmt.Fprintf(os.Stderr, "[Swift UI] %s\n", line)
		}
	}
}

// sendCommand sends a JSON command to the Swift UI
func (w *Subprocess) sendCommand(cmd map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	_, err = w.stdin.Write(append(data, '\n'))
	return err
}

// Show displays the preview window
func (w *Subprocess) Show() error {
	return w.sendCommand(map[string]interface{}{"command": "show"})
}

// Hide hides the preview window
func (w *Subprocess) Hide() error {
	return w.sendCommand(map[string]interface{}{"command": "hide"})
}

// SetText updates the preview text
func (w *Subprocess) SetText(text string) error {
	return w.sendCommand(map[string]interface{}{
		"command": "setText",
		"text":    text,
	})
}

// SetProcessing updates the processing indicator
func (w *Subprocess) SetProcessing(processing bool) error {
	return w.sendCommand(map[string]interface{}{
		"command":    "setProcessing",
		"processing": processing,
	})
}

// ClearText clears the preview text
func (w *Subprocess) ClearText() error {
	return w.sendCommand(map[string]interface{}{"command": "clearText"})
}

// ShowCalibration shows the calibration window
func (w *Subprocess) ShowCalibration() error {
	return w.sendCommand(map[string]interface{}{"command": "showCalibration"})
}

// HideCalibration hides the calibration window
func (w *Subprocess) HideCalibration() error {
	return w.sendCommand(map[string]interface{}{"command": "hideCalibration"})
}

// SetCalibrationStep sets the calibration wizard step
func (w *Subprocess) SetCalibrationStep(step int) error {
	return w.sendCommand(map[string]interface{}{
		"command": "setCalibrationStep",
		"step":    step,
	})
}

// SetCalibrationMessage sets the calibration message
func (w *Subprocess) SetCalibrationMessage(message string) error {
	return w.sendCommand(map[string]interface{}{
		"command": "setCalibrationMessage",
		"message": message,
	})
}

// SetCalibrationProgress updates the progress bar (0.0 to 1.0)
func (w *Subprocess) SetCalibrationProgress(value float64) error {
	return w.sendCommand(map[string]interface{}{
		"command": "setCalibrationProgress",
		"value":   value,
	})
}

// SetCalibrationStats displays the final calibration results
func (w *Subprocess) SetCalibrationStats(backgroundP95, speechP5, recommended float64) error {
	return w.sendCommand(map[string]interface{}{
		"command":       "setCalibrationStats",
		"backgroundP95": backgroundP95,
		"speechP5":      speechP5,
		"recommended":   recommended,
	})
}

// Close terminates the Swift UI process
func (w *Subprocess) Close() error {
	// Send quit command
	w.sendCommand(map[string]interface{}{"command": "quit"})

	// Wait for process to exit (with timeout)
	done := make(chan error)
	go func() {
		done <- w.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		// Force kill if not responding
		w.cmd.Process.Kill()
		return fmt.Errorf("UI process did not exit gracefully, killed")
	}
}
