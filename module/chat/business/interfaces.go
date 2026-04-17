package business

import (
	"context"
	"time"

	"be-chat-centrifugo/module/chat/dto"
	"be-chat-centrifugo/module/chat/model"
)

// UserStorage defines the contract for user data access.
type UserStorage interface {
	CreateUser(ctx context.Context, u *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	UpdateUserPublicKey(ctx context.Context, userID string, publicKey string) error
	SearchUsers(ctx context.Context, query string, excludeUserID string) ([]model.User, error)
}

// ConversationStorage defines the contract for conversation and participant data access.
type ConversationStorage interface {
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	AddParticipant(ctx context.Context, p *model.Participant) error
	AddParticipants(ctx context.Context, participants []model.Participant) error
	GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error)
	GetDetailedConversations(ctx context.Context, userID string) ([]dto.ConversationDetail, error)
	GetParticipantIDs(ctx context.Context, conversationID string) ([]string, error)
	GetConversationIDsByUserID(ctx context.Context, userID string) ([]string, error)
	GetDirectConversationBetween(ctx context.Context, user1ID, user2ID string) (*model.Conversation, error)
}

// MessageStorage defines the contract for message persistence (ScyllaDB).
type MessageStorage interface {
	InsertMessage(ctx context.Context, msg *model.Message) error
	GetMessages(ctx context.Context, conversationID string, beforeTS time.Time, limit int) ([]model.Message, error)
}

// MessagePublisher defines the contract for real-time message delivery (Centrifugo).
type MessagePublisher interface {
	PublishMessage(ctx context.Context, channel string, data interface{}) error
}

// PresenceStorage defines the contract for user presence tracking (Redis).
type PresenceStorage interface {
	SetOnline(ctx context.Context, userID string, ttl time.Duration) error
	SetOffline(ctx context.Context, userID string) error
	GetPresence(ctx context.Context, userID string) (*model.PresenceInfo, error)
	GetBulkPresence(ctx context.Context, userIDs []string) ([]model.PresenceInfo, error)
}

// FriendshipStorage defines the contract for friendship data access.
type FriendshipStorage interface {
	CreateFriendship(ctx context.Context, f *model.Friendship) error
	GetFriendship(ctx context.Context, id string) (*model.Friendship, error)
	GetFriendshipBetween(ctx context.Context, user1ID, user2ID string) (*model.Friendship, error)
	UpdateFriendshipStatus(ctx context.Context, id string, status string) error
	GetPendingRequests(ctx context.Context, userID string) ([]model.Friendship, error)
}
