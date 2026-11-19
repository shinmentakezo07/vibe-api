package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func relayOnce(t *testing.T, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	up := httptest.NewServer(handler)
	defer up.Close()
	resp, err := http.Get(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	(&Server{tracker: usage.New(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}).relay(c, resp, false, "/v1/chat/completions", "m", time.Now())
	return rec
}

// Identity-encoding headers must not cross the proxy: the body is readable
// plaintext and framed by the proxy, so a copied Content-Encoding/Length
// would desync the client.
func TestRelayDropsFramingHeaders(t *testing.T) {
	rec := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", "42")
		w.Header().Set("X-Custom", "keep-me")
		_, _ = w.Write([]byte(`{"model":"m"}`))
	})
	h := rec.Header()
	if h.Get("Content-Encoding") != "" {
		t.Fatalf("content-encoding copied: %q", h.Get("Content-Encoding"))
	}
	if h.Get("Content-Length") != "" {
		t.Fatalf("stale content-length copied: %q", h.Get("Content-Length"))
	}
	if h.Get("X-Custom") != "keep-me" {
		t.Fatalf("end-to-end header dropped: %q", h.Get("X-Custom"))
	}
}

// An upstream error body relays verbatim with its status — the OpenAI error
// envelope (including `code`) must survive untouched.
func TestRelayErrorBodyVerbatim(t *testing.T) {
	errBody := `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`
	rec := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(errBody))
	})
	if rec.Code != 401 {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != errBody {
		t.Fatalf("body altered: %s", rec.Body.String())
	}
}

// A 200 with a non-JSON body passes through verbatim (rewrite helper treats
// unmarshal failure as passthrough at a higher level).
func TestRelayNonJSON200Verbatim(t *testing.T) {
	rec := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json at all"))
	})
	if rec.Body.String() != "not json at all" {
		t.Fatalf("body altered: %s", rec.Body.String())
	}
}

// 204 No Content and other non-2xx-success shapes must not crash the tracker
// or rewrite path.
func TestRelay204NoContent(t *testing.T) {
	rec := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// Content blocked-shape rewrite: block content flattens, usage preserved.
func TestRelayRewritesBlockContent(t *testing.T) {
	rec := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	})
	body := rec.Body.String()
	if !strings.Contains(body, `"content":"hi"`) {
		t.Fatalf("content not flattened: %s", body)
	}
	if !strings.Contains(body, `"prompt_tokens":3`) {
		t.Fatalf("usage lost: %s", body)
	}
}
