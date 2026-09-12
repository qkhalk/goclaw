package providers

import "strings"

// NewClaudeOAuthProvider builds an Anthropic provider authenticated with a
// Claude Pro/Max OAuth access token (ClaudeDBTokenSource) instead of an API
// key: Bearer auth + the OAuth beta header, with service_tier/fast-mode
// injection disabled via the oauth AuthType.
func NewClaudeOAuthProvider(name string, tokenSource TokenSource, apiBase, defaultModel string, registry ModelRegistry) *AnthropicProvider {
	opts := []AnthropicOption{
		WithAnthropicName(name),
		WithAnthropicBaseURL(normalizeClaudeAPIBase(apiBase)),
		WithAnthropicRegistry(registry),
		WithAnthropicTokenSource(tokenSource),
	}
	if defaultModel != "" {
		opts = append(opts, WithAnthropicModel(defaultModel))
	}
	return NewAnthropicProvider("", opts...)
}

// normalizeClaudeAPIBase maps the bare Anthropic host (as stored in the DB row
// via ClaudeAPIBase) to the Messages API root: doRequest appends "/messages"
// to the base, so "https://api.anthropic.com" must become ".../v1". Custom
// bases (tests, proxies) are only slash-trimmed.
func normalizeClaudeAPIBase(apiBase string) string {
	b := strings.TrimRight(apiBase, "/")
	if b == "https://api.anthropic.com" {
		return b + "/v1"
	}
	return b
}
