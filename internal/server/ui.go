package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// serveUI serves the built dashboard and falls back to index.html for SPA
// routes. API paths never fall through to the UI.
func (s *Server) serveUI(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/v1") {
		c.JSON(http.StatusNotFound, anthropicError("invalid_request_error", "route not found"))
		return
	}

	index := filepath.Join(s.cfg.WebDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(setupPage))
		return
	}

	clean := path.Clean("/" + c.Request.URL.Path)
	file := filepath.Join(s.cfg.WebDir, clean)
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		http.ServeFile(c.Writer, c.Request, file)
		return
	}
	http.ServeFile(c.Writer, c.Request, index)
}

const setupPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Vibe API</title>
<style>
  body{margin:0;min-height:100vh;display:grid;place-items:center;background:#09090b;color:#e4e4e7;
       font:16px/1.6 ui-sans-serif,system-ui,sans-serif}
  main{max-width:34rem;padding:2rem}
  code{background:#18181b;border:1px solid #27272a;border-radius:.4rem;padding:.15rem .4rem}
  h1{font-size:1.4rem;margin:0 0 .5rem}
  p{color:#a1a1aa}
</style></head>
<body><main>
  <h1>Dashboard not built</h1>
  <p>Build the web UI, then reload this page:</p>
  <p><code>npm --prefix web install &amp;&amp; npm --prefix web run build</code></p>
  <p>The API itself is running. OpenAI: <code>/v1/chat/completions</code>,
     Anthropic: <code>/v1/messages</code>.</p>
</main></body></html>
`
