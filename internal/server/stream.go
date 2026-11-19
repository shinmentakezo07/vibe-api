package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/translator"
)

// streamSSE forwards Server-Sent Events line by line, rewrites Mistral chunk
// payloads into OpenAI shape, and coalesces tool-call argument fragments.
//
// Some upstream models stream tool arguments in fragments that themselves start
// with "{" or "[". OpenAI-compatible clients that treat such a chunk as a fresh
// JSON snapshot then discard the preceding fragments. Buffering the fragments
// and emitting one complete call at finish keeps the wire format valid for
// every client.
func streamSSE(w gin.ResponseWriter, src io.Reader, log *slog.Logger, onData func([]byte)) {
	reader := bufio.NewReader(src)
	state := &openAIStreamState{}
	for {
		line, err := reader.ReadString('\n')
		if data, ok := sseData(line); ok && data != "[DONE]" {
			onData([]byte(data))
			for _, chunk := range state.add([]byte(data)) {
				if !writeSSEData(w, chunk) {
					return
				}
			}
		} else if data == "[DONE]" {
			// Buffered tool calls, if any, must reach the client before the
			// terminal marker; clients stop reading at [DONE].
			for _, chunk := range state.flush() {
				if !writeSSEData(w, chunk) {
					return
				}
			}
			if _, werr := w.Write([]byte(line)); werr != nil {
				return
			}
			w.Flush()
			return
		} else if line != "" {
			if _, werr := w.Write([]byte(line)); werr != nil {
				return
			}
			w.Flush()
		}
		if err != nil {
			for _, chunk := range state.flush() {
				if !writeSSEData(w, chunk) {
					return
				}
			}
			if !errors.Is(err, io.EOF) {
				log.Error("relay stream", "error", err)
			}
			return
		}
	}
}

func writeSSEData(w gin.ResponseWriter, payload []byte) bool {
	_, err := w.Write(append(append([]byte("data: "), payload...), '\n'))
	if err != nil {
		return false
	}
	w.Flush()
	return true
}

type openAIStreamState struct {
	header map[string]any
	calls  map[int]*streamChoiceCalls
}

type streamChoiceCalls struct {
	index int
	calls map[int]*streamToolCall
}

type streamToolCall struct {
	index int
	id    string
	name  string
	kind  string
	args  string
}

// add translates one upstream chunk and returns the chunks to send downstream.
func (s *openAIStreamState) add(data []byte) [][]byte {
	chunk := translator.Chunk(data)
	var payload map[string]any
	if err := json.Unmarshal(chunk, &payload); err != nil {
		return [][]byte{chunk}
	}
	for _, key := range []string{"id", "object", "created", "model"} {
		if value, ok := payload[key]; ok {
			if s.header == nil {
				s.header = map[string]any{}
			}
			s.header[key] = value
		}
	}

	choices, _ := payload["choices"].([]any)
	finished := []int{}
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		if reason := choice["finish_reason"]; reason != nil {
			finished = append(finished, choiceIndex(choice))
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if toolCalls, ok := delta["tool_calls"].([]any); ok {
			s.addToolCalls(choiceIndex(choice), toolCalls)
			delete(delta, "tool_calls")
		}
	}

	var out [][]byte
	if len(finished) > 0 {
		if synthetic := s.synthetic(finished); synthetic != nil {
			out = append(out, synthetic)
		}
	}
	if hasContent(payload) {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return [][]byte{chunk}
		}
		out = append(out, encoded)
	}
	return out
}

func (s *openAIStreamState) addToolCalls(choice int, rawCalls []any) {
	group := s.calls[choice]
	if group == nil {
		group = &streamChoiceCalls{index: choice, calls: map[int]*streamToolCall{}}
		if s.calls == nil {
			s.calls = map[int]*streamChoiceCalls{}
		}
		s.calls[choice] = group
	}
	for position, raw := range rawCalls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		index := position
		if value, ok := integer(call["index"]); ok {
			index = value
		}
		buffered := group.calls[index]
		if buffered == nil {
			buffered = &streamToolCall{index: index}
			group.calls[index] = buffered
		}
		if value, _ := call["id"].(string); value != "" {
			buffered.id = value
		}
		if value, _ := call["type"].(string); value != "" {
			buffered.kind = value
		}
		function, _ := call["function"].(map[string]any)
		if value, _ := function["name"].(string); value != "" {
			buffered.name = value
		}
		if value, _ := function["arguments"].(string); value != "" {
			buffered.args += value
		}
	}
}

// flush emits any buffered calls when the upstream stream ends without a
// finish chunk.
func (s *openAIStreamState) flush() [][]byte {
	indexes := make([]int, 0, len(s.calls))
	for index := range s.calls {
		indexes = append(indexes, index)
	}
	if synthetic := s.synthetic(indexes); synthetic != nil {
		return [][]byte{synthetic}
	}
	return nil
}

func (s *openAIStreamState) synthetic(indexes []int) []byte {
	if len(indexes) == 0 {
		return nil
	}
	choices := make([]any, 0, len(indexes))
	choiceIndexes := append([]int(nil), indexes...)
	sort.Ints(choiceIndexes)
	for _, choiceIndex := range choiceIndexes {
		group := s.calls[choiceIndex]
		if group == nil {
			continue
		}
		callIndexes := make([]int, 0, len(group.calls))
		for index := range group.calls {
			callIndexes = append(callIndexes, index)
		}
		sort.Ints(callIndexes)
		calls := make([]any, 0, len(callIndexes))
		for _, callIndex := range callIndexes {
			call := group.calls[callIndex]
			function := map[string]any{"arguments": call.args}
			if function["arguments"] == "" {
				function["arguments"] = "{}"
			}
			if call.name != "" {
				function["name"] = call.name
			}
			out := map[string]any{
				"index":    call.index,
				"type":     call.kind,
				"function": function,
			}
			if out["type"] == "" {
				out["type"] = "function"
			}
			if call.id != "" {
				out["id"] = call.id
			}
			calls = append(calls, out)
		}
		choices = append(choices, map[string]any{
			"index":         choiceIndex,
			"delta":         map[string]any{"tool_calls": calls},
			"finish_reason": nil,
		})
	}
	payload := map[string]any{"choices": choices}
	for key, value := range s.header {
		payload[key] = value
	}
	for _, choiceIndex := range choiceIndexes {
		delete(s.calls, choiceIndex)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return encoded
}

func choiceIndex(choice map[string]any) int {
	if index, ok := integer(choice["index"]); ok {
		return index
	}
	return 0
}

func integer(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int(number), true
}

// hasContent reports whether a chunk still carries a delta, finish reason, or
// top-level fields after tool-call fragments are removed.
func hasContent(payload map[string]any) bool {
	choices, _ := payload["choices"].([]any)
	if len(choices) == 0 {
		return true
	}
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			return true
		}
		if choice["finish_reason"] != nil {
			return true
		}
		if delta, ok := choice["delta"].(map[string]any); ok && len(delta) > 0 {
			return true
		}
	}
	return false
}
