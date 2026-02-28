package logger

import (
	"context"
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

// contextKey is an unexported type for context keys.
type contextKey string

const logContextKey contextKey = "hivemind_log_ctx"

// Init initializes the global structured logger.
func Init(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// InitJSON initializes the global logger with JSON output (for production).
func InitJSON(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(os.Stderr, opts)
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

// WithContext creates a context with logging attributes attached.
// These attributes will be included in all log messages that use this context.
func WithContext(ctx context.Context, args ...any) context.Context {
	existing := contextAttrs(ctx)
	all := append(existing, args...)
	return context.WithValue(ctx, logContextKey, all)
}

// FromContext returns a logger with context-embedded attributes.
func FromContext(ctx context.Context) *slog.Logger {
	attrs := contextAttrs(ctx)
	if len(attrs) == 0 {
		return slog.Default()
	}
	return slog.Default().With(attrs...)
}

// CtxInfo logs at info level with attributes from context.
func CtxInfo(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Info(msg, args...)
}

// CtxWarn logs at warn level with attributes from context.
func CtxWarn(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Warn(msg, args...)
}

// CtxError logs at error level with attributes from context.
func CtxError(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Error(msg, args...)
}

// CtxDebug logs at debug level with attributes from context.
func CtxDebug(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Debug(msg, args...)
}

// contextAttrs retrieves log attributes from a context.
func contextAttrs(ctx context.Context) []any {
	v := ctx.Value(logContextKey)
	if v == nil {
		return nil
	}
	attrs, ok := v.([]any)
	if !ok {
		return nil
	}
	return attrs
}

// Room returns a logger tagged with room context.
func Room(roomID string) *slog.Logger {
	return With("room", roomID)
}

// Peer returns a logger tagged with peer context.
func Peer(peerID string) *slog.Logger {
	return With("peer", peerID)
}

// Inference returns a logger tagged with inference request context.
func Inference(requestID string) *slog.Logger {
	return With("request_id", requestID)
}

// Mesh returns a logger tagged with mesh networking context.
func Mesh() *slog.Logger {
	return With("component", "mesh")
}

// Worker returns a logger tagged with worker process context.
func Worker() *slog.Logger {
	return With("component", "worker")
}
