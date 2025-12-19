package service

import (
	"Mist/database"
	"Mist/global"
	"Mist/llm"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

type MessageService struct {
	MessageEventHandler func(event MessageEvent)
	streamingActive     int32
	cancelMu            sync.Mutex
	cancelFunc          context.CancelFunc
}

func NewMessageService() *MessageService {
	return &MessageService{}
}

// SetMessageEventHandler sets the handler for message events
func (s *MessageService) SetMessageEventHandler(handler func(event MessageEvent)) {
	s.MessageEventHandler = handler
}

// Message represents a chat message
type Message struct {
	ID      uint
	Role    string
	Content string
}

// SendMessageRequest contains the data needed to send a message
type SendMessageRequest struct {
	Content string
}

// SendMessageResponse contains the result of sending a message
type SendMessageResponse struct {
	UserMessageID uint
	AIMessageID   uint
	UserMessage   Message
	AIMessage     Message
}

// MessageEvent represents events that can occur during messaging
type MessageEvent struct {
	Type    string // "user_message_sent", "ai_response_start", "ai_response_chunk", "ai_response_complete", "error"
	Message *Message
	Content string
	Error   error
}

func normalizeEscapedNewlines(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\\r\\n", "\n")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	return s
}

// LoadMessages loads messages for the current conversation
func (s *MessageService) LoadMessages() ([]Message, error) {
	history, err := database.LoadHistoryMessages()
	if err != nil {
		return nil, fmt.Errorf("failed to load history messages: %v", err)
	}

	messages := make([]Message, len(history))
	for i, h := range history {
		messages[i] = Message{
			ID:      h.ID,
			Role:    h.Role,
			Content: h.Content,
		}
	}

	return messages, nil
}

// StreamMessage sends a user message and streams the AI response
func (s *MessageService) StreamMessage(req SendMessageRequest) {
	if !atomic.CompareAndSwapInt32(&s.streamingActive, 0, 1) {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	s.cancelFunc = cancel
	s.cancelMu.Unlock()
	// 1. Save user message and notify UI
	userMsgID, err := database.SaveMessage("user", req.Content)
	if err != nil {
		s.notifyError(fmt.Errorf("failed to save user message: %w", err))
		return
	}

	userMessage := Message{ID: userMsgID, Role: "user", Content: req.Content}
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{Type: "user_message_sent", Message: &userMessage})
	}

	// 2. Load history and prepare for LLM call
	messages, err := s.LoadMessages()
	if err != nil {
		s.notifyError(fmt.Errorf("failed to load messages: %w", err))
		return
	}

	globalMessages := toGlobalMessages(messages)
	globalMessages = append(globalMessages, global.Message{Role: "user", Content: req.Content})

	// 3. Notify UI that AI response is starting
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{Type: "ai_response_start"})
	}

	// 4. Start the main processing goroutine
	go func() {
		defer func() {
			atomic.StoreInt32(&s.streamingActive, 0)
			s.cancelMu.Lock()
			s.cancelFunc = nil
			s.cancelMu.Unlock()
		}()
		llmChan := make(chan string, 100)
		var textBuffer, jsonBuffer strings.Builder
		var wg sync.WaitGroup
		wg.Add(1)

		// Goroutine to process chunks from the LLM in real-time using a trigger-based state machine
		go func() {
			defer wg.Done()
			const jsonStartTrigger = "<DA" // Use a short, reliable trigger
			parsingMode := "text"          // "text" or "json"

			for chunk := range llmChan {
				if parsingMode == "json" {
					jsonBuffer.WriteString(chunk)
					continue
				}

				// We are in text mode, check for the trigger
				if idx := strings.Index(chunk, jsonStartTrigger); idx != -1 {
					// Found the trigger, switch to JSON mode
					textPart := chunk[:idx]
					jsonPart := chunk[idx:]

					if textPart != "" {
						normalized := normalizeEscapedNewlines(textPart)
						textBuffer.WriteString(normalized)
						if s.MessageEventHandler != nil {
							s.MessageEventHandler(MessageEvent{Type: "ai_response_chunk", Content: normalized})
						}
					}

					parsingMode = "json"
					jsonBuffer.WriteString(jsonPart)
				} else {
					// No trigger found, the whole chunk is text
					normalized := normalizeEscapedNewlines(chunk)
					textBuffer.WriteString(normalized)
					if s.MessageEventHandler != nil {
						s.MessageEventHandler(MessageEvent{Type: "ai_response_chunk", Content: normalized})
					}
				}
			}
		}()

		// This call blocks until the stream is finished, feeding llmChan.
		llmRes, err := llm.CallChatAPI(ctx, globalMessages, llmChan)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				wg.Wait()
				return
			}
			wg.Done()
			s.notifyError(fmt.Errorf("failed to get AI response: %w", err))
			return
		}
		log.Println("llmRes:", llmRes)
		// Wait for the chunk processor to finish
		wg.Wait()

		// Now that the stream is fully processed, clean up the JSON and save the results
		const jsonStartTag = "<DATA_JSON>"
		const jsonEndTag = "</DATA_JSON>"
		userResponse := textBuffer.String()
		jsonDataWithTags := jsonBuffer.String()
		cleanedJson := ""

		if startTagIdx := strings.Index(jsonDataWithTags, jsonStartTag); startTagIdx != -1 {
			contentStartIdx := startTagIdx + len(jsonStartTag)
			if endTagIdx := strings.Index(jsonDataWithTags, jsonEndTag); endTagIdx != -1 {
				cleanedJson = jsonDataWithTags[contentStartIdx:endTagIdx]
			}
		}

		aiMsgID, err := database.SaveMessage("assistant", userResponse)
		if err != nil {
			s.notifyError(fmt.Errorf("failed to save AI message: %w", err))
			return
		}

		if cleanedJson != "" {
			if err := database.SaveJsonData(cleanedJson); err != nil {
				s.notifyError(fmt.Errorf("failed to save JSON data: %w", err))
				return
			}
		}

		// Notify UI that the process is complete
		aiMessage := Message{ID: aiMsgID, Role: "assistant", Content: userResponse}
		if s.MessageEventHandler != nil {
			s.MessageEventHandler(MessageEvent{Type: "ai_response_complete", Message: &aiMessage})
		}
	}()
}

// notifyError is a helper to dispatch error events.
func (s *MessageService) notifyError(err error) {
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{Type: "error", Error: err})
	}
}

// toGlobalMessages converts local messages to the global format.
func toGlobalMessages(messages []Message) []global.Message {
	globalMessages := make([]global.Message, len(messages))
	for i, m := range messages {
		globalMessages[i] = global.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return globalMessages
}

// isJSON checks if a string is likely a JSON object.
// This is a simplified check; a more robust one might be needed.
func isJSON(s string) bool {
	return len(s) > 0 && s[0] == '{' && s[len(s)-1] == '}'
}

// DeleteMessage deletes a message by ID
func (s *MessageService) DeleteMessage(messageID uint) error {
	return database.DeleteMessage(messageID)
}

// ClearAllMessages clears all messages in current conversation
func (s *MessageService) ClearAllMessages() error {
	return database.ClearAllMessages()
}

func (s *MessageService) StopStreaming() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}
