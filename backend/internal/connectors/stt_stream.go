package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// wsStreamConn adapts coder/websocket.Conn to core.StreamConn.
type wsStreamConn struct {
	conn *websocket.Conn
}

func (c *wsStreamConn) Read(ctx context.Context) (core.StreamMessageType, []byte, error) {
	typ, data, err := c.conn.Read(ctx)
	if err != nil {
		return 0, nil, err
	}
	switch typ {
	case websocket.MessageBinary:
		return core.StreamMessageBinary, data, nil
	default:
		return core.StreamMessageText, data, nil
	}
}

func (c *wsStreamConn) Write(ctx context.Context, typ core.StreamMessageType, data []byte) error {
	msgType := websocket.MessageText
	if typ == core.StreamMessageBinary {
		msgType = websocket.MessageBinary
	}
	return c.conn.Write(ctx, msgType, data)
}

func (c *wsStreamConn) Close(code int, reason string) error {
	if code == 0 {
		code = int(websocket.StatusNormalClosure)
	}
	return c.conn.Close(websocket.StatusCode(code), reason)
}

// DialTranscriptionStream opens a realtime STT WebSocket for providers that
// support it. Currently xAI (wss://api.x.ai/v1/stt).
func (c *OpenAICompatible) DialTranscriptionStream(ctx context.Context, req *core.StreamingTranscriptionRequest, creds core.Credentials) (core.StreamConn, error) {
	if c.id != "xai" {
		return nil, &core.ProviderError{
			Kind:     core.ErrBadRequest,
			Provider: c.id,
			Model:    req.Model,
			Message:  "realtime speech-to-text is not supported for this provider",
		}
	}
	return c.dialXAITranscriptionStream(ctx, req, creds)
}

func (c *OpenAICompatible) dialXAITranscriptionStream(ctx context.Context, req *core.StreamingTranscriptionRequest, creds core.Credentials) (core.StreamConn, error) {
	base := c.baseURL(creds)
	wsBase, err := httpBaseToWebSocket(base)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	u, err := url.Parse(joinURL(wsBase, "stt"))
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	q := u.Query()
	for k, vals := range req.Params {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" || isKeiRouterSTTParam(key) {
			continue
		}
		for _, v := range vals {
			if v == "" {
				continue
			}
			q.Add(key, v)
		}
	}
	// Map OpenAI-style prompt to xAI keyterm when the client did not set keyterm.
	if q.Get("keyterm") == "" {
		if prompts := req.Params["prompt"]; len(prompts) > 0 && prompts[0] != "" {
			q.Add("keyterm", prompts[0])
		}
	}
	u.RawQuery = q.Encode()

	headers := http.Header{}
	for k, v := range c.headers(creds) {
		headers.Set(k, v)
	}

	// Prefer the shared/proxied HTTP client so account-level proxies apply.
	// Relay URLs are HTTP-body relays and cannot carry WebSocket upgrades.
	if creds.RelayURL != "" {
		return nil, &core.ProviderError{
			Kind:     core.ErrBadRequest,
			Provider: c.id,
			Model:    req.Model,
			Message:  "realtime STT does not support HTTP relay proxies; use a direct or CONNECT HTTP(S)/SOCKS proxy",
		}
	}
	client := clientFor(creds)
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(dialCtx, u.String(), &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: headers,
	})
	if err != nil {
		return nil, &core.ProviderError{
			Kind:     core.ErrUpstream,
			Provider: c.id,
			Model:    req.Model,
			Message:  "stt websocket dial: " + err.Error(),
			Cause:    err,
		}
	}
	// Raise read limit for large interim payloads / multichannel frames.
	conn.SetReadLimit(8 << 20)
	return &wsStreamConn{conn: conn}, nil
}

func httpBaseToWebSocket(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("empty base URL")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already websocket
	default:
		return "", fmt.Errorf("unsupported scheme %q for websocket base", u.Scheme)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// isKeiRouterSTTParam lists query keys consumed by KeiRouter itself and never
// forwarded upstream on the streaming STT connection.
func isKeiRouterSTTParam(key string) bool {
	switch key {
	case "model", "api_key", "access_token", "authorization", "token", "key", "prompt":
		return true
	default:
		return false
	}
}
