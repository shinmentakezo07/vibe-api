// Package translator converts between the OpenAI wire format and the shape
// Mistral accepts and returns.
package translator

// Request rewrites an OpenAI chat request into the shape Mistral accepts.
// Fields outside the translation pass through untouched, so parameters Mistral
// adds keep working without proxy changes.
func Request(payload map[string]any) {
	dropUnsupported(payload)
	normalizeReasoningEffort(payload)
	normalizeMaxTokens(payload)
	translateTools(payload)
	translateMessages(payload)
}

func dropUnsupported(payload map[string]any) {
	// Mistral emits usage in streams without being asked and rejects the field.
	delete(payload, "stream_options")
}

// normalizeReasoningEffort maps the effort names OpenAI and OpenRouter clients
// send onto the levels Mistral models accept.
func normalizeReasoningEffort(payload map[string]any) {
	// OpenRouter-style `reasoning: {"effort": ...}` carries the same setting.
	if reasoning, ok := payload["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok {
			if _, has := payload["reasoning_effort"]; !has {
				payload["reasoning_effort"] = effort
			}
		}
		delete(payload, "reasoning")
	}

	effort, ok := payload["reasoning_effort"].(string)
	if !ok {
		return
	}
	switch effort {
	case "none", "off":
		// Models that always reason reject the field; omitting it is the
		// compatible request.
		delete(payload, "reasoning_effort")
	case "minimal":
		payload["reasoning_effort"] = "low"
	case "xhigh", "max":
		payload["reasoning_effort"] = "high"
	}
}

func normalizeMaxTokens(payload map[string]any) {
	if value, ok := payload["max_completion_tokens"]; ok {
		if _, has := payload["max_tokens"]; !has {
			payload["max_tokens"] = value
		}
		delete(payload, "max_completion_tokens")
	}
}

func translateMessages(payload map[string]any) {
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
		sanitizeMessage(message)
	}
}

// sanitizeMessage drops fields OpenAI clients may send that Mistral's message
// schemas reject with 422.
func sanitizeMessage(message map[string]any) {
	switch message["role"] {
	case "assistant":
		keepFields(message, "role", "content", "tool_calls", "prefix")
	case "tool":
		keepFields(message, "role", "content", "tool_call_id", "name")
	default: // system, user
		keepFields(message, "role", "content")
	}
}

func keepFields(message map[string]any, fields ...string) {
	keep := make(map[string]bool, len(fields))
	for _, field := range fields {
		keep[field] = true
	}
	for field := range message {
		if !keep[field] {
			delete(message, field)
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
