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

	HasActiveUserDevice(ctx context.Context, userID, fingerprint string) (bool, error)
	InsertUserDevice(ctx context.Context, d *model.UserDevice) error
	UpdateUserDeviceLastLogin(ctx context.Context, userID, fingerprint string) error
	InsertUserDeviceChanged(ctx context.Context, c *model.UserDeviceChanged) error
}

// ConversationStorage defines the contract for conversation and participant data access.
type ConversationStorage interface {
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	GetConversationByID(ctx context.Context, conversationID string) (*model.Conversation, error)
	AddParticipant(ctx context.Context, p *model.Participant) error
	AddParticipants(ctx context.Context, participants []model.Participant) error
	GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error)
	GetDetailedConversations(ctx context.Context, userID string) ([]dto.ConversationDetail, error)
	GetParticipantIDs(ctx context.Context, conversationID string) ([]string, error)
	GetConversationIDsByUserID(ctx context.Context, userID string) ([]string, error)
	GetDirectConversationBetween(ctx context.Context, user1ID, user2ID string) (*model.Conversation, error)
}

type GroupStorage interface {
	CreateGroup(ctx context.Context, g *model.Group) error
	AddGroupMembers(ctx context.Context, members []model.GroupMember) error
	AddGroupMember(ctx context.Context, m *model.GroupMember) error
	GetGroupByID(ctx context.Context, groupID string) (*model.Group, error)
	GetGroupMember(ctx context.Context, groupID, userID string) (*model.GroupMember, error)
	GetGroupMemberEncryptedKey(ctx context.Context, groupID, userID string) (string, error)
	ListGroupsByUserID(ctx context.Context, userID string) ([]model.Group, error)
	ListGroupMembers(ctx context.Context, groupID string) ([]dto.GroupMemberInfo, error)

	// CreateGroupConversationAtomic creates:
	// - groups row
	// - group_members rows
	// - conversations row (id == group.id, type=GROUP)
	// - participants rows (conversation_id == group.id)
	// in a single transaction.
	CreateGroupConversationAtomic(ctx context.Context, g *model.Group, members []model.GroupMember, conv *model.Conversation, participants []model.Participant) error

	// AddGroupMemberAtomic adds:
	// - group_members row
	// - participants row
	// in a single transaction.
	AddGroupMemberAtomic(ctx context.Context, m *model.GroupMember, p *model.Participant) error
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
