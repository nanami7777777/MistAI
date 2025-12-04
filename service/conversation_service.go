package service

import (
	"Mist/database"
	"fmt"
)

// ConversationService handles conversation-related operations
type ConversationService struct{}

// NewConversationService creates a new ConversationService instance
func NewConversationService() *ConversationService {
	return &ConversationService{}
}

// Conversation represents a chat conversation
type Conversation struct {
	ID   uint
	Name string
}

// CreateConversation creates a new conversation
func (s *ConversationService) CreateConversation(name string) (*Conversation, error) {
	conv, err := database.CreateConversation(name)
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation: %v", err)
	}

	return &Conversation{
		ID:   conv.ID,
		Name: conv.Name,
	}, nil
}

// GetAllConversations retrieves all conversations
func (s *ConversationService) GetAllConversations() ([]Conversation, error) {
	convs, err := database.GetAllConversations()
	if err != nil {
		return nil, fmt.Errorf("failed to get all conversations: %v", err)
	}

	conversations := make([]Conversation, len(convs))
	for i, c := range convs {
		conversations[i] = Conversation{
			ID:   c.ID,
			Name: c.Name,
		}
	}

	return conversations, nil
}

// DeleteConversation deletes a conversation by ID
func (s *ConversationService) DeleteConversation(id uint) error {
	return database.DeleteConversation(id)
}

// SwitchConversation switches to a different conversation
func (s *ConversationService) SwitchConversation(convID uint) error {
	return database.SetCurrentConversationID(convID)
}

// GetCurrentConversationID gets the current conversation ID
func (s *ConversationService) GetCurrentConversationID() uint {
	return database.GetCurrentConversationID()
}
