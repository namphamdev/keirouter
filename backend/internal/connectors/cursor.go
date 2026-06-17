package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// cursorClientVersion advertises the CLI build to Cursor's agent endpoint.
// Cursor gates the agent.v1 protocol on a recognized CLI client version.
const cursorClientVersion = "cli-2026.01.09-231024f"

// cursorProviderIdentifier tags advertised MCP tools so Cursor routes their
// invocations back as exec handshake messages.
const cursorProviderIdentifier = "pi-agent"

// cursorNativeToolNames are tools Cursor implements internally; advertising
// them as MCP tools would shadow the native implementations.
var cursorNativeToolNames = map[string]bool{
	"bash": true, "read": true, "write": true, "delete": true,
	"ls": true, "grep": true, "lsp": true, "todo": true,
}

// Cursor drives Cursor's agent.v1 CLI protocol: a single bidirectional
// Connect-streaming RPC to /agent.v1.AgentService/Run over HTTP/2. The client
// sends an AgentRunRequest then services the server's exec/KV/blob handshake
// while consuming interaction updates (text, thinking, tool calls) until the
// turn ends.
//
// KeiRouter is an OpenAI-compatible proxy with no local filesystem, so every
// exec operation (shell, read, write, ...) is rejected; only the request
// context handshake (which advertises the caller's MCP tools) is honored.
type Cursor struct {
	id          string
	defaultBase string
}

// NewCursor builds a Cursor connector.
func NewCursor(id, defaultBaseURL string) *Cursor {
	base := defaultBaseURL
	if base == "" {
		base = "https://api2.cursor.sh"
	}
	return &Cursor{id: id, defaultBase: base}
}

func (c *Cursor) ID() string            { return c.id }
func (c *Cursor) Dialect() core.Dialect { return core.DialectCursor }

func (c *Cursor) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// Validate confirms a token is present. Cursor's agent endpoint has no cheap
// probe, so only credential presence can be checked without a billable run.
func (c *Cursor) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.AccessToken == "" && creds.APIKey == "" {
		return fmt.Errorf("validation failed for %s: no access token", c.id)
	}
	return nil
}

func (c *Cursor) token(creds core.Credentials) string {
	if creds.AccessToken != "" {
		return creds.AccessToken
	}
	return creds.APIKey
}

// headers builds the agent.v1 Connect header set with Bearer auth and the CLI
// client identity.
func (c *Cursor) headers(creds core.Credentials) map[string]string {
	h := map[string]string{
		"content-type":             "application/connect+proto",
		"connect-protocol-version": "1",
		"te":                       "trailers",
		"authorization":            "Bearer " + cleanCursorAgentToken(c.token(creds)),
		"x-ghost-mode":             "true",
		"x-cursor-client-version":  cursorClientVersion,
		"x-cursor-client-type":     "cli",
		"x-request-id":             uuid.NewString(),
	}
	return mergeHeaders(h, creds.Headers)
}

// Chat performs a Cursor run and folds the streamed result into one response.
func (c *Cursor) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	ch, err := c.Stream(ctx, req, creds, core.StreamConfig{})
	if err != nil {
		return nil, err
	}

	msg := core.Message{Role: core.RoleAssistant}
	var text, thinking string
	toolCalls := map[string]*core.ToolCall{}
	var toolOrder []string
	finish := core.FinishStop
	var usage core.Usage

	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkText:
			text += chunk.Delta
		case core.ChunkThinking:
			thinking += chunk.Delta
		case core.ChunkToolCall:
			if chunk.ToolCall == nil {
				continue
			}
			tc, ok := toolCalls[chunk.ToolCall.ID]
			if !ok {
				tc = &core.ToolCall{ID: chunk.ToolCall.ID, Name: chunk.ToolCall.Name, Arguments: append(json.RawMessage{}, chunk.ToolCall.Arguments...)}
				toolCalls[chunk.ToolCall.ID] = tc
				toolOrder = append(toolOrder, chunk.ToolCall.ID)
			} else {
				if chunk.ToolCall.Name != "" {
					tc.Name = chunk.ToolCall.Name
				}
				tc.Arguments = append(tc.Arguments, chunk.ToolCall.Arguments...)
			}
			finish = core.FinishToolCalls
		case core.ChunkUsage:
			if chunk.Usage != nil {
				usage = *chunk.Usage
			}
		case core.ChunkFinish:
			if chunk.FinishReason != "" {
				finish = chunk.FinishReason
			}
			if chunk.Usage != nil {
				usage = *chunk.Usage
			}
		case core.ChunkError:
			return nil, chunk.Err
		}
	}

	if thinking != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: thinking})
	}
	if text != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: text})
	}
	for _, id := range toolOrder {
		tc := toolCalls[id]
		if len(tc.Arguments) == 0 {
			tc.Arguments = json.RawMessage("{}")
		}
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartToolCall, ToolCall: tc})
	}

	return &core.ChatResponse{Model: req.Model, Message: msg, FinishReason: finish, Usage: usage}, nil
}

// deterministicMessageID derives a stable UUID-shaped id from a content key so
// identical history hashes to the same blob ids across requests.
func deterministicMessageID(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// cleanCursorAgentToken strips a "prefix::" segment from a Cursor token.
func cleanCursorAgentToken(token string) string {
	if i := strings.Index(token, "::"); i >= 0 {
		return token[i+2:]
	}
	return token
}

// cursorErr carries a classified error decoded from a Connect end-stream frame.
type cursorErr struct {
	kind    core.ErrorKind
	message string
}
