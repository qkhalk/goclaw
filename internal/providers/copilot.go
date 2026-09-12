package providers

// Copilot defaults. The API base is per-account and normally resolved at login
// via copilot_internal/user; the default covers the fallback path.
const (
	CopilotAPIBaseDefault = "https://api.individual.githubcopilot.com"
	DefaultCopilotModel   = "gpt-5"
)

// NewCopilotProvider builds an OpenAI-compatible provider for GitHub Copilot:
// dynamic Bearer auth from the token source plus the identity headers the
// Copilot API expects.
func NewCopilotProvider(name string, tokenSource TokenSource, apiBase, defaultModel string) *OpenAIProvider {
	if apiBase == "" {
		apiBase = CopilotAPIBaseDefault
	}
	if defaultModel == "" {
		defaultModel = DefaultCopilotModel
	}
	return NewOpenAIProvider(name, "", apiBase, defaultModel).
		WithTokenSource(tokenSource).
		WithExtraHeaders(map[string]string{
			"Copilot-Integration-Id": "copilot-developer-cli",
			"Editor-Version":         "vs-code/1.99.0",
			"User-Agent":             "GitHubCopilot/1.0",
		})
}
