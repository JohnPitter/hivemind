package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/logger"
)

// RequestLogger logs every incoming request with method, path, status, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", duration.Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusWriter wraps ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush implements http.Flusher for SSE streaming support.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RateLimiter applies a simple token-bucket rate limiter per IP.
func RateLimiter(requestsPerSec int) func(http.Handler) http.Handler {
	type visitor struct {
		tokens   int
		lastSeen time.Time
	}

	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
	)

	// Cleanup stale visitors every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])
			}

			mu.Lock()
			v, exists := visitors[ip]
			if !exists {
				v = &visitor{tokens: requestsPerSec}
				visitors[ip] = v
			}

			// Refill tokens based on elapsed time
			elapsed := time.Since(v.lastSeen).Seconds()
			v.tokens += int(elapsed * float64(requestsPerSec))
			if v.tokens > requestsPerSec {
				v.tokens = requestsPerSec
			}
			v.lastSeen = time.Now()

			if v.tokens <= 0 {
				mu.Unlock()
				w.Header().Set("Retry-After", "1")
				http.Error(w, `{"error":{"message":"rate limit exceeded","code":"rate_limit"}}`, http.StatusTooManyRequests)
				return
			}
			v.tokens--
			mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware adds CORS headers with configurable allowed origins.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RecoveryMiddleware catches panics and returns 500 with error JSON.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered", "error", fmt.Sprintf("%v", err), "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":{"message":"internal server error","code":"internal_error"}}`)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
