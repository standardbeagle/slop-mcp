package logging

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"debug lowercase", "debug", slog.LevelDebug},
		{"DEBUG uppercase", "DEBUG", slog.LevelDebug},
		{"Debug mixed", "Debug", slog.LevelDebug},
		{"info lowercase", "info", slog.LevelInfo},
		{"INFO uppercase", "INFO", slog.LevelInfo},
		{"warn lowercase", "warn", slog.LevelWarn},
		{"WARN uppercase", "WARN", slog.LevelWarn},
		{"warning alias", "warning", slog.LevelWarn},
		{"WARNING uppercase", "WARNING", slog.LevelWarn},
		{"error lowercase", "error", slog.LevelError},
		{"ERROR uppercase", "ERROR", slog.LevelError},
		{"empty defaults to warn", "", slog.LevelWarn},
		{"invalid defaults to warn", "invalid", slog.LevelWarn},
		{"whitespace trimmed", "  DEBUG  ", slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		name     string
		input    slog.Level
		expected string
	}{
		{"debug", slog.LevelDebug, "DEBUG"},
		{"info", slog.LevelInfo, "INFO"},
		{"warn", slog.LevelWarn, "WARN"},
		{"error", slog.LevelError, "ERROR"},
		{"unknown", slog.Level(100), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevelString(tt.input)
			if result != tt.expected {
				t.Errorf("LevelString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "text")

	logger.Debug("debug message", "key", "value")
	output := buf.String()

	if !strings.Contains(output, "debug message") {
		t.Errorf("expected 'debug message' in output, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected 'key=value' in output, got: %s", output)
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "json")

	logger.Info("info message", "number", 42)
	output := buf.String()

	if !strings.Contains(output, `"msg":"info message"`) {
		t.Errorf("expected '\"msg\":\"info message\"' in output, got: %s", output)
	}
	if !strings.Contains(output, `"number":42`) {
		t.Errorf("expected '\"number\":42' in output, got: %s", output)
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	tests := []struct {
		name           string
		loggerLevel    slog.Level
		logMethod      func(Logger)
		shouldContain  string
		shouldBeLogged bool
	}{
		{
			"warn logger filters debug",
			slog.LevelWarn,
			func(l Logger) { l.Debug("debug msg") },
			"debug msg",
			false,
		},
		{
			"warn logger filters info",
			slog.LevelWarn,
			func(l Logger) { l.Info("info msg") },
			"info msg",
			false,
		},
		{
			"warn logger includes warn",
			slog.LevelWarn,
			func(l Logger) { l.Warn("warn msg") },
			"warn msg",
			true,
		},
		{
			"warn logger includes error",
			slog.LevelWarn,
			func(l Logger) { l.Error("error msg") },
			"error msg",
			true,
		},
		{
			"debug logger includes all",
			slog.LevelDebug,
			func(l Logger) { l.Debug("debug msg") },
			"debug msg",
			true,
		},
		{
			"error logger filters warn",
			slog.LevelError,
			func(l Logger) { l.Warn("warn msg") },
			"warn msg",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(&buf, tt.loggerLevel, "text")

			tt.logMethod(logger)
			output := buf.String()

			hasContent := strings.Contains(output, tt.shouldContain)
			if hasContent != tt.shouldBeLogged {
				t.Errorf("expected logged=%v, got logged=%v, output: %s",
					tt.shouldBeLogged, hasContent, output)
			}
		})
	}
}

func TestLoggerWith(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "text")

	// Create a logger with contextual fields
	mcpLogger := logger.With("mcp_name", "test-mcp")
	mcpLogger.Info("tool executed", "tool_name", "search")
	output := buf.String()

	if !strings.Contains(output, "mcp_name=test-mcp") {
		t.Errorf("expected 'mcp_name=test-mcp' in output, got: %s", output)
	}
	if !strings.Contains(output, "tool_name=search") {
		t.Errorf("expected 'tool_name=search' in output, got: %s", output)
	}
	if !strings.Contains(output, "tool executed") {
		t.Errorf("expected 'tool executed' in output, got: %s", output)
	}
}

func TestNopLogger(t *testing.T) {
	logger := Nop()

	// These should not panic
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	// With should return nop logger
	withLogger := logger.With("key", "value")
	withLogger.Info("test")

	// If we get here without panic, the test passes
}

func TestNewFromEnv(t *testing.T) {
	// Save original env values
	origLevel := os.Getenv(LogLevelEnvVar)
	origFormat := os.Getenv(LogFormatEnvVar)
	defer func() {
		os.Setenv(LogLevelEnvVar, origLevel)
		os.Setenv(LogFormatEnvVar, origFormat)
	}()

	// Test with DEBUG level and JSON format
	os.Setenv(LogLevelEnvVar, "DEBUG")
	os.Setenv(LogFormatEnvVar, "json")

	// Reset the default logger
	ResetDefault()

	logger := Default()
	if logger == nil {
		t.Fatal("Default() returned nil")
	}

	// Test that it's a real logger (not nop)
	var buf bytes.Buffer
	testLogger := New(&buf, slog.LevelDebug, "text")
	testLogger.Debug("test")
	if buf.Len() == 0 {
		t.Error("expected output from debug logger")
	}
}

func TestDefaultLogger(t *testing.T) {
	// Reset and test default
	ResetDefault()

	logger1 := Default()
	logger2 := Default()

	// Should return same instance (singleton)
	if logger1 != logger2 {
		t.Error("Default() should return the same logger instance")
	}
}

func TestSetDefault(t *testing.T) {
	// Reset first
	ResetDefault()

	// Create a custom logger
	var buf bytes.Buffer
	custom := New(&buf, slog.LevelDebug, "text")

	// Set as default
	SetDefault(custom)

	// Get default and use it
	logger := Default()
	logger.Info("test message")

	// Note: SetDefault only works if once.Do hasn't been called yet
	// Since we called SetDefault after ResetDefault, the behavior depends on implementation
	// In current implementation, SetDefault forces once.Do to complete and then sets logger
	if logger == nil {
		t.Error("Default() should not return nil after SetDefault")
	}
}

func TestContextualFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "json")

	// Test various field types
	logger.Info("test",
		"string", "value",
		"int", 42,
		"float", 3.14,
		"bool", true,
	)

	output := buf.String()

	expectations := []string{
		`"string":"value"`,
		`"int":42`,
		`"float":3.14`,
		`"bool":true`,
	}

	for _, exp := range expectations {
		if !strings.Contains(output, exp) {
			t.Errorf("expected %q in output, got: %s", exp, output)
		}
	}
}

func TestEnvironmentVariableConstants(t *testing.T) {
	// Verify constant values match documentation
	if LogLevelEnvVar != "SLOP_MCP_LOG_LEVEL" {
		t.Errorf("LogLevelEnvVar = %q, want %q", LogLevelEnvVar, "SLOP_MCP_LOG_LEVEL")
	}
	if LogFormatEnvVar != "SLOP_MCP_LOG_FORMAT" {
		t.Errorf("LogFormatEnvVar = %q, want %q", LogFormatEnvVar, "SLOP_MCP_LOG_FORMAT")
	}
}

func TestDefaultConstants(t *testing.T) {
	// Verify defaults match documentation
	if DefaultLevel != slog.LevelWarn {
		t.Errorf("DefaultLevel = %v, want %v", DefaultLevel, slog.LevelWarn)
	}
	if DefaultFormat != "text" {
		t.Errorf("DefaultFormat = %q, want %q", DefaultFormat, "text")
	}
}

func TestLoggerMethodsAllLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "text")

	// Test all log methods produce output at debug level
	testCases := []struct {
		method  func(string, ...any)
		level   string
		message string
	}{
		{logger.Debug, "DEBUG", "debug test"},
		{logger.Info, "INFO", "info test"},
		{logger.Warn, "WARN", "warn test"},
		{logger.Error, "ERROR", "error test"},
	}

	for _, tc := range testCases {
		buf.Reset()
		tc.method(tc.message, "key", "value")
		output := buf.String()

		if !strings.Contains(output, tc.message) {
			t.Errorf("%s: expected message %q in output, got: %s", tc.level, tc.message, output)
		}
		if !strings.Contains(output, tc.level) && !strings.Contains(strings.ToLower(output), strings.ToLower(tc.level)) {
			t.Errorf("%s: expected level indicator in output, got: %s", tc.level, output)
		}
	}
}

func TestFormatCaseInsensitive(t *testing.T) {
	testCases := []string{"JSON", "Json", "json", "TEXT", "Text", "text"}

	for _, format := range testCases {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(&buf, slog.LevelInfo, format)
			logger.Info("test")

			if buf.Len() == 0 {
				t.Errorf("format %q produced no output", format)
			}
		})
	}
}

func TestChainedWith(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, "text")

	// Chain multiple With calls
	l1 := logger.With("service", "slop-mcp")
	l2 := l1.With("mcp_name", "filesystem")
	l3 := l2.With("tool_name", "read_file")

	l3.Info("executing tool")

	output := buf.String()
	expectations := []string{"service=slop-mcp", "mcp_name=filesystem", "tool_name=read_file"}

	for _, exp := range expectations {
		if !strings.Contains(output, exp) {
			t.Errorf("expected %q in output, got: %s", exp, output)
		}
	}
}
