package translator

import "encoding/json"

// Response turns a Mistral chat completion into OpenAI shape: the reasoning
// blocks Mistral returns inside `content` become a string `content` plus an
// optional `reasoning_content`, and tool calls get OpenAI-safe fields.
func Response(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	choices, _ := payload["choices"].([]any)
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		message, ok := choice["message"].(map[string]any)
		if !ok {
			continue
		}
		rewriteMessage(message)
		normalizeToolCalls(message, false)
		if len(toolCalls(message)) > 0 {
			if reason, _ := choice["finish_reason"].(string); reason == "" {
				choice["finish_reason"] = "tool_calls"
			}
		}
	}
	return json.Marshal(payload)
}

// Chunk turns one streamed Mistral `chat.completion.chunk` into OpenAI shape.
// It returns the input unchanged when nothing needed changing.
func Chunk(data []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}

	choices, _ := payload["choices"].([]any)
	changed := false
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}
		if rewriteMessage(delta) {
			changed = true
		}
		if normalizeToolCalls(delta, true) {
			changed = true
		}
	}
	if !changed {
		return data
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return rewritten
}

// rewriteMessage flattens an array-shaped `content` into a string and reports
// whether the message changed.
func rewriteMessage(message map[string]any) bool {
	blocks, ok := message["content"].([]any)
	if !ok {
		return false
	}
	text, reasoning := splitBlocks(blocks)
	message["content"] = text
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	return true
}

func splitBlocks(blocks []any) (text, reasoning string) {
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			text += blockText(block["text"])
		case "thinking":
			reasoning += blockText(block["thinking"])
		default:
			text += blockText(block["text"])
		}
	}
	return text, reasoning
}

// blockText accepts the shapes Mistral uses for text payloads: a plain string
// or a list of `{"type":"text","text":"..."}` blocks.
func blockText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var out string
		for _, raw := range v {
			block, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if nested, ok := block["text"]; ok {
				out += blockText(nested)
			}
		}
		return out
	case map[string]any:
		return blockText(v["text"])
	default:
		return ""
	}
}
