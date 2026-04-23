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
	msgStore      MessageStorage
	publisher     MessagePublisher
	convStore     ConversationStorage
	groupStore    GroupStorage
	userStore     UserStorage
	presenceStore PresenceStorage
	presenceTTL   time.Duration
	masterKey     string
}

// NewChatBusiness creates a new ChatBusiness with all required dependencies.
func NewChatBusiness(
	msgStore MessageStorage,
	publisher MessagePublisher,
	convStore ConversationStorage,
	groupStore GroupStorage,
	userStore UserStorage,
	presenceStore PresenceStorage,
	presenceTTL time.Duration,
	masterKey string,
) *ChatBusiness {
	return &ChatBusiness{
		msgStore:      msgStore,
		publisher:     publisher,
		convStore:     convStore,
		groupStore:    groupStore,
		userStore:     userStore,
		presenceStore: presenceStore,
		presenceTTL:   presenceTTL,
		masterKey:     masterKey,
	}
}

// --- Send Message ---

// SendMessageReq is the input for sending a message in a conversation.
// Hybrid E2EE: AES-GCM ciphertext in content_encrypted.
// Session key is wrapped twice (RSA-OAEP):
// - key_for_receiver: wrapped with receiver's public key
// - key_for_sender: wrapped with sender's public key (self-decryption/history)
// IV is in iv.
type SendMessageReq struct {
	ConversationID   string `json:"conversation_id" binding:"required"`
	ContentEncrypted string `json:"content_encrypted" binding:"required"`
	KeyForSender     string `json:"key_for_sender"`
	KeyForReceiver   string `json:"key_for_receiver"`
	IV               string `json:"iv"`
}

// SendMessage persists a message to ScyllaDB and publishes it to Centrifugo
// for real-time delivery. The storage layer generates MessageID and CreatedAt.
func (biz *ChatBusiness) SendMessage(ctx context.Context, senderID string, req *SendMessageReq) error {
	conv, err := biz.convStore.GetConversationByID(ctx, req.ConversationID)
	if err != nil {
		return fmt.Errorf("get conversation %q: %w", req.ConversationID, err)
	}

	// Construct message — MessageID and CreatedAt will be set by InsertMessage.
	msg := &model.Message{
		ConversationID:   req.ConversationID,
		GroupID:          "",
		SenderID:         senderID,
		ContentEncrypted: req.ContentEncrypted,
		KeyForSender:     req.KeyForSender,
		KeyForReceiver:   req.KeyForReceiver,
		IV:               req.IV,
		IsRead:           false,
	}

	channelPrefix := "chat"
	if conv.Type == model.ConversationTypeGroup {
		// For group messages, we persist group_id for downstream audit/tools.
		msg.GroupID = req.ConversationID
		channelPrefix = "groups"
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
		GroupID:          msg.GroupID,
		SenderID:         senderID,
		SenderName:       senderName,
		ContentEncrypted: msg.ContentEncrypted,
		KeyForSender:     msg.KeyForSender,
		KeyForReceiver:   msg.KeyForReceiver,
		IV:               msg.IV,
		CreatedAt:        msg.CreatedAt,
	}

	// Publish to Centrifugo. If publish fails, the message is still persisted —
	// we log the error but do not fail the request.
	channel := fmt.Sprintf("%s:%s", channelPrefix, req.ConversationID)
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
func (biz *ChatBusiness) GetConversationsByUserID(ctx context.Context, userID string) ([]dto.ConversationDetail, error) {
	details, err := biz.convStore.GetDetailedConversations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get detailed conversations for user %q: %w", userID, err)
	}
	return details, nil
}

// --- Message History ---

// GetMessages retrieves paginated message history for a conversation.
// If beforeTS is zero, it returns the latest messages.
func (biz *ChatBusiness) GetMessages(ctx context.Context, conversationID string, beforeTS time.Time, limit int) ([]model.Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	messages, err := biz.msgStore.GetMessages(ctx, conversationID, beforeTS, limit)
	if err != nil {
		return nil, fmt.Errorf("get messages for conversation %q: %w", conversationID, err)
	}

	return messages, nil
}

// --- Presence Management ---

// SetUserOnline marks a user as online and broadcasts the status to their chat partners.
func (biz *ChatBusiness) SetUserOnline(ctx context.Context, userID string) error {
	if err := biz.presenceStore.SetOnline(ctx, userID, biz.presenceTTL); err != nil {
		return fmt.Errorf("set user %q online: %w", userID, err)
	}

	biz.broadcastPresence(ctx, userID, model.PresenceStatusOnline)
	return nil
}

// SetUserOffline marks a user as offline and broadcasts the status to their chat partners.
func (biz *ChatBusiness) SetUserOffline(ctx context.Context, userID string) error {
	if err := biz.presenceStore.SetOffline(ctx, userID); err != nil {
		return fmt.Errorf("set user %q offline: %w", userID, err)
	}

	biz.broadcastPresence(ctx, userID, model.PresenceStatusOffline)
	return nil
}

// RefreshPresence refreshes the heartbeat TTL for an online user.
func (biz *ChatBusiness) RefreshPresence(ctx context.Context, userID string) error {
	if err := biz.presenceStore.SetOnline(ctx, userID, biz.presenceTTL); err != nil {
		return fmt.Errorf("refresh presence for user %q: %w", userID, err)
	}
	return nil
}

// GetBulkPresence retrieves presence status for multiple users.
func (biz *ChatBusiness) GetBulkPresence(ctx context.Context, userIDs []string) ([]model.PresenceInfo, error) {
	presences, err := biz.presenceStore.GetBulkPresence(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get bulk presence: %w", err)
	}
	return presences, nil
}

// broadcastPresence publishes a presence update to all conversation channels
// the user participates in, so co-participants receive real-time status updates.
func (biz *ChatBusiness) broadcastPresence(ctx context.Context, userID string, status model.PresenceStatus) {
	convIDs, err := biz.convStore.GetConversationIDsByUserID(ctx, userID)
	if err != nil {
		log.Printf("[WARN] failed to get conversations for presence broadcast of user %s: %v", userID, err)
		return
	}

	payload := &dto.PresenceUpdate{
		Type:     "presence_update",
		UserID:   userID,
		Status:   status,
		LastSeen: time.Now(),
	}

	for _, convID := range convIDs {
		// Broadcast to both namespaces. Clients subscribe to either `chat:` (DIRECT)
		// or `groups:` (GROUP), depending on conversation type.
		for _, prefix := range []string{"chat", "groups"} {
			channel := fmt.Sprintf("%s:%s", prefix, convID)
			if err := biz.publisher.PublishMessage(ctx, channel, payload); err != nil {
				log.Printf("[WARN] failed to broadcast presence to channel %s: %v", channel, err)
			}
		}
	}
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
