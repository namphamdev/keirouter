package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// handleAudioTranscriptionStream proxies a realtime STT WebSocket session.
//
// Client:  GET /v1/audio/transcriptions/stream?model=xai/grok-stt&...
//          Upgrade: websocket
// Upstream (xAI): wss://api.x.ai/v1/stt?...
//
// Binary frames (PCM audio) and text frames (JSON control + transcript events)
// are forwarded bidirectionally without transformation. KeiRouter-owned query
// params (model, auth tokens) are stripped before the upstream dial.
func (s *Server) handleAudioTranscriptionStream(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())

	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		r.Header.Get("Sec-WebSocket-Key") == "" {
		writeError(w, http.StatusUpgradeRequired, "websocket upgrade required (GET /v1/audio/transcriptions/stream)")
		return
	}

	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if model == "" {
		writeError(w, http.StatusBadRequest, "model is required (query param, e.g. xai/grok-stt)")
		return
	}

	opts, err := s.mediaOptions(r, model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	resolvedModel := modelTail(opts.Targets)

	// Collect upstream query params (everything except KeiRouter routing/auth).
	params := map[string][]string{}
	for k, vals := range r.URL.Query() {
		lk := strings.ToLower(k)
		if isKeiRouterStreamQuery(lk) {
			continue
		}
		params[lk] = append([]string(nil), vals...)
	}

	upstream, provider, perr := s.pipeline.DialTranscriptionStream(r.Context(), &core.StreamingTranscriptionRequest{
		Model:  resolvedModel,
		Params: params,
	}, opts)
	if perr != nil {
		s.logRequest(key.Name, provider, resolvedModel, 0, 0, 0, true, perr)
		s.writeMediaError(w, perr)
		return
	}

	client, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Auth is Bearer/API-key based; browsers may still set Origin.
		InsecureSkipVerify: true,
	})
	if err != nil {
		_ = upstream.Close(int(websocket.StatusInternalError), "client accept failed")
		s.log.Warn("stt stream: client accept failed", "err", err, "provider", provider, "model", resolvedModel)
		return
	}
	client.SetReadLimit(8 << 20)

	s.logRequest(key.Name, provider, resolvedModel, 0, 0, 0, true, nil)
	s.consoleLog.Log("INFO", fmt.Sprintf("Realtime STT stream opened · %s/%s", provider, resolvedModel),
		fmt.Sprintf("Key: %s", key.Name))

	// Proxy until either side closes. Use a detached context so client
	// disconnect tears down independently of the request body reader.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	var once sync.Once
	closeBoth := func(code websocket.StatusCode, reason string) {
		once.Do(func() {
			_ = client.Close(code, reason)
			_ = upstream.Close(int(code), reason)
			cancel()
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Client → upstream (PCM binary + JSON control frames).
	go func() {
		defer wg.Done()
		for {
			typ, data, rerr := client.Read(ctx)
			if rerr != nil {
				closeBoth(websocket.StatusNormalClosure, "client closed")
				return
			}
			msgType := core.StreamMessageText
			if typ == websocket.MessageBinary {
				msgType = core.StreamMessageBinary
			}
			if werr := upstream.Write(ctx, msgType, data); werr != nil {
				closeBoth(websocket.StatusGoingAway, "upstream write failed")
				return
			}
		}
	}()

	// Upstream → client (transcript JSON events).
	go func() {
		defer wg.Done()
		for {
			msgType, data, rerr := upstream.Read(ctx)
			if rerr != nil {
				closeBoth(websocket.StatusNormalClosure, "upstream closed")
				return
			}
			typ := websocket.MessageText
			if msgType == core.StreamMessageBinary {
				typ = websocket.MessageBinary
			}
			// Bound each write so a stalled client does not hang the pair forever.
			wctx, wcancel := context.WithTimeout(ctx, 30*time.Second)
			werr := client.Write(wctx, typ, data)
			wcancel()
			if werr != nil {
				closeBoth(websocket.StatusGoingAway, "client write failed")
				return
			}
		}
	}()

	wg.Wait()
	s.consoleLog.Log("INFO", fmt.Sprintf("Realtime STT stream closed · %s/%s", provider, resolvedModel),
		fmt.Sprintf("Key: %s", key.Name))
}

func isKeiRouterStreamQuery(key string) bool {
	switch key {
	case "model", "api_key", "access_token", "authorization", "token", "key":
		return true
	default:
		return false
	}
}
