// Package logging provides structured logging for slop-mcp.
//
// This package uses the standard library's log/slog for structured logging,
// with support for different log levels and output formats (text/JSON).
//
// Configuration via environment variables:
//   - SLOP_MCP_LOG_LEVEL: DEBUG, INFO, WARN, ERROR (default: WARN)
//   - SLOP_MCP_LOG_FORMAT: text, json (default: text)
//
// Since slop-mcp is a stdio-based MCP server, all logs go to stderr
// to avoid interfering with the JSON-RPC protocol on stdout.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Environment variable names for logging configuration.
const (
	LogLevelEnvVar  = "SLOP_MCP_LOG_LEVEL"
	LogFormatEnvVar = "SLOP_MCP_LOG_FORMAT"
)

// Default logging configuration.
const (
	DefaultLevel  = slog.LevelWarn
	DefaultFormat = "text"
)

// Logger is the interface for structured logging.
// It wraps slog.Logger to provide a simple API.
type Logger interface {
	// Debug logs a message at DEBUG level with optional key-value pairs.
	Debug(msg string, args ...any)

	// Info logs a message at INFO level with optional key-value pairs.
	Info(msg string, args ...any)

	// Warn logs a message at WARN level with optional key-value pairs.
	Warn(msg string, args ...any)

	// Error logs a message at ERROR level with optional key-value pairs.
	Error(msg string, args ...any)

	// With returns a new Logger with the given key-value pairs added to every log.
	With(args ...any) Logger
}

// logger implements the Logger interface using slog.
type logger struct {
	slog *slog.Logger
}

// Debug logs a message at DEBUG level.
func (l *logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

// Info logs a message at INFO level.
func (l *logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

// Warn logs a message at WARN level.
func (l *logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

// Error logs a message at ERROR level.
func (l *logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

// With returns a new Logger with the given key-value pairs added.
func (l *logger) With(args ...any) Logger {
	return &logger{slog: l.slog.With(args...)}
}

var (
	defaultLogger Logger
	once          sync.Once
)

// Default returns the default logger, initialized from environment variables.
// The logger is created once and reused for subsequent calls.
func Default() Logger {
	once.Do(func() {
		defaultLogger = NewFromEnv()
	})
	return defaultLogger
}

// NewFromEnv creates a new Logger configured from environment variables.
func NewFromEnv() Logger {
	level := ParseLevel(os.Getenv(LogLevelEnvVar))
	format := os.Getenv(LogFormatEnvVar)
	if format == "" {
		format = DefaultFormat
	}
	return New(os.Stderr, level, format)
}

// New creates a new Logger with the specified configuration.
// Output is written to w (typically os.Stderr for MCP servers).
// Level determines the minimum log level to output.
// Format can be "text" or "json".
func New(w io.Writer, level slog.Level, format string) Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return &logger{slog: slog.New(handler)}
}

// ParseLevel parses a log level string into a slog.Level.
// Valid values: DEBUG, INFO, WARN, ERROR (case-insensitive).
// Returns DefaultLevel (WARN) for empty or invalid values.
func ParseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return DefaultLevel
	}
}

// LevelString returns the string representation of a log level.
func LevelString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARN"
	case slog.LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// nopLogger is a logger that discards all output.
type nopLogger struct{}

func (nopLogger) Debug(string, ...any) {}
func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
func (n nopLogger) With(...any) Logger { return n }

// Nop returns a logger that discards all output.
// Useful for testing or when logging should be disabled.
func Nop() Logger {
	return nopLogger{}
}

// SetDefault sets the default logger.
// This should be called early in program initialization if needed.
// Note: This is not safe for concurrent use with Default().
func SetDefault(l Logger) {
	once.Do(func() {}) // Ensure once is done
	defaultLogger = l
}

// ResetDefault resets the default logger to be re-initialized on next call.
// This is primarily useful for testing.
func ResetDefault() {
	once = sync.Once{}
	defaultLogger = nil
}
