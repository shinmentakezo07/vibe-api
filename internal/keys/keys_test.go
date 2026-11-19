package keys

import "testing"

func testKeys() *Manager {
	return New([]Key{
		{ID: "1", Secret: "first", Enabled: true},
		{ID: "2", Secret: "second", Enabled: true},
		{ID: "3", Secret: "third", Enabled: false},
	}, false)
}

func TestPickUsesFirstWhenRoundRobinDisabled(t *testing.T) {
	m := testKeys()
	for range 5 {
		got, err := m.Pick()
		if err != nil || got.Secret != "first" {
			t.Fatalf("pick = %+v, %v", got, err)
		}
	}
}

func TestPickRotatesEnabledKeys(t *testing.T) {
	m := testKeys()
	m.SetRoundRobin(true)
	want := []string{"first", "second", "first", "second"}
	for i, expected := range want {
		got, err := m.Pick()
		if err != nil || got.Secret != expected {
			t.Fatalf("pick %d = %+v, %v; want %q", i, got, err, expected)
		}
	}
}

func TestPickSkipsDisabledAndFailsWhenEmpty(t *testing.T) {
	m := testKeys()
	m.SetRoundRobin(true)
	disabled := false
	m.Update("1", nil, &disabled)
	m.Update("2", nil, &disabled)
	if _, err := m.Pick(); err == nil {
		t.Fatal("expected no enabled keys")
	}
}

func TestContainsSecretAndHint(t *testing.T) {
	m := testKeys()
	if !m.ContainsSecret("second") || m.ContainsSecret("missing") {
		t.Fatal("secret lookup failed")
	}
	if got := Hint("abcdefgh"); got != "••••efgh" {
		t.Fatalf("hint = %q", got)
	}
}

func TestUsageAndErrorsAttributedPerKey(t *testing.T) {
	m := testKeys()
	m.SetRoundRobin(true)

	first, _ := m.Pick()
	second, _ := m.Pick()

	m.RecordUsage(first.ID, 10, 5, 15)
	m.RecordUsage("missing", 10, 5, 15)
	m.RecordUsage(first.ID, 1, 1, 2)
	m.RecordError(first.ID, "/v1/chat/completions", "mistral-large", 429, "rate limited")
	m.RecordError("missing", "/v1/chat/completions", "", 500, "ignored")

	views := m.Views()
	for _, view := range views {
		if view.ID == first.ID {
			if view.Stats.Requests != 2 || view.Stats.Prompt != 11 || view.Stats.Completion != 6 || view.Stats.Total != 17 {
				t.Fatalf("first stats = %+v", view.Stats)
			}
			if view.Stats.Errors != 1 || len(view.Errors) != 1 || view.Errors[0].Status != 429 || view.Errors[0].Message != "rate limited" {
				t.Fatalf("first errors = %+v", view.Errors)
			}
			if view.Stats.LastUsed.IsZero() {
				t.Fatal("last used not recorded")
			}
		}
		if view.ID == second.ID {
			if view.Stats.Requests != 0 || view.Stats.Errors != 0 || len(view.Errors) != 0 {
				t.Fatalf("second stats = %+v", view.Stats)
			}
		}
	}
}

func TestErrorFeedCapsAtTenEvents(t *testing.T) {
	m := testKeys()
	key, _ := m.Pick()
	for range 15 {
		m.RecordError(key.ID, "/v1/messages", "", 500, "boom")
	}
	views := m.Views()
	if len(views[0].Errors) != 10 || views[0].Stats.Errors != 15 {
		t.Fatalf("events = %d, total = %d", len(views[0].Errors), views[0].Stats.Errors)
	}
}
