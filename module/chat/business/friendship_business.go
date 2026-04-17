package business

import (
	"context"
	"fmt"
	"log"
	"time"

	"be-chat-centrifugo/module/chat/model"
)

type FriendshipBusiness struct {
	friendStore   FriendshipStorage
	userStore     UserStorage
	convStore     ConversationStorage
	publisher     MessagePublisher
}

func NewFriendshipBusiness(friendStore FriendshipStorage, userStore UserStorage, convStore ConversationStorage, publisher MessagePublisher) *FriendshipBusiness {
	return &FriendshipBusiness{
		friendStore: friendStore,
		userStore:   userStore,
		convStore:   convStore,
		publisher:   publisher,
	}
}

// SearchUsers searches for users by query excluding the caller.
func (biz *FriendshipBusiness) SearchUsers(ctx context.Context, query, excludeUserID string) ([]model.User, error) {
	return biz.userStore.SearchUsers(ctx, query, excludeUserID)
}

func (biz *FriendshipBusiness) GetPendingRequests(ctx context.Context, userID string) ([]model.Friendship, error) {
	return biz.friendStore.GetPendingRequests(ctx, userID)
}

// RequestFriend sends a friend request from requester to receiver.
func (biz *FriendshipBusiness) RequestFriend(ctx context.Context, requesterID, receiverID string) error {
	if requesterID == receiverID {
		return fmt.Errorf("cannot send friend request to yourself")
	}

	// Check if already friends or requested
	existing, err := biz.friendStore.GetFriendshipBetween(ctx, requesterID, receiverID)
	if err != nil {
		return fmt.Errorf("check existing friendship: %w", err)
	}

	if existing != nil {
		if existing.Status == model.FriendshipStatusPending {
			return fmt.Errorf("friend request already pending")
		}
		if existing.Status == model.FriendshipStatusAccepted {
			return fmt.Errorf("already friends")
		}
		// If rejected, we might allow re-request, but let's just update it
		if existing.Status == model.FriendshipStatusRejected {
			// for simplicity, we could update status, but we'll assume we error out or just recreate.
			// we'll update it to pending
			return biz.friendStore.UpdateFriendshipStatus(ctx, existing.ID, model.FriendshipStatusPending)
		}
	}

	f := &model.Friendship{
		RequesterID: requesterID,
		ReceiverID:  receiverID,
		Status:      model.FriendshipStatusPending,
	}

	if err := biz.friendStore.CreateFriendship(ctx, f); err != nil {
		return fmt.Errorf("create friend request: %w", err)
	}

	// Notify receiver via personal channel
	channel := fmt.Sprintf("user:#%s", receiverID)
	payload := map[string]interface{}{
		"type": "friend_request_received",
		"requester_id": requesterID,
	}
	biz.publisher.PublishMessage(ctx, channel, payload)

	return nil
}

// AcceptFriendRequest accepts an existing friend request and handles conversation creation.
func (biz *FriendshipBusiness) AcceptFriendRequest(ctx context.Context, userID, requestID string) error {
	f, err := biz.friendStore.GetFriendship(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get friendship request: %w", err)
	}

	if f.ReceiverID != userID {
		return fmt.Errorf("not authorized to accept this request")
	}
	if f.Status != model.FriendshipStatusPending {
		return fmt.Errorf("request is not pending")
	}

	if err := biz.friendStore.UpdateFriendshipStatus(ctx, f.ID, model.FriendshipStatusAccepted); err != nil {
		return fmt.Errorf("update status to accepted: %w", err)
	}

	// Check if direct conversation already exists
	conv, err := biz.convStore.GetDirectConversationBetween(ctx, f.RequesterID, f.ReceiverID)
	if err != nil {
		return fmt.Errorf("check direct conversation: %w", err)
	}

	if conv == nil {
		conv = &model.Conversation{
			Type: model.ConversationTypeDirect,
		}
		// Step 1: create conversation
		if err := biz.convStore.CreateConversation(ctx, conv); err != nil {
			return fmt.Errorf("create direct conversation: %w", err)
		}
		// Step 2: add both participants
		now := time.Now()
		participants := []model.Participant{
			{ConversationID: conv.ID, UserID: f.RequesterID, JoinedAt: now},
			{ConversationID: conv.ID, UserID: f.ReceiverID, JoinedAt: now},
		}
		if err := biz.convStore.AddParticipants(ctx, participants); err != nil {
			return fmt.Errorf("add participants to direct conversation: %w", err)
		}

		// Let's get public keys for both users
		user1, _ := biz.userStore.GetUserByID(ctx, f.RequesterID)
		user2, _ := biz.userStore.GetUserByID(ctx, f.ReceiverID)

		// Broadcast conversation created to BOTH users
		// To User1 (Requester) -> send info with User2's public key
		biz.publishConversationCreated(ctx, f.RequesterID, conv, f.ReceiverID, user2)
		// To User2 (Receiver) -> send info with User1's public key
		biz.publishConversationCreated(ctx, f.ReceiverID, conv, f.RequesterID, user1)
	}

	return nil
}

func (biz *FriendshipBusiness) publishConversationCreated(ctx context.Context, targetUserID string, conv *model.Conversation, peerID string, peerUser *model.User) {
	channel := fmt.Sprintf("user:#%s", targetUserID)
	
	// Default name fallback
	peerName := peerID
	peerPublicKey := ""
	if peerUser != nil {
		peerName = peerUser.Username
		peerPublicKey = peerUser.PublicKey
	}

	payload := map[string]interface{}{
		"type":            "conversation_created",
		"conversation_id": conv.ID,
		"c_type":          conv.Type,
		"created_at":      conv.CreatedAt,
		// Send peer metadata for immediate display & E2EE!
		"peer_id":         peerID,
		"peer_name":       peerName,
		"peer_public_key": peerPublicKey,
	}
	if err := biz.publisher.PublishMessage(ctx, channel, payload); err != nil {
		log.Printf("[WARN] failed to notify user %s of new conversation: %v", targetUserID, err)
	}
}

// RejectFriendRequest rejects an existing friend request.
func (biz *FriendshipBusiness) RejectFriendRequest(ctx context.Context, userID, requestID string) error {
	f, err := biz.friendStore.GetFriendship(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get friendship request: %w", err)
	}

	if f.ReceiverID != userID {
		return fmt.Errorf("not authorized to reject this request")
	}
	if f.Status != model.FriendshipStatusPending {
		return fmt.Errorf("request is not pending")
	}

	if err := biz.friendStore.UpdateFriendshipStatus(ctx, f.ID, model.FriendshipStatusRejected); err != nil {
		return fmt.Errorf("update status to rejected: %w", err)
	}

	return nil
}

