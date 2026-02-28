package providers

import (
	"context"
	"log/slog"
)

const (
	dashscopeDefaultBase  = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	dashscopeDefaultModel = "qwen3-max"
)

// DashScopeProvider wraps OpenAIProvider to handle DashScope-specific behaviors.
// Critical: DashScope does NOT support tools + streaming simultaneously.
// When tools are present, ChatStream falls back to non-streaming Chat().
type DashScopeProvider struct {
	*OpenAIProvider
}

func NewDashScopeProvider(apiKey, apiBase, defaultModel string) *DashScopeProvider {
	if apiBase == "" {
		apiBase = dashscopeDefaultBase
	}
	if defaultModel == "" {
		defaultModel = dashscopeDefaultModel
	}
	return &DashScopeProvider{
		OpenAIProvider: NewOpenAIProvider("dashscope", apiKey, apiBase, defaultModel),
	}
}

func (p *DashScopeProvider) Name() string { return "dashscope" }

// ChatStream handles DashScope's limitation: tools + streaming cannot coexist.
// When tools are present, falls back to non-streaming Chat() and synthesizes
// chunk callbacks for the caller.
func (p *DashScopeProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	if len(req.Tools) > 0 {
		slog.Debug("dashscope: tools present, falling back to non-streaming Chat")
		resp, err := p.Chat(ctx, req)
		if err != nil {
			return nil, err
		}
		if onChunk != nil {
			if resp.Thinking != "" {
				onChunk(StreamChunk{Thinking: resp.Thinking})
			}
			if resp.Content != "" {
				onChunk(StreamChunk{Content: resp.Content})
			}
			onChunk(StreamChunk{Done: true})
		}
		return resp, nil
	}
	return p.OpenAIProvider.ChatStream(ctx, req, onChunk)
}
