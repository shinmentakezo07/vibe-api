package usage

import "testing"

func TestFromMessage(t *testing.T) {
	model, tokens := FromMessage([]byte(`{"model":"m","usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16,"prompt_tokens_cached":3}}`))
	if model != "m" || !tokens.OK || tokens.Prompt != 12 || tokens.Completion != 4 || tokens.Total != 16 || tokens.Cached != 3 {
		t.Fatalf("model=%q tokens=%#v", model, tokens)
	}
	if _, empty := FromMessage([]byte(`{"choices":[]}`)); empty.OK {
		t.Fatal("reported usage without a usage block")
	}
}

func TestTrackerAggregates(t *testing.T) {
	tracker := New()
	tracker.Add("/v1/chat/completions", "m", 10, 2, 12, 0, false)
	tracker.Add("/v1/messages", "m", 5, 3, 8, 0, true)
	tracker.Add("/v1/chat/completions", "other", 1, 1, 2, 0, false)
	tracker.Add("/v1/chat/completions", "m", 0, 0, 0, 0, false) // ignored

	snapshot := tracker.Snapshot()
	if snapshot.Totals.Requests != 3 || snapshot.Totals.Prompt != 16 || snapshot.Totals.Completion != 6 || snapshot.Totals.Total != 22 {
		t.Fatalf("totals=%#v", snapshot.Totals)
	}
	if len(snapshot.ByModel) != 2 || len(snapshot.Recent) != 3 {
		t.Fatalf("byModel=%#v recent=%d", snapshot.ByModel, len(snapshot.Recent))
	}
	if snapshot.Recent[0].Model != "other" || snapshot.LastEvent == nil {
		t.Fatalf("recent=%#v", snapshot.Recent)
	}
}
