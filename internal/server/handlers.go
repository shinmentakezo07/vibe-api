package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/mistral"
	"vibe-api/internal/openai"
	"vibe-api/internal/translator"
	"vibe-api/internal/usage"
)

const maxBodyBytes = 32 << 20 // images inlined as data URIs

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// chatCompletions accepts OpenAI chat requests, normalizes them for Mistral,
// and relays the reply, streaming included.
func (s *Server) chatCompletions(c *gin.Context) {
	started := time.Now()
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, openai.NewError(err.Error(), "invalid_request_error"))
		return
	}
	if len(body) > maxBodyBytes {
		c.JSON(http.StatusRequestEntityTooLarge, openai.NewError("request body too large", "invalid_request_error"))
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, openai.NewError("invalid JSON: "+err.Error(), "invalid_request_error"))
		return
	}
	model, _ := payload["model"].(string)
	if model == "" {
		c.JSON(http.StatusBadRequest, openai.NewError("model is required", "invalid_request_error"))
		return
	}

	stream := openai.IsStream(payload)
	if s.cfg.Debug {
		s.log.Info("request headers", "headers", c.Request.Header)
		s.log.Info("request body", "body", string(body))
	}
	translator.Request(payload)

	body, err = json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, openai.NewError(err.Error(), "api_error"))
		return
	}

	resp, err := s.upstream.ChatCompletions(c.Request.Context(), body, stream, c.Request.Header)
	if err != nil {
		s.upstreamError(c, err)
		return
	}
	defer resp.Body.Close()

	s.relay(c, resp, stream, "/v1/chat/completions", model, started)
}

func (s *Server) listModels(c *gin.Context) {
	resp, err := s.upstream.Models(c.Request.Context(), c.Request.Header)
	if err != nil {
		s.upstreamError(c, err)
		return
	}
	defer resp.Body.Close()
	s.relay(c, resp, false, "/v1/models", "", time.Now())
}

func (s *Server) getModel(c *gin.Context) {
	resp, err := s.upstream.Model(c.Request.Context(), c.Param("id"), c.Request.Header)
	if err != nil {
		s.upstreamError(c, err)
		return
	}
	defer resp.Body.Close()
	s.relay(c, resp, false, "/v1/models", "", time.Now())
}

// relay copies the upstream status, selected headers, and body to the caller.
func (s *Server) relay(c *gin.Context, resp *http.Response, stream bool, route, model string, started time.Time) {
	copyResponseHeaders(c.Writer.Header(), resp.Header)
	if stream {
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Status(resp.StatusCode)

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		s.log.Error("upstream error", "status", resp.StatusCode, "body", string(body))
		_, _ = c.Writer.Write(body)
		return
	}

	if stream {
		streamSSE(c.Writer, resp.Body, s.log, func(data []byte) {
			s.track(route, model, data, true, time.Since(started))
		})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("read response", "error", err)
		}
		return
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.track(route, model, body, false, time.Since(started))
		if rewritten, rerr := translator.Response(body); rerr == nil {
			body = rewritten
		}
	}
	if _, err := c.Writer.Write(body); err != nil && !errors.Is(err, context.Canceled) {
		s.log.Error("relay response", "error", err)
	}
}

// track records token usage from an upstream JSON payload.
func (s *Server) track(route, model string, data []byte, stream bool, latency time.Duration) {
	reported, tokens := usage.FromMessage(data)
	if !tokens.OK {
		return
	}
	if reported != "" {
		model = reported
	}
	record, ok := s.tracker.Add(usage.Sample{
		Route:      route,
		Model:      model,
		Prompt:     tokens.Prompt,
		Completion: tokens.Completion,
		Total:      tokens.Total,
		Cached:     tokens.Cached,
		Reasoning:  tokens.Reasoning,
		LatencyMS:  int(latency.Milliseconds()),
		Stream:     stream,
	})
	if !ok || s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.db.AddUsage(ctx, record); err != nil {
		s.log.Error("persist usage", "error", err)
	}
}

// upstreamError reports a failed upstream call, except when the caller is gone.
func (s *Server) upstreamError(c *gin.Context, err error) {
	if c.Request.Context().Err() != nil {
		return
	}
	if errors.Is(err, mistral.ErrMissingKey) {
		c.JSON(http.StatusUnauthorized, openai.NewError("Mistral API key is not configured", "authentication_error"))
		return
	}
	c.JSON(http.StatusBadGateway, openai.NewError("upstream request failed: "+err.Error(), "api_error"))
}

// responseBlocked are headers the proxy must not copy: framing is rewritten
// here, and identity encoding keeps bodies readable for rewriting.
var responseBlocked = map[string]bool{
	"connection":         true,
	"content-encoding":   true,
	"content-length":     true,
	"keep-alive":         true,
	"proxy-authenticate": true,
	"te":                 true,
	"trailer":            true,
	"transfer-encoding":  true,
	"upgrade":            true,
}

func copyResponseHeaders(dst, src http.Header) {
	for name, values := range src {
		if responseBlocked[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}
