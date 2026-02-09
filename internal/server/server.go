package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	db         *database.DB
}

func New(cfg *config.Config, db *database.DB) *Server {
	mux := http.NewServeMux()

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr(),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		cfg: cfg,
		db:  db,
	}

	s.registerRoutes(mux)

	return s
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
}

func (s *Server) Start() error {
	log.Printf("Server started on %s", s.cfg.Addr())
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}
