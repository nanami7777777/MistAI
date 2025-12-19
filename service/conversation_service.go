package service

import (
	"Mist/database"
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ConversationService struct{}

func NewConversationService() *ConversationService {
	return &ConversationService{}
}

// Conversation represents a chat conversation
type Conversation struct {
	ID   primitive.ObjectID
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
func (s *ConversationService) DeleteConversation(id primitive.ObjectID) error {
	return database.DeleteConversation(id)
}

// SwitchConversation switches to a different conversation
func (s *ConversationService) SwitchConversation(convID primitive.ObjectID) error {
	return database.SetCurrentConversationID(convID)
}

// GetCurrentConversationID gets the current conversation ID
func (s *ConversationService) GetCurrentConversationID() primitive.ObjectID {
	return database.GetCurrentConversationID()
}
