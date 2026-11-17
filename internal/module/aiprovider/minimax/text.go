package minimax

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	chatPath = "/v1/text/chatcompletion_v2"
	// DefaultTextModel is used when the admin leaves the model empty.
	DefaultTextModel          = "MiniMax-M3.1-Flash"
	defaultMaxCompletionToken = 16384
	// maxCompletionTokenCeiling caps the automatic retry when a reasoning model
	// spends the whole budget thinking and returns no content.
	maxCompletionTokenCeiling = 32768
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	Temperature         *float64      `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	InputSensitive  bool     `json:"input_sensitive"`
	OutputSensitive bool     `json:"output_sensitive"`
	BaseResp        baseResp `json:"base_resp"`
}

// TextAdapter generates lyrics + style through MiniMax chat completion.
// Params (ai_provider.params): max_completion_tokens (int), temperature (float, (0,1]).
type TextAdapter struct {
	c client
}

// NewTextAdapter returns the lyrics & style adapter.
func NewTextAdapter(h *http.Client) *TextAdapter { return &TextAdapter{c: client{http: h}} }

var _ aiprovider.LyricsStyleProvider = (*TextAdapter)(nil)

// GenerateLyricsStyle sends the rendered prompt and parses the JSON reply.
func (a *TextAdapter) GenerateLyricsStyle(ctx context.Context, req aiprovider.LyricsStyleRequest, cfg aiprovider.Config) (aiprovider.LyricsStyleResult, error) {
	in := chatRequest{
		Model: modelOr(cfg.Model, DefaultTextModel),
		Messages: []chatMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		MaxCompletionTokens: intParam(cfg.Params, "max_completion_tokens", defaultMaxCompletionToken),
	}
	if t, ok := floatParam(cfg.Params, "temperature"); ok {
		in.Temperature = &t
	}

	content, err := a.chat(ctx, cfg, in)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, err
	}
	return aiprovider.ParseLyricsStyle(providerName, content)
}

// TestConnection sends a tiny chat request to validate the API key and model.
func (a *TextAdapter) TestConnection(ctx context.Context, cfg aiprovider.Config) error {
	_, err := a.chat(ctx, cfg, chatRequest{
		Model:               modelOr(cfg.Model, DefaultTextModel),
		Messages:            []chatMessage{{Role: "user", Content: "Reply with: ok"}},
		MaxCompletionTokens: 256,
	})
	return err
}

// chat sends one request. When a reasoning model uses the whole budget on thinking
// (finish_reason=length, empty content) it retries with a doubled budget up to the ceiling.
func (a *TextAdapter) chat(ctx context.Context, cfg aiprovider.Config, in chatRequest) (string, error) {
	for {
		content, cutOff, err := a.chatOnce(ctx, cfg, in)
		if err != nil {
			return "", err
		}
		if !cutOff {
			return content, nil
		}
		if in.MaxCompletionTokens >= maxCompletionTokenCeiling {
			return "", &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName,
				Message: fmt.Sprintf("reply cut off at max_completion_tokens=%d with no content (model spent the budget on reasoning)", in.MaxCompletionTokens)}
		}
		next := min(in.MaxCompletionTokens*2, maxCompletionTokenCeiling)
		slog.WarnContext(ctx, "minimax chat cut off with empty content, retrying with larger budget",
			"provider", providerName, "model", in.Model, "max_completion_tokens", in.MaxCompletionTokens, "retry_with", next)
		in.MaxCompletionTokens = next
	}
}

func (a *TextAdapter) chatOnce(ctx context.Context, cfg aiprovider.Config, in chatRequest) (content string, cutOff bool, err error) {
	var out chatResponse
	if err := a.c.post(ctx, cfg, chatPath, in, &out); err != nil {
		return "", false, err
	}
	if err := checkBase(out.BaseResp); err != nil {
		return "", false, err
	}
	finish, reasoningLen := "", 0
	if len(out.Choices) > 0 {
		finish = out.Choices[0].FinishReason
		content = out.Choices[0].Message.Content
		reasoningLen = len(out.Choices[0].Message.ReasoningContent)
	}
	slog.InfoContext(ctx, "minimax chat result", "provider", providerName, "model", in.Model,
		"max_completion_tokens", in.MaxCompletionTokens, "finish_reason", finish, "choices", len(out.Choices),
		"content_chars", len(content), "reasoning_chars", reasoningLen,
		"prompt_tokens", out.Usage.PromptTokens, "completion_tokens", out.Usage.CompletionTokens,
		"reasoning_tokens", out.Usage.CompletionTokensDetails.ReasoningTokens,
		"input_sensitive", out.InputSensitive, "output_sensitive", out.OutputSensitive)

	if out.InputSensitive || out.OutputSensitive {
		return "", false, &aiprovider.Error{Kind: aiprovider.KindContentFlagged, Provider: providerName, Message: "input or output flagged as sensitive"}
	}
	if len(out.Choices) == 0 {
		return "", false, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "no choices in reply"}
	}
	if finish == "length" && content == "" {
		return "", true, nil
	}
	return content, false, nil
}

func modelOr(model, fallback string) string {
	if model == "" {
		return fallback
	}
	return model
}
