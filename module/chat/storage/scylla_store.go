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

	var groupIDVal interface{} = nil
	if msg.GroupID != "" {
		parsed, err := gocql.ParseUUID(msg.GroupID)
		if err != nil {
			return fmt.Errorf("parse group_id %q to UUID: %w", msg.GroupID, err)
		}
		groupIDVal = parsed
	}

	msgID := gocql.TimeUUID()
	now := time.Now()

	// Write generated values back into the struct so that the caller
	// (business layer) can use the actual persisted values for Centrifugo publish.
	msg.MessageID = msgID.String()
	msg.CreatedAt = now

	q := s.session.Query(`
		INSERT INTO messages (conversation_id, group_id, created_at, message_id, sender_id, content_encrypted, key_for_sender, key_for_receiver, iv, is_read)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, convID, groupIDVal, now, msgID, msg.SenderID, msg.ContentEncrypted, msg.KeyForSender, msg.KeyForReceiver, msg.IV, msg.IsRead)

	if err := q.WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert message into ScyllaDB: %w", err)
	}

	log.Printf("[ScyllaDB] Message %s inserted for conversation %s", msgID.String(), msg.ConversationID)
	return nil
}

// GetMessages retrieves paginated messages for a conversation from ScyllaDB.
// If beforeTS is zero, it returns the latest messages (first page).
// Results are ordered by created_at DESC, limited to 'limit' items.
func (s *scyllaStore) GetMessages(ctx context.Context, conversationID string, beforeTS time.Time, limit int) ([]model.Message, error) {
	convID, err := gocql.ParseUUID(conversationID)
	if err != nil {
		return nil, fmt.Errorf("parse conversation_id %q to UUID: %w", conversationID, err)
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var query *gocql.Query
	if beforeTS.IsZero() {
		// First page: get latest messages
		query = s.session.Query(`
			SELECT conversation_id, group_id, created_at, message_id, sender_id, content_encrypted, key_for_sender, key_for_receiver, iv, is_read
			FROM messages
			WHERE conversation_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		`, convID, limit)
	} else {
		// Subsequent pages: get messages before the given timestamp
		query = s.session.Query(`
			SELECT conversation_id, group_id, created_at, message_id, sender_id, content_encrypted, key_for_sender, key_for_receiver, iv, is_read
			FROM messages
			WHERE conversation_id = ? AND created_at < ?
			ORDER BY created_at DESC
			LIMIT ?
		`, convID, beforeTS, limit)
	}

	iter := query.WithContext(ctx).Iter()
	var messages []model.Message

	var (
		cID       gocql.UUID
		gID       gocql.UUID
		createdAt time.Time
		msgID     gocql.UUID
		senderID  string
		content   string
		keyS      string
		keyR      string
		iv        string
		isRead    bool
	)

	for iter.Scan(&cID, &gID, &createdAt, &msgID, &senderID, &content, &keyS, &keyR, &iv, &isRead) {
		groupID := ""
		if gID != (gocql.UUID{}) {
			groupID = gID.String()
		}
		messages = append(messages, model.Message{
			ConversationID:   cID.String(),
			GroupID:          groupID,
			CreatedAt:        createdAt,
			MessageID:        msgID.String(),
			SenderID:         senderID,
			ContentEncrypted: content,
			KeyForSender:     keyS,
			KeyForReceiver:   keyR,
			IV:               iv,
			IsRead:           isRead,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("query messages for conversation %q: %w", conversationID, err)
	}

	return messages, nil
}
