package translator

import (
	"encoding/json"
	"testing"
)

func TestAnthropicRequest(t *testing.T) {
	payload := map[string]any{
		"model":      "claude-sonnet-4-5",
		"max_tokens": float64(256),
		"system":     "be brief",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "thinking", "thinking": "why"},
				map[string]any{"type": "text", "text": "running"},
				map[string]any{"type": "tool_use", "id": "toolu_1", "name": "bash", "input": map[string]any{"command": "ls"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "content": "a.go"},
			}},
		},
		"tools": []any{map[string]any{
			"name": "bash", "description": "run", "input_schema": map[string]any{"type": "object"},
		}},
		"tool_choice": map[string]any{"type": "any"},
	}

	req := AnthropicRequest(payload)
	if req["max_tokens"] != float64(256) {
		t.Fatalf("max_tokens=%v", req["max_tokens"])
	}
	if req["tool_choice"] != "required" {
		t.Fatalf("tool_choice=%v", req["tool_choice"])
	}
	tool := req["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "bash" || tool["parameters"] == nil {
		t.Fatalf("tool=%#v", tool)
	}

	messages := req["messages"].([]any)
	if messages[0].(map[string]any)["role"] != "system" || messages[0].(map[string]any)["content"] != "be brief" {
		t.Fatalf("system=%#v", messages[0])
	}
	assistant := messages[2].(map[string]any)
	blocks := assistant["content"].([]any)
	if blocks[0].(map[string]any)["type"] != "thinking" || blocks[1].(map[string]any)["text"] != "running" {
		t.Fatalf("assistant content=%#v", blocks)
	}
	if _, ok := assistant["reasoning_content"]; ok {
		t.Fatalf("reasoning_content reached Mistral: %#v", assistant)
	}
	call := assistant["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "toolu_1" || call["function"].(map[string]any)["arguments"] != `{"command":"ls"}` {
		t.Fatalf("call=%#v", call)
	}
	result := messages[3].(map[string]any)
	if result["role"] != "tool" || result["tool_call_id"] != "toolu_1" || result["content"] != "a.go" {
		t.Fatalf("result=%#v", result)
	}
}

func TestAnthropicResponse(t *testing.T) {
	in := []byte(`{"id":"abc","model":"m","usage":{"prompt_tokens":7,"completion_tokens":3},"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"why"}],"closed":true},{"type":"text","text":"run"}],"tool_calls":[{"id":"t1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]}}]}`)
	out, err := AnthropicResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "message" || msg.ID != "msg_abc" || msg.StopReason != "tool_use" {
		t.Fatalf("msg=%#v", msg)
	}
	if len(msg.Content) != 3 || msg.Content[0].Type != "thinking" || msg.Content[1].Text != "run" {
		t.Fatalf("content=%#v", msg.Content)
	}
	if msg.Content[2].Type != "tool_use" || msg.Content[2].Input["command"] != "ls" {
		t.Fatalf("tool=%#v", msg.Content[2])
	}
	if msg.Usage.Input != 7 || msg.Usage.Output != 3 {
		t.Fatalf("usage=%#v", msg.Usage)
	}
}

func TestAnthropicStreamToolUse(t *testing.T) {
	stream := NewAnthropicStream("m", "")
	var names []string
	var delta Event
	collect := func(events []Event) {
		for _, event := range events {
			names = append(names, event.Name)
			if event.Name == "message_delta" {
				delta = event
			}
		}
	}
	collect(stream.Start())
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"content":"hello"}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)))
	collect(stream.Finish())

	want := []string{
		"message_start",
		"content_block_start", "content_block_delta",
		"content_block_stop",
		"content_block_start", "content_block_delta",
		"content_block_stop", "message_delta", "message_stop",
	}
	if len(names) != len(want) {
		t.Fatalf("events=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("events=%v", names)
		}
	}
	usage := delta.Data["usage"].(map[string]any)
	if usage["input_tokens"] != 5 || usage["output_tokens"] != 2 {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestAnthropicStreamInterleavedToolDeltas(t *testing.T) {
	stream := NewAnthropicStream("m", "")
	var starts, stops []int
	deltas := map[int]string{}
	collect := func(events []Event) {
		for _, event := range events {
			switch event.Name {
			case "content_block_start":
				starts = append(starts, int(event.Data["index"].(int)))
			case "content_block_stop":
				stops = append(stops, int(event.Data["index"].(int)))
			case "content_block_delta":
				index := event.Data["index"].(int)
				delta := event.Data["delta"].(map[string]any)
				if partial, ok := delta["partial_json"].(string); ok {
					deltas[index] += partial
				}
			}
		}
	}
	collect(stream.Start())
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"one"}}]}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"two"}}]}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":"}}]}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"b\":"}}]}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`)))
	collect(stream.Feed([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}}]}}]}`)))
	collect(stream.Finish())

	if len(starts) != 2 || starts[0] != 0 || starts[1] != 1 {
		t.Fatalf("starts=%v", starts)
	}
	if deltas[0] != `{"a":1}` || deltas[1] != `{"b":2}` {
		t.Fatalf("deltas=%#v", deltas)
	}
	if len(stops) != 2 || stops[0] != 0 || stops[1] != 1 {
		t.Fatalf("stops=%v", stops)
	}
}
