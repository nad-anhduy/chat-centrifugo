package storage

import (
	"context"
	"log"
	"time"

	"be-chat-centrifugo/module/chat/model"

	"github.com/gocql/gocql"
)

type scyllaStore struct {
	session *gocql.Session
}

func NewScyllaStore(session *gocql.Session) *scyllaStore {
	return &scyllaStore{session: session}
}

func (s *scyllaStore) InsertMessage(ctx context.Context, msg *model.Message) error {
	log.Printf("Inserting message to Scylla: %+v", msg)

	convID, err := gocql.ParseUUID(msg.ConversationID)
	if err != nil {
		log.Printf("[ScyllaDB Error] Failed to parse conversation_id to UUID: %v", err)
		return err
	}

	msgID := gocql.TimeUUID()
	now := time.Now()

	// Update the message model with the actual values used for insertion
	// so that they reflect in downstream processes if needed.
	msg.MessageID = msgID.String()
	msg.CreatedAt = now

	q := s.session.Query(`
		INSERT INTO messages (conversation_id, created_at, message_id, sender_id, content_encrypted, is_read)
		VALUES (?, ?, ?, ?, ?, ?)
	`, convID, now, msgID, msg.SenderID, msg.ContentEncrypted, msg.IsRead)

	if err := q.WithContext(ctx).Exec(); err != nil {
		log.Printf("[ScyllaDB Error] Failed to execute INSERT INTO messages: %v", err)
		return err
	}

	log.Printf("[ScyllaDB Success] Message %s inserted successfully", msgID.String())
	return nil
}
