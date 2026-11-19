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

// relayOpenAI streams the given SSE frames through the OpenAI relay and
// returns what the client would receive.
func relayOpenAI(t *testing.T, frames []string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	streamSSE(c.Writer, strings.NewReader(strings.Join(frames, "\n\n")), slog.New(slog.NewTextHandler(io.Discard, nil)), func([]byte) {})
	return rec.Body.String()
}

// decodeChunks parses every non-[DONE] SSE data frame the relay emitted.
func decodeChunks(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, frame := range dataFrames(t, body) {
		if frame == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(frame), &chunk); err != nil {
			t.Fatalf("invalid output chunk %q: %v", frame, err)
		}
		out = append(out, chunk)
	}
	return out
}

func callArguments(t *testing.T, chunk map[string]any) []string {
	t.Helper()
	var args []string
	for _, rawChoice := range chunk["choices"].([]any) {
		choice := rawChoice.(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		calls, _ := delta["tool_calls"].([]any)
		for _, rawCall := range calls {
			call := rawCall.(map[string]any)
			function := call["function"].(map[string]any)
			args = append(args, function["arguments"].(string))
		}
	}
	return args
}

// Buffered tool-call fragments must be coalesced and reach the client BEFORE
// the [DONE] marker: OpenAI clients stop reading at [DONE] and would
// otherwise lose the call entirely.
func TestStreamSSEFlushesCallsBeforeDone(t *testing.T) {
	body := relayOpenAI(t, []string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"bash","arguments":"{\"cmd\":"}}]},"finish_reason":null}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]},"finish_reason":null}]}`,
		`data: [DONE]`,
	})
	doneAt := -1
	for i, frame := range dataFrames(t, body) {
		if frame == "[DONE]" {
			doneAt = i
		}
	}
	chunks := decodeChunks(t, body)
	if len(chunks) != 1 {
		t.Fatalf("chunks=%d, want a single coalesced call: %s", len(chunks), body)
	}
	args := callArguments(t, chunks[0])
	if len(args) != 1 || args[0] != `{"cmd":"ls"}` {
		t.Fatalf("arguments=%v", args)
	}
	// The synthetic frame is dataFrames index len(chunks) — before [DONE].
	if doneAt != len(chunks) {
		t.Fatalf("synthetic frame after [DONE]: doneAt=%d chunks=%d body=%s", doneAt, len(chunks), body)
	}
}

// Mid-stream upstream error frames pass through unchanged rather than being
// dropped or mangled by the coalescer.
func TestStreamSSERelaysUpstreamError(t *testing.T) {
	errorFrame := `{"error":{"message":"quota exceeded","type":"tokens_exceeded"}}`
	body := relayOpenAI(t, []string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		`data: ` + errorFrame,
		`data: [DONE]`,
	})
	var sawError bool
	for _, chunk := range decodeChunks(t, body) {
		if _, ok := chunk["error"]; ok {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("error frame dropped: %s", body)
	}
}

// Non-OpenAI SSE frames (comments, event lines) relay verbatim; chunks with
// no choices relay untouched.
func TestStreamSSERelaysFramingVerbatim(t *testing.T) {
	body := relayOpenAI(t, []string{
		`: keep-alive`,
		`event: ping`,
		`data: {"id":"x","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		`data:[DONE]`,
	})
	if !strings.Contains(body, ": keep-alive") || !strings.Contains(body, "event: ping") {
		t.Fatalf("framing lines dropped: %s", body)
	}
	if !strings.Contains(body, `"content":"hi"`) {
		t.Fatalf("content chunk lost: %s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Fatalf("DONE marker lost: %s", body)
	}
}

// A tool call whose id and name arrive in fragments after arguments still
// gets one complete, valid call with the correct id.
func TestStreamSSELateIDAndName(t *testing.T) {
	body := relayOpenAI(t, []string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"bash","arguments":"{\"a\":"}}]},"finish_reason":null}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"late"}]}}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	chunks := decodeChunks(t, body)
	var arg, id any
	for _, chunk := range chunks {
		for _, rawChoice := range chunk["choices"].([]any) {
			choice := rawChoice.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			calls, _ := delta["tool_calls"].([]any)
			for _, rawCall := range calls {
				call := rawCall.(map[string]any)
				function := call["function"].(map[string]any)
				arg = function["arguments"]
				id = call["id"]
				if name := function["name"]; name != nil {
					if name != "bash" {
						t.Fatalf("name=%v", name)
					}
				}
			}
		}
	}
	if arg != `{"a":1}` || id != "late" {
		t.Fatalf("arguments=%v id=%v body=%s", arg, id, body)
	}
}

// Arguments sent as a JSON object (not a string) in a stream delta are
// encoded before buffering.
func TestStreamSSEObjectArguments(t *testing.T) {
	body := relayOpenAI(t, []string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"bash","arguments":{"cmd":"ls"}}}]},"finish_reason":null}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	chunks := decodeChunks(t, body)
	found := false
	for _, chunk := range chunks {
		for _, args := range callArguments(t, chunk) {
			found = true
			var parsed map[string]any
			if err := json.Unmarshal([]byte(args), &parsed); err != nil {
				t.Fatalf("arguments not JSON: %q", args)
			}
			if parsed["cmd"] != "ls" {
				t.Fatalf("arguments=%q", args)
			}
		}
	}
	if !found {
		t.Fatalf("no tool call emitted: %s", body)
	}
}

// Two choices each with their own tool call stay separate; each gets its own
// synthetic frame with the right `index`.
func TestStreamSSETwoChoicesSeparateCalls(t *testing.T) {
	body := relayOpenAI(t, []string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"one","arguments":"{}"}}]}},{"index":1,"delta":{"tool_calls":[{"index":0,"id":"b","function":{"name":"two","arguments":"{}"}}]}}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	chunks := decodeChunks(t, body)
	seen := map[string]bool{}
	for _, chunk := range chunks {
		for _, rawChoice := range chunk["choices"].([]any) {
			choice := rawChoice.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			calls, _ := delta["tool_calls"].([]any)
			for _, rawCall := range calls {
				call := rawCall.(map[string]any)
				function := call["function"].(map[string]any)
				seen[function["name"].(string)] = true
				if int(choice["index"].(float64)) != int(call["index"].(float64)) &&
					call["id"] == "b" && choice["index"] == float64(0) {
					t.Fatalf("call crossed choices: %v", choice)
				}
			}
		}
	}
	if !seen["one"] || !seen["two"] {
		t.Fatalf("calls missing: %v body=%s", seen, body)
	}
}
