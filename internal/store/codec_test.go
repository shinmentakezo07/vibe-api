package store

import "testing"

func TestCodecRoundTrip(t *testing.T) {
	c, err := newCodec("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.seal("sk-test-value")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "sk-test-value" || sealed[:7] != encodedPrefix {
		t.Fatalf("value was not encrypted: %q", sealed)
	}
	plain, err := c.open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "sk-test-value" {
		t.Fatalf("got %q", plain)
	}
}

func TestCodecWithoutSecret(t *testing.T) {
	c, err := newCodec("")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.seal("sk-test-value")
	if err != nil {
		t.Fatal(err)
	}
	if sealed != "sk-test-value" {
		t.Fatalf("expected plaintext, got %q", sealed)
	}
	if _, err := c.open("enc:v1:AAAA"); err == nil {
		t.Fatal("expected encrypted value without APP_SECRET to fail")
	}
}
