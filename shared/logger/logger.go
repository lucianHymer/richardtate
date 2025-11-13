package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel converts a string to a LogLevel
func ParseLogLevel(level string) LogLevel {
	switch level {
	case "debug", "DEBUG":
		return LevelDebug
	case "info", "INFO":
		return LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return LevelWarn
	case "error", "ERROR":
		return LevelError
	case "fatal", "FATAL":
		return LevelFatal
	default:
		return LevelInfo // Default to INFO
	}
}

// OutputFormat determines how logs are formatted
type OutputFormat int

const (
	FormatText OutputFormat = iota
	FormatJSON
)

// ParseOutputFormat converts a string to an OutputFormat
func ParseOutputFormat(format string) OutputFormat {
	switch format {
	case "json", "JSON":
		return FormatJSON
	case "text", "TEXT":
		return FormatText
	default:
		return FormatText // Default to text
	}
}

// Logger provides structured logging with configurable output format
type Logger struct {
	level  LogLevel
	format OutputFormat
	output io.Writer
	mu     sync.Mutex
	fields map[string]interface{} // Global fields for all logs
}

// Config holds logger configuration
type Config struct {
	Level  LogLevel
	Format OutputFormat
	Output io.Writer
	Debug  bool // Convenience flag to set level to Debug
}

// New creates a new logger with the given configuration
func New(debug bool) *Logger {
	level := LevelInfo
	if debug {
		level = LevelDebug
	}

	return NewWithConfig(Config{
		Level:  level,
		Format: FormatText,
		Output: os.Stdout,
		Debug:  debug,
	})
}

// NewWithConfig creates a new logger with detailed configuration
func NewWithConfig(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := cfg.Level
	if cfg.Debug {
		level = LevelDebug
	}

	// Configure standard library logger for any legacy usage
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(cfg.Output)

	return &Logger{
		level:  level,
		format: cfg.Format,
		output: cfg.Output,
		fields: make(map[string]interface{}),
	}
}

// logEntry represents a single log entry
type logEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// log is the internal logging implementation
func (l *Logger) log(level LogLevel, component, message string, fields map[string]interface{}) {
	// Check if this log level should be output
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Merge global fields with log-specific fields
	allFields := make(map[string]interface{})
	for k, v := range l.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}

	entry := logEntry{
		Timestamp: time.Now().Format("2006/01/02 15:04:05.000000"),
		Level:     level.String(),
		Component: component,
		Message:   message,
		Fields:    allFields,
	}

	var output string

	switch l.format {
	case FormatJSON:
		// JSON format
		jsonBytes, err := json.Marshal(entry)
		if err != nil {
			// Fallback to text format on error
			output = fmt.Sprintf("%s [%s] %s\n", entry.Timestamp, entry.Level, message)
		} else {
			output = string(jsonBytes) + "\n"
		}

	case FormatText:
		// Text format with optional component tag
		if component != "" {
			output = fmt.Sprintf("%s [%s] [%s] %s", entry.Timestamp, entry.Level, component, message)
		} else {
			output = fmt.Sprintf("%s [%s] %s", entry.Timestamp, entry.Level, message)
		}

		// Append fields in text format if present
		if len(allFields) > 0 {
			output += " |"
			for k, v := range allFields {
				output += fmt.Sprintf(" %s=%v", k, v)
			}
		}
		output += "\n"
	}

	fmt.Fprint(l.output, output)

	// Fatal logs should exit
	if level == LevelFatal {
		os.Exit(1)
	}
}

// WithFields returns a new logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		level:  l.level,
		format: l.format,
		output: l.output,
		fields: newFields,
	}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, "", fmt.Sprintf(format, args...), nil)
}

// InfoWithFields logs an info message with structured fields
func (l *Logger) InfoWithFields(message string, fields map[string]interface{}) {
	l.log(LevelInfo, "", message, fields)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, "", fmt.Sprintf(format, args...), nil)
}

// ErrorWithFields logs an error message with structured fields
func (l *Logger) ErrorWithFields(message string, fields map[string]interface{}) {
	l.log(LevelError, "", message, fields)
}

// Debug logs a debug message (only if debug level is enabled)
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, "", fmt.Sprintf(format, args...), nil)
}

// DebugWithFields logs a debug message with structured fields
func (l *Logger) DebugWithFields(message string, fields map[string]interface{}) {
	l.log(LevelDebug, "", message, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, "", fmt.Sprintf(format, args...), nil)
}

// WarnWithFields logs a warning message with structured fields
func (l *Logger) WarnWithFields(message string, fields map[string]interface{}) {
	l.log(LevelWarn, "", message, fields)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelFatal, "", fmt.Sprintf(format, args...), nil)
}

// FatalWithFields logs a fatal error with structured fields and exits
func (l *Logger) FatalWithFields(message string, fields map[string]interface{}) {
	l.log(LevelFatal, "", message, fields)
}

// With returns a contextual logger with a component name
func (l *Logger) With(component string) *ContextLogger {
	return &ContextLogger{
		logger:    l,
		component: component,
	}
}

// ContextLogger wraps Logger with a component name for contextual logging
type ContextLogger struct {
	logger    *Logger
	component string
	fields    map[string]interface{}
}

// WithFields returns a new context logger with additional fields
func (c *ContextLogger) WithFields(fields map[string]interface{}) *ContextLogger {
	newFields := make(map[string]interface{})
	for k, v := range c.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &ContextLogger{
		logger:    c.logger,
		component: c.component,
		fields:    newFields,
	}
}

func (c *ContextLogger) Info(format string, args ...interface{}) {
	c.logger.log(LevelInfo, c.component, fmt.Sprintf(format, args...), c.fields)
}

func (c *ContextLogger) InfoWithFields(message string, fields map[string]interface{}) {
	allFields := c.mergeFields(fields)
	c.logger.log(LevelInfo, c.component, message, allFields)
}

func (c *ContextLogger) Error(format string, args ...interface{}) {
	c.logger.log(LevelError, c.component, fmt.Sprintf(format, args...), c.fields)
}

func (c *ContextLogger) ErrorWithFields(message string, fields map[string]interface{}) {
	allFields := c.mergeFields(fields)
	c.logger.log(LevelError, c.component, message, allFields)
}

func (c *ContextLogger) Debug(format string, args ...interface{}) {
	c.logger.log(LevelDebug, c.component, fmt.Sprintf(format, args...), c.fields)
}

func (c *ContextLogger) DebugWithFields(message string, fields map[string]interface{}) {
	allFields := c.mergeFields(fields)
	c.logger.log(LevelDebug, c.component, message, allFields)
}

func (c *ContextLogger) Warn(format string, args ...interface{}) {
	c.logger.log(LevelWarn, c.component, fmt.Sprintf(format, args...), c.fields)
}

func (c *ContextLogger) WarnWithFields(message string, fields map[string]interface{}) {
	allFields := c.mergeFields(fields)
	c.logger.log(LevelWarn, c.component, message, allFields)
}

func (c *ContextLogger) Fatal(format string, args ...interface{}) {
	c.logger.log(LevelFatal, c.component, fmt.Sprintf(format, args...), c.fields)
}

func (c *ContextLogger) FatalWithFields(message string, fields map[string]interface{}) {
	allFields := c.mergeFields(fields)
	c.logger.log(LevelFatal, c.component, message, allFields)
}

// mergeFields combines context fields with provided fields
func (c *ContextLogger) mergeFields(fields map[string]interface{}) map[string]interface{} {
	allFields := make(map[string]interface{})
	for k, v := range c.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}
	return allFields
}
