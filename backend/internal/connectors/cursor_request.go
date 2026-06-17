package connectors

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	pb "github.com/mydisha/keirouter/backend/internal/connectors/cursoragent"
	"github.com/mydisha/keirouter/backend/internal/core"
)

// cursorBlobStore is the per-conversation content-addressed store backing
// Cursor's KV handshake. History (user messages, steps, system/root-prompt
// JSON) is serialized into blobs keyed by sha256(content); the request carries
// only blob ids, and the server fetches blob bodies via getBlobArgs.
type cursorBlobStore struct {
	m map[string][]byte
}

func newCursorBlobStore() *cursorBlobStore {
	return &cursorBlobStore{m: map[string][]byte{}}
}

func cursorBlobID(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

// store writes data under sha256(data) and returns the blob id.
func (s *cursorBlobStore) store(data []byte) []byte {
	id := cursorBlobID(data)
	s.m[hex.EncodeToString(id)] = data
	return id
}

func (s *cursorBlobStore) get(id []byte) ([]byte, bool) {
	d, ok := s.m[hex.EncodeToString(id)]
	return d, ok
}

// cursorRootPromptPart mirrors the Vercel-AI-SDK content shape Cursor expects
// inside rootPromptMessagesJson.
type cursorRootPromptPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Image     string `json:"image,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

type cursorRootPromptMessage struct {
	Role    string                 `json:"role"`
	Content []cursorRootPromptPart `json:"content"`
}

type cursorSystemMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildCursorSystemPromptJSONs emits one system-message JSON per prompt so
// Cursor's blob cache hits independently per entry. A default greeting is used
// when no system prompt is present so the root-prompt head is never empty.
func buildCursorSystemPromptJSONs(system string) [][]byte {
	prompts := splitCursorSystemPrompts(system)
	if len(prompts) == 0 {
		b, _ := json.Marshal(cursorSystemMessage{Role: "system", Content: "You are a helpful assistant."})
		return [][]byte{b}
	}
	out := make([][]byte, 0, len(prompts))
	for _, p := range prompts {
		b, _ := json.Marshal(cursorSystemMessage{Role: "system", Content: p})
		out = append(out, b)
	}
	return out
}

func splitCursorSystemPrompts(system string) []string {
	system = strings.TrimSpace(system)
	if system == "" {
		return nil
	}
	return []string{system}
}

// messageText concatenates the text parts of a message.
func messageText(m core.Message) string {
	var b strings.Builder
	for i, p := range m.Content {
		if p.Type != core.PartText {
			continue
		}
		if b.Len() > 0 && i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

func messageHasImages(m core.Message) bool {
	for _, p := range m.Content {
		if p.Type == core.PartImage && p.Media != nil {
			return true
		}
	}
	return false
}

// toolResultText renders a tool-result message's content as text.
func toolResultText(m core.Message) (text string, isErr bool) {
	var b strings.Builder
	for _, p := range m.Content {
		if p.Type != core.PartToolResult || p.ToolResult == nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.ToolResult.Content)
		if p.ToolResult.IsError {
			isErr = true
		}
	}
	return b.String(), isErr
}

func isCursorUserRole(r core.Role) bool {
	return r == core.RoleUser || r == core.RoleSystem
}

// cursorUserContentKey hashes user content for deterministic message ids.
func cursorUserContentKey(m core.Message) string {
	if len(m.Content) == 1 && m.Content[0].Type == core.PartText {
		return strings.TrimSpace(m.Content[0].Text)
	}
	h := sha256.New()
	for _, p := range m.Content {
		switch p.Type {
		case core.PartText:
			h.Write([]byte("text"))
			h.Write([]byte(p.Text))
		case core.PartImage:
			if p.Media != nil {
				h.Write([]byte("image"))
				h.Write([]byte(p.Media.MIMEType))
				h.Write([]byte(p.Media.Data))
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// buildCursorRootPromptContent renders a message's content into the
// SDK-shaped parts used inside rootPromptMessagesJson.
func buildCursorRootPromptContent(m core.Message) []cursorRootPromptPart {
	var parts []cursorRootPromptPart
	for _, p := range m.Content {
		switch p.Type {
		case core.PartText:
			t := strings.TrimSpace(p.Text)
			if t != "" {
				parts = append(parts, cursorRootPromptPart{Type: "text", Text: t})
			}
		case core.PartImage:
			if p.Media != nil {
				parts = append(parts, cursorRootPromptPart{Type: "image", Image: p.Media.Data, MediaType: p.Media.MIMEType})
			}
		}
	}
	return parts
}

// findLastUserMessageIndex returns the index of the last user/system message.
func findLastUserMessageIndex(msgs []core.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if isCursorUserRole(msgs[i].Role) {
			return i
		}
	}
	return -1
}

// createCursorUserMessage builds a UserMessage, attaching decoded inline images
// as selected context when present.
func createCursorUserMessage(m core.Message, text, messageID string) *pb.UserMessage {
	um := &pb.UserMessage{Text: text, MessageId: messageID}
	var images []*pb.SelectedImage
	for _, p := range m.Content {
		if p.Type != core.PartImage || p.Media == nil {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(p.Media.Data)
		if err != nil {
			continue
		}
		images = append(images, &pb.SelectedImage{
			Uuid:         uuid.NewString(),
			MimeType:     p.Media.MIMEType,
			DataOrBlobId: &pb.SelectedImage_Data{Data: data},
		})
	}
	if len(images) > 0 {
		um.SelectedContext = &pb.SelectedContext{SelectedImages: images}
	}
	return um
}

// buildRootPromptMessagesJSON builds the rootPromptMessagesJson blob ids for the
// system prompt plus prior history (excluding the active user message). Cursor's
// server builds the actual model prompt from this field.
func buildRootPromptMessagesJSON(msgs []core.Message, systemPromptIDs [][]byte, store *cursorBlobStore, activeIdx int) [][]byte {
	entries := make([][]byte, 0, len(systemPromptIDs)+len(msgs))
	entries = append(entries, systemPromptIDs...)

	push := func(v any) {
		b, _ := json.Marshal(v)
		entries = append(entries, store.store(b))
	}

	for i := 0; i < len(msgs); i++ {
		if i == activeIdx {
			break
		}
		m := msgs[i]
		switch {
		case isCursorUserRole(m.Role):
			content := buildCursorRootPromptContent(m)
			if len(content) == 0 {
				continue
			}
			push(cursorRootPromptMessage{Role: "user", Content: content})
		case m.Role == core.RoleAssistant:
			text := messageText(m)
			if text == "" {
				continue
			}
			push(cursorRootPromptMessage{Role: "assistant", Content: []cursorRootPromptPart{{Type: "text", Text: text}}})
		case m.Role == core.RoleTool:
			text, isErr := toolResultText(m)
			if text == "" {
				continue
			}
			prefix := "[Tool Result]"
			if isErr {
				prefix = "[Tool Error]"
			}
			push(cursorRootPromptMessage{Role: "user", Content: []cursorRootPromptPart{{Type: "text", Text: prefix + "\n" + text}}})
		}
	}
	return entries
}

// buildConversationTurns groups prior messages into Cursor turns (a user message
// plus following assistant/tool steps), serializing each into blobs and
// returning the turn blob ids. The active user message is excluded.
func buildConversationTurns(msgs []core.Message, store *cursorBlobStore, activeIdx int) [][]byte {
	var turns [][]byte
	i := 0
	for i < len(msgs) {
		m := msgs[i]
		if !isCursorUserRole(m.Role) {
			i++
			continue
		}
		if i == activeIdx {
			break
		}
		userText := messageText(m)
		if strings.TrimSpace(userText) == "" && !messageHasImages(m) {
			i++
			continue
		}

		userMsg := createCursorUserMessage(m, userText, deterministicMessageID("u:"+itoa(len(turns))+":"+cursorUserContentKey(m)))
		userBytes, _ := proto.Marshal(userMsg)
		userBlobID := store.store(userBytes)

		var stepBlobIDs [][]byte
		i++
		for i < len(msgs) && !isCursorUserRole(msgs[i].Role) {
			step := msgs[i]
			switch step.Role {
			case core.RoleAssistant:
				if text := messageText(step); text != "" {
					cs := &pb.ConversationStep{Message: &pb.ConversationStep_AssistantMessage{AssistantMessage: &pb.AssistantMessage{Text: text}}}
					b, _ := proto.Marshal(cs)
					stepBlobIDs = append(stepBlobIDs, store.store(b))
				}
			case core.RoleTool:
				if text, isErr := toolResultText(step); text != "" {
					prefix := "[Tool Result]"
					if isErr {
						prefix = "[Tool Error]"
					}
					cs := &pb.ConversationStep{Message: &pb.ConversationStep_AssistantMessage{AssistantMessage: &pb.AssistantMessage{Text: prefix + "\n" + text}}}
					b, _ := proto.Marshal(cs)
					stepBlobIDs = append(stepBlobIDs, store.store(b))
				}
			}
			i++
		}

		agentTurn := &pb.AgentConversationTurnStructure{UserMessage: userBlobID, Steps: stepBlobIDs}
		turn := &pb.ConversationTurnStructure{Turn: &pb.ConversationTurnStructure_AgentConversationTurn{AgentConversationTurn: agentTurn}}
		b, _ := proto.Marshal(turn)
		turns = append(turns, store.store(b))
	}
	return turns
}

// buildMcpToolDefinitions advertises the caller's non-native tools to Cursor,
// encoding each tool's JSON schema as a protobuf Value.
func buildMcpToolDefinitions(tools []core.Tool) []*pb.McpToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	var defs []*pb.McpToolDefinition
	for _, t := range tools {
		if cursorNativeToolNames[t.Name] {
			continue
		}
		schema := cursorToolSchemaValue(t.Parameters)
		inputSchema, _ := proto.Marshal(schema)
		defs = append(defs, &pb.McpToolDefinition{
			Name:               t.Name,
			Description:        t.Description,
			ProviderIdentifier: cursorProviderIdentifier,
			ToolName:           t.Name,
			InputSchema:        inputSchema,
		})
	}
	return defs
}

// buildCursorRunRequest assembles the AgentClientMessage{runRequest} wire bytes
// from the canonical request, populating system-prompt blobs, history turns,
// rootPromptMessagesJson, and the user/resume action.
func buildCursorRunRequest(req *core.ChatRequest, conversationID string, store *cursorBlobStore) ([]byte, error) {
	systemPromptIDs := make([][]byte, 0)
	for _, j := range buildCursorSystemPromptJSONs(req.System) {
		systemPromptIDs = append(systemPromptIDs, store.store(j))
	}

	msgs := req.Messages
	activeIdx := len(msgs) - 1
	var activeUser *core.Message
	if activeIdx >= 0 && isCursorUserRole(msgs[activeIdx].Role) {
		activeUser = &msgs[activeIdx]
	}

	var userText string
	var hasImages bool
	if activeUser != nil {
		userText = strings.TrimSpace(messageText(*activeUser))
		hasImages = messageHasImages(*activeUser)
	}

	action := &pb.ConversationAction{}
	if activeUser != nil && (userText != "" || hasImages) {
		action.Action = &pb.ConversationAction_UserMessageAction{
			UserMessageAction: &pb.UserMessageAction{
				UserMessage: createCursorUserMessage(*activeUser, userText, uuid.NewString()),
			},
		}
	} else {
		action.Action = &pb.ConversationAction_ResumeAction{ResumeAction: &pb.ResumeAction{}}
	}

	turnsIdx := -1
	if activeUser != nil {
		turnsIdx = activeIdx
	}
	turns := buildConversationTurns(msgs, store, turnsIdx)
	rootPrompt := buildRootPromptMessagesJSON(msgs, systemPromptIDs, store, turnsIdx)

	conversationState := &pb.ConversationStateStructure{
		RootPromptMessagesJson: rootPrompt,
		Turns:                  turns,
	}

	modelDetails := &pb.ModelDetails{
		ModelId:        req.Model,
		DisplayModelId: req.Model,
		DisplayName:    req.Model,
	}

	convID := conversationID
	runReq := &pb.AgentRunRequest{
		ConversationState: conversationState,
		Action:            action,
		ModelDetails:      modelDetails,
		ConversationId:    &convID,
	}

	clientMsg := &pb.AgentClientMessage{Message: &pb.AgentClientMessage_RunRequest{RunRequest: runReq}}
	return proto.Marshal(clientMsg)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
