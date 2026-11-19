// Package openai holds the OpenAI wire types shared by the proxy.
package openai

// Error is the OpenAI-style error envelope.
type Error struct {
	Error Detail `json:"error"`
}

// Detail is the body of an Error.
type Detail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// NewError builds an OpenAI-compatible error body.
func NewError(message, kind string) Error {
	return Error{Error: Detail{Message: message, Type: kind}}
}

// IsStream reports whether the request asked for a streamed reply.
func IsStream(payload map[string]any) bool {
	stream, _ := payload["stream"].(bool)
	return stream
}
