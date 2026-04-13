package business

import (
	"context"
	"fmt"
	"time"

	"be-chat-centrifugo/module/chat/model"
	"github.com/gocql/gocql"
)

type ChatBusiness struct {
	msgStore  MessageStorage
	publisher MessagePublisher
	convStore ConversationStorage
}

func NewChatBusiness(msgStore MessageStorage, publisher MessagePublisher, convStore ConversationStorage) *ChatBusiness {
	return &ChatBusiness{
		msgStore:  msgStore,
		publisher: publisher,
		convStore: convStore,
	}
}

type SendMessageReq struct {
	ConversationID   string `json:"conversation_id"`
	ContentEncrypted string `json:"content_encrypted"`
}

func (biz *ChatBusiness) SendMessage(ctx context.Context, senderID string, req *SendMessageReq) error {
	// Construct message
	msg := &model.Message{
		ConversationID:   req.ConversationID,
		CreatedAt:        time.Now(),
		MessageID:        gocql.TimeUUID().String(),
		SenderID:         senderID,
		ContentEncrypted: req.ContentEncrypted,
		IsRead:           false,
	}

	// Persist to ScyllaDB
	if err := biz.msgStore.InsertMessage(ctx, msg); err != nil {
		return err
	}

	// Publish to Centrifugo
	channel := fmt.Sprintf("chat:%s", req.ConversationID)
	return biz.publisher.PublishMessage(ctx, channel, msg)
}
