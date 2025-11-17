package transcription

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// TestParakeetTranscriber tests the basic functionality with mock subprocess
func TestParakeetTranscriber(t *testing.T) {
	// Skip if on macOS (would use real script, not mock)
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping on macOS - would attempt to use real Parakeet worker")
	}

	// Create test logger
	log := logger.New(logger.Config{
		Level:  logger.LevelDebug,
		Format: logger.FormatText,
		Output: os.Stdout,
	})

	config := ParakeetConfig{
		ModelPath: "mock-model",
		Logger:    log,
	}

	// Create transcriber
	transcriber, err := NewParakeetTranscriber(config)
	if err != nil {
		t.Fatalf("Failed to create Parakeet transcriber: %v", err)
	}
	defer transcriber.Close()

	// Test transcription with 1 second of silence
	samples := make([]float32, 16000)
	text, err := transcriber.Transcribe(samples)
	if err != nil {
		t.Fatalf("Transcription failed: %v", err)
	}

	// Mock should return something
	if text == "" {
		t.Error("Expected non-empty transcription from mock")
	}

	t.Logf("Mock transcription: %s", text)
}

// TestParakeetTranscriberMultipleChunks tests multiple transcription calls
func TestParakeetTranscriberMultipleChunks(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping on macOS - would attempt to use real Parakeet worker")
	}

	log := logger.New(logger.Config{
		Level:  logger.LevelDebug,
		Format: logger.FormatText,
		Output: os.Stdout,
	})

	config := ParakeetConfig{
		ModelPath: "mock-model",
		Logger:    log,
	}

	transcriber, err := NewParakeetTranscriber(config)
	if err != nil {
		t.Fatalf("Failed to create Parakeet transcriber: %v", err)
	}
	defer transcriber.Close()

	// Send multiple chunks
	for i := 0; i < 5; i++ {
		samples := make([]float32, 16000) // 1 second chunks
		text, err := transcriber.Transcribe(samples)
		if err != nil {
			t.Fatalf("Transcription %d failed: %v", i, err)
		}

		if text == "" {
			t.Errorf("Chunk %d returned empty transcription", i)
		}

		t.Logf("Chunk %d: %s", i, text)
	}
}

// TestParakeetTranscriberGracefulShutdown tests that Close() works properly
func TestParakeetTranscriberGracefulShutdown(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping on macOS - would attempt to use real Parakeet worker")
	}

	log := logger.New(logger.Config{
		Level:  logger.LevelDebug,
		Format: logger.FormatText,
		Output: os.Stdout,
	})

	config := ParakeetConfig{
		ModelPath: "mock-model",
		Logger:    log,
	}

	transcriber, err := NewParakeetTranscriber(config)
	if err != nil {
		t.Fatalf("Failed to create Parakeet transcriber: %v", err)
	}

	// Do one transcription
	samples := make([]float32, 16000)
	_, err = transcriber.Transcribe(samples)
	if err != nil {
		t.Fatalf("Transcription failed: %v", err)
	}

	// Close should complete within reasonable time
	done := make(chan struct{})
	go func() {
		err := transcriber.Close()
		if err != nil {
			t.Errorf("Close returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Close() timed out - subprocess didn't shut down")
	}
}

// TestParakeetTranscriberProcessDeath tests behavior when subprocess dies
func TestParakeetTranscriberProcessDeath(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping on macOS - would attempt to use real Parakeet worker")
	}

	log := logger.New(logger.Config{
		Level:  logger.LevelDebug,
		Format: logger.FormatText,
		Output: os.Stdout,
	})

	config := ParakeetConfig{
		ModelPath: "mock-model",
		Logger:    log,
	}

	transcriber, err := NewParakeetTranscriber(config)
	if err != nil {
		t.Fatalf("Failed to create Parakeet transcriber: %v", err)
	}
	defer transcriber.Close()

	// Kill the subprocess
	if err := transcriber.cmd.Process.Kill(); err != nil {
		t.Fatalf("Failed to kill process: %v", err)
	}

	// Wait a moment for process to die
	time.Sleep(100 * time.Millisecond)

	// Transcription should fail with clear error
	samples := make([]float32, 16000)
	_, err = transcriber.Transcribe(samples)
	if err == nil {
		t.Error("Expected error when transcribing after process death")
	}

	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}
