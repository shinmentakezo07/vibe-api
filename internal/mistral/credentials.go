package mistral

import "context"

type credentialKey struct{}

// WithAPIKey attaches the selected upstream credential to a request context.
func WithAPIKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, credentialKey{}, key)
}

func apiKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(credentialKey{}).(string)
	return key
}
