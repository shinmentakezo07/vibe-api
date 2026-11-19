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
	for i := 0; i < 5; i++ {
		got, err := m.Pick()
		if err != nil || got != "first" {
			t.Fatalf("pick %d = %q, %v", i, got, err)
		}
	}
}

func TestPickRotatesEnabledKeys(t *testing.T) {
	m := testKeys()
	m.SetRoundRobin(true)
	want := []string{"first", "second", "first", "second"}
	for i, expected := range want {
		got, err := m.Pick()
		if err != nil || got != expected {
			t.Fatalf("pick %d = %q, %v; want %q", i, got, err, expected)
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
