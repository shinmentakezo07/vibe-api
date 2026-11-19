package translator

import "encoding/json"

// Event is one Anthropic SSE event.
type Event struct {
	Name string
	Data map[string]any
}

// AnthropicStream turns Mistral chat chunks into Anthropic Messages events.
type AnthropicStream struct {
	model  string
	id     string
	next   int
	open   *contentBlock
	tools  map[int]*contentBlock
	reason string
	input  int
	output int
	done   bool
}

type contentBlock struct {
	index int
	kind  string // text, thinking, tool_use
}

// NewAnthropicStream starts a stream for one message.
func NewAnthropicStream(model, id string) *AnthropicStream {
	return &AnthropicStream{
		model: model,
		id:    anthropicMessageID(id),
		tools: map[int]*contentBlock{},
	}
}

// Start emits message_start.
func (s *AnthropicStream) Start() []Event {
	return []Event{{Name: "message_start", Data: map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.id,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         s.model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	}}}
}

// Feed consumes one Mistral chunk, already normalized by Chunk, and returns
// the Anthropic events it produces.
func (s *AnthropicStream) Feed(data []byte) []Event {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if usage, ok := payload["usage"].(map[string]any); ok {
		s.input = usageInt(usage, "prompt_tokens")
		s.output = usageInt(usage, "completion_tokens")
	}

	choice := firstChoice(payload)
	if choice == nil {
		return nil
	}
	delta, _ := choice["delta"].(map[string]any)
	if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
		s.reason = reason
	}

	var events []Event
	if thinking, _ := delta["reasoning_content"].(string); thinking != "" {
		events = append(events, s.startBlock("thinking")...)
		events = append(events, s.delta("thinking_delta", map[string]any{"thinking": thinking})...)
	}
	if text, _ := delta["content"].(string); text != "" {
		events = append(events, s.startBlock("text")...)
		events = append(events, s.delta("text_delta", map[string]any{"text": text})...)
	}

	if calls, ok := delta["tool_calls"].([]any); ok {
		for i, raw := range calls {
			call, _ := raw.(map[string]any)
			index := toolCallIndex(call, i)
			block := s.tools[index]
			if block == nil {
				events = append(events, s.startBlock("tool_use", call)...)
				block = s.open
				s.tools[index] = block
			}
			function, _ := call["function"].(map[string]any)
			if arguments, _ := function["arguments"].(string); arguments != "" {
				events = append(events, s.delta("input_json_delta", map[string]any{
					"partial_json": arguments,
				})...)
			}
		}
	}
	return events
}

// Finish emits the closing block events and message_delta/message_stop once.
func (s *AnthropicStream) Finish() []Event {
	if s.done {
		return nil
	}
	s.done = true

	usage := map[string]any{"output_tokens": s.output}
	// Mistral reports prompt tokens only with the final chunk. Anthropic treats
	// message_delta usage as cumulative, so the true input count goes here
	// rather than remaining 0 from message_start.
	if s.input > 0 {
		usage["input_tokens"] = s.input
	}

	var events []Event
	events = append(events, s.closeBlock()...)
	events = append(events, Event{Name: "message_delta", Data: map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": anthropicStopReason(s.reason), "stop_sequence": nil},
		"usage": usage,
	}})
	events = append(events, Event{Name: "message_stop", Data: map[string]any{"type": "message_stop"}})
	return events
}

// startBlock closes the open block and opens one of the given kind. Tool use
// takes the call's id and name.
func (s *AnthropicStream) startBlock(kind string, call ...map[string]any) []Event {
	if s.open != nil && s.open.kind == kind && kind != "tool_use" {
		return nil
	}
	events := s.closeBlock()

	block := map[string]any{"type": kind}
	switch kind {
	case "text":
		block["text"] = ""
	case "thinking":
		block["thinking"] = ""
		block["signature"] = ""
	case "tool_use":
		if len(call) > 0 {
			function, _ := call[0]["function"].(map[string]any)
			block["id"] = call[0]["id"]
			block["name"] = function["name"]
			block["input"] = map[string]any{}
		}
	}

	s.open = &contentBlock{index: s.next, kind: kind}
	s.next++
	return append(events, Event{Name: "content_block_start", Data: map[string]any{
		"type":          "content_block_start",
		"index":         s.open.index,
		"content_block": block,
	}})
}

func (s *AnthropicStream) delta(kind string, delta map[string]any) []Event {
	if s.open == nil {
		return nil
	}
	delta["type"] = kind
	return []Event{{Name: "content_block_delta", Data: map[string]any{
		"type":  "content_block_delta",
		"index": s.open.index,
		"delta": delta,
	}}}
}

func (s *AnthropicStream) closeBlock() []Event {
	if s.open == nil {
		return nil
	}
	index := s.open.index
	s.open = nil
	return []Event{{Name: "content_block_stop", Data: map[string]any{
		"type": "content_block_stop", "index": index,
	}}}
}

func toolCallIndex(call map[string]any, fallback int) int {
	switch index := call["index"].(type) {
	case float64:
		return int(index)
	case int:
		return index
	default:
		return fallback
	}
}
