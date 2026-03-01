package api

import (
	"fmt"
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware that validates Bearer token authentication.
// If the key is empty, the middleware is a no-op (auth disabled).
func APIKeyAuth(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":{"message":"missing authorization header","code":"unauthorized"}}`)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if token == auth || token != key {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":{"message":"invalid API key","code":"forbidden"}}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodyMiddleware limits the request body size.
func MaxBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
