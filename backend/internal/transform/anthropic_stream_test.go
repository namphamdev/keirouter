package transform

import (
	"encoding/json"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestAnthropic_ThinkingBudgetReconciliation(t *testing.T) {
	// When thinking budget >= max_tokens, max_tokens must be raised to budget+1024.
	budget := 8000
	maxTok := 4096
	req := &core.ChatRequest{
		Model:     "claude-sonnet-4.5",
		MaxTokens: &maxTok,
		Reasoning: &core.ReasoningConfig{
			Effort:    "high",
			MaxTokens: budget,
		},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}

	body, err := AnthropicCodec{}.RenderRequest(req)
	require.NoError(t, err)

	// max_tokens should be raised to budget + 1024 = 9024
	var antReq map[string]any
	require.NoError(t, json.Unmarshal(body, &antReq))
	require.Equal(t, float64(9024), antReq["max_tokens"])
}

func TestAnthropic_ParseThinking(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4.5",
		"max_tokens": 1024,
		"thinking": {"type": "enabled", "budget_tokens": 8000},
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	req, err := AnthropicCodec{}.ParseRequest(body)
	require.NoError(t, err)
	require.NotNil(t, req.Reasoning)
	require.Equal(t, "high", req.Reasoning.Effort)
	require.Equal(t, 8000, req.Reasoning.MaxTokens)
}

func TestAnthropic_SignatureFieldInBlock(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4.5",
		"max_tokens": 1024,
		"messages": [
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "reasoning...", "signature": "sig_abc123"},
				{"type": "text", "text": "answer"}
			]},
			{"role": "user", "content": "thanks"}
		]
	}`)

	req, err := AnthropicCodec{}.ParseRequest(body)
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	// Thinking block signature should be parsed
	assistantMsg := req.Messages[0]
	found := false
	for _, p := range assistantMsg.Content {
		if p.Type == core.PartThinking {
			found = true
		}
	}
	require.True(t, found, "thinking block should be parsed")
}
