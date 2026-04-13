package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"be-chat-centrifugo/module/chat/model"

	"github.com/gocql/gocql"
)

type scyllaStore struct {
	session *gocql.Session
}

// NewScyllaStore creates a new ScyllaDB-backed message store.
func NewScyllaStore(session *gocql.Session) *scyllaStore {
	return &scyllaStore{session: session}
}

// InsertMessage persists a message to ScyllaDB.
// It generates the MessageID (TimeUUID) and CreatedAt timestamp,
// writing them back into the msg struct for downstream consumers.
func (s *scyllaStore) InsertMessage(ctx context.Context, msg *model.Message) error {
	convID, err := gocql.ParseUUID(msg.ConversationID)
	if err != nil {
		return fmt.Errorf("parse conversation_id %q to UUID: %w", msg.ConversationID, err)
	}

	msgID := gocql.TimeUUID()
	now := time.Now()

	// Write generated values back into the struct so that the caller
	// (business layer) can use the actual persisted values for Centrifugo publish.
	msg.MessageID = msgID.String()
	msg.CreatedAt = now

	q := s.session.Query(`
		INSERT INTO messages (conversation_id, created_at, message_id, sender_id, content_encrypted, is_read)
		VALUES (?, ?, ?, ?, ?, ?)
	`, convID, now, msgID, msg.SenderID, msg.ContentEncrypted, msg.IsRead)

	if err := q.WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert message into ScyllaDB: %w", err)
	}

	log.Printf("[ScyllaDB] Message %s inserted for conversation %s", msgID.String(), msg.ConversationID)
	return nil
}
