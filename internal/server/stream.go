package server

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"vibe-api/internal/translator"
)

// streamSSE forwards Server-Sent Events line by line and rewrites Mistral
// chunk payloads into OpenAI shape as they pass through.
func streamSSE(w gin.ResponseWriter, src io.Reader, log *slog.Logger, onData func([]byte)) {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if data, ok := sseData(line); ok && data != "[DONE]" {
			onData([]byte(data))
		}
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
	rewritten := translator.Chunk([]byte(data))
	if string(rewritten) == data {
		return []byte(line)
	}
	suffix := "\n"
	if strings.HasSuffix(line, "\r\n") {
		suffix = "\r\n"
	}
	return append(append([]byte("data: "), rewritten...), suffix...)
}
