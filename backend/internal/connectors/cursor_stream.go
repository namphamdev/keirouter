package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/google/uuid"

	pb "github.com/mydisha/keirouter/backend/internal/connectors/cursoragent"
	"github.com/mydisha/keirouter/backend/internal/core"
)

// cursorHeartbeatInterval matches Cursor's CLI: a clientHeartbeat keeps the
// bidi stream alive while the server works.
const cursorHeartbeatInterval = 5 * time.Second

// cursorToolSchemaValue converts a tool's JSON-schema parameters into a
// protobuf Value. Cursor expects a structpb-encoded object; an empty schema
// falls back to {"type":"object","properties":{},"required":[]}.
func cursorToolSchemaValue(params json.RawMessage) *structpb.Value {
	if len(params) > 0 {
		var v any
		if err := json.Unmarshal(params, &v); err == nil {
			if val, err := structpb.NewValue(v); err == nil {
				return val
			}
		}
	}
	fallback, _ := structpb.NewValue(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	})
	return fallback
}

// cursorStreamState tracks the open assistant blocks while consuming
// interaction updates.
type cursorStreamState struct {
	textOpen     bool
	thinkingOpen bool

	toolOpen    bool
	toolID      string
	toolName    string
	toolKind    string // "mcp" or "todo"
	toolPartial string

	sawTokenDelta bool
	usage         core.Usage

	finish core.FinishReason
}

// Stream runs a Cursor agent turn and emits canonical chunks. It opens the bidi
// Run stream, sends the initial request, then services the server's exec/KV
// handshake while translating interaction updates into chunks until turnEnded.
func (c *Cursor) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	if c.token(creds) == "" {
		return nil, &core.ProviderError{Kind: core.ErrAuth, Provider: c.id, Model: req.Model, Message: "cursor access token is required"}
	}

	conversationID := req.Metadata.ContextAffinityKey
	if conversationID == "" {
		conversationID = uuid.NewString()
	}

	store := newCursorBlobStore()
	requestBytes, err := buildCursorRunRequest(req, conversationID, store)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: "build request: " + err.Error(), Cause: err}
	}

	toolDefs := buildMcpToolDefinitions(req.Tools)

	endpoint := cursorEndpoint(c.baseURL(creds))
	stream, err := openCursorStream(ctx, endpoint, c.headers(creds))
	if err != nil {
		if pe := core.AsProviderError(err); pe != nil {
			return nil, err
		}
		return nil, transportError(ctx, c.id, req.Model, err)
	}

	if err := stream.writer.writeMessage(requestBytes); err != nil {
		stream.close()
		return nil, transportError(ctx, c.id, req.Model, err)
	}

	out := make(chan core.StreamChunk, 32)

	go func() {
		defer close(out)
		defer stream.close()

		ttft := newTTFTTracker(cfg)
		emit := func(ch core.StreamChunk) bool {
			ttft.maybeReport(ch)
			select {
			case out <- ch:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// Heartbeat goroutine keeps the stream alive until the reader returns.
		hbCtx, hbCancel := context.WithCancel(ctx)
		defer hbCancel()
		go c.heartbeatLoop(hbCtx, stream)

		st := &cursorStreamState{finish: core.FinishStop}
		turnEnded := false

		for {
			frame, err := stream.reader.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				if ctx.Err() != nil {
					return
				}
				emit(core.StreamChunk{Type: core.ChunkError, Err: &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: "stream read: " + err.Error(), Cause: err}})
				return
			}

			if frame.endStream() {
				if cerr := parseConnectEndStreamError(frame.payload); cerr != nil {
					emit(core.StreamChunk{Type: core.ChunkError, Err: &core.ProviderError{Kind: cerr.kind, Provider: c.id, Model: req.Model, Message: cerr.message}})
					return
				}
				break
			}

			var sm pb.AgentServerMessage
			if err := proto.Unmarshal(frame.payload, &sm); err != nil {
				continue
			}

			if done := c.handleServerMessage(&sm, st, store, stream, toolDefs, emit); done {
				turnEnded = true
				break
			}
		}

		// Close any still-open blocks.
		if st.thinkingOpen {
			if !emit(core.StreamChunk{Type: core.ChunkThinking, Delta: ""}) {
				return
			}
		}
		_ = turnEnded

		finishUsage := st.usage
		if finishUsage.TotalTokens > 0 || finishUsage.CompletionTokens > 0 {
			emit(core.StreamChunk{Type: core.ChunkUsage, Usage: &finishUsage})
		}
		emit(core.StreamChunk{Type: core.ChunkFinish, FinishReason: st.finish, Usage: usagePtr(finishUsage)})
	}()

	return out, nil
}

func usagePtr(u core.Usage) *core.Usage {
	if u.TotalTokens == 0 && u.CompletionTokens == 0 && u.PromptTokens == 0 {
		return nil
	}
	return &u
}

// heartbeatLoop writes clientHeartbeat frames until the context is cancelled.
func (c *Cursor) heartbeatLoop(ctx context.Context, stream *cursorStream) {
	ticker := time.NewTicker(cursorHeartbeatInterval)
	defer ticker.Stop()
	hb := &pb.AgentClientMessage{Message: &pb.AgentClientMessage_ClientHeartbeat{ClientHeartbeat: &pb.ClientHeartbeat{}}}
	payload, err := proto.Marshal(hb)
	if err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := stream.writer.writeMessage(payload); err != nil {
				return
			}
		}
	}
}

// handleServerMessage dispatches one AgentServerMessage. It returns true when
// the turn has ended.
func (c *Cursor) handleServerMessage(sm *pb.AgentServerMessage, st *cursorStreamState, store *cursorBlobStore, stream *cursorStream, toolDefs []*pb.McpToolDefinition, emit func(core.StreamChunk) bool) bool {
	switch m := sm.GetMessage().(type) {
	case *pb.AgentServerMessage_InteractionUpdate:
		return c.processInteractionUpdate(m.InteractionUpdate, st, emit)
	case *pb.AgentServerMessage_KvServerMessage:
		c.handleKvServerMessage(m.KvServerMessage, store, stream)
	case *pb.AgentServerMessage_ExecServerMessage:
		c.handleExecServerMessage(m.ExecServerMessage, stream, toolDefs)
	case *pb.AgentServerMessage_ConversationCheckpointUpdate:
		c.handleCheckpoint(m.ConversationCheckpointUpdate, st)
	}
	return false
}

// processInteractionUpdate translates one interaction update into chunks,
// returning true on turnEnded.
func (c *Cursor) processInteractionUpdate(u *pb.InteractionUpdate, st *cursorStreamState, emit func(core.StreamChunk) bool) bool {
	switch m := u.GetMessage().(type) {
	case *pb.InteractionUpdate_TextDelta:
		delta := m.TextDelta.GetText()
		if delta == "" {
			return false
		}
		st.textOpen = true
		emit(core.StreamChunk{Type: core.ChunkText, Delta: delta})
	case *pb.InteractionUpdate_ThinkingDelta:
		delta := m.ThinkingDelta.GetText()
		if delta == "" {
			return false
		}
		st.thinkingOpen = true
		emit(core.StreamChunk{Type: core.ChunkThinking, Delta: delta})
	case *pb.InteractionUpdate_ThinkingCompleted:
		st.thinkingOpen = false
	case *pb.InteractionUpdate_ToolCallStarted:
		c.toolCallStarted(m.ToolCallStarted, st, emit)
	case *pb.InteractionUpdate_PartialToolCall:
		c.partialToolCall(m.PartialToolCall, st, emit)
	case *pb.InteractionUpdate_ToolCallCompleted:
		c.toolCallCompleted(m.ToolCallCompleted, st, emit)
	case *pb.InteractionUpdate_TokenDelta:
		st.sawTokenDelta = true
		st.usage.CompletionTokens += int(m.TokenDelta.GetTokens())
		st.usage.TotalTokens = st.usage.PromptTokens + st.usage.CompletionTokens
	case *pb.InteractionUpdate_TurnEnded:
		st.finish = core.FinishStop
		return true
	}
	return false
}

func (c *Cursor) toolCallStarted(u *pb.ToolCallStartedUpdate, st *cursorStreamState, emit func(core.StreamChunk) bool) {
	tc := u.GetToolCall()
	if tc == nil {
		return
	}
	if mcp := tc.GetMcpToolCall(); mcp != nil {
		args := mcp.GetArgs()
		id := ""
		name := ""
		if args != nil {
			id = args.GetToolCallId()
			name = args.GetName()
			if name == "" {
				name = args.GetToolName()
			}
		}
		if id == "" {
			id = uuid.NewString()
		}
		st.toolOpen = true
		st.toolID = id
		st.toolName = name
		st.toolKind = "mcp"
		st.toolPartial = ""
		st.finish = core.FinishToolCalls
		emit(core.StreamChunk{Type: core.ChunkToolCall, ToolCall: &core.ToolCall{ID: id, Name: name, Arguments: json.RawMessage("")}})
		return
	}
	if todoArgs := buildCursorTodoArgs(tc); todoArgs != nil {
		id := u.GetCallId()
		if id == "" {
			id = uuid.NewString()
		}
		raw, _ := json.Marshal(todoArgs)
		st.toolOpen = true
		st.toolID = id
		st.toolName = "todo"
		st.toolKind = "todo"
		st.finish = core.FinishToolCalls
		emit(core.StreamChunk{Type: core.ChunkToolCall, ToolCall: &core.ToolCall{ID: id, Name: "todo", Arguments: json.RawMessage(raw)}})
	}
}

func (c *Cursor) partialToolCall(u *pb.PartialToolCallUpdate, st *cursorStreamState, emit func(core.StreamChunk) bool) {
	if !st.toolOpen || st.toolKind != "mcp" {
		return
	}
	c.applyArgsTextDelta(u.GetArgsTextDelta(), st, emit)
}

// applyArgsTextDelta handles Cursor's cumulative args_text_delta snapshots: each
// value is the full args text so far, so the new suffix is recovered by
// stripping the prefix already buffered.
func (c *Cursor) applyArgsTextDelta(snapshot string, st *cursorStreamState, emit func(core.StreamChunk) bool) {
	if snapshot == "" {
		return
	}
	chunk := snapshot
	if strings.HasPrefix(snapshot, st.toolPartial) {
		chunk = snapshot[len(st.toolPartial):]
	}
	if chunk == "" {
		return
	}
	st.toolPartial += chunk
	emit(core.StreamChunk{Type: core.ChunkToolCall, ToolCall: &core.ToolCall{ID: st.toolID, Arguments: json.RawMessage(chunk)}})
}

func (c *Cursor) toolCallCompleted(u *pb.ToolCallCompletedUpdate, st *cursorStreamState, emit func(core.StreamChunk) bool) {
	if !st.toolOpen {
		return
	}
	tc := u.GetToolCall()
	switch st.toolKind {
	case "mcp":
		if tc != nil {
			if mcp := tc.GetMcpToolCall(); mcp != nil && mcp.GetArgs() != nil {
				decoded := decodeCursorMcpArgsMap(mcp.GetArgs().GetArgs())
				merged := mergeCursorMcpToolCallArgs(parseCursorStreamingArgs(st.toolPartial), decoded)
				if raw, err := json.Marshal(merged); err == nil {
					emit(core.StreamChunk{Type: core.ChunkToolCall, ToolCall: &core.ToolCall{ID: st.toolID, Arguments: json.RawMessage(finalArgsDelta(st.toolPartial, string(raw)))}})
				}
			}
		}
	case "todo":
		if tc != nil {
			if todoArgs := buildCursorTodoArgs(tc); todoArgs != nil {
				if raw, err := json.Marshal(todoArgs); err == nil {
					emit(core.StreamChunk{Type: core.ChunkToolCall, ToolCall: &core.ToolCall{ID: st.toolID, Arguments: json.RawMessage(raw)}})
				}
			}
		}
	}
	st.toolOpen = false
	st.toolID = ""
	st.toolName = ""
	st.toolKind = ""
	st.toolPartial = ""
}

// finalArgsDelta returns the suffix of full not already buffered in partial, so
// the consumer's accumulated arguments equal the authoritative completion form.
func finalArgsDelta(partial, full string) string {
	if strings.HasPrefix(full, partial) {
		return full[len(partial):]
	}
	return full
}

func (c *Cursor) handleCheckpoint(checkpoint *pb.ConversationStateStructure, st *cursorStreamState) {
	if st.sawTokenDelta || checkpoint == nil {
		return
	}
	td := checkpoint.GetTokenDetails()
	if td == nil {
		return
	}
	used := int(td.GetUsedTokens())
	if used <= 0 {
		return
	}
	if st.usage.CompletionTokens != used {
		st.usage.CompletionTokens = used
		st.usage.TotalTokens = st.usage.PromptTokens + st.usage.CompletionTokens
	}
}
