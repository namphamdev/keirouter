package transform

import (
	"encoding/json"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestGemini_ThoughtSignatureRoundTrip(t *testing.T) {
	body := []byte(`{
	  "contents": [{
	    "role": "model",
	    "parts": [{
	      "functionCall": {"name": "Glob", "args": {"patterns": "*.go"}},
	      "thoughtSignature": "sig-from-model"
	    }]
	  }]
	}`)
	req, err := GeminiCodec{}.ParseRequest(body)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	require.Equal(t, "sig-from-model", req.Messages[0].Content[0].Signature)

	out, err := GeminiCodec{}.RenderRequest(req)
	require.NoError(t, err)
	var wire struct {
		Contents []struct {
			Parts []map[string]json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(out, &wire))
	require.Len(t, wire.Contents, 1)
	require.Len(t, wire.Contents[0].Parts, 1)
	var sig string
	require.NoError(t, json.Unmarshal(wire.Contents[0].Parts[0]["thoughtSignature"], &sig))
	require.Equal(t, "sig-from-model", sig)
}

func TestGemini_ThoughtSignatureSkipOnFirstToolCallWithoutSig(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{{
			Role: core.RoleAssistant,
			Content: []core.ContentPart{{
				Type: core.PartToolCall,
				ToolCall: &core.ToolCall{
					ID:        "call_1",
					Name:      "Glob",
					Arguments: json.RawMessage(`{"patterns":"*.go"}`),
				},
			}},
		}},
	}
	out, err := GeminiCodec{}.RenderRequest(req)
	require.NoError(t, err)
	var wire struct {
		Contents []struct {
			Parts []map[string]json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(out, &wire))
	var sig string
	require.NoError(t, json.Unmarshal(wire.Contents[0].Parts[0]["thoughtSignature"], &sig))
	require.Equal(t, geminiThoughtSignatureSkip, sig)
}

func TestOpenAI_GoogleThoughtSignatureRoundTrip(t *testing.T) {
	body := []byte(`{
	  "model": "gemini-3-flash",
	  "messages": [{
	    "role": "assistant",
	    "tool_calls": [{
	      "id": "call_1",
	      "type": "function",
	      "function": {"name": "Glob", "arguments": "{}"},
	      "extra_content": {"google": {"thought_signature": "sig-abc"}}
	    }]
	  }]
	}`)
	req, err := OpenAICodec{}.ParseRequest(body)
	require.NoError(t, err)
	require.Equal(t, "sig-abc", req.Messages[0].Content[0].Signature)

	out, err := OpenAICodec{}.RenderRequest(req)
	require.NoError(t, err)
	var wire struct {
		Messages []struct {
			ToolCalls []struct {
				ExtraContent struct {
					Google struct {
						ThoughtSignature string `json:"thought_signature"`
					} `json:"google"`
				} `json:"extra_content"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(out, &wire))
	require.Equal(t, "sig-abc", wire.Messages[0].ToolCalls[0].ExtraContent.Google.ThoughtSignature)
}
