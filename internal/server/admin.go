package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// status describes the proxy's credential and runtime configuration. The key
// itself is never returned.
func (s *Server) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"keyConfigured":  s.upstream.HasAPIKey(),
		"keyHint":        s.upstream.KeyHint(),
		"anthropicModel": s.cfg.AnthropicModel,
		"proxyAuth":      s.cfg.ProxyKey != "",
		"upstream":       s.cfg.BaseURL,
		"storage":        s.storageMode(),
	})
}

type keyPayload struct {
	APIKey string `json:"apiKey"`
	Key    string `json:"key"`
}

// setKey validates a candidate credential against Mistral, then swaps it in
// without a restart.
func (s *Server) setKey(c *gin.Context) {
	var payload keyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON body"))
		return
	}
	key := strings.TrimSpace(payload.APIKey)
	if key == "" {
		key = strings.TrimSpace(payload.Key)
	}
	if key == "" {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "apiKey is required"))
		return
	}
	if len(key) > 512 {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "apiKey is too long"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := s.upstream.ValidateKey(ctx, key); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", err.Error()))
		return
	}

	if s.db != nil {
		if err := s.db.SaveAPIKey(ctx, key); err != nil {
			s.log.Error("persist API key", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not save the API key"))
			return
		}
	}
	s.upstream.SetAPIKey(key)
	s.log.Info("upstream API key updated")
	s.status(c)
}

// clearKey removes the stored and in-memory credential.
func (s *Server) clearKey(c *gin.Context) {
	if s.db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := s.db.ClearAPIKey(ctx); err != nil {
			s.log.Error("clear API key", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not clear the API key"))
			return
		}
	}
	s.upstream.SetAPIKey("")
	s.log.Info("upstream API key cleared")
	s.status(c)
}

// usageSnapshot serves persisted totals when Postgres is configured.
func (s *Server) usageSnapshot(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusOK, s.tracker.Snapshot())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	snapshot, err := s.db.Usage(ctx, 200)
	if err != nil {
		s.log.Error("query usage", "error", err)
		c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not load usage"))
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

// storageMode reports where the key and usage history live.
func (s *Server) storageMode() string {
	if s.db != nil {
		return "postgres"
	}
	return "memory"
}
