// Package server wires the HTTP routes, middleware, and handlers together.
package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/config"
	"vibe-api/internal/mistral"
	"vibe-api/internal/store"
	"vibe-api/internal/usage"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	cfg      config.Config
	upstream *mistral.Client
	tracker  *usage.Tracker
	db       *store.Postgres
	log      *slog.Logger
}

// New builds a Server. db may be nil to keep the key and usage in memory.
func New(cfg config.Config, upstream *mistral.Client, db *store.Postgres, log *slog.Logger) *Server {
	return &Server{cfg: cfg, upstream: upstream, tracker: usage.New(), db: db, log: log}
}

// Router returns the fully configured Gin engine.
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), s.requestLogger(), cors())
	_ = r.SetTrustedProxies(nil)

	r.GET("/healthz", s.health)

	api := r.Group("/api")
	if s.cfg.ProxyKey != "" {
		api.Use(s.requireKey)
	}
	api.GET("/status", s.status)
	api.POST("/key", s.setKey)
	api.DELETE("/key", s.clearKey)
	api.GET("/usage", s.usageSnapshot)

	v1 := r.Group("/v1")
	if s.cfg.ProxyKey != "" {
		v1.Use(s.requireKey)
	}
	v1.GET("/models", s.listModels)
	v1.GET("/models/:id", s.getModel)
	v1.POST("/chat/completions", s.chatCompletions)
	v1.POST("/messages", s.anthropicMessages)

	r.NoRoute(s.serveUI)

	return r
}
