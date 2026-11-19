package openai

import (
	"encoding/json"
	"testing"
)

func TestRewriteCompletion(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"why"}],"closed":true},{"type":"text","text":"hello"}]}}]}`)
	out, err := RewriteCompletion(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	msg := payload.Choices[0].Message
	if msg.Content != "hello" || msg.Reasoning != "why" {
		t.Fatalf("content=%q reasoning=%q", msg.Content, msg.Reasoning)
	}
}

func TestRewriteChunk(t *testing.T) {
	in := []byte(`{"choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"why"}]}]}}]}`)
	out := RewriteChunk(in)
	var payload struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	delta := payload.Choices[0].Delta
	if delta.Content != "" || delta.Reasoning != "why" {
		t.Fatalf("content=%q reasoning=%q", delta.Content, delta.Reasoning)
	}
}

func TestRewriteChunkLeavesPlainDelta(t *testing.T) {
	in := []byte(`{"choices":[{"delta":{"content":"hi"}}]}`)
	if out := RewriteChunk(in); string(out) != string(in) {
		t.Fatalf("rewrote unchanged chunk: %s", out)
	}
}

func TestNormalize(t *testing.T) {
	payload := map[string]any{
		"stream_options":        map[string]any{"include_usage": true},
		"max_completion_tokens": float64(64),
		"messages":              []any{map[string]any{"role": "developer", "content": "x"}},
	}
	Normalize(payload)
	if _, ok := payload["stream_options"]; ok {
		t.Fatal("stream_options not removed")
	}
	if payload["max_tokens"] != float64(64) {
		t.Fatalf("max_tokens=%v", payload["max_tokens"])
	}
	role := payload["messages"].([]any)[0].(map[string]any)["role"]
	if role != "system" {
		t.Fatalf("role=%v", role)
	}
}

func TestNormalizeReplaysReasoningAsThinkingBlocks(t *testing.T) {
	payload := map[string]any{
		"messages": []any{map[string]any{
			"role":              "assistant",
			"content":           "the answer",
			"reasoning_content": "the reason",
		}},
	}
	Normalize(payload)
	msg := payload["messages"].([]any)[0].(map[string]any)
	if _, ok := msg["reasoning_content"]; ok {
		t.Fatal("reasoning_content reached Mistral")
	}
	blocks, ok := msg["content"].([]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("content=%#v", msg["content"])
	}
	thinking := blocks[0].(map[string]any)
	texts := thinking["thinking"].([]any)
	if thinking["type"] != "thinking" || texts[0].(map[string]any)["text"] != "the reason" {
		t.Fatalf("thinking=%#v", thinking)
	}
	if blocks[1].(map[string]any)["text"] != "the answer" {
		t.Fatalf("text block=%#v", blocks[1])
	}
}

func TestNormalizeDropsUnsupportedOffEffort(t *testing.T) {
	for _, effort := range []string{"none", "off"} {
		payload := map[string]any{"reasoning_effort": effort, "messages": []any{}}
		Normalize(payload)
		if _, ok := payload["reasoning_effort"]; ok {
			t.Fatalf("%q was forwarded", effort)
		}
	}
	payload := map[string]any{"reasoning_effort": "high", "messages": []any{}}
	Normalize(payload)
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("high was dropped: %#v", payload)
	}
}

func TestNormalizeClampsEffortExtremes(t *testing.T) {
	cases := map[string]any{"minimal": "low", "xhigh": "high", "max": "high"}
	for in, want := range cases {
		payload := map[string]any{"reasoning_effort": in, "messages": []any{}}
		Normalize(payload)
		if payload["reasoning_effort"] != want {
			t.Fatalf("%s -> %v, want %s", in, payload["reasoning_effort"], want)
		}
	}
}
