package translator

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// translateTools maps the optional legacy OpenAI function-calling surface onto
// tools/tool_calls and makes every tool call wire-safe for Mistral.
func translateTools(payload map[string]any) {
	legacyFunctions(payload)

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
// get an index so clients can accumulate them.
func normalizeToolCalls(message map[string]any, ensureIndex bool) bool {
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
		if ensureIndex {
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
			if arguments == "" {
				function["arguments"] = "{}"
			}
		case nil:
			function["arguments"] = "{}"
		default:
			if encoded, err := json.Marshal(arguments); err == nil {
				function["arguments"] = string(encoded)
			}
		}
		if _, ok := call["id"]; !ok {
			call["id"] = callID(function)
		}
	}
	return true
}

// callID derives a stable id so a legacy function result can be matched back
// to the call that produced it.
func callID(function map[string]any) string {
	sum := sha1.Sum([]byte(fmt.Sprint(function["name"], function["arguments"])))
	return "call_" + hex.EncodeToString(sum[:4])
}
