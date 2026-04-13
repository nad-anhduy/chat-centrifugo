package business

import (
	"context"

	"be-chat-centrifugo/module/chat/model"
)

// UserStorage defines the contract for user data access.
type UserStorage interface {
	CreateUser(ctx context.Context, u *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	UpdateUserPublicKey(ctx context.Context, userID string, publicKey string) error
}

// ConversationStorage defines the contract for conversation and participant data access.
type ConversationStorage interface {
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	AddParticipant(ctx context.Context, p *model.Participant) error
	AddParticipants(ctx context.Context, participants []model.Participant) error
	GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error)
}

// MessageStorage defines the contract for message persistence (ScyllaDB).
type MessageStorage interface {
	InsertMessage(ctx context.Context, msg *model.Message) error
}

// MessagePublisher defines the contract for real-time message delivery (Centrifugo).
type MessagePublisher interface {
	PublishMessage(ctx context.Context, channel string, data interface{}) error
}
