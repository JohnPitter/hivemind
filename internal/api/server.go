package api

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/services"
)

// Server holds the HTTP API server configuration.
type Server struct {
	router  chi.Router
	port    int
	webFS   fs.FS
	roomSvc services.RoomService
	infSvc  services.InferenceService
}

// NewServer creates a new API server with all routes and middleware.
func NewServer(port int, webFS fs.FS, roomSvc services.RoomService, infSvc services.InferenceService) *Server {
	s := &Server{
		router:  chi.NewRouter(),
		port:    port,
		webFS:   webFS,
		roomSvc: roomSvc,
		infSvc:  infSvc,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(RequestLogger)
	s.router.Use(RateLimiter(100)) // 100 req/s
	s.router.Use(CORSMiddleware)
	s.router.Use(RecoveryMiddleware)
}

func (s *Server) setupRoutes() {
	infHandler := handlers.NewInferenceHandler(s.infSvc)
	roomHandler := handlers.NewRoomHandler(s.roomSvc)
	healthHandler := handlers.NewHealthHandler(s.roomSvc)

	// OpenAI-compatible API routes
	s.router.Route("/v1", func(r chi.Router) {
		r.Post("/chat/completions", infHandler.ChatCompletions)
		r.Post("/images/generations", infHandler.ImageGeneration)
		r.Get("/models", infHandler.ListModels)
	})

	// HiveMind-specific routes
	s.router.Route("/room", func(r chi.Router) {
		r.Post("/create", roomHandler.Create)
		r.Post("/join", roomHandler.Join)
		r.Delete("/leave", roomHandler.Leave)
		r.Get("/status", roomHandler.Status)
	})

	s.router.Get("/health", healthHandler.Health)
	s.router.Get("/metrics", healthHandler.Metrics)

	// Web dashboard (SPA) — must be last
	if s.webFS != nil {
		webHandler := handlers.NewWebHandler(s.webFS, s.roomSvc, s.infSvc)
		s.router.Route("/api", func(r chi.Router) {
			r.Get("/room/status", webHandler.HandleRoomStatusJSON)
			r.Get("/health", webHandler.HandleHealthJSON)
			r.Get("/models", webHandler.HandleModelsJSON)
		})

		// Serve static files and SPA fallback
		fileServer := http.FileServer(http.FS(s.webFS))
		s.router.Handle("/*", handlers.SPAHandler(fileServer, s.webFS))
	}
}

// Addr returns the server listen address.
func (s *Server) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.port)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr(), s.router)
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}
