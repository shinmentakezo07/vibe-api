package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/keys"
)

// status describes the proxy's credential pool and runtime configuration.
// Secrets are never returned.
func (s *Server) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"keyConfigured":  s.keys.EnabledCount() > 0,
		"keyHint":        s.keys.FirstHint(),
		"keyCount":       s.keys.Count(),
		"enabledKeys":    s.keys.EnabledCount(),
		"roundRobin":     s.keys.RoundRobin(),
		"anthropicModel": s.cfg.AnthropicModel,
		"proxyAuth":      s.cfg.ProxyKey != "",
		"upstream":       s.cfg.BaseURL,
		"storage":        s.storageMode(),
	})
}

type keysPayload struct {
	Keys       []keys.View `json:"keys"`
	RoundRobin bool        `json:"roundRobin"`
	Storage    string      `json:"storage"`
}

func (s *Server) keyList() keysPayload {
	return keysPayload{
		Keys:       s.keys.Views(),
		RoundRobin: s.keys.RoundRobin(),
		Storage:    s.storageMode(),
	}
}

// listKeys returns every key with a masked hint.
func (s *Server) listKeys(c *gin.Context) {
	c.JSON(http.StatusOK, s.keyList())
}

type addKeyPayload struct {
	Label  string `json:"label"`
	APIKey string `json:"apiKey"`
	Key    string `json:"key"`
}

// addKey validates a candidate against Mistral, then stores and enables it.
func (s *Server) addKey(c *gin.Context) {
	var payload addKeyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON body"))
		return
	}
	secret := strings.TrimSpace(payload.APIKey)
	if secret == "" {
		secret = strings.TrimSpace(payload.Key)
	}
	if secret == "" {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "apiKey is required"))
		return
	}
	if len(secret) > 512 {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "apiKey is too long"))
		return
	}
	label := strings.TrimSpace(payload.Label)
	if len(label) > 100 {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "label is too long"))
		return
	}
	if label == "" {
		label = "Mistral key"
	}
	if s.keys.ContainsSecret(secret) {
		c.JSON(http.StatusConflict, anthropicError("invalid_request_error", "this API key is already configured"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := s.upstream.ValidateKey(ctx, secret); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", err.Error()))
		return
	}

	var key keys.Key
	if s.db != nil {
		stored, err := s.db.AddAPIKey(ctx, label, secret)
		if err != nil {
			s.log.Error("save API key", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not save the API key"))
			return
		}
		key = stored
	} else {
		key = keys.Key{
			ID:        keys.NewID(),
			Label:     label,
			Secret:    secret,
			Enabled:   true,
			CreatedAt: time.Now(),
			Source:    "memory",
		}
	}
	s.keys.Add(key)
	s.log.Info("API key added", "id", key.ID, "source", key.Source)
	c.JSON(http.StatusOK, s.keyList())
}

type updateKeyPayload struct {
	Label   *string `json:"label"`
	Enabled *bool   `json:"enabled"`
}

// updateKey changes a label or toggles a key without deleting it.
func (s *Server) updateKey(c *gin.Context) {
	var payload updateKeyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON body"))
		return
	}
	if payload.Label == nil && payload.Enabled == nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "nothing to update"))
		return
	}
	id := c.Param("id")
	key, ok := s.keys.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, anthropicError("invalid_request_error", "API key not found"))
		return
	}

	label := key.Label
	if payload.Label != nil {
		label = strings.TrimSpace(*payload.Label)
		if label == "" {
			c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "label cannot be empty"))
			return
		}
		if len(label) > 100 {
			c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "label is too long"))
			return
		}
	}
	enabled := key.Enabled
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	if s.db != nil && key.Source == "database" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := s.db.UpdateAPIKey(ctx, id, label, enabled); err != nil {
			s.log.Error("update API key", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not update the API key"))
			return
		}
	}
	s.keys.Update(id, &label, &enabled)
	c.JSON(http.StatusOK, s.keyList())
}

// deleteKey removes a key from the pool and, when persisted, from Postgres.
func (s *Server) deleteKey(c *gin.Context) {
	id := c.Param("id")
	key, ok := s.keys.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, anthropicError("invalid_request_error", "API key not found"))
		return
	}
	if s.db != nil && key.Source == "database" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := s.db.DeleteAPIKey(ctx, id); err != nil {
			s.log.Error("delete API key", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not delete the API key"))
			return
		}
	}
	s.keys.Delete(id)
	s.log.Info("API key removed", "id", id)
	c.JSON(http.StatusOK, s.keyList())
}

type roundRobinPayload struct {
	Enabled bool `json:"enabled"`
}

// setRoundRobin persists and applies the rotation setting.
func (s *Server) setRoundRobin(c *gin.Context) {
	var payload roundRobinPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON body"))
		return
	}
	if s.db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := s.db.SaveRoundRobin(ctx, payload.Enabled); err != nil {
			s.log.Error("save round-robin setting", "error", err)
			c.JSON(http.StatusInternalServerError, anthropicError("api_error", "could not save the setting"))
			return
		}
	}
	s.keys.SetRoundRobin(payload.Enabled)
	c.JSON(http.StatusOK, s.keyList())
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

// storageMode reports where keys and usage live.
func (s *Server) storageMode() string {
	if s.db != nil {
		return "postgres"
	}
	return "memory"
}
