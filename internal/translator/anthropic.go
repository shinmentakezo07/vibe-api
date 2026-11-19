package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnthropicRequest converts an Anthropic Messages request into the Mistral
// chat-completions request the upstream client sends.
func AnthropicRequest(payload map[string]any) map[string]any {
	stream, _ := payload["stream"].(bool)
	out := map[string]any{"model": payload["model"], "stream": stream}

	if maxTokens, ok := payload["max_tokens"]; ok {
		out["max_tokens"] = maxTokens
	}
	for _, field := range []string{"temperature", "top_p", "top_k"} {
		if value, ok := payload[field]; ok {
			out[field] = value
		}
	}
	if stop, ok := payload["stop_sequences"]; ok {
		out["stop"] = stop
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if user, ok := metadata["user_id"].(string); ok && user != "" {
			out["metadata"] = map[string]any{"user_id": user}
		}
	}
	if effort := anthropicThinkingEffort(payload["thinking"]); effort != "" {
		out["reasoning_effort"] = effort
	}
	if tools := anthropicTools(payload["tools"]); len(tools) > 0 {
		out["tools"] = tools
		if choice := anthropicToolChoice(payload["tool_choice"]); choice != nil {
			out["tool_choice"] = choice
		}
	}

	out["messages"] = anthropicMessages(payload)
	// Reuse the OpenAI-side normalization: reasoning replay, message field
	// sanitization, and tool-call safety apply to this request too.
	Request(out)
	return out
}

// anthropicMessages turns the system prompt and Anthropic content blocks into
// Mistral messages, splitting tool results into their own tool messages.
func anthropicMessages(payload map[string]any) []any {
	var messages []any
	if system := flattenText(payload["system"]); system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}

	rawMessages, _ := payload["messages"].([]any)
	for _, raw := range rawMessages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		if role == "assistant" {
			messages = append(messages, anthropicAssistantMessage(message["content"]))
			continue
		}
		messages = append(messages, anthropicUserMessages(message["content"])...)
	}
	return messages
}

func anthropicAssistantMessage(content any) map[string]any {
	message := map[string]any{"role": "assistant"}
	if text, ok := content.(string); ok {
		message["content"] = text
		return message
	}

	var text, thinking string
	var calls []any
	for _, raw := range contentBlocks(content) {
		switch raw["type"] {
		case "text":
			part, _ := raw["text"].(string)
			text += part
		case "thinking":
			part, _ := raw["thinking"].(string)
			thinking += part
		case "tool_use":
			name, _ := raw["name"].(string)
			id, _ := raw["id"].(string)
			// A missing input must not serialize as the string "null":
			// OpenAI arguments is a JSON object string.
			input, ok := raw["input"].(map[string]any)
			if !ok {
				input = map[string]any{}
			}
			arguments, _ := json.Marshal(input)
			calls = append(calls, map[string]any{
				"id":   id,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": string(arguments),
				},
			})
		}
	}
	message["content"] = text
	if thinking != "" {
		message["reasoning_content"] = thinking
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	return message
}

// anthropicUserMessages yields tool messages for tool_result blocks and one
// user message for the remaining text and images.
func anthropicUserMessages(content any) []any {
	if text, ok := content.(string); ok {
		return []any{map[string]any{"role": "user", "content": text}}
	}

	var messages []any
	var parts []any
	flush := func() {
		if len(parts) == 0 {
			return
		}
		if len(parts) == 1 && parts[0].(map[string]any)["type"] == "text" {
			messages = append(messages, map[string]any{"role": "user", "content": parts[0].(map[string]any)["text"]})
		} else {
			messages = append(messages, map[string]any{"role": "user", "content": parts})
		}
		parts = nil
	}

	for _, block := range contentBlocks(content) {
		switch block["type"] {
		case "tool_result":
			flush()
			id, _ := block["tool_use_id"].(string)
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": id,
				"content":      flattenText(block["content"]),
			})
		case "image":
			if part := anthropicImagePart(block); part != nil {
				parts = append(parts, part)
			}
		default:
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, map[string]any{"type": "text", "text": text})
			}
		}
	}
	flush()
	return messages
}

func anthropicImagePart(block map[string]any) map[string]any {
	source, _ := block["source"].(map[string]any)
	switch source["type"] {
	case "base64":
		media, _ := source["media_type"].(string)
		data, _ := source["data"].(string)
		if media == "" || data == "" {
			return nil
		}
		return map[string]any{"type": "image_url", "image_url": map[string]any{
			"url": "data:" + media + ";base64," + data,
		}}
	case "url":
		if url, _ := source["url"].(string); url != "" {
			return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}
		}
	}
	return nil
}

func anthropicTools(value any) []any {
	tools, _ := value.([]any)
	out := make([]any, 0, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		if name == "" {
			continue
		}
		function := map[string]any{"name": name}
		if description, ok := tool["description"].(string); ok {
			function["description"] = description
		}
		if schema, ok := tool["input_schema"]; ok {
			function["parameters"] = schema
		}
		out = append(out, map[string]any{"type": "function", "function": function})
	}
	return out
}

func anthropicToolChoice(value any) any {
	choice, _ := value.(map[string]any)
	switch choice["type"] {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		if name, _ := choice["name"].(string); name != "" {
			return map[string]any{"type": "function", "function": map[string]any{"name": name}}
		}
	}
	return nil
}

func anthropicThinkingEffort(value any) string {
	thinking, _ := value.(map[string]any)
	if thinking["type"] != "enabled" {
		return ""
	}
	budget, _ := thinking["budget_tokens"].(float64)
	switch {
	case budget > 0 && budget < 2048:
		return "low"
	case budget > 0 && budget < 8192:
		return "medium"
	default:
		return "high"
	}
}

func contentBlocks(value any) []map[string]any {
	raw, _ := value.([]any)
	blocks := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if block, ok := item.(map[string]any); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// flattenText joins an Anthropic text value: a string, or a list of blocks
// whose text fields are strings or nested lists.
func flattenText(value any) string {
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
			if block["type"] == "image" {
				out += "[Image]"
				continue
			}
			out += flattenText(block["text"])
		}
		return out
	case map[string]any:
		return flattenText(v["text"])
	default:
		return ""
	}
}

// AnthropicResponse converts a Mistral completion into an Anthropic message.
func AnthropicResponse(body []byte) ([]byte, error) {
	normalized, err := Response(body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		return nil, err
	}

	choice := firstChoice(payload)
	if choice == nil {
		return nil, fmt.Errorf("completion response has no choices")
	}
	message, _ := choice["message"].(map[string]any)

	var content []any
	if reasoning, _ := message["reasoning_content"].(string); reasoning != "" {
		content = append(content, map[string]any{
			"type": "thinking", "thinking": reasoning, "signature": "",
		})
	}
	if text, _ := message["content"].(string); text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	if calls, ok := message["tool_calls"].([]any); ok {
		for _, raw := range calls {
			call, _ := raw.(map[string]any)
			function, _ := call["function"].(map[string]any)
			var input any = map[string]any{}
			if arguments, ok := function["arguments"].(string); ok && arguments != "" {
				_ = json.Unmarshal([]byte(arguments), &input)
			}
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    call["id"],
				"name":  function["name"],
				"input": input,
			})
		}
	}
	if content == nil {
		content = []any{}
	}

	usage, _ := payload["usage"].(map[string]any)
	out := map[string]any{
		"id":            anthropicMessageID(payload["id"]),
		"type":          "message",
		"role":          "assistant",
		"model":         payload["model"],
		"content":       content,
		"stop_reason":   anthropicStopReason(choice["finish_reason"]),
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  usageInt(usage, "prompt_tokens"),
			"output_tokens": usageInt(usage, "completion_tokens"),
		},
	}
	return json.Marshal(out)
}

func usageInt(usage map[string]any, key string) int {
	value, _ := usage[key].(float64)
	return int(value)
}

func firstChoice(payload map[string]any) map[string]any {
	choices, _ := payload["choices"].([]any)
	for _, raw := range choices {
		if choice, ok := raw.(map[string]any); ok {
			return choice
		}
	}
	return nil
}

func anthropicStopReason(reason any) string {
	text, _ := reason.(string)
	switch text {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}

func anthropicMessageID(id any) string {
	text, _ := id.(string)
	if text == "" {
		return "msg_proxy"
	}
	return "msg_" + strings.TrimPrefix(text, "msg_")
}
