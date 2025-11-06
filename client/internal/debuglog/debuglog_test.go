package debuglog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	// Create temp dir
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}
}

func TestDisabledLogger(t *testing.T) {
	// Empty path should create disabled logger
	logger, err := New("")
	if err != nil {
		t.Fatalf("Failed to create disabled logger: %v", err)
	}
	defer logger.Close()

	// Should not error when logging
	if err := logger.LogChunk("test"); err != nil {
		t.Errorf("Disabled logger should not error: %v", err)
	}
}

func TestLogChunk(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log several chunks
	texts := []string{"Hello world", "This is a test", "Final chunk"}
	for _, text := range texts {
		if err := logger.LogChunk(text); err != nil {
			t.Errorf("Failed to log chunk: %v", err)
		}
	}

	logger.Close()

	// Read and verify log entries
	entries := readLogEntries(t, logPath)
	if len(entries) != len(texts) {
		t.Errorf("Expected %d entries, got %d", len(texts), len(entries))
	}

	for i, entry := range entries {
		if entry.Type != MessageTypeChunk {
			t.Errorf("Entry %d: expected type %s, got %s", i, MessageTypeChunk, entry.Type)
		}
		if entry.Text != texts[i] {
			t.Errorf("Entry %d: expected text %q, got %q", i, texts[i], entry.Text)
		}
		if entry.ChunkID != i+1 {
			t.Errorf("Entry %d: expected chunk_id %d, got %d", i, i+1, entry.ChunkID)
		}
	}
}

func TestLogComplete(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	fullText := "Complete transcription text"
	duration := 45.5

	if err := logger.LogComplete(fullText, duration); err != nil {
		t.Errorf("Failed to log complete: %v", err)
	}

	logger.Close()

	// Read and verify
	entries := readLogEntries(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Type != MessageTypeComplete {
		t.Errorf("Expected type %s, got %s", MessageTypeComplete, entry.Type)
	}
	if entry.FullText != fullText {
		t.Errorf("Expected full_text %q, got %q", fullText, entry.FullText)
	}
	if entry.Duration != duration {
		t.Errorf("Expected duration %f, got %f", duration, entry.Duration)
	}
}

func TestLogInserted(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	location := "Obsidian"
	length := 523

	if err := logger.LogInserted(location, length); err != nil {
		t.Errorf("Failed to log inserted: %v", err)
	}

	logger.Close()

	// Read and verify
	entries := readLogEntries(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Type != MessageTypeInserted {
		t.Errorf("Expected type %s, got %s", MessageTypeInserted, entry.Type)
	}
	if entry.Location != location {
		t.Errorf("Expected location %q, got %q", location, entry.Location)
	}
	if entry.Length != length {
		t.Errorf("Expected length %d, got %d", length, entry.Length)
	}
}

func TestRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	rotatedPath := filepath.Join(tmpDir, RotatedLogFileName)

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Write enough data to trigger rotation (8MB)
	// Each entry is ~100 bytes, so write 90k entries to guarantee rotation
	largeText := strings.Repeat("x", 90) // 90 char text
	for i := 0; i < 90000; i++ {
		if err := logger.LogChunk(largeText); err != nil {
			t.Fatalf("Failed to log chunk %d: %v", i, err)
		}
	}

	// Check that rotation happened
	if _, err := os.Stat(rotatedPath); os.IsNotExist(err) {
		t.Error("Rotated log file was not created")
	}

	// Verify current log is smaller than max size
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}
	if info.Size() >= MaxLogSize {
		t.Errorf("Log file size %d exceeds max size %d after rotation", info.Size(), MaxLogSize)
	}

	// Verify rotated log exists and has content
	rotatedInfo, err := os.Stat(rotatedPath)
	if err != nil {
		t.Fatalf("Failed to stat rotated log: %v", err)
	}
	if rotatedInfo.Size() == 0 {
		t.Error("Rotated log file is empty")
	}
}

func TestHomeDirectoryExpansion(t *testing.T) {
	// Test that ~ expands to home directory
	logger, err := New("~/.test-debug.log")
	if err != nil {
		t.Fatalf("Failed to create logger with ~ path: %v", err)
	}
	defer logger.Close()

	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, ".test-debug.log")

	if logger.path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, logger.path)
	}

	// Cleanup
	os.Remove(expectedPath)
}

// Helper function to read log entries from file
func readLogEntries(t *testing.T, path string) []LogEntry {
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("Failed to unmarshal log entry: %v", err)
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error: %v", err)
	}

	return entries
}
