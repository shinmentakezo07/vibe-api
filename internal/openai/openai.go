// Package openai holds OpenAI wire types and the small compatibility rewrites
// Mistral's API needs.
package openai

// Error is the OpenAI-style error envelope.
type Error struct {
	Error Detail `json:"error"`
}

// Detail is the body of an Error.
type Detail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// NewError builds an OpenAI-compatible error body.
func NewError(message, kind string) Error {
	return Error{Error: Detail{Message: message, Type: kind}}
}

// Normalize rewrites the OpenAI fields Mistral rejects and leaves every other
// field untouched, so newly added OpenAI parameters keep passing through.
func Normalize(payload map[string]any) {
	// Mistral emits usage in streams without being asked and rejects the field.
	delete(payload, "stream_options")

	// Normalize OpenAI-style effort names to the levels Mistral models take.
	// "none"/"off" is dropped because models that always reason reject it;
	// the extremes clamp to the nearest supported level.
	if effort, ok := payload["reasoning_effort"].(string); ok {
		switch effort {
		case "none", "off":
			delete(payload, "reasoning_effort")
		case "minimal":
			payload["reasoning_effort"] = "low"
		case "xhigh", "max":
			payload["reasoning_effort"] = "high"
		}
	}

	if value, ok := payload["max_completion_tokens"]; ok {
		if _, has := payload["max_tokens"]; !has {
			payload["max_tokens"] = value
		}
		delete(payload, "max_completion_tokens")
	}

	messages, _ := payload["messages"].([]any)
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if message["role"] == "developer" {
			message["role"] = "system"
		}
		if message["role"] == "assistant" {
			replayReasoning(message)
		}
	}
}

// replayReasoning folds OpenAI's `reasoning_content` string back into the
// structured thinking blocks Mistral accepts on an assistant message. Mistral
// rejects the flat field with 422 on every request after the first.
func replayReasoning(message map[string]any) {
	reasoning, _ := message["reasoning_content"].(string)
	if reasoning == "" {
		reasoning, _ = message["reasoning"].(string)
	}
	delete(message, "reasoning_content")
	delete(message, "reasoning")
	if reasoning == "" {
		return
	}

	content, _ := message["content"].(string)
	blocks := []any{map[string]any{
		"type":     "thinking",
		"thinking": []any{map[string]any{"type": "text", "text": reasoning}},
	}}
	if content != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": content})
	}
	message["content"] = blocks
}

// IsStream reports whether the request asked for a streamed reply.
func IsStream(payload map[string]any) bool {
	stream, _ := payload["stream"].(bool)
	return stream
}
