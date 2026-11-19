package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/mistral"
	"vibe-api/internal/openai"
)

// requireKey rejects callers whose bearer token is not the configured proxy key.
func (s *Server) requireKey(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if token != s.cfg.ProxyKey {
		c.AbortWithStatusJSON(http.StatusUnauthorized, openai.NewError("invalid proxy API key", "authentication_error"))
		return
	}
	c.Next()
}

// requireAPIKey selects a healthy upstream credential for this request.
func (s *Server) requireAPIKey(c *gin.Context) {
	key, err := s.keys.Pick()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, openai.NewError("Mistral API key is not configured", "authentication_error"))
		return
	}
	c.Request = c.Request.WithContext(mistral.WithAPIKey(c.Request.Context(), key))
	c.Next()
}

// cors allows browser-based OpenAI clients to call the proxy directly.
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// requestLogger emits one structured line per request.
func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		size := max(c.Writer.Size(), 0)
		s.log.Info("request",
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"bytes", size,
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
