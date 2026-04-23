package storage

import (
	"context"
	"fmt"
	"time"

	"be-chat-centrifugo/module/chat/dto"
	"be-chat-centrifugo/module/chat/model"
	"gorm.io/gorm"
)

type postgresStore struct {
	db *gorm.DB
}

// NewPostgresStore creates a new Postgres-backed store for users, conversations, and participants.
func NewPostgresStore(db *gorm.DB) *postgresStore {
	return &postgresStore{db: db}
}

// --- User operations ---

// CreateUser persists a new user record.
func (s *postgresStore) CreateUser(ctx context.Context, u *model.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

// GetUserByUsername retrieves a user by their unique username.
func (s *postgresStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("get user by username %q: %w", username, err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by their UUID primary key.
func (s *postgresStore) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, fmt.Errorf("get user by id %q: %w", id, err)
	}
	return &user, nil
}

// UpdateUserPublicKey updates the public key for a given user.
func (s *postgresStore) UpdateUserPublicKey(ctx context.Context, userID string, publicKey string) error {
	result := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("public_key", publicKey)
	if result.Error != nil {
		return fmt.Errorf("update public key for user %q: %w", userID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user %q not found", userID)
	}
	return nil
}

// HasActiveUserDevice reports whether this fingerprint is already registered as active for the user.
func (s *postgresStore) HasActiveUserDevice(ctx context.Context, userID, fingerprint string) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.UserDevice{}).
		Where("user_id = ? AND device_fingerprint = ? AND status = ?", userID, fingerprint, model.UserDeviceStatusActive).
		Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("check user device: %w", err)
	}
	return n > 0, nil
}

// InsertUserDevice inserts a new device row.
func (s *postgresStore) InsertUserDevice(ctx context.Context, d *model.UserDevice) error {
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return fmt.Errorf("insert user device: %w", err)
	}
	return nil
}

// UpdateUserDeviceLastLogin bumps last_login for an existing active device fingerprint.
func (s *postgresStore) UpdateUserDeviceLastLogin(ctx context.Context, userID, fingerprint string) error {
	res := s.db.WithContext(ctx).Model(&model.UserDevice{}).
		Where("user_id = ? AND device_fingerprint = ? AND status = ?", userID, fingerprint, model.UserDeviceStatusActive).
		Update("last_login", time.Now())
	if res.Error != nil {
		return fmt.Errorf("update user device last_login: %w", res.Error)
	}
	return nil
}

// InsertUserDeviceChanged appends an audit row for a new or changed device context.
func (s *postgresStore) InsertUserDeviceChanged(ctx context.Context, c *model.UserDeviceChanged) error {
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("insert user_device_changed: %w", err)
	}
	return nil
}

// --- Conversation operations ---

// CreateConversation persists a new conversation record.
func (s *postgresStore) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	return s.db.WithContext(ctx).Create(conv).Error
}

func (s *postgresStore) GetConversationByID(ctx context.Context, conversationID string) (*model.Conversation, error) {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ?", conversationID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation by id %q: %w", conversationID, err)
	}
	return &conv, nil
}

// GetConversationsByUserID returns all conversations that a user is a participant of,
// ordered by most recently updated first.
func (s *postgresStore) GetConversationsByUserID(ctx context.Context, userID string) ([]model.Conversation, error) {
	var conversations []model.Conversation
	err := s.db.WithContext(ctx).
		Joins("JOIN participants ON participants.conversation_id = conversations.id").
		Where("participants.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, fmt.Errorf("get conversations by user %q: %w", userID, err)
	}
	return conversations, nil
}

// GetDetailedConversations returns all conversations with participants and their public keys.
func (s *postgresStore) GetDetailedConversations(ctx context.Context, userID string) ([]dto.ConversationDetail, error) {
	var conversations []model.Conversation
	err := s.db.WithContext(ctx).
		Joins("JOIN participants ON participants.conversation_id = conversations.id").
		Where("participants.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, fmt.Errorf("get detailed conversations for user %q: %w", userID, err)
	}

	details := make([]dto.ConversationDetail, 0, len(conversations))
	for _, conv := range conversations {
		var participants []dto.ParticipantDetail
		err := s.db.WithContext(ctx).
			Table("participants").
			Select("users.id as user_id, users.username, users.public_key").
			Joins("JOIN users ON users.id = participants.user_id").
			Where("participants.conversation_id = ?", conv.ID).
			Scan(&participants).Error
		if err != nil {
			return nil, fmt.Errorf("get participants for conversation %q: %w", conv.ID, err)
		}

		// For direct chats, we want to display the peer's name as the conversation name.
		displayName := conv.Name
		if conv.Type == model.ConversationTypeDirect {
			for _, p := range participants {
				if p.UserID != userID {
					displayName = p.Username
					break
				}
			}
		}

		encryptedGroupKey := ""
		if conv.Type == model.ConversationTypeGroup {
			// Return per-user encrypted group key (group_members.group_id == conversation.id).
			// Logic: SELECT encrypted_group_key FROM group_members WHERE group_id = ? AND user_id = ?.
			// If missing, we still return conversation but client can show "request rekey".
			_ = s.db.WithContext(ctx).
				Table("group_members").
				Select("encrypted_group_key").
				Where("group_id = ? AND user_id = ?", conv.ID, userID).
				Limit(1).
				Scan(&encryptedGroupKey).Error
		}

		details = append(details, dto.ConversationDetail{
			ID:                conv.ID,
			Name:              displayName,
			Type:              conv.Type,
			EncryptedGroupKey: encryptedGroupKey,
			Participants:      participants,
			CreatedAt:         conv.CreatedAt,
			UpdatedAt:         conv.UpdatedAt,
		})
	}

	return details, nil
}

// --- Participant operations ---

// AddParticipant inserts a single participant record.
func (s *postgresStore) AddParticipant(ctx context.Context, p *model.Participant) error {
	return s.db.WithContext(ctx).Create(p).Error
}

// AddParticipants inserts multiple participant records in a single batch operation.
func (s *postgresStore) AddParticipants(ctx context.Context, participants []model.Participant) error {
	if len(participants) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&participants).Error
}

// CreateGroupConversation creates a conversation and inserts all participants
// in a single Postgres transaction for atomicity. If any step fails, the entire
// operation is rolled back.
func (s *postgresStore) CreateGroupConversation(ctx context.Context, conv *model.Conversation, participants []model.Participant) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(conv).Error; err != nil {
			return fmt.Errorf("create conversation: %w", err)
		}

		// Stamp the generated conversation ID onto each participant.
		now := time.Now()
		for i := range participants {
			participants[i].ConversationID = conv.ID
			participants[i].JoinedAt = now
		}

		if err := tx.Create(&participants).Error; err != nil {
			return fmt.Errorf("insert participants: %w", err)
		}

		return nil
	})
}

// GetParticipantIDs returns all user IDs that are participants of a given conversation.
func (s *postgresStore) GetParticipantIDs(ctx context.Context, conversationID string) ([]string, error) {
	var userIDs []string
	err := s.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("conversation_id = ?", conversationID).
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("get participant IDs for conversation %q: %w", conversationID, err)
	}
	return userIDs, nil
}

// GetConversationIDsByUserID returns all conversation IDs the user belongs to.
func (s *postgresStore) GetConversationIDsByUserID(ctx context.Context, userID string) ([]string, error) {
	var conversationIDs []string
	err := s.db.WithContext(ctx).
		Model(&model.Participant{}).
		Where("user_id = ?", userID).
		Pluck("conversation_id", &conversationIDs).Error
	if err != nil {
		return nil, fmt.Errorf("get conversation IDs for user %q: %w", userID, err)
	}
	return conversationIDs, nil
}

// GetDirectConversationBetween checks if a 1-1 conversation already exists between two users.
func (s *postgresStore) GetDirectConversationBetween(ctx context.Context, user1ID, user2ID string) (*model.Conversation, error) {
	var convs []model.Conversation
	err := s.db.WithContext(ctx).Raw(`
		SELECT c.* FROM conversations c
		JOIN participants p1 ON c.id = p1.conversation_id
		JOIN participants p2 ON c.id = p2.conversation_id
		WHERE c.type = ? AND p1.user_id = ? AND p2.user_id = ?
		LIMIT 1;
	`, model.ConversationTypeDirect, user1ID, user2ID).Scan(&convs).Error

	if err != nil {
		return nil, fmt.Errorf("get direct conversation between %q and %q: %w", user1ID, user2ID, err)
	}
	if len(convs) == 0 {
		return nil, nil // Not found, but no error
	}
	return &convs[0], nil
}

// --- Group operations ---

func (s *postgresStore) CreateGroup(ctx context.Context, g *model.Group) error {
	return s.db.WithContext(ctx).Create(g).Error
}

func (s *postgresStore) AddGroupMembers(ctx context.Context, members []model.GroupMember) error {
	if len(members) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&members).Error
}

func (s *postgresStore) AddGroupMember(ctx context.Context, m *model.GroupMember) error {
	return s.db.WithContext(ctx).Create(m).Error
}

func (s *postgresStore) GetGroupByID(ctx context.Context, groupID string) (*model.Group, error) {
	var g model.Group
	if err := s.db.WithContext(ctx).Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, fmt.Errorf("get group by id %q: %w", groupID, err)
	}
	return &g, nil
}

func (s *postgresStore) GetGroupMember(ctx context.Context, groupID, userID string) (*model.GroupMember, error) {
	var m model.GroupMember
	tx := s.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Limit(1).Find(&m)
	if tx.Error != nil {
		return nil, fmt.Errorf("get group member (g=%q,u=%q): %w", groupID, userID, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &m, nil
}

func (s *postgresStore) GetGroupMemberEncryptedKey(ctx context.Context, groupID, userID string) (string, error) {
	var key string
	tx := s.db.WithContext(ctx).Model(&model.GroupMember{}).
		Select("encrypted_group_key").
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Limit(1).
		Scan(&key)
	if tx.Error != nil {
		return "", fmt.Errorf("get encrypted_group_key (g=%q,u=%q): %w", groupID, userID, tx.Error)
	}
	if key == "" {
		return "", fmt.Errorf("encrypted_group_key not found (g=%q,u=%q)", groupID, userID)
	}
	return key, nil
}

func (s *postgresStore) ListGroupsByUserID(ctx context.Context, userID string) ([]model.Group, error) {
	var groups []model.Group
	err := s.db.WithContext(ctx).
		Table("groups").
		Joins("JOIN group_members gm ON gm.group_id = groups.id").
		Where("gm.user_id = ?", userID).
		Order("groups.updated_at DESC").
		Scan(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("list groups by user %q: %w", userID, err)
	}
	return groups, nil
}

func (s *postgresStore) ListGroupMembers(ctx context.Context, groupID string) ([]dto.GroupMemberInfo, error) {
	var out []dto.GroupMemberInfo
	type row struct {
		UserID    string `gorm:"column:user_id"`
		Username  string `gorm:"column:username"`
		AvatarURL string `gorm:"column:avatar_url"`
		Role      string `gorm:"column:role"`
	}
	var rows []row
	err := s.db.WithContext(ctx).
		Table("group_members gm").
		Select("gm.user_id, u.username, u.avatar_url, gm.role").
		Joins("JOIN users u ON u.id = gm.user_id").
		Where("gm.group_id = ?", groupID).
		Order("CASE WHEN gm.role IN ('creator','admin') THEN 0 ELSE 1 END, u.username ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list group members for group %q: %w", groupID, err)
	}

	out = make([]dto.GroupMemberInfo, 0, len(rows))
	for _, r := range rows {
		role := "member"
		if r.Role == string(model.GroupRoleCreator) || r.Role == string(model.GroupRoleAdmin) {
			role = "owner"
		}
		out = append(out, dto.GroupMemberInfo{
			UserID:   r.UserID,
			Username: r.Username,
			Avatar:   r.AvatarURL,
			Role:     role,
		})
	}
	return out, nil
}

func (s *postgresStore) CreateGroupConversationAtomic(ctx context.Context, g *model.Group, members []model.GroupMember, conv *model.Conversation, participants []model.Participant) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(g).Error; err != nil {
			return fmt.Errorf("create group: %w", err)
		}

		// Ensure conversation uses the same ID as group.
		conv.ID = g.ID
		if err := tx.Create(conv).Error; err != nil {
			return fmt.Errorf("create conversation for group %q: %w", g.ID, err)
		}

		now := time.Now()
		for i := range members {
			members[i].GroupID = g.ID
			if members[i].JoinedAt.IsZero() {
				members[i].JoinedAt = now
			}
			if members[i].Role == "" {
				members[i].Role = model.GroupRoleMember
			}
		}
		if len(members) > 0 {
			if err := tx.Create(&members).Error; err != nil {
				return fmt.Errorf("insert group_members for group %q: %w", g.ID, err)
			}
		}

		for i := range participants {
			participants[i].ConversationID = g.ID
			if participants[i].JoinedAt.IsZero() {
				participants[i].JoinedAt = now
			}
		}
		if len(participants) > 0 {
			if err := tx.Create(&participants).Error; err != nil {
				return fmt.Errorf("insert participants for group %q: %w", g.ID, err)
			}
		}

		return nil
	})
}

func (s *postgresStore) AddGroupMemberAtomic(ctx context.Context, m *model.GroupMember, p *model.Participant) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("insert group_member (g=%q,u=%q): %w", m.GroupID, m.UserID, err)
		}
		if err := tx.Create(p).Error; err != nil {
			return fmt.Errorf("insert participant for group (g=%q,u=%q): %w", p.ConversationID, p.UserID, err)
		}
		return nil
	})
}

// --- Search Users ---

// SearchUsers finds users by email or username ignoring case, excluding the caller.
func (s *postgresStore) SearchUsers(ctx context.Context, query string, excludeUserID string) ([]model.User, error) {
	var users []model.User
	wildcard := "%" + query + "%"
	err := s.db.WithContext(ctx).
		Where("id != ? AND (username ILIKE ? OR email ILIKE ?)", excludeUserID, wildcard, wildcard).
		Limit(20). // limit results for safety
		Find(&users).Error
	if err != nil {
		return nil, fmt.Errorf("search users by %q: %w", query, err)
	}
	return users, nil
}

// --- Friendship operations ---

func (s *postgresStore) CreateFriendship(ctx context.Context, f *model.Friendship) error {
	return s.db.WithContext(ctx).Create(f).Error
}

func (s *postgresStore) GetFriendship(ctx context.Context, id string) (*model.Friendship, error) {
	var f model.Friendship
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&f).Error; err != nil {
		return nil, fmt.Errorf("get friendship %q: %w", id, err)
	}
	return &f, nil
}

func (s *postgresStore) GetFriendshipBetween(ctx context.Context, user1ID, user2ID string) (*model.Friendship, error) {
	var f model.Friendship
	tx := s.db.WithContext(ctx).
		Where("(requester_id = ? AND receiver_id = ?) OR (requester_id = ? AND receiver_id = ?)", user1ID, user2ID, user2ID, user1ID).
		Limit(1).
		Find(&f)
	if tx.Error != nil {
		return nil, fmt.Errorf("get friendship between %q and %q: %w", user1ID, user2ID, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &f, nil
}

func (s *postgresStore) UpdateFriendshipStatus(ctx context.Context, id string, status string) error {
	err := s.db.WithContext(ctx).Model(&model.Friendship{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		return fmt.Errorf("update friendship %q status to %q: %w", id, status, err)
	}
	return nil
}

func (s *postgresStore) GetPendingRequests(ctx context.Context, userID string) ([]model.Friendship, error) {
	var requests []model.Friendship
	// Load the requester info as well
	err := s.db.WithContext(ctx).
		Preload("Requester").
		Where("receiver_id = ? AND status = ?", userID, model.FriendshipStatusPending).
		Order("created_at DESC").
		Find(&requests).Error
	if err != nil {
		return nil, fmt.Errorf("get pending requests for %q: %w", userID, err)
	}
	return requests, nil
}
