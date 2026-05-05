package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHonorsLogLevel(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, Options{Level: "warn"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer logger.Close()

	logger.Info("hidden")
	logger.Warn("visible")

	text := output.String()
	if strings.Contains(text, "hidden") {
		t.Fatalf("logger output %q contains info message below warn level", text)
	}
	if !strings.Contains(text, "visible") {
		t.Fatalf("logger output %q does not contain warn message", text)
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	var output bytes.Buffer
	if _, err := New(&output, Options{Level: "verbose"}); err == nil {
		t.Fatalf("New() error = nil, want unknown level error")
	}
}

func TestNewWritesLogFile(t *testing.T) {
	var output bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "logs", "server.log")
	logger, err := New(&output, Options{Level: "info", FilePath: logPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	logger.Info("persisted")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	if !strings.Contains(string(content), "persisted") {
		t.Fatalf("log file %q does not contain message", string(content))
	}
}

func TestNewColorizesConsoleOutput(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, Options{Level: "info", Color: true})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer logger.Close()

	logger.Info("colored")
	if !strings.Contains(output.String(), "\x1b[32m") {
		t.Fatalf("logger output %q does not contain info color code", output.String())
	}
}
