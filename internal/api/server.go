package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/joaopedro/hivemind/internal/config"
	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/services"
)

// Server holds the HTTP API server configuration.
type Server struct {
	router       chi.Router
	port         int
	host         string
	webFS        fs.FS
	roomSvc      services.RoomService
	infSvc       services.InferenceService
	cfg          *config.Config
	apiKey       string
	authEnabled  bool
}

// NewServer creates a new API server with all routes and middleware.
func NewServer(port int, webFS fs.FS, roomSvc services.RoomService, infSvc services.InferenceService, cfg *config.Config) *Server {
	apiKey := os.Getenv("HIVEMIND_API_KEY")
	s := &Server{
		router:      chi.NewRouter(),
		port:        port,
		webFS:       webFS,
		roomSvc:     roomSvc,
		infSvc:      infSvc,
		cfg:         cfg,
		apiKey:      apiKey,
		authEnabled: apiKey != "",
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(RequestLogger)
	s.router.Use(RateLimiter(s.cfg.API.RateLimit))

	corsOrigins := []string{"http://localhost:5173", "http://localhost:8080"}
	if envOrigins := os.Getenv("HIVEMIND_API_CORS_ORIGINS"); envOrigins != "" {
		corsOrigins = strings.Split(envOrigins, ",")
		for i := range corsOrigins {
			corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
		}
	}
	s.router.Use(CORSMiddleware(corsOrigins))

	s.router.Use(RecoveryMiddleware)
}

func (s *Server) setupRoutes() {
	infHandler := handlers.NewInferenceHandler(s.infSvc)
	roomHandler := handlers.NewRoomHandler(s.roomSvc)
	healthHandler := handlers.NewHealthHandler(s.roomSvc)
	catalogHandler := handlers.NewCatalogHandler()

	maxBody := s.cfg.API.MaxBodyBytes

	// OpenAI-compatible API routes (auth-protected)
	s.router.Route("/v1", func(r chi.Router) {
		if s.authEnabled {
			r.Use(APIKeyAuth(s.apiKey))
		}
		r.Use(MaxBodyMiddleware(maxBody))
		r.Post("/chat/completions", infHandler.ChatCompletions)
		r.Post("/images/generations", infHandler.ImageGeneration)
		r.Get("/models", infHandler.ListModels)
		r.Get("/models/catalog", catalogHandler.ListCatalog)
	})

	// HiveMind-specific routes (auth-protected)
	s.router.Route("/room", func(r chi.Router) {
		if s.authEnabled {
			r.Use(APIKeyAuth(s.apiKey))
		}
		r.Use(MaxBodyMiddleware(maxBody))
		r.Post("/create", roomHandler.Create)
		r.Post("/join", roomHandler.Join)
		r.Delete("/leave", roomHandler.Leave)
		r.Get("/status", roomHandler.Status)
	})

	// Public routes
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
	host := "127.0.0.1"
	if s.host != "" {
		host = s.host
	}
	return fmt.Sprintf("%s:%d", host, s.port)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr(), s.router)
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}
