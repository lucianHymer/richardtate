package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Logger provides structured logging with optional file output
type Logger struct {
	debug       bool
	file        *os.File
	maxFileSize int64
	filePath    string
	mu          sync.Mutex
}

// New creates a new logger
func New(debug bool, filePath string, maxSize int) (*Logger, error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	l := &Logger{
		debug:       debug,
		filePath:    filePath,
		maxFileSize: int64(maxSize),
	}

	if filePath != "" {
		if err := l.openLogFile(); err != nil {
			return nil, err
		}
	}

	return l, nil
}

// openLogFile opens or creates the log file
func (l *Logger) openLogFile() error {
	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file

	// Set multi-writer to write to both stdout and file
	if l.file != nil {
		log.SetOutput(io.MultiWriter(os.Stdout, l.file))
	} else {
		log.SetOutput(os.Stdout)
	}

	return nil
}

// rotateLogFile rotates the log file if it exceeds max size
func (l *Logger) rotateLogFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	// Check file size
	info, err := l.file.Stat()
	if err != nil {
		return err
	}

	if info.Size() < l.maxFileSize {
		return nil
	}

	// Close current file
	l.file.Close()

	// Truncate the file (simple rotation - just reset)
	// For a rolling log, we could keep the last N bytes instead
	if err := os.Truncate(l.filePath, 0); err != nil {
		return err
	}

	// Reopen file
	return l.openLogFile()
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.rotateLogFile()
	log.Printf("[INFO] "+format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.rotateLogFile()
	log.Printf("[ERROR] "+format, args...)
}

// Debug logs a debug message (only if debug is enabled)
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debug {
		l.rotateLogFile()
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.rotateLogFile()
	log.Printf("[WARN] "+format, args...)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.rotateLogFile()
	log.Fatalf("[FATAL] "+format, args...)
}

// Close closes the log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// With returns a contextual logger with a prefix
func (l *Logger) With(prefix string) *ContextLogger {
	return &ContextLogger{
		logger: l,
		prefix: prefix,
	}
}

// ContextLogger wraps Logger with a prefix
type ContextLogger struct {
	logger *Logger
	prefix string
}

func (c *ContextLogger) Info(format string, args ...interface{}) {
	c.logger.Info(fmt.Sprintf("[%s] %s", c.prefix, format), args...)
}

func (c *ContextLogger) Error(format string, args ...interface{}) {
	c.logger.Error(fmt.Sprintf("[%s] %s", c.prefix, format), args...)
}

func (c *ContextLogger) Debug(format string, args ...interface{}) {
	c.logger.Debug(fmt.Sprintf("[%s] %s", c.prefix, format), args...)
}

func (c *ContextLogger) Warn(format string, args ...interface{}) {
	c.logger.Warn(fmt.Sprintf("[%s] %s", c.prefix, format), args...)
}

func (c *ContextLogger) Fatal(format string, args ...interface{}) {
	c.logger.Fatal(fmt.Sprintf("[%s] %s", c.prefix, format), args...)
}
