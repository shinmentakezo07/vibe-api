package server

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"vibe-api/internal/translator"
)

func edgeTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// Upstream error frames mid-stream (quota, overload, key revocation) must be
// surfaced as an Anthropic error event, not folded into a silent end_turn.
func TestAnthropicStreamUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	upstream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		`data: {"error":{"message":"quota exceeded","type":"tokens_exceeded"}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	streamAnthropic(c.Writer, strings.NewReader(upstream), translator.NewAnthropicStream("m", ""), edgeTestServer(t), func([]byte) {})
	body := rec.Body.String()
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "quota exceeded") {
		t.Fatalf("upstream error not surfaced: %s", body)
	}
}

// Normal chunks carrying an error field must NOT be mistaken for error frames.
func TestAnthropicStreamChunkWithErrorField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	upstream := strings.Join([]string{
		`data: {"id":"x","model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}],"error":null}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	streamAnthropic(c.Writer, strings.NewReader(upstream), translator.NewAnthropicStream("m", ""), edgeTestServer(t), func([]byte) {})
	body := rec.Body.String()
	if strings.Contains(body, "event: error") {
		t.Fatalf("normal chunk treated as error: %s", body)
	}
	if !strings.Contains(body, "text_delta") {
		t.Fatalf("text delta lost: %s", body)
	}
}

// Missing [DONE]: EOF alone must still close all blocks and end the message.
func TestAnthropicStreamNoDoneMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	upstream := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"
	streamAnthropic(c.Writer, strings.NewReader(upstream), translator.NewAnthropicStream("m", ""), edgeTestServer(t), func([]byte) {})
	body := rec.Body.String()
	for _, want := range []string{"content_block_stop", "message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s: %s", want, body)
		}
	}
}

// SSE comments, event frames, and `data:` without a space must all be handled
// per the SSE grammar.
func TestAnthropicStreamSSEFraming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	upstream := strings.Join([]string{
		": keep-alive",
		`event: ping`,
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		`data:[DONE]`,
		``,
	}, "\n\n")
	streamAnthropic(c.Writer, strings.NewReader(upstream), translator.NewAnthropicStream("m", ""), edgeTestServer(t), func([]byte) {})
	body := rec.Body.String()
	if !strings.Contains(body, "text_delta") || !strings.Contains(body, "message_stop") {
		t.Fatalf("framing broke the stream: %s", body)
	}
}
