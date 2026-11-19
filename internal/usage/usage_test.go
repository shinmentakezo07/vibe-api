package usage

import "testing"

func TestFromMessage(t *testing.T) {
	model, tokens := FromMessage([]byte(`{"model":"m","usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16,"prompt_tokens_cached":3,"completion_tokens_details":{"reasoning_tokens":2}}}`))
	if model != "m" || !tokens.OK || tokens.Prompt != 12 || tokens.Completion != 4 || tokens.Total != 16 || tokens.Cached != 3 || tokens.Reasoning != 2 {
		t.Fatalf("model=%q tokens=%#v", model, tokens)
	}
	_, openaiStyle := FromMessage([]byte(`{"usage":{"prompt_tokens":5,"prompt_tokens_details":{"cached_tokens":4}}}`))
	if openaiStyle.Cached != 4 {
		t.Fatalf("openai cached tokens=%#v", openaiStyle)
	}
	_, alternate := FromMessage([]byte(`{"usage":{"prompt_tokens":9,"cached_tokens":5}}`))
	if alternate.Cached != 5 {
		t.Fatalf("alternate cached tokens=%#v", alternate)
	}
	if _, empty := FromMessage([]byte(`{"choices":[]}`)); empty.OK {
		t.Fatal("reported usage without a usage block")
	}
}

func TestTrackerAggregates(t *testing.T) {
	tracker := New()
	tracker.Add(Sample{Route: "/v1/chat/completions", Model: "m", Prompt: 10, Completion: 2, Total: 12, LatencyMS: 120})
	tracker.Add(Sample{Route: "/v1/messages", Model: "m", Prompt: 5, Completion: 3, Total: 8, Cached: 4, Reasoning: 1, LatencyMS: 300, Stream: true})
	tracker.Add(Sample{Route: "/v1/chat/completions", Model: "other", Prompt: 1, Completion: 1, Total: 2})
	tracker.Add(Sample{Route: "/v1/chat/completions", Model: "m"}) // ignored

	snapshot := tracker.Snapshot()
	if snapshot.Totals.Requests != 3 || snapshot.Totals.Prompt != 16 || snapshot.Totals.Completion != 6 || snapshot.Totals.Total != 22 {
		t.Fatalf("totals=%#v", snapshot.Totals)
	}
	if snapshot.Totals.Cached != 4 || snapshot.Totals.Reasoning != 1 {
		t.Fatalf("extended totals=%#v", snapshot.Totals)
	}
	if len(snapshot.ByModel) != 2 || len(snapshot.Recent) != 3 {
		t.Fatalf("byModel=%#v recent=%d", snapshot.ByModel, len(snapshot.Recent))
	}
	if snapshot.Recent[0].Model != "other" || snapshot.LastEvent == nil {
		t.Fatalf("recent=%#v", snapshot.Recent)
	}
}
