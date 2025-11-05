package logger

import (
	"fmt"
	"log"
	"os"
)

// Logger provides structured logging
type Logger struct {
	debug bool
}

// New creates a new logger
func New(debug bool) *Logger {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
	return &Logger{debug: debug}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// Debug logs a debug message (only if debug is enabled)
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	log.Fatalf("[FATAL] "+format, args...)
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
