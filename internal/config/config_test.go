package config

import "testing"

func TestListenAddr(t *testing.T) {
	cases := []struct {
		listen string
		port   string
		want   string
	}{
		{"", "", ":8080"},
		{"", "9090", ":9090"},
		{"", ":9091", ":9091"},
		{":7000", "9090", ":7000"},
	}
	for _, tc := range cases {
		t.Setenv("LISTEN_ADDR", tc.listen)
		t.Setenv("PORT", tc.port)
		if got := listenAddr(); got != tc.want {
			t.Fatalf("LISTEN_ADDR=%q PORT=%q -> %q, want %q", tc.listen, tc.port, got, tc.want)
		}
	}
}
