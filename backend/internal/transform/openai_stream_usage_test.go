package transform

import (
	"encoding/json"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestOpenAI_RenderRequestForProvider_XAIStreamIncludesUsage(t *testing.T) {
	req := &core.ChatRequest{Model: "grok-composer-2.5-fast", Stream: true}
	body, err := OpenAICodec{}.RenderRequestForProvider(req, "xai")
	if err != nil {
		t.Fatal(err)
	}
	var got oaiRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.StreamOpts == nil || !got.StreamOpts.IncludeUsage {
		t.Fatalf("xai stream request missing stream_options.include_usage: %s", body)
	}
}

func TestOpenAI_RenderRequestForProvider_NonXAIStreamOmitsUsage(t *testing.T) {
	req := &core.ChatRequest{Model: "gpt-4o", Stream: true}
	body, err := OpenAICodec{}.RenderRequestForProvider(req, "openai")
	if err != nil {
		t.Fatal(err)
	}
	var got oaiRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.StreamOpts != nil {
		t.Fatalf("openai stream should not set stream_options: %s", body)
	}
}
