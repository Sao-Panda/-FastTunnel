package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger wraps slog with file rotation support.
type Logger struct {
	*slog.Logger
	file *os.File
}

// New creates a new Logger from config.
func New(level, format, filePath string) (*Logger, error) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var handler slog.Handler
	var f *os.File

	if filePath != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return nil, err
		}
		var err error
		f, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		// Write to both stdout and file
		multi := io.MultiWriter(os.Stdout, f)
		if format == "text" {
			handler = slog.NewTextHandler(multi, opts)
		} else {
			handler = slog.NewJSONHandler(multi, opts)
		}
	} else {
		if format == "text" {
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		}
	}

	return &Logger{
		Logger: slog.New(handler),
		file:   f,
	}, nil
}

// Close closes the log file if one is open.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
