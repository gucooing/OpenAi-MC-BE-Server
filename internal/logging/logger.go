package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	Level    string
	Color    bool
	FilePath string
}

type Logger struct {
	*slog.Logger
	file *os.File
}

func New(output io.Writer, options Options) (*Logger, error) {
	var level slog.Level
	switch strings.ToLower(strings.TrimSpace(options.Level)) {
	case "debug":
		level = slog.LevelDebug
	case "", "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q", options.Level)
	}

	var file *os.File
	writer := output
	if options.Color {
		writer = colorWriter{output: output}
	}
	if options.FilePath != "" {
		if err := os.MkdirAll(filepath.Dir(options.FilePath), 0o755); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
		openedFile, err := os.OpenFile(options.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file %q: %w", options.FilePath, err)
		}
		file = openedFile
		writer = io.MultiWriter(writer, openedFile)
	}

	return &Logger{
		Logger: slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})),
		file:   file,
	}, nil
}

func (logger *Logger) Close() error {
	if logger.file == nil {
		return nil
	}
	return logger.file.Close()
}

type colorWriter struct {
	output io.Writer
}

func (writer colorWriter) Write(message []byte) (int, error) {
	color := ""
	text := string(message)
	switch {
	case strings.Contains(text, "level=DEBUG"):
		color = "\x1b[36m"
	case strings.Contains(text, "level=INFO"):
		color = "\x1b[32m"
	case strings.Contains(text, "level=WARN"):
		color = "\x1b[33m"
	case strings.Contains(text, "level=ERROR"):
		color = "\x1b[31m"
	}
	if color == "" {
		if _, err := writer.output.Write(message); err != nil {
			return 0, err
		}
		return len(message), nil
	}
	if _, err := fmt.Fprintf(writer.output, "%s%s\x1b[0m", color, text); err != nil {
		return 0, err
	}
	return len(message), nil
}
