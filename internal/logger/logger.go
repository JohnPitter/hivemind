package logger

import (
	"log/slog"
	"os"
)

// Level aliases for convenience.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

var defaultLogger *slog.Logger

// Init initializes the global structured logger.
func Init(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// Debug logs at debug level with context tags.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs at info level with context tags.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs at warn level with context tags.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs at error level with context tags.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

// With returns a logger with preset attributes.
func With(args ...any) *slog.Logger {
	return slog.Default().With(args...)
}
