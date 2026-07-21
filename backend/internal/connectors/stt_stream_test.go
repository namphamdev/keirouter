package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestHTTPBaseToWebSocket(t *testing.T) {
	cases := map[string]string{
		"https://api.x.ai/v1": "wss://api.x.ai/v1",
		"http://localhost:9/": "ws://localhost:9",
		"wss://api.x.ai/v1":   "wss://api.x.ai/v1",
	}
	for in, want := range cases {
		got, err := httpBaseToWebSocket(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got)
	}
	_, err := httpBaseToWebSocket("ftp://x")
	require.Error(t, err)
}

func TestOpenAICompatible_DialTranscriptionStream_XAI(t *testing.T) {
	// Echo-style upstream: wait for client text then reply; accept binary passthrough path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/stt", r.URL.Path)
		require.Equal(t, "Bearer xai-stream", r.Header.Get("Authorization"))
		require.Equal(t, "en", r.URL.Query().Get("language"))
		require.Equal(t, "true", r.URL.Query().Get("interim_results"))
		require.Equal(t, "KeiRouter", r.URL.Query().Get("keyterm"))
		// model must not leak as upstream query
		require.Empty(t, r.URL.Query().Get("model"))

		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Ready event first (xAI transcript.created).
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"transcript.created"}`)))

		// Read PCM binary.
		typ, data, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, websocket.MessageBinary, typ)
		require.Equal(t, []byte{1, 2, 3, 4}, data)

		// Read audio.done control.
		typ, data, err = conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, websocket.MessageText, typ)
		require.Contains(t, string(data), "audio.done")

		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"transcript.done","text":"hello stream","duration":0.5}`)))
	}))
	defer srv.Close()

	// httptest is http → connector rewrites to ws (same host/port).
	c := NewOpenAICompatible("xai", srv.URL)
	stream, err := c.DialTranscriptionStream(context.Background(), &core.StreamingTranscriptionRequest{
		Model: "grok-stt",
		Params: map[string][]string{
			"language":         {"en"},
			"interim_results":  {"true"},
			"prompt":           {"KeiRouter"},
			"model":            {"should-not-forward"},
			"authorization":    {"should-not-forward"},
		},
	}, core.Credentials{APIKey: "xai-stream"})
	require.NoError(t, err)
	defer stream.Close(0, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	typ, data, err := stream.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, core.StreamMessageText, typ)
	require.Contains(t, string(data), "transcript.created")

	require.NoError(t, stream.Write(ctx, core.StreamMessageBinary, []byte{1, 2, 3, 4}))
	require.NoError(t, stream.Write(ctx, core.StreamMessageText, []byte(`{"type":"audio.done"}`)))

	typ, data, err = stream.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, core.StreamMessageText, typ)
	require.Contains(t, string(data), "hello stream")
}

func TestOpenAICompatible_DialTranscriptionStream_Unsupported(t *testing.T) {
	c := NewOpenAICompatible("openai", "https://api.openai.com/v1")
	_, err := c.DialTranscriptionStream(context.Background(), &core.StreamingTranscriptionRequest{
		Model: "whisper-1",
	}, core.Credentials{APIKey: "sk"})
	require.Error(t, err)
	var pe *core.ProviderError
	require.ErrorAs(t, err, &pe)
	require.Contains(t, strings.ToLower(pe.Message), "not supported")
}

func TestOpenAICompatible_DialTranscriptionStream_RejectsRelay(t *testing.T) {
	c := NewOpenAICompatible("xai", "https://api.x.ai/v1")
	_, err := c.DialTranscriptionStream(context.Background(), &core.StreamingTranscriptionRequest{
		Model: "grok-stt",
	}, core.Credentials{APIKey: "xai", RelayURL: "https://relay.example/proxy"})
	require.Error(t, err)
	var pe *core.ProviderError
	require.ErrorAs(t, err, &pe)
	require.Contains(t, pe.Message, "relay")
}
