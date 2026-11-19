package server

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/openai"
)

// streamSSE forwards Server-Sent Events line by line and rewrites Mistral
// chunk payloads into OpenAI shape as they pass through.
func streamSSE(w gin.ResponseWriter, src io.Reader, log *slog.Logger) {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			if _, werr := w.Write(rewriteSSELine(line)); werr != nil {
				return
			}
			w.Flush()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Error("relay stream", "error", err)
			}
			return
		}
	}
}

func rewriteSSELine(line string) []byte {
	trimmed := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(trimmed, "data:") {
		return []byte(line)
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "" || data == "[DONE]" {
		return []byte(line)
	}
	rewritten := openai.RewriteChunk([]byte(data))
	if string(rewritten) == data {
		return []byte(line)
	}
	suffix := "\n"
	if strings.HasSuffix(line, "\r\n") {
		suffix = "\r\n"
	}
	return append(append([]byte("data: "), rewritten...), suffix...)
}
