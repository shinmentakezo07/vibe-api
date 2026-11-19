package openai

import "encoding/json"

// RewriteCompletion turns a Mistral chat completion into OpenAI shape: the
// reasoning blocks Mistral returns inside `content` become a string `content`
// plus an optional `reasoning_content`.
func RewriteCompletion(body []byte) ([]byte, error) {
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
		if message, ok := choice["message"].(map[string]any); ok {
			rewriteMessage(message)
		}
	}
	return json.Marshal(payload)
}

// RewriteChunk turns one streamed Mistral `chat.completion.chunk` into OpenAI
// shape. It returns the input unchanged when it is not a JSON object.
func RewriteChunk(data []byte) []byte {
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
		if delta, ok := choice["delta"].(map[string]any); ok {
			if rewriteMessage(delta) {
				changed = true
			}
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
