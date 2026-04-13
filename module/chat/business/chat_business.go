package business

import (
	"context"
	"fmt"
	"log"
	"time"

	"be-chat-centrifugo/module/chat/dto"
	"be-chat-centrifugo/module/chat/model"
)

// ChatBusiness encapsulates the core messaging and conversation use cases.
type ChatBusiness struct {
	msgStore  MessageStorage
	publisher MessagePublisher
	convStore ConversationStorage
	userStore UserStorage
}

// NewChatBusiness creates a new ChatBusiness with all required dependencies.
func NewChatBusiness(
	msgStore MessageStorage,
	publisher MessagePublisher,
	convStore ConversationStorage,
	userStore UserStorage,
) *ChatBusiness {
	return &ChatBusiness{
		msgStore:  msgStore,
		publisher: publisher,
		convStore: convStore,
		userStore: userStore,
	}
}

// --- Send Message ---

// SendMessageReq is the input for sending a message in a conversation.
type SendMessageReq struct {
	ConversationID   string `json:"conversation_id" binding:"required"`
	ContentEncrypted string `json:"content_encrypted" binding:"required"`
}

// SendMessage persists a message to ScyllaDB and publishes it to Centrifugo
// for real-time delivery. The storage layer generates MessageID and CreatedAt.
func (biz *ChatBusiness) SendMessage(ctx context.Context, senderID string, req *SendMessageReq) error {
	// Construct message — MessageID and CreatedAt will be set by InsertMessage.
	msg := &model.Message{
		ConversationID:   req.ConversationID,
		SenderID:         senderID,
		ContentEncrypted: req.ContentEncrypted,
		IsRead:           false,
	}

	// Persist to ScyllaDB
	if err := biz.msgStore.InsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("failed to persist message: %w", err)
	}

	// Resolve sender name for the real-time payload (graceful degradation).
	senderName := senderID // fallback to sender_id if lookup fails
	if sender, err := biz.userStore.GetUserByID(ctx, senderID); err != nil {
		log.Printf("[WARN] failed to resolve sender name for %s: %v", senderID, err)
	} else {
		senderName = sender.Username
	}

	// Build enriched payload for Centrifugo clients.
	payload := &dto.CentrifugoMessagePayload{
		MessageID:        msg.MessageID,
		ConversationID:   msg.ConversationID,
		SenderID:         senderID,
		SenderName:       senderName,
		ContentEncrypted: msg.ContentEncrypted,
		CreatedAt:        msg.CreatedAt,
	}

	// Publish to Centrifugo. If publish fails, the message is still persisted —
	// we log the error but do not fail the request.
	channel := fmt.Sprintf("chat:%s", req.ConversationID)
	if err := biz.publisher.PublishMessage(ctx, channel, payload); err != nil {
		log.Printf("[WARN] message %s persisted but failed to publish to Centrifugo: %v", msg.MessageID, err)
	}

	return nil
}

// --- Public Key Management ---

// GetUserPublicKey retrieves the public key for a user by their ID.
func (biz *ChatBusiness) GetUserPublicKey(ctx context.Context, userID string) (string, error) {
	user, err := biz.userStore.GetUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get public key for user %q: %w", userID, err)
	}
	if user.PublicKey == "" {
		return "", fmt.Errorf("user %q has no public key registered", userID)
	}
	return user.PublicKey, nil
}

// UpdateUserPublicKey updates the public key for a user (called on login/key rotation).
func (biz *ChatBusiness) UpdateUserPublicKey(ctx context.Context, userID string, publicKey string) error {
	if publicKey == "" {
		return fmt.Errorf("public key cannot be empty")
	}
	return biz.userStore.UpdateUserPublicKey(ctx, userID, publicKey)
}

// --- Conversation Management ---

// CreateGroupReq is the input for creating a group conversation.
type CreateGroupReq struct {
	Name      string   `json:"name" binding:"required"`
	MemberIDs []string `json:"member_ids" binding:"required,min=2"`
}

// CreateGroupConversation creates a group conversation and adds all members
// (including the creator) as participants in a single transaction.
func (biz *ChatBusiness) CreateGroupConversation(ctx context.Context, creatorID string, req *CreateGroupReq) (*model.Conversation, error) {
	conv := &model.Conversation{
		Name: req.Name,
		Type: model.ConversationTypeGroup,
	}

	// Ensure creator is included and deduplicate member IDs.
	allMemberIDs := uniqueStrings(append(req.MemberIDs, creatorID))

	participants := make([]model.Participant, 0, len(allMemberIDs))
	now := time.Now()
	for _, uid := range allMemberIDs {
		participants = append(participants, model.Participant{
			ConversationID: "", // will be stamped by the transactional method
			UserID:         uid,
			JoinedAt:       now,
		})
	}

	if err := biz.convStore.CreateConversation(ctx, conv); err != nil {
		return nil, fmt.Errorf("create group conversation: %w", err)
	}

	// Stamp conversation ID and insert participants.
	for i := range participants {
		participants[i].ConversationID = conv.ID
	}

	if err := biz.convStore.AddParticipants(ctx, participants); err != nil {
		return nil, fmt.Errorf("add participants to group %q: %w", conv.ID, err)
	}

	return conv, nil
}

// GetConversationsByUserID returns all conversations the user is a participant of.
func (biz *ChatBusiness) GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error) {
	conversations, err := biz.convStore.GetConversationsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get conversations for user %q: %w", userID, err)
	}
	return conversations, nil
}

// uniqueStrings removes duplicate strings while preserving order.
func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
