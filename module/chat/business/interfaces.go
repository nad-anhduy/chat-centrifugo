package business

import (
	"context"

	"be-chat-centrifugo/module/chat/model"
)

type UserStorage interface {
	CreateUser(ctx context.Context, u *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
}

type ConversationStorage interface {
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	AddParticipant(ctx context.Context, p *model.Participant) error
}

type MessageStorage interface {
	InsertMessage(ctx context.Context, msg *model.Message) error
}

type MessagePublisher interface {
	PublishMessage(ctx context.Context, channel string, data interface{}) error
}
