package storage

import (
	"context"
	"fmt"
	"time"

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
