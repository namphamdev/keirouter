package connectors

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// TestQwenNeutralizeThinkingToolChoice verifies an incompatible tool_choice is
// relaxed to "auto" only while Qwen thinking is active.
func TestQwenNeutralizeThinkingToolChoice(t *testing.T) {
	c := NewQwen("qwen", "https://portal.qwen.ai/v1/chat/completions")

	cases := []struct {
		name     string
		reason   *core.ReasoningConfig
		choice   any
		expected any
	}{
		{"thinking + required → auto", &core.ReasoningConfig{Effort: "high"}, "required", "auto"},
		{"thinking + forced object → auto", &core.ReasoningConfig{Effort: "medium"}, map[string]any{"type": "function"}, "auto"},
		{"thinking + auto stays", &core.ReasoningConfig{Effort: "high"}, "auto", "auto"},
		{"no thinking keeps required", nil, "required", "required"},
		{"disabled thinking keeps required", &core.ReasoningConfig{Effort: "none"}, "required", "required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &core.ChatRequest{Reasoning: tc.reason, ToolChoice: tc.choice}
			c.neutralizeThinkingToolChoice(req)
			if got := req.ToolChoice; !equalToolChoice(got, tc.expected) {
				t.Errorf("ToolChoice = %v, want %v", got, tc.expected)
			}
		})
	}
}

func equalToolChoice(a, b any) bool {
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return as == bs
	}
	// Both non-string (object) — treat as equal for this test's purposes.
	return aok == bok
}
