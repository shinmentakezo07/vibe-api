package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/translator"
)

// anthropicMessages serves the Anthropic Messages API on top of Mistral.
func (s *Server) anthropicMessages(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", err.Error()))
		return
	}
	if len(body) > maxBodyBytes {
		c.JSON(http.StatusRequestEntityTooLarge, anthropicError("invalid_request_error", "request body too large"))
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}
	model, _ := payload["model"].(string)
	if model == "" || strings.HasPrefix(strings.ToLower(model), "claude") {
		payload["model"] = s.cfg.AnthropicModel
	}
	model, _ = payload["model"].(string)
	stream, _ := payload["stream"].(bool)

	request := translator.AnthropicRequest(payload)
	if s.cfg.Debug {
		s.log.Info("anthropic request", "body", request)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, anthropicError("api_error", err.Error()))
		return
	}

	resp, err := s.upstream.ChatCompletions(c.Request.Context(), encoded, stream, c.Request.Header)
	if err != nil {
		s.upstreamError(c, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		s.relayAnthropicError(c, resp)
		return
	}

	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		c.Status(resp.StatusCode)
		streamAnthropic(c.Writer, resp.Body, translator.NewAnthropicStream(model, ""), s)
		return
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		if !errors.Is(err, c.Request.Context().Err()) {
			s.log.Error("read anthropic response", "error", err)
		}
		return
	}
	translated, err := translator.AnthropicResponse(body)
	if err != nil {
		c.JSON(http.StatusBadGateway, anthropicError("api_error", err.Error()))
		return
	}
	c.Header("Content-Type", "application/json")
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(translated)
}

// relayAnthropicError reports an upstream failure in Anthropic's envelope.
func (s *Server) relayAnthropicError(c *gin.Context, resp *http.Response) {
	body, _ := io.ReadAll(resp.Body)
	s.log.Error("upstream error", "status", resp.StatusCode, "body", string(body))

	message := upstreamMessage(body)
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	c.JSON(resp.StatusCode, anthropicError(anthropicErrorKind(resp.StatusCode), message))
}

func upstreamMessage(body []byte) string {
	var payload struct {
		Detail []struct {
			Msg string `json:"msg"`
		} `json:"detail"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if payload.Message != "" {
		return payload.Message
	}
	if len(payload.Detail) > 0 {
		return payload.Detail[0].Msg
	}
	return ""
}

func anthropicErrorKind(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status == http.StatusForbidden:
		return "permission_error"
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}

func anthropicError(kind, message string) gin.H {
	return gin.H{"type": "error", "error": gin.H{"type": kind, "message": message}}
}

// streamAnthropic relays Mistral chunks as Anthropic Messages SSE events.
func streamAnthropic(w gin.ResponseWriter, src io.Reader, stream *translator.AnthropicStream, s *Server) {
	writeAnthropicEvents(w, stream.Start())

	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if data, ok := sseData(line); ok {
			if data == "[DONE]" {
				writeAnthropicEvents(w, stream.Finish())
				return
			}
			writeAnthropicEvents(w, stream.Feed(translator.Chunk([]byte(data))))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Error("anthropic stream", "error", err)
			}
			writeAnthropicEvents(w, stream.Finish())
			return
		}
	}
}

// sseData extracts the payload of a `data:` line.
func sseData(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(trimmed, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")), true
}

func writeAnthropicEvents(w gin.ResponseWriter, events []translator.Event) {
	for _, event := range events {
		data, err := json.Marshal(event.Data)
		if err != nil {
			continue
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, data); err != nil {
			return
		}
	}
	w.Flush()
}
