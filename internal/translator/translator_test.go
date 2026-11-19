package translator

import (
	"encoding/json"
	"testing"
)

func TestResponse(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"why"}],"closed":true},{"type":"text","text":"hello"}]}}]}`)
	out, err := Response(in)
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

func TestChunk(t *testing.T) {
	in := []byte(`{"choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"why"}]}]}}]}`)
	out := Chunk(in)
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

func TestChunkLeavesPlainDelta(t *testing.T) {
	in := []byte(`{"choices":[{"delta":{"content":"hi"}}]}`)
	if out := Chunk(in); string(out) != string(in) {
		t.Fatalf("rewrote unchanged chunk: %s", out)
	}
}

func TestRequest(t *testing.T) {
	payload := map[string]any{
		"stream_options":        map[string]any{"include_usage": true},
		"max_completion_tokens": float64(64),
		"messages":              []any{map[string]any{"role": "developer", "content": "x"}},
	}
	Request(payload)
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
	Request(payload)
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
		Request(payload)
		if _, ok := payload["reasoning_effort"]; ok {
			t.Fatalf("%q was forwarded", effort)
		}
	}
	payload := map[string]any{"reasoning_effort": "high", "messages": []any{}}
	Request(payload)
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("high was dropped: %#v", payload)
	}
}

func TestNormalizeClampsEffortExtremes(t *testing.T) {
	cases := map[string]any{"minimal": "low", "xhigh": "high", "max": "high"}
	for in, want := range cases {
		payload := map[string]any{"reasoning_effort": in, "messages": []any{}}
		Request(payload)
		if payload["reasoning_effort"] != want {
			t.Fatalf("%s -> %v, want %s", in, payload["reasoning_effort"], want)
		}
	}
}

func TestRequestLegacyFunctions(t *testing.T) {
	payload := map[string]any{
		"functions": []any{map[string]any{"name": "bash", "parameters": map[string]any{"type": "object"}}},
		"messages": []any{
			map[string]any{
				"role":          "assistant",
				"function_call": map[string]any{"name": "bash", "arguments": `{"command":"ls"}`},
			},
			map[string]any{"role": "function", "name": "bash", "content": "a.go"},
		},
	}
	Request(payload)

	tools := payload["tools"].([]any)
	if tools[0].(map[string]any)["type"] != "function" {
		t.Fatalf("tools=%#v", tools)
	}
	messages := payload["messages"].([]any)
	call := messages[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] == "" || call["type"] != "function" {
		t.Fatalf("call=%#v", call)
	}
	result := messages[1].(map[string]any)
	if result["role"] != "tool" || result["tool_call_id"] != call["id"] {
		t.Fatalf("result=%#v", result)
	}
}

func TestResponseNormalizesToolArguments(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"function":{"name":"bash","arguments":{"command":"ls"}}}]},"finish_reason":""}]}`)
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
	if call.Function.Arguments != `{"command":"ls"}` || call.Type != "function" || call.ID == "" {
		t.Fatalf("call=%#v", call)
	}
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason=%q", choice.FinishReason)
	}
}

func TestChunkAddsToolCallIndex(t *testing.T) {
	in := []byte(`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"bash","arguments":"{}"}}]}}]}`)
	out := Chunk(in)
	var payload struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					Index *int `json:"index"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	index := payload.Choices[0].Delta.ToolCalls[0].Index
	if index == nil || *index != 0 {
		t.Fatalf("index=%v", index)
	}
}

func TestRequestSanitizesForeignMessageFields(t *testing.T) {
	payload := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "name": "alice", "content": "hi"},
			map[string]any{
				"role": "assistant", "content": "hello", "refusal": nil,
				"reasoning_content": "because", "annotations": []any{},
			},
		},
	}
	Request(payload)
	messages := payload["messages"].([]any)
	user := messages[0].(map[string]any)
	if _, ok := user["name"]; ok {
		t.Fatalf("user name reached Mistral: %#v", user)
	}
	assistant := messages[1].(map[string]any)
	for _, field := range []string{"refusal", "annotations", "reasoning_content"} {
		if _, ok := assistant[field]; ok {
			t.Fatalf("%s reached Mistral", field)
		}
	}
	if _, ok := assistant["content"].([]any); !ok {
		t.Fatalf("reasoning was not replayed: %#v", assistant["content"])
	}
}

func TestRequestOpenRouterReasoning(t *testing.T) {
	payload := map[string]any{"reasoning": map[string]any{"effort": "high"}}
	Request(payload)
	if _, ok := payload["reasoning"]; ok {
		t.Fatal("reasoning object reached Mistral")
	}
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("effort=%v", payload["reasoning_effort"])
	}
}

func TestChunkPreservesToolArgumentFragments(t *testing.T) {
	first := Chunk([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"bash","arguments":""}}]}}]}`))
	var payload struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(first, &payload); err != nil {
		t.Fatal(err)
	}
	call := payload.Choices[0].Delta.ToolCalls[0]
	if call.Function.Arguments != "" {
		t.Fatalf("first delta arguments=%q, want empty", call.Function.Arguments)
	}
	if call.ID != "t1" {
		t.Fatalf("first delta id=%q", call.ID)
	}

	noID := Chunk([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"bash"}}]}}]}`))
	var noIDPayload struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(noID, &noIDPayload); err != nil {
		t.Fatal(err)
	}
	if got := noIDPayload.Choices[0].Delta.ToolCalls[0].ID; got != "" {
		t.Fatalf("synthetic id added in stream: %q", got)
	}

	fragment := []byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]}}]}`)
	rewritten := Chunk(fragment)
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload.Choices[0].Delta.ToolCalls[0].Function.Arguments; got != `{"command":` {
		t.Fatalf("fragment arguments=%q", got)
	}
}
