package translator

import (
	"encoding/json"
	"testing"
)

// OpenAI object-form tool_choice variants normalize onto what Mistral's
// schema accepts: ToolChoiceEnum strings, or an object naming a function.
func TestRequestNormalizesToolChoiceObjects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"required-object", `{"type":"required"}`, `"required"`},
		{"allowed-tools", `{"type":"allowed_tools","mode":"auto"}`, `"required"`},
		{"named-function", `{"type":"function","function":{"name":"bash"}}`, `{"type":"function","function":{"name":"bash"}}`},
		{"auto-string", `"auto"`, `"auto"`},
		{"none-string", `"none"`, `"none"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(`{"model":"m","messages":[],"tool_choice":`+tc.in+`}`), &payload); err != nil {
				t.Fatal(err)
			}
			Request(payload)
			var got, want any
			encodedGot, _ := json.Marshal(payload["tool_choice"]) // string or map
			if err := json.Unmarshal(encodedGot, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			normalizedGot, _ := json.Marshal(got)
			normalizedWant, _ := json.Marshal(want)
			if string(normalizedGot) != string(normalizedWant) {
				t.Fatalf("tool_choice=%s want %s", normalizedGot, normalizedWant)
			}
		})
	}
}

// A tool_use block with missing or non-object input must serialize as an
// empty JSON object, never the string "null".
func TestAnthropicRequestToolUseWithoutInput(t *testing.T) {
	for _, block := range []string{
		`{"type":"tool_use","id":"t1","name":"run"}`,
		`{"type":"tool_use","id":"t1","name":"run","input":"not an object"}`,
	} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(`{"model":"m","max_tokens":10,"messages":[
			{"role":"assistant","content":[`+block+`]},{"role":"user","content":"go"}]}`), &payload); err != nil {
			t.Fatal(err)
		}
		out := AnthropicRequest(payload)
		messages := out["messages"].([]any)
		assistant := messages[0].(map[string]any)
		calls := assistant["tool_calls"].([]any)
		call := calls[0].(map[string]any)
		function := call["function"].(map[string]any)
		if function["arguments"] != "{}" {
			t.Fatalf("input=%s arguments=%v", block, function["arguments"])
		}
	}
}

// The developer role maps to system; unknown roles pass through untouched.
func TestRequestDeveloperRoleMapsToSystem(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"model":"m","messages":[{"role":"developer","content":"sys"},{"role":"user","content":"hi"}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	Request(payload)
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["role"] != "system" {
		t.Fatalf("role=%v", messages[0].(map[string]any)["role"])
	}
}

// Garbage entries among messages are skipped without breaking translation.
func TestRequestGarbageMessagesSkipped(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"model":"m","messages":["str",42,null,{"role":"user","content":"ok"}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	Request(payload)
	messages := payload["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("message count changed: %d", len(messages))
	}
	if messages[3].(map[string]any)["content"] != "ok" {
		t.Fatalf("valid message corrupted: %v", messages[3])
	}
}

// reasoning_effort aliases collapse onto Mistral's levels; unknown values
// pass through for Mistral to judge.
func TestRequestReasoningEffortAliases(t *testing.T) {
	cases := map[string]string{
		"none":    "", // dropped
		"off":     "", // dropped
		"minimal": "low",
		"xhigh":   "high",
		"max":     "high",
	}
	for effort, want := range cases {
		var payload map[string]any
		if err := json.Unmarshal([]byte(`{"model":"m","messages":[],"reasoning_effort":"`+effort+`"}`), &payload); err != nil {
			t.Fatal(err)
		}
		Request(payload)
		got, _ := payload["reasoning_effort"].(string)
		if got != want {
			t.Fatalf("effort=%s got=%q want=%q", effort, got, want)
		}
	}
}

// OpenRouter reasoning.effort must not clobber an explicit reasoning_effort.
func TestRequestReasoningEffortPrecedence(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"model":"m","messages":[],"reasoning":{"effort":"high"},"reasoning_effort":"low"}`), &payload); err != nil {
		t.Fatal(err)
	}
	Request(payload)
	if payload["reasoning_effort"] != "low" {
		t.Fatalf("precedence broken: %v", payload["reasoning_effort"])
	}
	if payload["reasoning"] != nil {
		t.Fatal("reasoning object not dropped")
	}
}

// max_completion_tokens folds into max_tokens; explicit max_tokens wins.
func TestRequestMaxCompletionTokens(t *testing.T) {
	var only map[string]any
	json.Unmarshal([]byte(`{"model":"m","messages":[],"max_completion_tokens":100}`), &only)
	Request(only)
	if only["max_tokens"] != float64(100) || only["max_completion_tokens"] != nil {
		t.Fatalf("fold failed: max_tokens=%v", only["max_tokens"])
	}
}

// Assistant tool_calls win over a legacy function_call on the same message.
func TestRequestToolCallsWinOverFunctionCall(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"model":"m","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"t9","type":"function","function":{"name":"one","arguments":"{}"}}],"function_call":{"name":"two","arguments":"{}"}}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	Request(payload)
	message := payload["messages"].([]any)[0].(map[string]any)
	calls := message["tool_calls"].([]any)
	function := calls[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "one" {
		t.Fatalf("legacy call won: %v", calls)
	}
	if _, has := message["function_call"]; has {
		t.Fatal("function_call not dropped")
	}
}

// tool_result blocks split into their own tool messages; images degrade to
// a placeholder and remaining text yields the follow-up user message.
func TestAnthropicRequestToolResultAndText(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"model":"m","max_tokens":10,"messages":[
		{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"read","input":{"path":"x"}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":[{"type":"text","text":"data"}]},{"type":"text","text":"go on"}]}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	out := AnthropicRequest(payload)
	messages := out["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages=%d", len(messages))
	}
	result := messages[1].(map[string]any)
	if result["role"] != "tool" || result["tool_call_id"] != "tu1" || result["content"] != "data" {
		t.Fatalf("tool message=%v", result)
	}
	followUp := messages[2].(map[string]any)
	if followUp["role"] != "user" || followUp["content"] != "go on" {
		t.Fatalf("follow-up=%v", followUp)
	}
}
