package logger

import (
	"log/slog"
	"os"
	"path/filepath"
)

var Log *slog.Logger

// NewLogger initializes and returns a new logger instance.
func NewLogger(levelStr string) *slog.Logger {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".vulpix", "logs")
	os.MkdirAll(logDir, 0755)

	file, err := os.OpenFile(filepath.Join(logDir, "vulpix.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return slog.Default()
	}

	var level slog.Level
	switch levelStr {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	l := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: level,
	}))
	Log = l
	slog.SetDefault(l)
	return l
}

// Init initializes the global logger.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(home, ".vulpix", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(filepath.Join(logDir, "vulpix.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	Log = slog.New(handler)
	slog.SetDefault(Log)

	return nil
}
