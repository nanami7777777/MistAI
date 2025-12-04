package service

import (
	"Mist/database"
	"Mist/global"
	"Mist/llm"
	"fmt"
)

// MessageService handles message-related operations
type MessageService struct {
	MessageEventHandler func(event MessageEvent)
}

// NewMessageService creates a new MessageService instance
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

// SendMessage sends a user message and gets an AI response
func (s *MessageService) SendMessage(req SendMessageRequest) (*SendMessageResponse, error) {
	// Save user message
	userMsgID, err := database.SaveMessage("user", req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to save user message: %v", err)
	}

	userMessage := Message{
		ID:      userMsgID,
		Role:    "user",
		Content: req.Content,
	}

	// Notify that user message was sent
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type:    "user_message_sent",
			Message: &userMessage,
		})
	}

	// Get conversation history
	messages, err := s.LoadMessages()
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %v", err)
	}

	// Convert to global messages
	globalMessages := make([]global.Message, len(messages))
	for i, m := range messages {
		globalMessages[i] = global.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	// Add the new user message
	globalMessages = append(globalMessages, global.Message{
		Role:    "user",
		Content: req.Content,
	})

	// Notify that AI response is starting
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type: "ai_response_start",
		})
	}

	// Get AI response
	response, err := llm.CallChatAPI(globalMessages, nil)
	if err != nil {
		// Notify about error
		if s.MessageEventHandler != nil {
			s.MessageEventHandler(MessageEvent{
				Type:  "error",
				Error: err,
			})
		}
		response = "Error: " + err.Error()
	}

	// Save AI response
	aiMsgID, err := database.SaveMessage("assistant", response)
	if err != nil {
		return nil, fmt.Errorf("failed to save AI message: %v", err)
	}

	aiMessage := Message{
		ID:      aiMsgID,
		Role:    "assistant",
		Content: response,
	}

	// Notify that AI response is complete
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type:    "ai_response_complete",
			Message: &aiMessage,
		})
	}

	return &SendMessageResponse{
		UserMessageID: userMsgID,
		AIMessageID:   aiMsgID,
		UserMessage:   userMessage,
		AIMessage:     aiMessage,
	}, nil
}

// StreamMessage sends a user message and streams the AI response
func (s *MessageService) StreamMessage(req SendMessageRequest, responseChan chan string) (*SendMessageResponse, error) {
	// Save user message
	userMsgID, err := database.SaveMessage("user", req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to save user message: %v", err)
	}

	userMessage := Message{
		ID:      userMsgID,
		Role:    "user",
		Content: req.Content,
	}

	// Notify that user message was sent
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type:    "user_message_sent",
			Message: &userMessage,
		})
	}

	// Get conversation history
	messages, err := s.LoadMessages()
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %v", err)
	}

	// Convert to global messages
	globalMessages := make([]global.Message, len(messages))
	for i, m := range messages {
		globalMessages[i] = global.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	// Add the new user message
	globalMessages = append(globalMessages, global.Message{
		Role:    "user",
		Content: req.Content,
	})

	// Notify that AI response is starting
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type: "ai_response_start",
		})
	}

	// Create a wrapper channel to intercept chunks
	wrapperChan := make(chan string)
	go func() {
		for chunk := range wrapperChan {
			// Forward to caller's channel
			if responseChan != nil {
				responseChan <- chunk
			}

			// Notify about chunk
			if s.MessageEventHandler != nil {
				s.MessageEventHandler(MessageEvent{
					Type:    "ai_response_chunk",
					Content: chunk,
				})
			}
		}
		if responseChan != nil {
			close(responseChan)
		}
	}()

	// Get AI response with streaming
	response, err := llm.CallChatAPI(globalMessages, wrapperChan)
	if err != nil {
		// Notify about error
		if s.MessageEventHandler != nil {
			s.MessageEventHandler(MessageEvent{
				Type:  "error",
				Error: err,
			})
		}
		response = "Error: " + err.Error()
	}

	// Save AI response
	aiMsgID, err := database.SaveMessage("assistant", response)
	if err != nil {
		return nil, fmt.Errorf("failed to save AI message: %v", err)
	}

	aiMessage := Message{
		ID:      aiMsgID,
		Role:    "assistant",
		Content: response,
	}

	// Notify that AI response is complete
	if s.MessageEventHandler != nil {
		s.MessageEventHandler(MessageEvent{
			Type:    "ai_response_complete",
			Message: &aiMessage,
		})
	}

	return &SendMessageResponse{
		UserMessageID: userMsgID,
		AIMessageID:   aiMsgID,
		UserMessage:   userMessage,
		AIMessage:     aiMessage,
	}, nil
}

// DeleteMessage deletes a message by ID
func (s *MessageService) DeleteMessage(messageID uint) error {
	return database.DeleteMessage(messageID)
}

// ClearAllMessages clears all messages in current conversation
func (s *MessageService) ClearAllMessages() error {
	return database.ClearAllMessages()
}
