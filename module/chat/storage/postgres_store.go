package storage

import (
	"context"
	"fmt"
	"time"

	"be-chat-centrifugo/module/chat/dto"
	"be-chat-centrifugo/module/chat/model"
	"gorm.io/gorm"
)

type postgresStore struct {
	db *gorm.DB
}

// NewPostgresStore creates a new Postgres-backed store for users, conversations, and participants.
func NewPostgresStore(db *gorm.DB) *postgresStore {
	return &postgresStore{db: db}
}

// --- User operations ---

// CreateUser persists a new user record.
func (s *postgresStore) CreateUser(ctx context.Context, u *model.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

// GetUserByUsername retrieves a user by their unique username.
func (s *postgresStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("get user by username %q: %w", username, err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by their UUID primary key.
func (s *postgresStore) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, fmt.Errorf("get user by id %q: %w", id, err)
	}
	return &user, nil
}

// UpdateUserPublicKey updates the public key for a given user.
func (s *postgresStore) UpdateUserPublicKey(ctx context.Context, userID string, publicKey string) error {
	result := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("public_key", publicKey)
	if result.Error != nil {
		return fmt.Errorf("update public key for user %q: %w", userID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user %q not found", userID)
	}
	return nil
}

// --- Conversation operations ---

// CreateConversation persists a new conversation record.
func (s *postgresStore) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	return s.db.WithContext(ctx).Create(conv).Error
}

// GetConversationsByUserID returns all conversations that a user is a participant of,
// ordered by most recently updated first.
func (s *postgresStore) GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error) {
	var conversations []model.Conversation
	err := s.db.WithContext(ctx).
		Joins("JOIN participants ON participants.conversation_id = conversations.id").
		Where("participants.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, fmt.Errorf("get conversations by user %q: %w", userID, err)
	}
	return conversations, nil
}

// GetDetailedConversations returns all conversations with participants and their public keys.
func (s *postgresStore) GetDetailedConversations(ctx context.Context, userID string) ([]dto.ConversationDetail, error) {
	var conversations []model.Conversation
	err := s.db.WithContext(ctx).
		Joins("JOIN participants ON participants.conversation_id = conversations.id").
		Where("participants.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, fmt.Errorf("get detailed conversations for user %q: %w", userID, err)
	}

	details := make([]dto.ConversationDetail, 0, len(conversations))
	for _, conv := range conversations {
		var participants []dto.ParticipantDetail
		err := s.db.WithContext(ctx).
			Table("participants").
			Select("users.id as user_id, users.username, users.public_key").
			Joins("JOIN users ON users.id = participants.user_id").
			Where("participants.conversation_id = ?", conv.ID).
			Scan(&participants).Error
		if err != nil {
			return nil, fmt.Errorf("get participants for conversation %q: %w", conv.ID, err)
		}

		// For direct chats, we want to display the peer's name as the conversation name.
		displayName := conv.Name
		if conv.Type == model.ConversationTypeDirect {
			for _, p := range participants {
				if p.UserID != userID {
					displayName = p.Username
					break
				}
			}
		}

		details = append(details, dto.ConversationDetail{
			ID:           conv.ID,
			Name:         displayName,
			Type:         conv.Type,
			Participants: participants,
			CreatedAt:    conv.CreatedAt,
			UpdatedAt:    conv.UpdatedAt,
		})
	}

	return details, nil
}

// --- Participant operations ---

// AddParticipant inserts a single participant record.
func (s *postgresStore) AddParticipant(ctx context.Context, p *model.Participant) error {
	return s.db.WithContext(ctx).Create(p).Error
}

// AddParticipants inserts multiple participant records in a single batch operation.
func (s *postgresStore) AddParticipants(ctx context.Context, participants []model.Participant) error {
	if len(participants) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&participants).Error
}

// CreateGroupConversation creates a conversation and inserts all participants
// in a single Postgres transaction for atomicity. If any step fails, the entire
// operation is rolled back.
func (s *postgresStore) CreateGroupConversation(ctx context.Context, conv *model.Conversation, participants []model.Participant) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(conv).Error; err != nil {
			return fmt.Errorf("create conversation: %w", err)
		}

		// Stamp the generated conversation ID onto each participant.
		now := time.Now()
		for i := range participants {
			participants[i].ConversationID = conv.ID
			participants[i].JoinedAt = now
		}

		if err := tx.Create(&participants).Error; err != nil {
			return fmt.Errorf("insert participants: %w", err)
		}

		return nil
	})
}

// GetParticipantIDs returns all user IDs that are participants of a given conversation.
func (s *postgresStore) GetParticipantIDs(ctx context.Context, conversationID string) ([]string, error) {
	var userIDs []string
	err := s.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("conversation_id = ?", conversationID).
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("get participant IDs for conversation %q: %w", conversationID, err)
	}
	return userIDs, nil
}

// GetConversationIDsByUserID returns all conversation IDs the user belongs to.
func (s *postgresStore) GetConversationIDsByUserID(ctx context.Context, userID string) ([]string, error) {
	var conversationIDs []string
	err := s.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("user_id = ?", userID).
		Pluck("conversation_id", &conversationIDs).Error
	if err != nil {
		return nil, fmt.Errorf("get conversation IDs for user %q: %w", userID, err)
	}
	return conversationIDs, nil
}

// GetDirectConversationBetween checks if a 1-1 conversation already exists between two users.
func (s *postgresStore) GetDirectConversationBetween(ctx context.Context, user1ID, user2ID string) (*model.Conversation, error) {
	var convs []model.Conversation
	err := s.db.WithContext(ctx).Raw(`
		SELECT c.* FROM conversations c
		JOIN participants p1 ON c.id = p1.conversation_id
		JOIN participants p2 ON c.id = p2.conversation_id
		WHERE c.type = ? AND p1.user_id = ? AND p2.user_id = ?
		LIMIT 1;
	`, model.ConversationTypeDirect, user1ID, user2ID).Scan(&convs).Error

	if err != nil {
		return nil, fmt.Errorf("get direct conversation between %q and %q: %w", user1ID, user2ID, err)
	}
	if len(convs) == 0 {
		return nil, nil // Not found, but no error
	}
	return &convs[0], nil
}

// --- Search Users ---

// SearchUsers finds users by email or username ignoring case, excluding the caller.
func (s *postgresStore) SearchUsers(ctx context.Context, query string, excludeUserID string) ([]model.User, error) {
	var users []model.User
	wildcard := "%" + query + "%"
	err := s.db.WithContext(ctx).
		Where("id != ? AND (username ILIKE ? OR email ILIKE ?)", excludeUserID, wildcard, wildcard).
		Limit(20). // limit results for safety
		Find(&users).Error
	if err != nil {
		return nil, fmt.Errorf("search users by %q: %w", query, err)
	}
	return users, nil
}

// --- Friendship operations ---

func (s *postgresStore) CreateFriendship(ctx context.Context, f *model.Friendship) error {
	return s.db.WithContext(ctx).Create(f).Error
}

func (s *postgresStore) GetFriendship(ctx context.Context, id string) (*model.Friendship, error) {
	var f model.Friendship
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&f).Error; err != nil {
		return nil, fmt.Errorf("get friendship %q: %w", id, err)
	}
	return &f, nil
}

func (s *postgresStore) GetFriendshipBetween(ctx context.Context, user1ID, user2ID string) (*model.Friendship, error) {
	var f model.Friendship
	err := s.db.WithContext(ctx).
		Where("(requester_id = ? AND receiver_id = ?) OR (requester_id = ? AND receiver_id = ?)", user1ID, user2ID, user2ID, user1ID).
		First(&f).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get friendship between %q and %q: %w", user1ID, user2ID, err)
	}
	return &f, nil
}

func (s *postgresStore) UpdateFriendshipStatus(ctx context.Context, id string, status string) error {
	err := s.db.WithContext(ctx).Model(&model.Friendship{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		return fmt.Errorf("update friendship %q status to %q: %w", id, status, err)
	}
	return nil
}

func (s *postgresStore) GetPendingRequests(ctx context.Context, userID string) ([]model.Friendship, error) {
	var requests []model.Friendship
	// Load the requester info as well
	err := s.db.WithContext(ctx).
		Preload("Requester").
		Where("receiver_id = ? AND status = ?", userID, model.FriendshipStatusPending).
		Order("created_at DESC").
		Find(&requests).Error
	if err != nil {
		return nil, fmt.Errorf("get pending requests for %q: %w", userID, err)
	}
	return requests, nil
}
