// Package server wires the HTTP routes, middleware, and handlers together.
package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/config"
	"vibe-api/internal/mistral"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	cfg      config.Config
	upstream *mistral.Client
	log      *slog.Logger
}

// New builds a Server.
func New(cfg config.Config, upstream *mistral.Client, log *slog.Logger) *Server {
	return &Server{cfg: cfg, upstream: upstream, log: log}
}

// Router returns the fully configured Gin engine.
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), s.requestLogger(), cors())
	_ = r.SetTrustedProxies(nil)

	r.GET("/healthz", s.health)

	v1 := r.Group("/v1")
	if s.cfg.ProxyKey != "" {
		v1.Use(s.requireKey)
	}
	v1.GET("/models", s.listModels)
	v1.GET("/models/:id", s.getModel)
	v1.POST("/chat/completions", s.chatCompletions)
	v1.POST("/messages", s.anthropicMessages)

	return r
}
