package connectors

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/mydisha/keirouter/backend/internal/connectors/cursoragent"
)

// handleKvServerMessage services Cursor's blob KV handshake: getBlobArgs is
// answered from the local blob store, setBlobArgs writes into it.
func (c *Cursor) handleKvServerMessage(kv *pb.KvServerMessage, store *cursorBlobStore, stream *cursorStream) {
	switch m := kv.GetMessage().(type) {
	case *pb.KvServerMessage_GetBlobArgs:
		result := &pb.GetBlobResult{}
		if data, ok := store.get(m.GetBlobArgs.GetBlobId()); ok {
			result.BlobData = data
		}
		c.writeClient(stream, &pb.AgentClientMessage{
			Message: &pb.AgentClientMessage_KvClientMessage{
				KvClientMessage: &pb.KvClientMessage{
					Id:      kv.GetId(),
					Message: &pb.KvClientMessage_GetBlobResult{GetBlobResult: result},
				},
			},
		})
	case *pb.KvServerMessage_SetBlobArgs:
		store.m[hexKey(m.SetBlobArgs.GetBlobId())] = m.SetBlobArgs.GetBlobData()
		c.writeClient(stream, &pb.AgentClientMessage{
			Message: &pb.AgentClientMessage_KvClientMessage{
				KvClientMessage: &pb.KvClientMessage{
					Id:      kv.GetId(),
					Message: &pb.KvClientMessage_SetBlobResult{SetBlobResult: &pb.SetBlobResult{}},
				},
			},
		})
	}
}

// handleExecServerMessage services Cursor's exec handshake. KeiRouter has no
// local filesystem, so the only honored op is requestContextArgs (advertising
// the caller's MCP tools); every other op is rejected or no-op acknowledged so
// the server does not hang waiting.
func (c *Cursor) handleExecServerMessage(exec *pb.ExecServerMessage, stream *cursorStream, toolDefs []*pb.McpToolDefinition) {
	id := exec.GetId()
	execID := exec.GetExecId()

	send := func(ecm *pb.ExecClientMessage) {
		ecm.Id = id
		ecm.ExecId = execID
		c.writeClient(stream, &pb.AgentClientMessage{
			Message: &pb.AgentClientMessage_ExecClientMessage{ExecClientMessage: ecm},
		})
	}

	switch exec.GetMessage().(type) {
	case *pb.ExecServerMessage_RequestContextArgs:
		rc := &pb.RequestContext{Tools: toolDefs}
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_RequestContextResult{
			RequestContextResult: &pb.RequestContextResult{
				Result: &pb.RequestContextResult_Success{Success: &pb.RequestContextSuccess{RequestContext: rc}},
			},
		}})
	case *pb.ExecServerMessage_ReadArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_ReadResult{ReadResult: buildReadRejected(exec.GetReadArgs().GetPath())}})
	case *pb.ExecServerMessage_LsArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_LsResult{LsResult: buildLsRejected(exec.GetLsArgs().GetPath())}})
	case *pb.ExecServerMessage_GrepArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_GrepResult{GrepResult: buildGrepError()}})
	case *pb.ExecServerMessage_WriteArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_WriteResult{WriteResult: buildWriteRejected(exec.GetWriteArgs().GetPath())}})
	case *pb.ExecServerMessage_DeleteArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_DeleteResult{DeleteResult: buildDeleteRejected(exec.GetDeleteArgs().GetPath())}})
	case *pb.ExecServerMessage_ShellArgs, *pb.ExecServerMessage_ShellStreamArgs:
		args := exec.GetShellArgs()
		if args == nil {
			args = exec.GetShellStreamArgs()
		}
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_ShellResult{ShellResult: buildShellRejected(args)}})
	case *pb.ExecServerMessage_DiagnosticsArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_DiagnosticsResult{DiagnosticsResult: buildDiagnosticsRejected(exec.GetDiagnosticsArgs().GetPath())}})
	case *pb.ExecServerMessage_McpArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_McpResult{McpResult: buildMcpError("Tool not available")}})
	case *pb.ExecServerMessage_BackgroundShellSpawnArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_BackgroundShellSpawnResult{BackgroundShellSpawnResult: &pb.BackgroundShellSpawnResult{
			Result: &pb.BackgroundShellSpawnResult_Rejected{Rejected: &pb.ShellRejected{Reason: "Not implemented"}},
		}}})
	case *pb.ExecServerMessage_WriteShellStdinArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_WriteShellStdinResult{WriteShellStdinResult: &pb.WriteShellStdinResult{
			Result: &pb.WriteShellStdinResult_Error{Error: &pb.WriteShellStdinError{Error: "Not implemented"}},
		}}})
	case *pb.ExecServerMessage_FetchArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_FetchResult{FetchResult: &pb.FetchResult{
			Result: &pb.FetchResult_Error{Error: &pb.FetchError{Url: exec.GetFetchArgs().GetUrl(), Error: "Not implemented"}},
		}}})
	case *pb.ExecServerMessage_ListMcpResourcesExecArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_ListMcpResourcesExecResult{ListMcpResourcesExecResult: &pb.ListMcpResourcesExecResult{}}})
	case *pb.ExecServerMessage_ReadMcpResourceExecArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_ReadMcpResourceExecResult{ReadMcpResourceExecResult: &pb.ReadMcpResourceExecResult{}}})
	case *pb.ExecServerMessage_RecordScreenArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_RecordScreenResult{RecordScreenResult: &pb.RecordScreenResult{}}})
	case *pb.ExecServerMessage_ComputerUseArgs:
		send(&pb.ExecClientMessage{Message: &pb.ExecClientMessage_ComputerUseResult{ComputerUseResult: &pb.ComputerUseResult{}}})
	default:
		// Bare acknowledgement so the server does not hang.
		send(&pb.ExecClientMessage{})
	}
}

func (c *Cursor) writeClient(stream *cursorStream, msg *pb.AgentClientMessage) {
	b, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	_ = stream.writer.writeMessage(b)
}

func hexKey(id []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(id)*2)
	for i, b := range id {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
}

// --- exec rejection result builders ---

func buildReadRejected(path string) *pb.ReadResult {
	return &pb.ReadResult{Result: &pb.ReadResult_Rejected{Rejected: &pb.ReadRejected{Path: path, Reason: "Tool not available"}}}
}

func buildLsRejected(path string) *pb.LsResult {
	return &pb.LsResult{Result: &pb.LsResult_Rejected{Rejected: &pb.LsRejected{Path: path, Reason: "Tool not available"}}}
}

func buildGrepError() *pb.GrepResult {
	return &pb.GrepResult{Result: &pb.GrepResult_Error{Error: &pb.GrepError{Error: "Tool not available"}}}
}

func buildWriteRejected(path string) *pb.WriteResult {
	return &pb.WriteResult{Result: &pb.WriteResult_Rejected{Rejected: &pb.WriteRejected{Path: path, Reason: "Tool not available"}}}
}

func buildDeleteRejected(path string) *pb.DeleteResult {
	return &pb.DeleteResult{Result: &pb.DeleteResult_Rejected{Rejected: &pb.DeleteRejected{Path: path, Reason: "Tool not available"}}}
}

func buildShellRejected(args *pb.ShellArgs) *pb.ShellResult {
	cmd := ""
	wd := ""
	if args != nil {
		cmd = args.GetCommand()
		wd = args.GetWorkingDirectory()
	}
	return &pb.ShellResult{Result: &pb.ShellResult_Rejected{Rejected: &pb.ShellRejected{Command: cmd, WorkingDirectory: wd, Reason: "Tool not available"}}}
}

func buildDiagnosticsRejected(path string) *pb.DiagnosticsResult {
	return &pb.DiagnosticsResult{Result: &pb.DiagnosticsResult_Rejected{Rejected: &pb.DiagnosticsRejected{Path: path, Reason: "Tool not available"}}}
}

func buildMcpError(msg string) *pb.McpResult {
	return &pb.McpResult{Result: &pb.McpResult_Error{Error: &pb.McpError{Error: msg}}}
}

// --- MCP / todo arg decoding ---

// decodeCursorMcpArgsMap decodes Cursor's per-key protobuf Value encoded args
// into a plain map.
func decodeCursorMcpArgsMap(args map[string][]byte) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = decodeCursorMcpArgValue(v)
	}
	return out
}

func decodeCursorMcpArgValue(value []byte) any {
	var v structpb.Value
	if err := proto.Unmarshal(value, &v); err == nil {
		decoded := v.AsInterface()
		if s, ok := decoded.(string); ok {
			return parseCursorToolArgsJSON(s)
		}
		return decoded
	}
	return parseCursorToolArgsJSON(string(value))
}

// parseCursorToolArgsJSON parses a tool-args string, normalizing Python-style
// literals (None/True/False) and falling back to the raw string.
func parseCursorToolArgsJSON(text string) any {
	trimmed := text
	if trimmed == "" {
		return text
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return v
	}
	return text
}

// parseCursorStreamingArgs parses accumulated streaming JSON, returning a map
// when possible.
func parseCursorStreamingArgs(partial string) map[string]any {
	if partial == "" {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(partial), &v); err == nil {
		return v
	}
	return nil
}

// mergeCursorMcpToolCallArgs merges the authoritative completion args over the
// streamed args, but keeps a streamed structured value when completion
// downgraded it to a string (Cursor omits/flattens oversized params).
func mergeCursorMcpToolCallArgs(streamed, completion map[string]any) map[string]any {
	merged := map[string]any{}
	for k, v := range streamed {
		merged[k] = v
	}
	for k, cv := range completion {
		if sv, ok := merged[k]; ok {
			if _, cs := cv.(string); cs {
				if isStructured(sv) {
					continue
				}
			}
		}
		merged[k] = cv
	}
	return merged
}

func isStructured(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

// cursorTodoArgs is the canonical todo tool argument shape.
type cursorTodoArgs struct {
	Todos []cursorTodoItem `json:"todos"`
}

type cursorTodoItem struct {
	ID         string `json:"id,omitempty"`
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status"`
}

// buildCursorTodoArgs maps an UpdateTodos tool call into canonical todo args,
// returning nil when the call is not a todo update.
func buildCursorTodoArgs(tc *pb.ToolCall) *cursorTodoArgs {
	update := tc.GetUpdateTodosToolCall()
	if update == nil || update.GetArgs() == nil {
		return nil
	}
	todos := update.GetArgs().GetTodos()
	if todos == nil {
		return nil
	}
	out := &cursorTodoArgs{Todos: make([]cursorTodoItem, 0, len(todos))}
	for _, t := range todos {
		out.Todos = append(out.Todos, cursorTodoItem{
			ID:         t.GetId(),
			Content:    t.GetContent(),
			ActiveForm: t.GetContent(),
			Status:     mapCursorTodoStatus(t.GetStatus()),
		})
	}
	return out
}

func mapCursorTodoStatus(status int32) string {
	switch status {
	case 2:
		return "in_progress"
	case 3:
		return "completed"
	default:
		return "pending"
	}
}
