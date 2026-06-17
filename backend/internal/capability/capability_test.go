package capability

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestOf(t *testing.T) {
	tests := []struct {
		model    string
		expected core.CapabilitySet
	}{
		// mimo-v2.5 (exact) should have vision
		{
			model: "mimo-v2.5",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapLongContext,
			),
		},
		// mimo-v2.5-pro (substring match for "mimo") should NOT have vision
		{
			model: "mimo-v2.5-pro",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapLongContext,
			),
		},
		// mimo-v2-omni (substring match for "mimo-v2-omni") should have vision
		{
			model: "mimo-v2-omni",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapLongContext,
			),
		},
		// mimo (substring match) should NOT have vision
		{
			model: "mimo",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapLongContext,
			),
		},
		// Cursor composer family supports agentic tool use.
		{
			model: "composer-2.5",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapReasoning,
				core.CapLongContext,
			),
		},
		// Cursor auto-picker model id.
		{
			model: "default",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapReasoning,
				core.CapLongContext,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := Of(tt.model)
			if !got.Satisfies(tt.expected) || !tt.expected.Satisfies(got) {
				t.Errorf("Of(%q) = %v, want %v", tt.model, got, tt.expected)
			}
		})
	}
}

func TestSupportsComposerWithTools(t *testing.T) {
	req := &core.ChatRequest{
		Stream: true,
		Tools:  []core.Tool{{Name: "bash"}},
	}
	required := Required(req)
	if !Supports("composer-2.5", required) {
		t.Fatalf("composer-2.5 should satisfy tool+stream request, required=%v", required.Slice())
	}
}