package translator

import (
	"encoding/json"
	"sort"
)

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
	index int    // -1 until the block has a content_block_start event
	kind  string // text, thinking, tool_use
	id    any    // tool_use fields that may arrive after the first fragment
	name  any
	args  string // argument fragments buffered before the block can start
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
		events = append(events, s.deltaAt(s.open.index, "thinking_delta", map[string]any{"thinking": thinking})...)
	}
	if text, _ := delta["content"].(string); text != "" {
		events = append(events, s.startBlock("text")...)
		events = append(events, s.deltaAt(s.open.index, "text_delta", map[string]any{"text": text})...)
	}

	if calls, ok := delta["tool_calls"].([]any); ok {
		for i, raw := range calls {
			call, _ := raw.(map[string]any)
			index := toolCallIndex(call, i)
			function, _ := call["function"].(map[string]any)
			arguments, _ := function["arguments"].(string)
			events = append(events, s.toolCall(index, call, function, arguments)...)
		}
	}
	return events
}

// toolCall opens the tool_use block for a call index on its first delta and
// forwards argument fragments. Fragments that arrive before the id/name do
// not start the block: content_block_start must carry both, so the pending
// fragments are buffered and flushed once the block can open.
func (s *AnthropicStream) toolCall(index int, call, function map[string]any, arguments string) []Event {
	block := s.tools[index]
	if block == nil {
		block = &contentBlock{index: -1, kind: "tool_use"}
		s.tools[index] = block
	}
	block.id = firstNonNil(call["id"], block.id)
	block.name = firstNonNil(function["name"], block.name)
	if arguments != "" {
		block.args += arguments
	}

	if block.index < 0 {
		// Not started yet: flush when id and name are both known.
		if block.id == nil || block.name == nil {
			return nil
		}
		events := s.closeTextBlock()
		block.index = s.next
		s.next++
		events = append(events, Event{Name: "content_block_start", Data: map[string]any{
			"type":  "content_block_start",
			"index": block.index,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    block.id,
				"name":  block.name,
				"input": map[string]any{},
			},
		}})
		events = append(events, s.flushArgs(block)...)
		return events
	}
	return s.flushArgs(block)
}

// flushArgs emits the buffered argument fragments as one input_json_delta.
func (s *AnthropicStream) flushArgs(block *contentBlock) []Event {
	if block.args == "" {
		return nil
	}
	partial := block.args
	block.args = ""
	return s.deltaAt(block.index, "input_json_delta", map[string]any{
		"partial_json": partial,
	})
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
	events = append(events, s.closeAllBlocks()...)
	events = append(events, Event{Name: "message_delta", Data: map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": anthropicStopReason(s.reason), "stop_sequence": nil},
		"usage": usage,
	}})
	events = append(events, Event{Name: "message_stop", Data: map[string]any{"type": "message_stop"}})
	return events
}

// startBlock opens a text or thinking block. Starting a new block closes the
// previous one; open tool blocks stay open so interleaved argument deltas can
// target their own index.
func (s *AnthropicStream) startBlock(kind string) []Event {
	if s.open != nil && s.open.kind == kind {
		return nil
	}
	events := s.closeTextBlock()

	block := map[string]any{"type": kind}
	switch kind {
	case "text":
		block["text"] = ""
	case "thinking":
		block["thinking"] = ""
		block["signature"] = ""
	}

	s.open = &contentBlock{index: s.next, kind: kind}
	s.next++
	return append(events, Event{Name: "content_block_start", Data: map[string]any{
		"type":          "content_block_start",
		"index":         s.open.index,
		"content_block": block,
	}})
}

func (s *AnthropicStream) deltaAt(index int, kind string, delta map[string]any) []Event {
	delta["type"] = kind
	return []Event{{Name: "content_block_delta", Data: map[string]any{
		"type":  "content_block_delta",
		"index": index,
		"delta": delta,
	}}}
}

func (s *AnthropicStream) closeTextBlock() []Event {
	if s.open == nil {
		return nil
	}
	index := s.open.index
	s.open = nil
	return []Event{{Name: "content_block_stop", Data: map[string]any{
		"type": "content_block_stop", "index": index,
	}}}
}

// closeAllBlocks emits one content_block_stop per open block in index order.
func (s *AnthropicStream) closeAllBlocks() []Event {
	blocks := make([]*contentBlock, 0, len(s.tools)+1)
	if s.open != nil {
		blocks = append(blocks, s.open)
		s.open = nil
	}
	for _, block := range s.tools {
		if block.index < 0 {
			continue // block never got a content_block_start on the wire
		}
		blocks = append(blocks, block)
	}
	s.tools = map[int]*contentBlock{}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].index < blocks[j].index })

	events := make([]Event, 0, len(blocks))
	for _, block := range blocks {
		events = append(events, Event{Name: "content_block_stop", Data: map[string]any{
			"type": "content_block_stop", "index": block.index,
		}})
	}
	return events
}

// firstNonNil returns the first value that is present and non-empty, so a
// tool_use block can start before its name/id fragment arrives.
func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil && value != "" {
			return value
		}
	}
	return nil
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
