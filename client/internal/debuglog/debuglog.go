package debuglog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// MaxLogSize is 8MB as specified in the plan
	MaxLogSize = 8 * 1024 * 1024

	// LogFileName is the default log file name
	LogFileName = "debug.log"

	// RotatedLogFileName is the name for rotated logs
	RotatedLogFileName = "debug.log.1"
)

// MessageType represents the type of log entry
type MessageType string

const (
	MessageTypeChunk    MessageType = "chunk"
	MessageTypeComplete MessageType = "complete"
	MessageTypeInserted MessageType = "inserted"
)

// LogEntry represents a single log entry in JSON format
type LogEntry struct {
	Timestamp string      `json:"timestamp"`
	Type      MessageType `json:"type"`
	Text      string      `json:"text,omitempty"`
	ChunkID   int         `json:"chunk_id,omitempty"`
	FullText  string      `json:"full_text,omitempty"`
	Duration  float64     `json:"duration_seconds,omitempty"`
	Location  string      `json:"location,omitempty"`
	Length    int         `json:"length,omitempty"`
}

// Logger handles debug logging with rotation
type Logger struct {
	file     *os.File
	mu       sync.Mutex
	path     string
	chunkID  int
	disabled bool
}

// New creates a new debug logger
// If path is empty string, logging is disabled
func New(path string) (*Logger, error) {
	if path == "" {
		return &Logger{disabled: true}, nil
	}

	// Expand home directory if present
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := &Logger{
		file: file,
		path: path,
	}

	// Check if rotation is needed on startup
	if err := logger.checkRotation(); err != nil {
		file.Close()
		return nil, err
	}

	return logger, nil
}

// LogChunk logs a transcription chunk
func (l *Logger) LogChunk(text string) error {
	if l.disabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.chunkID++

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      MessageTypeChunk,
		Text:      text,
		ChunkID:   l.chunkID,
	}

	return l.writeEntry(entry)
}

// LogComplete logs a complete transcription session
func (l *Logger) LogComplete(fullText string, durationSeconds float64) error {
	if l.disabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      MessageTypeComplete,
		FullText:  fullText,
		Duration:  durationSeconds,
	}

	return l.writeEntry(entry)
}

// LogInserted logs when text is inserted into the target application
func (l *Logger) LogInserted(location string, length int) error {
	if l.disabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      MessageTypeInserted,
		Location:  location,
		Length:    length,
	}

	return l.writeEntry(entry)
}

// writeEntry writes a log entry and syncs to disk
func (l *Logger) writeEntry(entry LogEntry) error {
	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write with newline
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	// Sync to disk immediately (safety requirement)
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync log file: %w", err)
	}

	// Check if rotation is needed
	return l.checkRotation()
}

// checkRotation checks if the log file exceeds MaxLogSize and rotates if needed
func (l *Logger) checkRotation() error {
	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	if info.Size() < MaxLogSize {
		return nil
	}

	// Close current file
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}

	// Rotate: delete old .1 file if exists, rename current to .1
	rotatedPath := filepath.Join(filepath.Dir(l.path), RotatedLogFileName)
	os.Remove(rotatedPath) // Ignore error if file doesn't exist
	if err := os.Rename(l.path, rotatedPath); err != nil {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Open new file
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new log file: %w", err)
	}

	l.file = file
	return nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.disabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}
