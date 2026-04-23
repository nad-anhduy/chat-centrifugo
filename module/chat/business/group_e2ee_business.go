package business

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"time"

	"be-chat-centrifugo/module/chat/dto"
	"be-chat-centrifugo/module/chat/model"
	"be-chat-centrifugo/pkg/userkeypair"
	"github.com/google/uuid"
)

type CreateE2EEGroupReq struct {
	Name    string                  `json:"name" binding:"required"`
	Avatar  string                  `json:"avatar"`
	Members []CreateE2EEGroupMember `json:"members" binding:"required,min=2"`
}

type CreateE2EEGroupMember struct {
	UserID            string          `json:"user_id" binding:"required"`
	EncryptedGroupKey string          `json:"encrypted_group_key" binding:"required"`
	Role              model.GroupRole `json:"role"`
}

type AddGroupMemberReq struct {
	UserID            string          `json:"user_id" binding:"required"`
	EncryptedGroupKey string          `json:"encrypted_group_key" binding:"required"`
	Role              model.GroupRole `json:"role"`
}

func (biz *ChatBusiness) CreateE2EEGroup(ctx context.Context, creatorID string, req *CreateE2EEGroupReq) (*model.Group, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if len(req.Members) < 2 {
		return nil, errors.New("members must have at least 2 users")
	}

	// Ensure creator has a key copy.
	creatorHasKey := false
	seen := map[string]struct{}{}
	for _, m := range req.Members {
		if m.UserID == "" {
			return nil, errors.New("member user_id is required")
		}
		if m.EncryptedGroupKey == "" {
			return nil, fmt.Errorf("encrypted_group_key missing for user %q", m.UserID)
		}
		if _, ok := seen[m.UserID]; ok {
			return nil, fmt.Errorf("duplicate member user_id %q", m.UserID)
		}
		seen[m.UserID] = struct{}{}
		if m.UserID == creatorID {
			creatorHasKey = true
		}
	}
	if !creatorHasKey {
		return nil, errors.New("creator must be included in members with encrypted_group_key")
	}

	g := &model.Group{
		Name:      req.Name,
		Avatar:    req.Avatar,
		CreatorID: creatorID,
	}
	// Ensure group has a UUID before insert. This avoids Postgres errors like:
	// invalid input syntax for type uuid: "" (SQLSTATE 22P02)
	if g.ID == "" {
		g.ID = uuid.NewString()
	}

	now := time.Now()
	members := make([]model.GroupMember, 0, len(req.Members))
	participants := make([]model.Participant, 0, len(req.Members))
	for _, m := range req.Members {
		role := m.Role
		if role == "" {
			if m.UserID == creatorID {
				role = model.GroupRoleCreator
			} else {
				role = model.GroupRoleMember
			}
		}

		members = append(members, model.GroupMember{
			GroupID:           "", // stamped in atomic call
			UserID:            m.UserID,
			EncryptedGroupKey: m.EncryptedGroupKey,
			Role:              role,
			JoinedAt:          now,
		})
		participants = append(participants, model.Participant{
			ConversationID: "", // stamped in atomic call (== group_id)
			UserID:         m.UserID,
			JoinedAt:       now,
		})
	}

	conv := &model.Conversation{
		ID:   g.ID, // keep conversation_id == group_id for group chats
		Name: req.Name,
		Type: model.ConversationTypeGroup,
	}

	if err := biz.groupStore.CreateGroupConversationAtomic(ctx, g, members, conv, participants); err != nil {
		return nil, fmt.Errorf("create e2ee group atomically: %w", err)
	}

	// Notify all members (including creator) so their Sidebar can update without reload.
	// Publish is best-effort: DB is already committed, so we log on failure.
	for _, m := range req.Members {
		channel := fmt.Sprintf("user_notif:%s", m.UserID)
		payload := map[string]interface{}{
			"type": "ADDED_TO_GROUP",
			"data": map[string]interface{}{
				"id":                  g.ID,
				"name":                g.Name,
				"is_group":            true,
				"encrypted_group_key": m.EncryptedGroupKey,
				"last_message":        "You were added to this group",
			},
		}
		if err := biz.publisher.PublishMessage(ctx, channel, payload); err != nil {
			log.Printf("[WARN] failed to publish ADDED_TO_GROUP to %s: %v", channel, err)
		}
	}

	return g, nil
}

func (biz *ChatBusiness) AddGroupMember(ctx context.Context, inviterID string, groupID string, req *AddGroupMemberReq) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if groupID == "" {
		return errors.New("group_id is required")
	}
	if req.UserID == "" {
		return errors.New("user_id is required")
	}
	if req.EncryptedGroupKey == "" {
		return errors.New("encrypted_group_key is required")
	}

	// AuthZ: inviter must be in group and be creator/admin.
	inv, err := biz.groupStore.GetGroupMember(ctx, groupID, inviterID)
	if err != nil {
		return fmt.Errorf("get inviter membership: %w", err)
	}
	if inv == nil {
		return errors.New("inviter is not a group member")
	}
	if inv.Role != model.GroupRoleCreator && inv.Role != model.GroupRoleAdmin {
		return errors.New("inviter is not allowed to add members")
	}

	// Ensure target user exists.
	if _, err := biz.userStore.GetUserByID(ctx, req.UserID); err != nil {
		return fmt.Errorf("target user not found: %w", err)
	}

	role := req.Role
	if role == "" {
		role = model.GroupRoleMember
	}

	now := time.Now()
	m := &model.GroupMember{
		GroupID:           groupID,
		UserID:            req.UserID,
		EncryptedGroupKey: req.EncryptedGroupKey,
		Role:              role,
		JoinedAt:          now,
	}
	p := &model.Participant{
		ConversationID: groupID,
		UserID:         req.UserID,
		JoinedAt:       now,
	}

	if err := biz.groupStore.AddGroupMemberAtomic(ctx, m, p); err != nil {
		return fmt.Errorf("add group member atomically: %w", err)
	}

	// Notify the added member so they can see the group immediately.
	// Publish is best-effort: membership is already committed.
	groupName := groupID
	if g, err := biz.groupStore.GetGroupByID(ctx, groupID); err != nil {
		log.Printf("[WARN] failed to load group %s for notification: %v", groupID, err)
	} else if g != nil && g.Name != "" {
		groupName = g.Name
	}

	channel := fmt.Sprintf("user_notif:%s", req.UserID)
	payload := map[string]interface{}{
		"type": "ADDED_TO_GROUP",
		"data": map[string]interface{}{
			"id":                  groupID,
			"name":                groupName,
			"is_group":            true,
			"encrypted_group_key": req.EncryptedGroupKey,
			"last_message":        "You were added to this group",
		},
	}
	if err := biz.publisher.PublishMessage(ctx, channel, payload); err != nil {
		log.Printf("[WARN] failed to publish ADDED_TO_GROUP to %s: %v", channel, err)
	}

	return nil
}

func (biz *ChatBusiness) ListGroupMembers(ctx context.Context, requesterID string, groupID string) ([]dto.GroupMemberInfo, error) {
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}
	if requesterID == "" {
		return nil, errors.New("requester_id is required")
	}

	// AuthZ: requester must be a member of the group.
	m, err := biz.groupStore.GetGroupMember(ctx, groupID, requesterID)
	if err != nil {
		return nil, fmt.Errorf("get requester membership: %w", err)
	}
	if m == nil {
		return nil, errors.New("forbidden")
	}

	members, err := biz.groupStore.ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	return members, nil
}

// AuditGroupMessages decrypts all messages for a group using the creator's key copy.
// This is an internal routine (NOT exposed via HTTP by default).
func (biz *ChatBusiness) AuditGroupMessages(ctx context.Context, groupID string) ([]string, error) {
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}
	if biz.masterKey == "" {
		return nil, errors.New("MASTER_KEY is not configured")
	}

	g, err := biz.groupStore.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}

	creator, err := biz.userStore.GetUserByID(ctx, g.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("get creator user: %w", err)
	}
	if creator.PrivateKey == "" {
		return nil, errors.New("creator has no encrypted private key stored")
	}

	privPEM, err := userkeypair.DecryptPEMWithMasterKey(biz.masterKey, creator.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt creator private key with MASTER_KEY: %w", err)
	}

	encKeyB64, err := biz.groupStore.GetGroupMemberEncryptedKey(ctx, groupID, g.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("get creator encrypted_group_key: %w", err)
	}

	groupKeyRaw, err := rsaOAEPDecryptB64(privPEM, encKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decrypt group key with creator private key: %w", err)
	}
	if len(groupKeyRaw) != 32 {
		return nil, fmt.Errorf("unexpected group key length: got %d bytes, want 32", len(groupKeyRaw))
	}

	// Walk all messages by paging through Scylla (DESC).
	var out []string
	var before time.Time
	for {
		msgs, err := biz.msgStore.GetMessages(ctx, groupID, before, 500)
		if err != nil {
			return nil, fmt.Errorf("load messages page: %w", err)
		}
		if len(msgs) == 0 {
			break
		}

		for _, m := range msgs {
			pt, err := aesGCMDecryptB64(groupKeyRaw, m.IV, m.ContentEncrypted)
			if err != nil {
				out = append(out, fmt.Sprintf("[message_id=%s decrypt_error=%v]", m.MessageID, err))
				continue
			}
			out = append(out, pt)
		}

		// Next page cursor: oldest message in this page (because DESC).
		before = msgs[len(msgs)-1].CreatedAt
	}

	return out, nil
}

func rsaOAEPDecryptB64(privateKeyPEM string, ciphertextB64 string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	priv, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an RSA private key")
	}

	pt, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("rsa oaep decrypt: %w", err)
	}
	return pt, nil
}

func aesGCMDecryptB64(keyRaw []byte, ivB64 string, ciphertextB64 string) (string, error) {
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return "", fmt.Errorf("iv base64 decode: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("ciphertext base64 decode: %w", err)
	}
	block, err := aes.NewCipher(keyRaw)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
