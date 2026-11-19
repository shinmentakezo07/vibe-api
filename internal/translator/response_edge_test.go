package translator

import (
	"encoding/json"
	"testing"
)

// Mistral returns thinking blocks inside content; the OpenAI shape needs a
// flat text content plus reasoning_content.
func TestResponseFlattensThinkingAndText(t *testing.T) {
	in := []byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":[{"type":"thinking","thinking":"why"},{"type":"text","text":"answer"},{"type":"text","text":" more"}]}}]}`)
	out, err := Response(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	m := payload.Choices[0].Message
	if m.Content != "answer more" || m.ReasoningContent != "why" {
		t.Fatalf("content=%q reasoning=%q", m.Content, m.ReasoningContent)
	}
}

// Sparse tool calls (no id, no type, no arguments) get OpenAI-safe defaults.
func TestResponseSynthesizesSparseToolCall(t *testing.T) {
	in := []byte(`{"model":"m","choices":[{"finish_reason":"","message":{"content":"hi","tool_calls":[{"function":{"name":"bash"}}]}}]}`)
	out, err := Response(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	choice := payload.Choices[0]
	call := choice.Message.ToolCalls[0]
	if call.ID == "" || call.Type != "function" || call.Function.Arguments != "{}" {
		t.Fatalf("call=%+v", call)
	}
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason=%q", choice.FinishReason)
	}
}

// Object-shaped arguments are re-encoded as a JSON string.
func TestResponseEncodesObjectArguments(t *testing.T) {
	in := []byte(`{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"content":"hi","tool_calls":[{"id":"t1","type":"function","function":{"name":"bash","arguments":{"cmd":"ls"}}}]}}]}`)
	out, err := Response(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	args := payload.Choices[0].Message.ToolCalls[0].Function.Arguments
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Fatalf("arguments not JSON: %q", args)
	}
	if parsed["cmd"] != "ls" {
		t.Fatalf("arguments=%q", args)
	}
}

// Invalid JSON is an error so the caller can fall back to raw passthrough.
func TestResponseRejectsInvalidJSON(t *testing.T) {
	if _, err := Response([]byte(`{"model": `)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// A tool-only Anthropic reply: no text content block, valid input JSON.
func TestAnthropicResponseToolOnly(t *testing.T) {
	in := []byte(`{"id":"abc","model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"t1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`)
	out, err := AnthropicResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Content []struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Content) != 1 {
		t.Fatalf("content=%+v", payload.Content)
	}
	block := payload.Content[0]
	if block.Type != "tool_use" || block.ID != "t1" || block.Name != "bash" || block.Input["cmd"] != "ls" {
		t.Fatalf("block=%+v", block)
	}
	if payload.StopReason != "tool_use" {
		t.Fatalf("stop_reason=%q", payload.StopReason)
	}
}

// Unparseable tool arguments degrade to an empty input object, not a parse
// error the client can do nothing with.
func TestAnthropicResponseBadArgumentsDegrade(t *testing.T) {
	in := []byte(`{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"content":"h","tool_calls":[{"id":"t1","type":"function","function":{"name":"bash","arguments":"{not json"}}]}}]}`)
	out, err := AnthropicResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Content []struct {
			Type  string         `json:"type"`
			Input map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	checked := false
	for _, block := range payload.Content {
		if block.Type != "tool_use" {
			continue
		}
		checked = true
		if block.Input == nil {
			t.Fatalf("tool_use input must not be null: %s", out)
		}
	}
	if !checked {
		t.Fatalf("no tool_use block in output: %s", out)
	}
}

// Empty choices is an error: the anthropic envelope has nothing to build from.
func TestAnthropicResponseNoChoicesIsError(t *testing.T) {
	if _, err := AnthropicResponse([]byte(`{"model":"m","choices":[]}`)); err == nil {
		t.Fatal("expected error for empty choices")
	}
}
