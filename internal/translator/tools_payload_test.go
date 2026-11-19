package translator

import (
	"encoding/json"
	"testing"
)

// Regression: a tool_use block must carry a real id/name even when the first
// delta for its index holds only an argument fragment and the id/name delta
// arrives afterwards.
func TestAnthropicStreamToolNameAfterArguments(t *testing.T) {
	s := NewAnthropicStream("m", "")
	s.Start()
	chunks := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"bash"}}]}}]}`,
	}
	var started map[string]any
	for _, c := range chunks {
		for _, e := range s.Feed(Chunk([]byte(c))) {
			if e.Name == "content_block_start" {
				started = e.Data["content_block"].(map[string]any)
			}
		}
	}
	if started == nil {
		t.Fatal("no content_block_start emitted")
	}
	if started["id"] != "t1" || started["name"] != "bash" || started["type"] != "tool_use" {
		t.Fatalf("content_block=%v", started)
	}
}

// Regression: two request-side tool calls with identical name and arguments
// must still get distinct generated ids so tool results can be matched back.
func TestRequestDistinctCallIDsForIdenticalCalls(t *testing.T) {
	payload := map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "tool_calls": []any{
				map[string]any{"function": map[string]any{"name": "bash", "arguments": `{"x":1}`}},
				map[string]any{"function": map[string]any{"name": "bash", "arguments": `{"x":1}`}},
			}},
		},
	}
	Request(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Messages []struct {
			ToolCalls []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	calls := out.Messages[0].ToolCalls
	if len(calls) != 2 {
		t.Fatalf("calls=%d", len(calls))
	}
	if calls[0].ID == "" || calls[0].ID == calls[1].ID {
		t.Fatalf("ids=%q %q", calls[0].ID, calls[1].ID)
	}
}
