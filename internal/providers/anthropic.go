package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultClaudeModel   = "claude-sonnet-4-5-20250929"
	anthropicAPIBase     = "https://api.anthropic.com/v1"
	anthropicAPIVersion  = "2023-06-01"
)

// AnthropicProvider implements Provider using the Anthropic Claude API via net/http.
type AnthropicProvider struct {
	apiKey       string
	baseURL      string
	defaultModel string
	client       *http.Client
	retryConfig  RetryConfig
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey string, opts ...AnthropicOption) *AnthropicProvider {
	p := &AnthropicProvider{
		apiKey:       apiKey,
		baseURL:      anthropicAPIBase,
		defaultModel: defaultClaudeModel,
		client:       &http.Client{Timeout: 120 * time.Second},
		retryConfig:  DefaultRetryConfig(),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

type AnthropicOption func(*AnthropicProvider)

func WithAnthropicModel(model string) AnthropicOption {
	return func(p *AnthropicProvider) { p.defaultModel = model }
}

func WithAnthropicBaseURL(baseURL string) AnthropicOption {
	return func(p *AnthropicProvider) {
		if baseURL != "" {
			p.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

func (p *AnthropicProvider) Name() string            { return "anthropic" }
func (p *AnthropicProvider) DefaultModel() string     { return p.defaultModel }
func (p *AnthropicProvider) SupportsThinking() bool   { return true }

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	body := p.buildRequestBody(model, req, false)

	return RetryDo(ctx, p.retryConfig, func() (*ChatResponse, error) {
		respBody, err := p.doRequest(ctx, body)
		if err != nil {
			return nil, err
		}
		defer respBody.Close()

		var resp anthropicResponse
		if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
			return nil, fmt.Errorf("anthropic: decode response: %w", err)
		}

		return p.parseResponse(&resp), nil
	})
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	body := p.buildRequestBody(model, req, true)

	// Retry only the connection phase; once streaming starts, no retry.
	respBody, err := RetryDo(ctx, p.retryConfig, func() (io.ReadCloser, error) {
		return p.doRequest(ctx, body)
	})
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	result := &ChatResponse{FinishReason: "stop"}
	// Accumulate raw JSON fragments for each tool call by index
	toolCallJSON := make(map[int]string)

	// Track content blocks for RawAssistantContent (needed for thinking block passback)
	var rawContentBlocks []json.RawMessage
	var currentBlockType string
	// Track thinking token count by accumulated chunk size
	thinkingChars := 0

	scanner := bufio.NewScanner(respBody)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line for large thinking chunks
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		// Track event type
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		switch currentEvent {
		case "message_start":
			var ev anthropicMessageStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if result.Usage == nil {
					result.Usage = &Usage{}
				}
				if ev.Message.Usage.InputTokens > 0 {
					result.Usage.PromptTokens = ev.Message.Usage.InputTokens
				}
				result.Usage.CacheCreationTokens = ev.Message.Usage.CacheCreationInputTokens
				result.Usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			}

		case "content_block_start":
			var ev anthropicContentBlockStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				currentBlockType = ev.ContentBlock.Type
				if ev.ContentBlock.Type == "tool_use" {
					result.ToolCalls = append(result.ToolCalls, ToolCall{
						ID:        ev.ContentBlock.ID,
						Name:      strings.TrimSpace(ev.ContentBlock.Name),
						Arguments: make(map[string]interface{}),
					})
				}
				// Store raw content_block for later reconstruction
				rawContentBlocks = append(rawContentBlocks, json.RawMessage(fmt.Sprintf(`{"type":"%s"`, ev.ContentBlock.Type)))
			}

		case "content_block_delta":
			var ev anthropicContentBlockDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				switch ev.Delta.Type {
				case "text_delta":
					result.Content += ev.Delta.Text
					if onChunk != nil {
						onChunk(StreamChunk{Content: ev.Delta.Text})
					}
				case "thinking_delta":
					result.Thinking += ev.Delta.Thinking
					thinkingChars += len(ev.Delta.Thinking)
					if onChunk != nil {
						onChunk(StreamChunk{Thinking: ev.Delta.Thinking})
					}
				case "input_json_delta":
					if len(result.ToolCalls) > 0 {
						idx := len(result.ToolCalls) - 1
						toolCallJSON[idx] += ev.Delta.PartialJSON
					}
				case "signature_delta":
					// Signature is captured in content_block_stop via raw block reconstruction
				}
			}

		case "content_block_stop":
			// Reconstruct the complete content block for RawAssistantContent
			if len(rawContentBlocks) > 0 {
				idx := len(rawContentBlocks) - 1
				block := p.buildRawBlock(currentBlockType, result, toolCallJSON, idx)
				if block != nil {
					rawContentBlocks[idx] = block
				}
			}
			currentBlockType = ""

		case "message_delta":
			var ev anthropicMessageDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if ev.Delta.StopReason != "" {
					switch ev.Delta.StopReason {
					case "tool_use":
						result.FinishReason = "tool_calls"
					case "max_tokens":
						result.FinishReason = "length"
					default:
						result.FinishReason = "stop"
					}
				}
				if ev.Usage.OutputTokens > 0 {
					if result.Usage == nil {
						result.Usage = &Usage{}
					}
					result.Usage.CompletionTokens = ev.Usage.OutputTokens
				}
			}

		case "error":
			var ev anthropicErrorEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				return nil, fmt.Errorf("anthropic stream error: %s: %s", ev.Error.Type, ev.Error.Message)
			}

		case "message_stop":
			// Stream complete
		}
	}

	// Parse accumulated tool call JSON arguments
	for i, rawJSON := range toolCallJSON {
		if rawJSON != "" {
			args := make(map[string]interface{})
			_ = json.Unmarshal([]byte(rawJSON), &args)
			result.ToolCalls[i].Arguments = args
		}
	}

	if result.Usage != nil {
		result.Usage.TotalTokens = result.Usage.PromptTokens + result.Usage.CompletionTokens
		// Estimate thinking tokens from accumulated character count (~4 chars per token)
		if thinkingChars > 0 {
			result.Usage.ThinkingTokens = thinkingChars / 4
		}
	}

	// Preserve raw content blocks for tool use passback
	if len(rawContentBlocks) > 0 && len(result.ToolCalls) > 0 {
		if b, err := json.Marshal(rawContentBlocks); err == nil {
			result.RawAssistantContent = b
		}
	}

	if onChunk != nil {
		onChunk(StreamChunk{Done: true})
	}

	return result, nil
}

// buildRawBlock reconstructs a complete content block from streaming data.
// This is needed to preserve thinking blocks (with signatures) for tool use passback.
func (p *AnthropicProvider) buildRawBlock(blockType string, result *ChatResponse, toolCallJSON map[int]string, _ int) json.RawMessage {
	switch blockType {
	case "thinking":
		block := map[string]interface{}{
			"type":     "thinking",
			"thinking": result.Thinking,
		}
		if b, err := json.Marshal(block); err == nil {
			return b
		}
	case "text":
		block := map[string]interface{}{
			"type": "text",
			"text": result.Content,
		}
		if b, err := json.Marshal(block); err == nil {
			return b
		}
	case "tool_use":
		if len(result.ToolCalls) > 0 {
			tc := result.ToolCalls[len(result.ToolCalls)-1]
			// Parse accumulated JSON for this tool call
			args := make(map[string]interface{})
			for i, rawJSON := range toolCallJSON {
				if i == len(result.ToolCalls)-1 && rawJSON != "" {
					_ = json.Unmarshal([]byte(rawJSON), &args)
				}
			}
			block := map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": args,
			}
			if b, err := json.Marshal(block); err == nil {
				return b
			}
		}
	case "redacted_thinking":
		// Pass through as-is (we don't have the encrypted data in streaming)
		block := map[string]interface{}{
			"type": "redacted_thinking",
		}
		if b, err := json.Marshal(block); err == nil {
			return b
		}
	}
	return nil
}

func (p *AnthropicProvider) buildRequestBody(model string, req ChatRequest, stream bool) map[string]interface{} {
	// Separate system messages and build conversation messages
	var systemBlocks []map[string]interface{}
	var messages []map[string]interface{}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			systemBlocks = append(systemBlocks, map[string]interface{}{
				"type": "text",
				"text": msg.Content,
			})

		case "user":
			if len(msg.Images) > 0 {
				var blocks []map[string]interface{}
				for _, img := range msg.Images {
					blocks = append(blocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": img.MimeType,
							"data":       img.Data,
						},
					})
				}
				if msg.Content != "" {
					blocks = append(blocks, map[string]interface{}{
						"type": "text",
						"text": msg.Content,
					})
				}
				messages = append(messages, map[string]interface{}{
					"role":    "user",
					"content": blocks,
				})
			} else {
				messages = append(messages, map[string]interface{}{
					"role":    "user",
					"content": msg.Content,
				})
			}

		case "assistant":
			// If we have raw content blocks (from Anthropic thinking), use them directly
			// to preserve thinking blocks + signatures for tool use passback.
			if msg.RawAssistantContent != nil {
				var rawBlocks []json.RawMessage
				if json.Unmarshal(msg.RawAssistantContent, &rawBlocks) == nil && len(rawBlocks) > 0 {
					messages = append(messages, map[string]interface{}{
						"role":    "assistant",
						"content": rawBlocks,
					})
					continue
				}
			}

			var blocks []map[string]interface{}
			if msg.Content != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Arguments,
				})
			}
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": blocks,
			})

		case "tool":
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     msg.Content,
					},
				},
			})
		}
	}

	body := map[string]interface{}{
		"model":         model,
		"max_tokens":    4096,
		"messages":      messages,
		"cache_control": map[string]interface{}{"type": "ephemeral"},
	}

	if stream {
		body["stream"] = true
	}

	if len(systemBlocks) > 0 {
		body["system"] = systemBlocks
	}

	// Translate tools to Anthropic format
	if len(req.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range req.Tools {
			cleanedParams := CleanSchemaForProvider("anthropic", t.Function.Parameters)
			tool := map[string]interface{}{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": cleanedParams,
			}
			tools = append(tools, tool)
		}
		body["tools"] = tools
	}

	// Merge options
	if v, ok := req.Options[OptMaxTokens]; ok {
		body["max_tokens"] = v
	}
	if v, ok := req.Options[OptTemperature]; ok {
		body["temperature"] = v
	}

	// Enable extended thinking if thinking_level is set
	if level, ok := req.Options[OptThinkingLevel].(string); ok && level != "" && level != "off" {
		budget := anthropicThinkingBudget(level)
		body["thinking"] = map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": budget,
		}
		// Anthropic requires no temperature when thinking is enabled
		delete(body, "temperature")
		// Ensure max_tokens accommodates thinking budget + response
		if maxTok, ok := body["max_tokens"].(int); !ok || maxTok < budget+4096 {
			body["max_tokens"] = budget + 8192
		}
	}

	return body
}

// anthropicThinkingBudget maps a thinking level to a token budget.
func anthropicThinkingBudget(level string) int {
	switch level {
	case "low":
		return 4096
	case "medium":
		return 10000
	case "high":
		return 32000
	default:
		return 10000
	}
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body interface{}) (io.ReadCloser, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	// Add beta header for interleaved thinking when thinking is enabled
	if bodyMap, ok := body.(map[string]interface{}); ok {
		if _, hasThinking := bodyMap["thinking"]; hasThinking {
			httpReq.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
		}
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &HTTPError{
			Status:     resp.StatusCode,
			Body:       fmt.Sprintf("anthropic: %s", string(respBody)),
			RetryAfter: retryAfter,
		}
	}

	return resp.Body, nil
}

func (p *AnthropicProvider) parseResponse(resp *anthropicResponse) *ChatResponse {
	result := &ChatResponse{}
	thinkingChars := 0

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Thinking += block.Thinking
			thinkingChars += len(block.Thinking)
		case "redacted_thinking":
			// Encrypted thinking — cannot display but must preserve for passback
		case "tool_use":
			args := make(map[string]interface{})
			_ = json.Unmarshal(block.Input, &args)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      strings.TrimSpace(block.Name),
				Arguments: args,
			})
		}
	}

	switch resp.StopReason {
	case "tool_use":
		result.FinishReason = "tool_calls"
	case "max_tokens":
		result.FinishReason = "length"
	default:
		result.FinishReason = "stop"
	}

	result.Usage = &Usage{
		PromptTokens:        resp.Usage.InputTokens,
		CompletionTokens:    resp.Usage.OutputTokens,
		TotalTokens:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
		CacheCreationTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadTokens:     resp.Usage.CacheReadInputTokens,
	}
	if thinkingChars > 0 {
		result.Usage.ThinkingTokens = thinkingChars / 4
	}

	// Preserve raw content blocks for tool use passback
	if len(result.ToolCalls) > 0 {
		if b, err := json.Marshal(resp.Content); err == nil {
			result.RawAssistantContent = b
		}
	}

	return result
}

// --- Anthropic API types (internal) ---

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                 `json:"stop_reason"`
	Usage      anthropicUsage         `json:"usage"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`  // for type="thinking"
	Signature string          `json:"signature,omitempty"` // encrypted thinking verification
	Data      string          `json:"data,omitempty"`      // for type="redacted_thinking"
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- Streaming event types ---

type anthropicMessageStartEvent struct {
	Message struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
}

type anthropicContentBlockStartEvent struct {
	Index        int                   `json:"index"`
	ContentBlock anthropicContentBlock `json:"content_block"`
}

type anthropicContentBlockDeltaEvent struct {
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`    // for thinking_delta
		Signature   string `json:"signature,omitempty"`   // for signature_delta
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthropicMessageDeltaEvent struct {
	Delta struct {
		StopReason string `json:"stop_reason,omitempty"`
	} `json:"delta"`
	Usage anthropicUsage `json:"usage"`
}

type anthropicErrorEvent struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
