package connectors

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/mydisha/keirouter/backend/internal/connectors/cursoragent"
	"github.com/mydisha/keirouter/backend/internal/core"

	"github.com/stretchr/testify/require"
)

func TestCursorFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := &cursorFrameWriter{w: &buf}
	payload := []byte("hello-cursor-frame")
	require.NoError(t, w.writeMessage(payload))

	r := &cursorFrameReader{r: bufio.NewReader(&buf)}
	frame, err := r.next()
	require.NoError(t, err)
	require.False(t, frame.endStream())
	require.Equal(t, payload, frame.payload)
}

func TestCursorParseConnectEndStreamError(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantNil bool
		kind    core.ErrorKind
	}{
		{"empty", "", true, ""},
		{"no error", `{}`, true, ""},
		{"rate limit", `{"error":{"code":"resource_exhausted","message":"slow down"}}`, false, core.ErrRateLimit},
		{"auth", `{"error":{"code":"unauthenticated","message":"no"}}`, false, core.ErrAuth},
		{"bad request", `{"error":{"code":"invalid_argument","message":"bad"}}`, false, core.ErrBadRequest},
		{"generic", `{"error":{"code":"internal","message":"boom"}}`, false, core.ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseConnectEndStreamError([]byte(tc.payload))
			if tc.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.kind, got.kind)
		})
	}
}

func TestCursorMapTodoStatus(t *testing.T) {
	require.Equal(t, "in_progress", mapCursorTodoStatus(2))
	require.Equal(t, "completed", mapCursorTodoStatus(3))
	require.Equal(t, "pending", mapCursorTodoStatus(0))
	require.Equal(t, "pending", mapCursorTodoStatus(1))
}

func TestCursorMergeMcpToolCallArgs(t *testing.T) {
	t.Run("completion overrides streamed", func(t *testing.T) {
		streamed := map[string]any{"a": "old", "b": float64(1)}
		completion := map[string]any{"a": "new"}
		merged := mergeCursorMcpToolCallArgs(streamed, completion)
		require.Equal(t, "new", merged["a"])
		require.Equal(t, float64(1), merged["b"])
	})
	t.Run("keeps streamed structured value over string downgrade", func(t *testing.T) {
		streamed := map[string]any{"payload": map[string]any{"k": "v"}}
		completion := map[string]any{"payload": "{\"k\":\"v\"}"}
		merged := mergeCursorMcpToolCallArgs(streamed, completion)
		require.Equal(t, map[string]any{"k": "v"}, merged["payload"])
	})
}

func TestCursorBlobStoreRoundTrip(t *testing.T) {
	store := newCursorBlobStore()
	data := []byte("payload")
	id := store.store(data)
	got, ok := store.get(id)
	require.True(t, ok)
	require.Equal(t, data, got)

	_, ok = store.get([]byte("missing"))
	require.False(t, ok)
}

func TestCursorDeterministicMessageID(t *testing.T) {
	a := deterministicMessageID("same")
	b := deterministicMessageID("same")
	c := deterministicMessageID("different")
	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
	require.Len(t, a, len("00000000-0000-0000-0000-000000000000"))
}

func TestCursorCleanAgentToken(t *testing.T) {
	require.Equal(t, "abc", cleanCursorAgentToken("prefix::abc"))
	require.Equal(t, "abc", cleanCursorAgentToken("abc"))
}

func TestCursorBuildRunRequest_UserMessageAction(t *testing.T) {
	req := &core.ChatRequest{
		Model:  "claude-sonnet",
		System: "be helpful",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello there"}}},
		},
	}
	store := newCursorBlobStore()
	wire, err := buildCursorRunRequest(req, "conv-1", store)
	require.NoError(t, err)

	var msg pb.AgentClientMessage
	require.NoError(t, proto.Unmarshal(wire, &msg))
	run := msg.GetRunRequest()
	require.NotNil(t, run)
	require.Equal(t, "conv-1", run.GetConversationId())
	require.Equal(t, "claude-sonnet", run.GetModelDetails().GetModelId())

	action := run.GetAction()
	require.NotNil(t, action.GetUserMessageAction())
	require.Equal(t, "hello there", action.GetUserMessageAction().GetUserMessage().GetText())
}

func TestCursorBuildRunRequest_ResumeAction(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-sonnet",
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: "prior"}}},
		},
	}
	store := newCursorBlobStore()
	wire, err := buildCursorRunRequest(req, "conv-2", store)
	require.NoError(t, err)

	var msg pb.AgentClientMessage
	require.NoError(t, proto.Unmarshal(wire, &msg))
	require.NotNil(t, msg.GetRunRequest().GetAction().GetResumeAction())
}

func TestCursorBuildMcpToolDefinitions_FiltersNative(t *testing.T) {
	tools := []core.Tool{
		{Name: "bash", Description: "native"},
		{Name: "search_docs", Description: "custom", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
	}
	defs := buildMcpToolDefinitions(tools)
	require.Len(t, defs, 1)
	require.Equal(t, "search_docs", defs[0].GetName())
	require.Equal(t, cursorProviderIdentifier, defs[0].GetProviderIdentifier())
	require.NotEmpty(t, defs[0].GetInputSchema())
}

func TestCursorToolSchemaValue_Fallback(t *testing.T) {
	v := cursorToolSchemaValue(nil)
	require.NotNil(t, v)
	obj := v.GetStructValue()
	require.NotNil(t, obj)
	require.Equal(t, "object", obj.GetFields()["type"].GetStringValue())
}
