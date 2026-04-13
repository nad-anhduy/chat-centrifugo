package storage

import (
	"context"

	"be-chat-centrifugo/module/chat/model"
	"gorm.io/gorm"
)

type postgresStore struct {
	db *gorm.DB
}

func NewPostgresStore(db *gorm.DB) *postgresStore {
	return &postgresStore{db: db}
}

func (s *postgresStore) CreateUser(ctx context.Context, u *model.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *postgresStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *postgresStore) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	return s.db.WithContext(ctx).Create(conv).Error
}

func (s *postgresStore) AddParticipant(ctx context.Context, p *model.Participant) error {
	return s.db.WithContext(ctx).Create(p).Error
}
