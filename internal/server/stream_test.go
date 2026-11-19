package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStreamSSECoalescesToolArgumentFragments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	upstream := strings.Join([]string{
		`data: {"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","type":"function","function":{"name":"read_files","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"files\": "}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"[{\"path\":\"/x\"}]}"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"total_tokens":7}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	streamSSE(context.Writer, strings.NewReader(upstream), slog.New(slog.NewTextHandler(io.Discard, nil)), func([]byte) {})

	var calls []string
	var sawFinish bool
	var sawDone bool
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("unexpected SSE line %q", line)
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			sawDone = true
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Function struct {
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("invalid output chunk %q: %v", data, err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if chunk.Choices[0].FinishReason != nil {
			sawFinish = true
		}
		for _, call := range chunk.Choices[0].Delta.ToolCalls {
			calls = append(calls, call.Function.Arguments)
		}
	}
	if !sawDone || !sawFinish {
		t.Fatalf("stream incomplete: done=%v finish=%v", sawDone, sawFinish)
	}
	if len(calls) != 1 {
		t.Fatalf("tool call chunks=%d, want one complete call: %#v", len(calls), calls)
	}
	if calls[0] != `{"files": [{"path":"/x"}]}` {
		t.Fatalf("arguments=%q", calls[0])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(calls[0]), &parsed); err != nil {
		t.Fatalf("coalesced arguments are not JSON: %v", err)
	}
}
