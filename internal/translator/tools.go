package translator

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// callSeq makes generated call ids unique even when two calls share a name
// and arguments.
var callSeq atomic.Uint64

func nextCallSeq() uint64 { return callSeq.Add(1) }

// translateTools maps the optional legacy OpenAI function-calling surface onto
// tools/tool_calls and makes every tool call wire-safe for Mistral.
func translateTools(payload map[string]any) {
	legacyFunctions(payload)
	normalizeToolChoice(payload)
	messages, _ := payload["messages"].([]any)
	callIDs := map[string]string{} // function name -> call id, for legacy results

	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		switch message["role"] {
		case "assistant":
			translateLegacyFunctionCall(message)
			normalizeToolCalls(message, false)
			for _, rawCall := range toolCalls(message) {
				call, _ := rawCall.(map[string]any)
				function, _ := call["function"].(map[string]any)
				name, _ := function["name"].(string)
				id, _ := call["id"].(string)
				if name != "" && id != "" {
					callIDs[name] = id
				}
			}
		case "function":
			message["role"] = "tool"
			if _, ok := message["tool_call_id"]; !ok {
				name, _ := message["name"].(string)
				if id := callIDs[name]; id != "" {
					message["tool_call_id"] = id
				}
			}
		}
	}
}

func legacyFunctions(payload map[string]any) {
	if functions, ok := payload["functions"].([]any); ok && len(functions) > 0 {
		if _, hasTools := payload["tools"]; !hasTools {
			tools := make([]any, 0, len(functions))
			for _, raw := range functions {
				function, ok := raw.(map[string]any)
				if ok {
					tools = append(tools, map[string]any{"type": "function", "function": function})
				}
			}
			payload["tools"] = tools
		}
		delete(payload, "functions")
	}

	choice, ok := payload["function_call"]
	if !ok {
		return
	}
	delete(payload, "function_call")
	if _, hasChoice := payload["tool_choice"]; hasChoice {
		return
	}
	switch value := choice.(type) {
	case string:
		if value == "auto" || value == "none" {
			payload["tool_choice"] = value
		}
	case map[string]any:
		if name, _ := value["name"].(string); name != "" {
			payload["tool_choice"] = map[string]any{
				"type":     "function",
				"function": map[string]any{"name": name},
			}
		}
	}
}

// normalizeToolChoice maps the object forms OpenAI and OpenRouter clients send
// onto what Mistral's schema accepts. An object with only a mode type —
// {"type":"required"} or {"type":"allowed_tools"} — is not a valid Mistral
// ToolChoice (which requires a function); it becomes the string form.
// Anthropic's {"type":"any"} arrives here too, via anthropicToolChoice, so it
// must map as well.
func normalizeToolChoice(payload map[string]any) {
	choice, ok := payload["tool_choice"].(map[string]any)
	if !ok {
		return
	}
	if function, ok := choice["function"].(map[string]any); ok {
		if name, _ := function["name"].(string); name != "" {
			payload["tool_choice"] = map[string]any{
				"type":     "function",
				"function": map[string]any{"name": name},
			}
			return
		}
	}
	switch choice["type"] {
	case "required", "any", "allowed_tools":
		payload["tool_choice"] = "required"
	}
}

// translateLegacyFunctionCall turns an assistant `function_call` into a
// tool_calls entry so Mistral sees one shape.
func translateLegacyFunctionCall(message map[string]any) {
	call, ok := message["function_call"].(map[string]any)
	if !ok {
		return
	}
	delete(message, "function_call")
	if _, hasCalls := message["tool_calls"]; hasCalls {
		return
	}
	message["tool_calls"] = []any{map[string]any{
		"id":       callID(call),
		"type":     "function",
		"function": call,
	}}
}

func toolCalls(message map[string]any) []any {
	calls, _ := message["tool_calls"].([]any)
	return calls
}

// normalizeToolCalls makes each call match the wire contract Mistral accepts:
// a string id, type "function", and JSON-string arguments. Stream deltas also
// get an index so clients can accumulate them. Streaming must preserve empty
// argument fragments: injecting "{}" or a synthetic id into every delta breaks
// clients that concatenate argument fragments by index.
func normalizeToolCalls(message map[string]any, stream bool) bool {
	calls, ok := message["tool_calls"].([]any)
	if !ok {
		return false
	}
	for i, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := call["type"]; !ok {
			call["type"] = "function"
		}
		if stream {
			if _, ok := call["index"]; !ok {
				call["index"] = i
			}
		}
		function, ok := call["function"].(map[string]any)
		if !ok {
			continue
		}
		switch arguments := function["arguments"].(type) {
		case string:
			if arguments == "" && !stream {
				function["arguments"] = "{}"
			}
		case nil:
			if stream {
				delete(function, "arguments")
			} else {
				function["arguments"] = "{}"
			}
		default:
			if encoded, err := json.Marshal(arguments); err == nil {
				function["arguments"] = string(encoded)
			}
		}
		if _, ok := call["id"]; !ok && !stream {
			call["id"] = callID(function)
		}
	}
	return true
}

// callID derives a stable id so a legacy function result can be matched back
// to the call that produced it.
func callID(function map[string]any) string {
	sum := sha1.Sum([]byte(fmt.Sprint(function["name"], function["arguments"], nextCallSeq())))
	return "call_" + hex.EncodeToString(sum[:4])
}
